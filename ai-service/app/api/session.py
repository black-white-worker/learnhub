"""会话管理 API。

- GET    /api/ai/session/history?course_id=1  查当前用户在某课程下的对话历史
- DELETE /api/ai/session?course_id=1          清空当前用户在某课程下的对话历史
"""
from fastapi import APIRouter, HTTPException, Request

from app.rag.session import get_history, clear
from app.core.config import settings

router = APIRouter(prefix="/api/ai/session", tags=["session"])


def get_user_id(req: Request) -> int:
    uid = req.headers.get("X-User-Id")
    if not uid:
        raise HTTPException(401, "missing X-User-Id")
    try:
        return int(uid)
    except ValueError:
        raise HTTPException(400, "invalid X-User-Id")


@router.get("/history")
async def history(req: Request, course_id: int = 0):
    user_id = get_user_id(req)
    if course_id <= 0:
        raise HTTPException(400, "course_id 必须大于 0")
    msgs = get_history(user_id, course_id)
    return {
        "user_id": user_id,
        "course_id": course_id,
        "messages": msgs,
        "rounds": len(msgs) // 2,
        "max_rounds": settings.session_max_rounds,
    }


@router.delete("")
async def clear_session(req: Request, course_id: int = 0):
    user_id = get_user_id(req)
    if course_id <= 0:
        raise HTTPException(400, "course_id 必须大于 0")
    clear(user_id, course_id)
    return {"deleted": True, "user_id": user_id, "course_id": course_id}
