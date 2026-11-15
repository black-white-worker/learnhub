"""知识库 API：

- GET  /api/ai/kb/status  覆盖度（哪些资料没解析正文）变成可查询的运营数据
- POST /api/ai/kb/ingest  Go 在讲义上传成功后触发的灌库（后台线程执行，立刻返回）
- POST /api/ai/kb/purge   讲义删除后清理切片与向量，避免「删了还能搜到」

status 只返回聚合数字，不返回 kb_unanswered 里的具体提问文本（避免泄露学生问题）。
三个接口都需经 Go 代理/后端注入可信的 X-Internal-Token（见 main.py 中间件）。
"""
from __future__ import annotations
import asyncio
from fastapi import APIRouter, HTTPException, Request
from pydantic import BaseModel

from app.kb import pipeline
from app.kb.store import KnowledgeStoreUnavailable, get_knowledge_store

router = APIRouter(prefix="/api/ai/kb", tags=["ai-kb"])


def _require_internal(request: Request) -> None:
    """灌库/清理会改知识库，比 status 更敏感：没有可信身份头一律拒绝。"""
    if not request.headers.get("X-User-Id") and not request.headers.get("X-Internal-Token"):
        raise HTTPException(401, "missing X-User-Id")


@router.get("/status")
async def kb_status(course_id: int, request: Request):
    if not request.headers.get("X-User-Id"):
        raise HTTPException(401, "missing X-User-Id")
    if course_id <= 0:
        raise HTTPException(400, "invalid course_id")
    try:
        store = await asyncio.to_thread(get_knowledge_store)
        return await asyncio.to_thread(store.coverage, course_id)
    except KnowledgeStoreUnavailable as e:
        # 严格模式下 Mongo 挂了：明确 503，而不是伪装成空知识库
        raise HTTPException(503, str(e)) from None
    except Exception as e:
        raise HTTPException(503, f"知识库不可用（{type(e).__name__}）") from None


class IngestReq(BaseModel):
    """resource_id 为空 = 整门课重灌；指定则只灌这一份讲义。"""

    course_id: int
    resource_id: int | None = None


class PurgeReq(BaseModel):
    course_id: int
    resource_id: int


@router.post("/ingest")
async def kb_ingest(body: IngestReq, request: Request):
    """讲义上传后触发灌库：解析 → 切片 → MongoDB + Chroma 双写。

    立刻返回 accepted，真正的解析在后台守护线程里跑（大讲义要几十秒到几分钟）。
    同一门课已有任务在跑时**排队**（queued=true），不丢任务 —— 老师连传两份讲义，
    第二份必须也会被灌进去。
    """
    _require_internal(request)
    if body.course_id <= 0:
        raise HTTPException(400, "invalid course_id")
    if body.resource_id is not None and body.resource_id <= 0:
        raise HTTPException(400, "invalid resource_id")

    state = pipeline.start_ingest(body.course_id, body.resource_id)
    queued = state == pipeline.QUEUED
    return {
        "course_id": body.course_id,
        "resource_id": body.resource_id,
        "accepted": True,
        "queued": queued,
        "running": pipeline.is_running(body.course_id),
        "pending": pipeline.pending_count(body.course_id),
        "reason": "这门课已有灌库任务在执行，本次已排队" if queued else "",
    }


@router.post("/purge")
async def kb_purge(body: PurgeReq, request: Request):
    """讲义删除后清理索引。同步执行（只是几条 delete），失败如实返回。"""
    _require_internal(request)
    if body.course_id <= 0 or body.resource_id <= 0:
        raise HTTPException(400, "invalid course_id/resource_id")
    result = await asyncio.to_thread(
        pipeline.purge_resource, body.course_id, body.resource_id)
    return result
