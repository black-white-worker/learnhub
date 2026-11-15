"""批改任务状态存储（Redis）。

为什么用 Redis 而不是 MySQL：任务态是"几分钟内有效、丢了可以重跑"的临时数据，
成绩本身才是要落库的东西 —— 落库由 worker 回调 Go 完成，这里只记进度。

存的是一个 JSON 串（不是 hash 字段）：只有一个 worker 顺序消费同一个任务，
不存在并发写同一 key，读改写够用，字段扩展也不用改序列化代码。

key: ai:grade:task:{taskId}   TTL 7 天（结果查完就可以让它过期）
"""
import json
from datetime import datetime, timezone

import redis.asyncio as aioredis

from app.core.config import settings

_r = aioredis.from_url(settings.redis_url, decode_responses=True)

TASK_KEY_PREFIX = "ai:grade:task:"
TASK_TTL_SECONDS = 7 * 24 * 3600

# 状态机：pending → processing → done | failed | manual
# manual = LLM 输出解析不出来，成绩一律不写，等老师手批
STATUS_PENDING = "pending"
STATUS_PROCESSING = "processing"
STATUS_DONE = "done"
STATUS_FAILED = "failed"
STATUS_MANUAL = "manual"


def _key(task_id: str) -> str:
    return f"{TASK_KEY_PREFIX}{task_id}"


def _now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


async def create_task(task_id: str, payload: dict) -> dict:
    """建任务。payload 里是 Go 传过来的批改入参（submitId / items 等）。"""
    task = {
        "taskId": task_id,
        "status": STATUS_PENDING,
        "submitId": payload.get("submitId"),
        "assignmentId": payload.get("assignmentId"),
        "courseId": payload.get("courseId"),
        "studentId": payload.get("studentId"),
        "itemCount": len(payload.get("items") or []),
        "retry": 0,
        "createdAt": _now(),
        "updatedAt": _now(),
        "result": None,
        "error": "",
    }
    await _r.set(_key(task_id), json.dumps(task, ensure_ascii=False), ex=TASK_TTL_SECONDS)
    return task


async def get_task(task_id: str) -> dict | None:
    raw = await _r.get(_key(task_id))
    if not raw:
        return None
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return None


async def save_task(task: dict) -> dict:
    """整体写回（读改写：调用方拿到的是刚 merge 过的 dict）。"""
    task["updatedAt"] = _now()
    await _r.set(
        _key(task["taskId"]), json.dumps(task, ensure_ascii=False), ex=TASK_TTL_SECONDS
    )
    return task


async def update_task(task_id: str, **fields) -> dict | None:
    """改几个字段并写回；任务不存在返回 None。"""
    task = await get_task(task_id)
    if not task:
        return None
    task.update(fields)
    return await save_task(task)
