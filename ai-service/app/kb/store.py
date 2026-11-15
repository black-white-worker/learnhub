"""知识库存储层：MongoDB 主存储 + 内存降级，接口完全等价。

分工（面试可讲）：
- Chroma 只做语义向量检索；MongoDB 才是「灌了什么、解析到哪一步、哪里还空着」的事实源；
- kb_documents  每个资源一条，status 让「视频转写 TODO」从静默失败变成可见状态；
- kb_chunks     切片正文 + keywords 数组（中文 bigram 索引），支撑词法兜底召回；
- kb_ingestions 每次灌库运行日志；kb_unanswered 答不上来的问题 → 覆盖度闭环；
- GridFS        存 PDF/DOCX 原文件，解析策略升级后可重放，不必回 MinIO 重拉。

降级语义：
- MONGODB_ENABLED=false 或连不上 → InMemoryKnowledgeStore(degraded=True)，服务照常跑；
- MONGODB_STRICT=true（上线建议）→ 连不上直接抛 KnowledgeStoreUnavailable，
  避免「以为灌进去了其实没有」。注意内存降级只在单进程内有效，跨进程不共享。
"""
from __future__ import annotations
import logging
import threading
import uuid
from abc import ABC, abstractmethod
from datetime import datetime, timezone
from typing import Any

from app.core.config import settings
from app.kb.search import chunk_key
from app.kb.tokens import lexical_score, tokenize

log = logging.getLogger(__name__)

STATUS_PENDING = "pending"
STATUS_PARSED = "parsed"
STATUS_FALLBACK = "fallback"     # 只灌了名称+链接（原文件没拿到 / 解析未实现）
STATUS_FAILED = "failed"


class KnowledgeStoreUnavailable(RuntimeError):
    """严格模式下 MongoDB 不可用。"""


def _now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def build_chunk_doc(course_id: int, resource_id: int, chunk_index: int,
                    text: str, metadata: dict | None = None) -> dict:
    """构造一条 kb_chunks 文档；keywords 是词法召回的索引字段。"""
    return {
        "chunk_id": chunk_key(f"{course_id}:{resource_id}:{chunk_index}:{text}"),
        "course_id": course_id,
        "resource_id": resource_id,
        "chunk_index": chunk_index,
        "text": text,
        "keywords": tokenize(text),
        "metadata": dict(metadata or {}),
        "content_hash": chunk_key(text),
    }


class KnowledgeStore(ABC):
    backend: str = "abstract"
    degraded: bool = False

    @abstractmethod
    def upsert_document(self, doc: dict) -> None: ...

    @abstractmethod
    def list_documents(self, course_id: int) -> list[dict]: ...

    @abstractmethod
    def delete_resource(self, course_id: int, resource_id: int) -> None: ...

    @abstractmethod
    def replace_chunks(self, course_id: int, resource_id: int,
                       chunks: list[dict]) -> None: ...

    @abstractmethod
    def search_chunks(self, course_id: int, tokens: list[str],
                      limit: int) -> list[dict]: ...

    @abstractmethod
    def start_ingestion(self, course_id: int, meta: dict | None = None) -> str: ...

    @abstractmethod
    def finish_ingestion(self, run_id: str, stats: dict) -> None: ...

    @abstractmethod
    def log_unanswered(self, user_id: int, course_id: int, query: str,
                       reason: str = "no_hit") -> None: ...

    @abstractmethod
    def coverage(self, course_id: int) -> dict: ...

    @abstractmethod
    def put_file(self, key: str, data: bytes, meta: dict | None = None) -> str: ...

    @abstractmethod
    def get_file(self, key: str) -> bytes | None: ...

    @abstractmethod
    def ping(self) -> bool: ...


