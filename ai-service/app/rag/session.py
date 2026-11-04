"""会话历史：Redis 存，按 user_id+course_id 隔离，只留最近 N 轮。

注意：用 redis.asyncio 异步客户端，不阻塞 FastAPI 事件循环。
"""
import json
import redis.asyncio as aioredis
from app.core.config import settings

_r = aioredis.from_url(settings.redis_url, decode_responses=True)


def _key(user_id: int, course_id: int) -> str:
    return f"ai:session:u{user_id}:c{course_id}"


async def get_history(user_id: int, course_id: int) -> list[dict]:
    raw = await _r.get(_key(user_id, course_id))
    if not raw:
        return []
    try:
        return json.loads(raw)
    except Exception:
        return []


async def append_turn(user_id: int, course_id: int, user_msg: str, ai_msg: str):
    """追加一轮对话，截断到最近 N 轮（N*2 条消息）。"""
    hist = await get_history(user_id, course_id)
    hist.append({"role": "user", "content": user_msg})
    hist.append({"role": "assistant", "content": ai_msg})
    keep = settings.session_max_rounds * 2
    hist = hist[-keep:]
    await _r.setex(_key(user_id, course_id), settings.session_ttl, json.dumps(hist, ensure_ascii=False))


async def clear(user_id: int, course_id: int):
    await _r.delete(_key(user_id, course_id))
