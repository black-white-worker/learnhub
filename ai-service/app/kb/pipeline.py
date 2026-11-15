"""知识灌库的后台执行器：把「上传讲义 → 能答疑」这条链路接上。

为什么需要它：`scripts/ingest.py` 只能手工跑。老师上传讲义后知识库一直是空的，
学生一问就是「资料中没有相关内容」——功能看起来坏了，其实是没入库。
这里提供一个进程内的后台队列，供 `POST /api/ai/kb/ingest` 调用：

- 同一门课串行执行（并发灌同一门课会让 Chroma 的「先删后写」互相踩踏，
  出现切片数对不上的诡异状态）；
- 忙时**排队**而不是丢弃：老师连传两份讲义，第二份不能被静默跳过；
- 用守护线程而不是 asyncio 任务：ingest 全程是同步阻塞调用
  （pymysql / pymongo / chromadb / OpenAI SDK），扔进事件循环会把答疑一起卡死。
"""
from __future__ import annotations

import logging
import threading
from typing import Iterable

log = logging.getLogger(__name__)

# course_id -> 是否有一次灌库在跑
_running: dict[int, bool] = {}
# course_id -> 等待中的任务（resource_id 列表，None 表示整门课）
_pending: dict[int, list[int | None]] = {}
_guard = threading.Lock()

STARTED = "started"
QUEUED = "queued"


def is_running(course_id: int) -> bool:
    with _guard:
        return bool(_running.get(course_id))


def pending_count(course_id: int) -> int:
    with _guard:
        return len(_pending.get(course_id) or [])


def run_ingest(course_id: int, resource_id: int | None = None) -> dict:
    """同步执行一次灌库（整门课或单个资源），返回统计。异常只记录不抛出。"""
    from app.kb.manifest import clear_cache
    from scripts.ingest import fetch_resources, ingest_course

    # 刚上传的讲义必须立刻出现在清单里：进程内清单缓存不能挡住「上传完就能问」。
    clear_cache(course_id)

    rows: list[dict] | None = None
    if resource_id is not None:
        rows = [r for r in fetch_resources(course_id) if int(r["id"]) == int(resource_id)]
        if not rows:
            log.warning("课程 %s 资源 %s 不存在，跳过灌库", course_id, resource_id)
            return {"course_id": course_id, "resource_id": resource_id,
                    "resources": 0, "skipped": True}

    try:
        stats = ingest_course(course_id, resources=rows)
        log.info("课程 %s 灌库完成：%s 个资源，%s 个切片，失败 %s",
                 course_id, stats.get("resources"), stats.get("chunks"), stats.get("failed"))
        return stats
    except Exception as e:  # noqa: BLE001 —— 后台线程吞掉异常，绝不让进程退出
        log.warning("课程 %s 灌库异常（%s）", course_id, type(e).__name__)
        return {"course_id": course_id, "resource_id": resource_id, "error": type(e).__name__}


def _worker(course_id: int, resource_id: int | None) -> None:
    """一次只跑一个任务；跑完把队列里的下一个接着跑，队列空了才释放课程锁。"""
    nxt = resource_id
    while True:
        try:
            run_ingest(course_id, nxt)
        finally:
            with _guard:
                queued = _pending.get(course_id) or []
                if queued:
                    nxt = queued.pop(0)
                    continue
                _running[course_id] = False
                return


def start_ingest(course_id: int, resource_id: int | None = None) -> str:
    """提交一次灌库，返回 STARTED（立即开跑）或 QUEUED（排队等前一个跑完）。"""
    with _guard:
        if _running.get(course_id):
            _pending.setdefault(course_id, []).append(resource_id)
            return QUEUED
        _running[course_id] = True
    threading.Thread(
        target=_worker, args=(course_id, resource_id),
        name=f"kb-ingest-{course_id}", daemon=True,
    ).start()
    return STARTED


def purge_resource(course_id: int, resource_id: int) -> dict:
    """资源被删除时清掉它的切片与向量（否则会留下「删了还能搜到」的幽灵资料）。"""
    removed = {"course_id": course_id, "resource_id": resource_id,
               "chunks_cleared": False, "vectors_cleared": False}
    from app.kb.store import get_knowledge_store
    from app.kb.manifest import clear_cache

    # 清单缓存里有刚删掉的这份讲义：不失效的话答疑还会照着旧清单去引用它
    clear_cache(course_id)

    try:
        get_knowledge_store().delete_resource(course_id, resource_id)
        removed["chunks_cleared"] = True
    except Exception as e:  # noqa: BLE001
        log.warning("清理课程 %s 资源 %s 切片失败（%s）", course_id, resource_id, type(e).__name__)

    try:
        from app.rag.vector_store import get_vector_store
        get_vector_store().delete_by_resource(course_id, resource_id)
        removed["vectors_cleared"] = True
    except Exception as e:  # noqa: BLE001
        log.warning("清理课程 %s 资源 %s 向量失败（%s）", course_id, resource_id, type(e).__name__)
    return removed


def start_batch_ingest(course_ids: Iterable[int]) -> int:
    """运维用途：一次性排队多门课（回填历史课程）。返回实际提交的课程数。"""
    submitted = 0
    for cid in course_ids:
        if start_ingest(int(cid)) == STARTED:
            submitted += 1
    return submitted