class InMemoryKnowledgeStore(KnowledgeStore):
    """进程内实现：测试替身 + MongoDB 不可用时的降级后端（数据不持久）。"""

    backend = "memory"

    def __init__(self, degraded: bool = False):
        self.degraded = degraded
        self._docs: dict[tuple[int, int], dict] = {}
        self._chunks: dict[tuple[int, int], list[dict]] = {}
        self._runs: dict[str, dict] = {}
        self._unanswered: list[dict] = []
        self._files: dict[str, bytes] = {}
        self._lock = threading.RLock()

    @staticmethod
    def _rk(course_id: int, resource_id: int) -> tuple[int, int]:
        return (course_id, resource_id)

    def upsert_document(self, doc: dict) -> None:
        d = dict(doc)
        d["updated_at"] = _now()
        with self._lock:
            self._docs[self._rk(d["course_id"], d["resource_id"])] = d

    def list_documents(self, course_id: int) -> list[dict]:
        with self._lock:
            docs = [dict(d) for (cid, _), d in self._docs.items() if cid == course_id]
        return sorted(docs, key=lambda d: d.get("resource_id", 0))

    def delete_resource(self, course_id: int, resource_id: int) -> None:
        k = self._rk(course_id, resource_id)
        with self._lock:
            old = self._docs.pop(k, None)
            self._chunks.pop(k, None)
            if old and old.get("file_key"):
                self._files.pop(old["file_key"], None)

    def replace_chunks(self, course_id: int, resource_id: int,
                       chunks: list[dict]) -> None:
        with self._lock:
            self._chunks[self._rk(course_id, resource_id)] = [dict(c) for c in chunks]

    def search_chunks(self, course_id: int, tokens: list[str],
                      limit: int) -> list[dict]:
        if not tokens or limit <= 0:
            return []
        scored: list[tuple[float, dict]] = []
        with self._lock:
            all_chunks = [dict(c) for (cid, _), cs in self._chunks.items()
                          if cid == course_id for c in cs]
        for c in all_chunks:
            overlap = set(tokens) & set(c.get("keywords") or [])
            if len(overlap) < settings.retrieval_lexical_min_tokens:
                continue
            score = lexical_score(tokens, c.get("keywords") or [])
            if score > 0:
                scored.append((score, c))
        scored.sort(key=lambda x: (-x[0], x[1].get("chunk_index", 0)))
        return [c for _, c in scored[:limit]]

    def start_ingestion(self, course_id: int, meta: dict | None = None) -> str:
        run_id = uuid.uuid4().hex
        with self._lock:
            self._runs[run_id] = {
                "run_id": run_id, "course_id": course_id, "status": "running",
                "started_at": _now(), **(meta or {}),
            }
        return run_id

    def finish_ingestion(self, run_id: str, stats: dict) -> None:
        with self._lock:
            if run_id in self._runs:
                self._runs[run_id].update(stats)
                self._runs[run_id]["status"] = stats.get("status", "done")
                self._runs[run_id]["finished_at"] = _now()

    def log_unanswered(self, user_id: int, course_id: int, query: str,
                       reason: str = "no_hit") -> None:
        with self._lock:
            self._unanswered.append({
                "user_id": user_id, "course_id": course_id, "query": query,
                "reason": reason, "created_at": _now(),
            })

    def coverage(self, course_id: int) -> dict:
        with self._lock:
            docs = [dict(d) for (cid, _), d in self._docs.items() if cid == course_id]
            chunks = sum(len(cs) for (cid, _), cs in self._chunks.items() if cid == course_id)
            unanswered = sum(1 for u in self._unanswered if u["course_id"] == course_id)
            runs = [dict(r) for r in self._runs.values() if r["course_id"] == course_id]
        by_status: dict[str, int] = {}
        for d in docs:
            by_status[d.get("status", "unknown")] = by_status.get(d.get("status", "unknown"), 0) + 1
        return {
            "course_id": course_id,
            "resources": len(docs),
            "by_status": by_status,
            "parsed_resources": by_status.get(STATUS_PARSED, 0),
            "fallback_resources": by_status.get(STATUS_FALLBACK, 0),
            "chunks": chunks,
            "unanswered_queries": unanswered,
            "ingestion_runs": len(runs),
            "backend": self.backend,
            "degraded": self.degraded,
        }

    def put_file(self, key: str, data: bytes, meta: dict | None = None) -> str:
        with self._lock:
            self._files[key] = bytes(data)
        return f"memory://{key}"

    def get_file(self, key: str) -> bytes | None:
        with self._lock:
            return self._files.get(key)

    def ping(self) -> bool:
        return True


