"""HTTP API：
- POST /api/ai/chat          SSE 流式答疑
其他接口：summary.py（学习总结）、generate.py（AI出题）、session.py（会话管理）。
"""
from datetime import date
import json
from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel
from typing import Optional
import redis.asyncio as aioredis

from app.core.config import settings
from app.graph.rag_graph import stream_answer
from app.rag.session import append_turn

router = APIRouter(prefix="/api/ai", tags=["ai"])
_r = aioredis.from_url(settings.redis_url, decode_responses=True)


def get_user_id(req: Request) -> int:
    uid = req.headers.get("X-User-Id")
    if not uid:
        raise HTTPException(401, "missing X-User-Id")
    try:
        return int(uid)
    except ValueError:
        raise HTTPException(400, "invalid X-User-Id")


class ChatReq(BaseModel):
    query: str
    course_id: int
    session_id: Optional[str] = None


@router.post("/chat")
async def chat(body: ChatReq, req: Request):
    """SSE 流式答疑。"""
    user_id = get_user_id(req)

    # 限流：每天每用户 N 次（异步 Redis，不阻塞事件循环）
    key = f"ai:limit:u{user_id}:{date.today().isoformat()}"
    cnt = await _r.incr(key)
    if cnt == 1:
        await _r.expire(key, 86400)
    if cnt > settings.daily_token_limit:
        raise HTTPException(429, "今日 AI 调用已达上限")

    state = {"user_id": user_id, "course_id": body.course_id, "query": body.query}

    async def event_stream():
        answer_parts = []
        try:
            async for chunk in stream_answer(state):
                if chunk.startswith("data: ") and '"delta"' in chunk:
                    try:
                        d = json.loads(chunk[6:])
                        if "delta" in d:
                            answer_parts.append(d["delta"])
                    except Exception:
                        pass
                yield chunk
        except Exception as e:
            yield f"data: {json.dumps({'error': str(e)}, ensure_ascii=False)}\n\n"
        finally:
            # 写完历史（异步）
            if answer_parts:
                await append_turn(user_id, body.course_id, body.query, "".join(answer_parts))

    return StreamingResponse(event_stream(), media_type="text/event-stream", headers={
        "Cache-Control": "no-cache",
        "X-Accel-Buffering": "no",
    })