class MongoKnowledgeStore(KnowledgeStore):
    """MongoDB 实现；pymongo 为同步客户端，在线检索侧用 asyncio.to_thread 调用。"""

    backend = "mongodb"
    degraded = False

    def __init__(self):
        # 延迟导入：pymongo 未安装 / 版本问题不会在 import app 时炸掉整个服务
        from pymongo import MongoClient
        import gridfs

        kwargs: dict[str, Any] = {
            "host": settings.mongodb_url,
            "serverSelectionTimeoutMS": settings.mongodb_server_selection_timeout_ms,
            "connectTimeoutMS": settings.mongodb_connect_timeout_ms,
            "maxPoolSize": settings.mongodb_max_pool_size,
            "minPoolSize": settings.mongodb_min_pool_size,
            "tls": settings.mongodb_tls,
        }
        if settings.mongodb_replica_set:
            kwargs["replicaSet"] = settings.mongodb_replica_set
        if settings.mongodb_username:
            kwargs["username"] = settings.mongodb_username
            kwargs["password"] = settings.mongodb_password
            kwargs["authSource"] = settings.mongodb_auth_source

        self._client = MongoClient(**kwargs)
        self._client.admin.command("ping")   # 立刻验证，失败由工厂决定降级还是报错
        self._db = self._client[settings.mongodb_db]
        self._fs = gridfs.GridFS(self._db, collection=settings.kb_gridfs_bucket)
        self._ensure_indexes()

    def _col(self, name: str):
        return self._db[f"{settings.kb_collection_prefix}_{name}"]

    def _ensure_indexes(self) -> None:
        """索引失败只告警不阻断：查询退化成全表扫描也比服务起不来强。"""
        try:
            self._col("documents").create_index(
                [("course_id", 1), ("resource_id", 1)],
                unique=True, name="uk_course_resource")
            self._col("chunks").create_index(
                [("course_id", 1), ("resource_id", 1)], name="idx_course_resource")
            self._col("chunks").create_index(
                [("course_id", 1), ("keywords", 1)], name="idx_course_keywords")
            self._col("unanswered").create_index(
                [("course_id", 1), ("created_at", -1)], name="idx_course_time")
        except Exception as e:
            log.warning("知识库索引创建失败（%s），查询将退化为集合扫描", type(e).__name__)

    def upsert_document(self, doc: dict) -> None:
        d = dict(doc)
        d["updated_at"] = _now()
        self._col("documents").update_one(
            {"course_id": d["course_id"], "resource_id": d["resource_id"]},
            {"$set": d}, upsert=True)

    def list_documents(self, course_id: int) -> list[dict]:
        return list(self._col("documents").find(
            {"course_id": course_id}, {"_id": 0}).sort("resource_id", 1))

    def delete_resource(self, course_id: int, resource_id: int) -> None:
        old = self._col("documents").find_one(
            {"course_id": course_id, "resource_id": resource_id}, {"file_key": 1})
        self._col("documents").delete_one(
            {"course_id": course_id, "resource_id": resource_id})
        self._col("chunks").delete_many(
            {"course_id": course_id, "resource_id": resource_id})
        if old and old.get("file_key"):
            for grid in self._fs.find({"filename": old["file_key"]}):
                self._fs.delete(grid._id)

    def replace_chunks(self, course_id: int, resource_id: int,
                       chunks: list[dict]) -> None:
        col = self._col("chunks")
        col.delete_many({"course_id": course_id, "resource_id": resource_id})
        if chunks:
            col.insert_many([dict(c) for c in chunks])

    def search_chunks(self, course_id: int, tokens: list[str],
                      limit: int) -> list[dict]:
        if not tokens or limit <= 0:
            return []
        # 不在数据库侧先 limit 再排序：那样会把排在后面的高分候选直接砍掉。
        # (course_id, keywords) 复合索引已把候选缩到「命中过任一 token 的切片」，
        # 课程级数据量下全量取回、用与内存实现相同的 lexical_score 精确打分最稳妥。
        cur = self._col("chunks").find(
            {"course_id": course_id, "keywords": {"$in": tokens}}, {"_id": 0})
        scored: list[tuple[float, dict]] = []
        for c in cur:
            overlap = set(tokens) & set(c.get("keywords") or [])
            if len(overlap) < settings.retrieval_lexical_min_tokens:
                continue
            score = lexical_score(tokens, c.get("keywords") or [])
            if score > 0:
                scored.append((score, c))
        scored.sort(key=lambda x: (-x[0], x[1].get("chunk_index", 0)))
        return [c for _, c in scored[:limit]]

    def start_ingestion(self, course_id: int, meta: dict | None = None) -> str:
        run_id = uuid.uuid4().hex
        self._col("ingestions").insert_one({
            "run_id": run_id, "course_id": course_id, "status": "running",
            "started_at": _now(), **(meta or {}),
        })
        return run_id

    def finish_ingestion(self, run_id: str, stats: dict) -> None:
        self._col("ingestions").update_one(
            {"run_id": run_id},
            {"$set": {**stats, "status": stats.get("status", "done"),
                      "finished_at": _now()}})

    def log_unanswered(self, user_id: int, course_id: int, query: str,
                       reason: str = "no_hit") -> None:
        self._col("unanswered").insert_one({
            "user_id": user_id, "course_id": course_id, "query": query,
            "reason": reason, "created_at": _now(),
        })

    def coverage(self, course_id: int) -> dict:
        docs = self.list_documents(course_id)
        by_status: dict[str, int] = {}
        for d in docs:
            st = d.get("status", "unknown")
            by_status[st] = by_status.get(st, 0) + 1
        return {
            "course_id": course_id,
            "resources": len(docs),
            "by_status": by_status,
            "parsed_resources": by_status.get(STATUS_PARSED, 0),
            "fallback_resources": by_status.get(STATUS_FALLBACK, 0),
            "chunks": self._col("chunks").count_documents({"course_id": course_id}),
            "unanswered_queries": self._col("unanswered").count_documents(
                {"course_id": course_id}),
            "ingestion_runs": self._col("ingestions").count_documents(
                {"course_id": course_id}),
            "backend": self.backend,
            "degraded": self.degraded,
        }

    def put_file(self, key: str, data: bytes, meta: dict | None = None) -> str:
        # 同名文件先删旧版本：重灌/重解析不会产生无限增长的孤儿文件
        for old in self._fs.find({"filename": key}):
            self._fs.delete(old._id)
        fid = self._fs.put(data, filename=key, metadata=dict(meta or {}))
        return str(fid)

    def get_file(self, key: str) -> bytes | None:
        grid = self._fs.find_one({"filename": key})
        return grid.read() if grid else None

    def ping(self) -> bool:
        self._client.admin.command("ping")
        return True


_store: KnowledgeStore | None = None
_store_lock = threading.Lock()


def get_knowledge_store() -> KnowledgeStore:
    """懒加载单例：第一次真正用到知识库时才连 Mongo，不在 import 时做 IO。"""
    global _store
    if _store is not None:
        return _store
    with _store_lock:
        if _store is not None:
            return _store
        if not settings.mongodb_enabled:
            _store = InMemoryKnowledgeStore(degraded=True)
            return _store
        try:
            _store = MongoKnowledgeStore()
            log.info("知识库已连接 MongoDB（%s）", settings.mongodb_db)
        except Exception as e:
            # 不把原始异常/连接串写进日志：里面可能带账号密码
            if settings.mongodb_strict:
                raise KnowledgeStoreUnavailable(
                    f"MongoDB 不可用（{type(e).__name__}）且 MONGODB_STRICT=true"
                ) from None
            log.warning("MongoDB 不可用（%s），知识库降级为进程内存储；"
                        "数据不持久，跨进程不共享", type(e).__name__)
            _store = InMemoryKnowledgeStore(degraded=True)
    return _store


def reset_knowledge_store() -> None:
    """测试 / 配置变更后重建连接。"""
    global _store
    with _store_lock:
        _store = None
