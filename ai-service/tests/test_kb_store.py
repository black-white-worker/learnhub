"""知识库存储层测试。

- 内存实现：既验证功能，也验证它作为「Mongo 故障降级后端」的行为等价；
- Mongo 实现：用 fake collection/cursor/GridFS 验证真实调用逻辑（不连真库）；
- 工厂：验证关闭 / 故障降级 / 严格模式 / 单例重置。
"""
import pytest
from unittest.mock import MagicMock, patch

from app.kb import store as store_mod
from app.kb.store import (
    InMemoryKnowledgeStore, KnowledgeStoreUnavailable, MongoKnowledgeStore,
    STATUS_FALLBACK, STATUS_PARSED, build_chunk_doc,
)


@pytest.fixture(autouse=True)
def _reset_factory():
    store_mod.reset_knowledge_store()
    yield
    store_mod.reset_knowledge_store()


# ---------------------------------------------------------------- fake Mongo
class _FakeCursor(list):
    def sort(self, field, direction=1):
        return _FakeCursor(sorted(self, key=lambda d: d.get(field, 0),
                                  reverse=direction < 0))

    def limit(self, n):
        raise AssertionError("词法检索不应在打分排序前做数据库侧 limit")


class _FakeCollection:
    def __init__(self, name):
        self.name = name
        self.docs: list[dict] = []
        self.indexes: list = []

    @staticmethod
    def _match(doc, flt):
        for k, v in (flt or {}).items():
            if isinstance(v, dict) and "$in" in v:
                dv = doc.get(k) or []
                if not any(x in v["$in"] for x in dv):
                    return False
            elif doc.get(k) != v:
                return False
        return True

    def create_index(self, keys, **kw):
        self.indexes.append((keys, kw))
        return kw.get("name", "idx")

    def update_one(self, flt, upd, upsert=False):
        for d in self.docs:
            if self._match(d, flt):
                d.update(upd.get("$set", {}))
                return MagicMock(matched_count=1)
        if upsert:
            new = dict(flt)
            new.update(upd.get("$set", {}))
            self.docs.append(new)
        return MagicMock(matched_count=0)

    def insert_one(self, doc):
        self.docs.append(dict(doc))

    def insert_many(self, docs):
        self.docs.extend(dict(d) for d in docs)

    def find(self, flt=None, proj=None):
        return _FakeCursor([dict(d) for d in self.docs if self._match(d, flt)])

    def find_one(self, flt=None, proj=None):
        for d in self.docs:
            if self._match(d, flt):
                return dict(d)
        return None

    def delete_one(self, flt):
        before = len(self.docs)
        self.docs = [d for d in self.docs if not self._match(d, flt)]
        return MagicMock(deleted_count=before - len(self.docs))

    def delete_many(self, flt):
        return self.delete_one(flt)

    def count_documents(self, flt):
        return len(self.find(flt))


class _FakeDB:
    def __init__(self):
        self.cols: dict[str, _FakeCollection] = {}

    def __getitem__(self, name):
        return self.cols.setdefault(name, _FakeCollection(name))


class _GridOut:
    def __init__(self, fid, filename, data):
        self._id, self.filename, self._data = fid, filename, data

    def read(self):
        return self._data


class _FakeGridFS:
    def __init__(self):
        self.files: list[_GridOut] = []
        self._seq = 0

    def find(self, flt):
        return [g for g in self.files if g.filename == flt.get("filename")]

    def find_one(self, flt):
        r = self.find(flt)
        return r[0] if r else None

    def delete(self, fid):
        self.files = [g for g in self.files if g._id != fid]

    def put(self, data, filename=None, metadata=None):
        self._seq += 1
        fid = f"fid{self._seq}"
        self.files.append(_GridOut(fid, filename, bytes(data)))
        return fid


@pytest.fixture
def mongo_store(monkeypatch):
    db, grid = _FakeDB(), _FakeGridFS()
    client = MagicMock()
    client.admin.command.return_value = {"ok": 1}
    # client["<dbname>"] 要返回 database 对象，database["<collection>"] 才返回集合
    client.__getitem__.side_effect = lambda name: db
    monkeypatch.setattr(store_mod.settings, "mongodb_url", "mongodb://fake:27017")
    with patch("pymongo.MongoClient", return_value=client), \
         patch("gridfs.GridFS", return_value=grid):
        s = MongoKnowledgeStore()
    s._fake_db, s._fake_grid = db, grid
    return s


# ---------------------------------------------------------------- 内存实现
class TestInMemoryStore:
    def test_upsert_document_is_idempotent(self):
        s = InMemoryKnowledgeStore()
        s.upsert_document({"course_id": 1, "resource_id": 7, "status": STATUS_PARSED})
        s.upsert_document({"course_id": 1, "resource_id": 7, "status": STATUS_FALLBACK})
        docs = s.list_documents(1)
        assert len(docs) == 1
        assert docs[0]["status"] == STATUS_FALLBACK
        assert "updated_at" in docs[0]

    def test_replace_chunks_removes_stale_ones(self):
        s = InMemoryKnowledgeStore()
        s.replace_chunks(1, 7, [build_chunk_doc(1, 7, i, f"旧{i}") for i in range(3)])
        s.replace_chunks(1, 7, [build_chunk_doc(1, 7, 0, "新内容")])
        hits = s.search_chunks(1, ["新内"], limit=10)
        assert len(hits) == 1 and hits[0]["text"] == "新内容"

    def test_search_ranks_by_lexical_score(self):
        s = InMemoryKnowledgeStore()
        s.replace_chunks(1, 7, [
            build_chunk_doc(1, 7, 0, "检索增强生成 RAG 的原理"),
            build_chunk_doc(1, 7, 1, "课程签到规则说明"),
            build_chunk_doc(1, 7, 2, "RAG 检索与向量库"),
        ])
        hits = s.search_chunks(1, ["rag", "检索"], limit=3)
        assert len(hits) >= 2
        assert hits[0]["text"] != "课程签到规则说明"

    def test_course_isolation(self):
        s = InMemoryKnowledgeStore()
        s.replace_chunks(1, 7, [build_chunk_doc(1, 7, 0, "检索增强生成")])
        assert s.search_chunks(2, ["检索"], limit=3) == []
        assert s.coverage(2)["resources"] == 0

    def test_empty_query_returns_nothing(self):
        s = InMemoryKnowledgeStore()
        s.replace_chunks(1, 7, [build_chunk_doc(1, 7, 0, "任意内容")])
        assert s.search_chunks(1, [], limit=3) == []
        assert s.search_chunks(1, ["任意"], limit=0) == []

    def test_delete_resource_cleans_file(self):
        s = InMemoryKnowledgeStore()
        s.put_file("c1/r7.pdf", b"bytes")
        s.upsert_document({"course_id": 1, "resource_id": 7, "file_key": "c1/r7.pdf"})
        s.replace_chunks(1, 7, [build_chunk_doc(1, 7, 0, "x")])
        s.delete_resource(1, 7)
        assert s.list_documents(1) == []
        assert s.get_file("c1/r7.pdf") is None
        assert s.search_chunks(1, ["任意"], 3) == []

    def test_coverage_counts_statuses_and_unanswered(self):
        s = InMemoryKnowledgeStore()
        s.upsert_document({"course_id": 1, "resource_id": 1, "status": STATUS_PARSED})
        s.upsert_document({"course_id": 1, "resource_id": 2, "status": STATUS_FALLBACK})
        s.replace_chunks(1, 1, [build_chunk_doc(1, 1, i, f"切片{i}") for i in range(4)])
        s.start_ingestion(1)
        s.log_unanswered(9, 1, "没讲到的知识点")
        c = s.coverage(1)
        assert c["resources"] == 2 and c["parsed_resources"] == 1
        assert c["fallback_resources"] == 1 and c["chunks"] == 4
        assert c["unanswered_queries"] == 1 and c["ingestion_runs"] == 1
        assert c["backend"] == "memory"

    def test_ingestion_lifecycle(self):
        s = InMemoryKnowledgeStore()
        rid = s.start_ingestion(1, {"mode": "full"})
        s.finish_ingestion(rid, {"status": "done", "chunks": 12})
        c = s.coverage(1)
        assert c["ingestion_runs"] == 1

    def test_put_get_file_and_ping(self):
        s = InMemoryKnowledgeStore()
        fid = s.put_file("k", b"abc", {"course_id": 1})
        assert s.get_file("k") == b"abc"
        assert fid.startswith("memory://")
        assert s.ping() is True


class TestBuildChunkDoc:
    def test_all_required_fields(self):
        d = build_chunk_doc(1, 7, 3, "检索增强生成 RAG", {"title": "讲义", "page": 2})
        assert d["course_id"] == 1 and d["resource_id"] == 7
        assert d["chunk_index"] == 3 and d["text"] == "检索增强生成 RAG"
        assert "检索" in d["keywords"] and "rag" in d["keywords"]
        assert d["metadata"] == {"title": "讲义", "page": 2}
        assert d["content_hash"] and d["chunk_id"]

    def test_chunk_ids_differ_by_index(self):
        a = build_chunk_doc(1, 7, 0, "同文")
        b = build_chunk_doc(1, 7, 1, "同文")
        assert a["chunk_id"] != b["chunk_id"]
        assert a["content_hash"] == b["content_hash"]


# ---------------------------------------------------------------- Mongo 实现
class TestMongoStoreLogic:
    def test_creates_required_indexes(self, mongo_store):
        cols = mongo_store._fake_db.cols
        doc_idx = cols["kb_documents"].indexes
        chunk_idx = cols["kb_chunks"].indexes
        unanswered_idx = cols["kb_unanswered"].indexes
        assert any(kw.get("unique") for _, kw in doc_idx)
        assert any("keywords" in dict(keys) for keys, _ in chunk_idx)
        assert len(unanswered_idx) == 1

    def test_upsert_document_idempotent(self, mongo_store):
        mongo_store.upsert_document({"course_id": 1, "resource_id": 7, "status": STATUS_PARSED})
        mongo_store.upsert_document({"course_id": 1, "resource_id": 7, "status": STATUS_FALLBACK})
        docs = mongo_store.list_documents(1)
        assert len(docs) == 1 and docs[0]["status"] == STATUS_FALLBACK

    def test_replace_chunks_deletes_stale_first(self, mongo_store):
        mongo_store.replace_chunks(1, 7, [build_chunk_doc(1, 7, i, f"旧{i}") for i in range(3)])
        mongo_store.replace_chunks(1, 7, [build_chunk_doc(1, 7, 0, "新内容")])
        chunks = mongo_store._fake_db.cols["kb_chunks"].docs
        assert len(chunks) == 1 and chunks[0]["text"] == "新内容"

    def test_search_chunks_ranks_without_db_side_limit(self, mongo_store):
        """_FakeCursor.limit 会直接抛错：保证打分排序前没有被截断。"""
        mongo_store.replace_chunks(1, 7, [
            build_chunk_doc(1, 7, 0, "检索增强生成 RAG 原理"),
            build_chunk_doc(1, 7, 1, "课程签到规则"),
            build_chunk_doc(1, 7, 2, "RAG 检索与向量"),
        ])
        hits = mongo_store.search_chunks(1, ["rag", "检索"], limit=2)
        assert len(hits) == 2
        assert "签到" not in hits[0]["text"]

    def test_put_file_deletes_old_version(self, mongo_store):
        mongo_store.put_file("c1/r7.pdf", b"v1")
        mongo_store.put_file("c1/r7.pdf", b"v2")
        assert len(mongo_store._fake_grid.files) == 1
        assert mongo_store.get_file("c1/r7.pdf") == b"v2"

    def test_delete_resource_cleans_gridfs(self, mongo_store):
        mongo_store.put_file("c1/r7.pdf", b"v1")
        mongo_store.upsert_document({"course_id": 1, "resource_id": 7,
                                     "file_key": "c1/r7.pdf"})
        mongo_store.replace_chunks(1, 7, [build_chunk_doc(1, 7, 0, "x")])
        mongo_store.delete_resource(1, 7)
        assert mongo_store._fake_grid.files == []
        assert mongo_store._fake_db.cols["kb_chunks"].docs == []

    def test_coverage_and_unanswered(self, mongo_store):
        mongo_store.upsert_document({"course_id": 1, "resource_id": 1, "status": STATUS_PARSED})
        mongo_store.start_ingestion(1)
        mongo_store.log_unanswered(9, 1, "没讲到")
        c = mongo_store.coverage(1)
        assert c["parsed_resources"] == 1
        assert c["unanswered_queries"] == 1
        assert c["ingestion_runs"] == 1
        assert c["backend"] == "mongodb" and c["degraded"] is False

    def test_ping(self, mongo_store):
        assert mongo_store.ping() is True


# ---------------------------------------------------------------- 工厂降级
class TestStoreFactory:
    def test_disabled_returns_degraded_memory(self, monkeypatch):
        monkeypatch.setattr(store_mod.settings, "mongodb_enabled", False)
        s = store_mod.get_knowledge_store()
        assert isinstance(s, InMemoryKnowledgeStore) and s.degraded is True

    def test_connection_failure_degrades_when_not_strict(self, monkeypatch):
        monkeypatch.setattr(store_mod.settings, "mongodb_enabled", True)
        monkeypatch.setattr(store_mod.settings, "mongodb_strict", False)
        with patch.object(store_mod, "MongoKnowledgeStore",
                          side_effect=RuntimeError("boom")):
            s = store_mod.get_knowledge_store()
        assert isinstance(s, InMemoryKnowledgeStore) and s.degraded is True

    def test_strict_mode_raises_and_does_not_leak_connection_string(self, monkeypatch):
        monkeypatch.setattr(store_mod.settings, "mongodb_enabled", True)
        monkeypatch.setattr(store_mod.settings, "mongodb_strict", True)
        monkeypatch.setattr(store_mod.settings, "mongodb_url", "mongodb://u:p@host:27017")
        with patch.object(store_mod, "MongoKnowledgeStore",
                          side_effect=RuntimeError("mongodb://u:p@host:27017 refused")):
            with pytest.raises(KnowledgeStoreUnavailable) as exc:
                store_mod.get_knowledge_store()
        assert "u:p" not in str(exc.value)     # 连接串/密码不进异常文本
        assert "boom" not in str(exc.value)

    def test_singleton_and_reset(self, monkeypatch):
        monkeypatch.setattr(store_mod.settings, "mongodb_enabled", False)
        assert store_mod.get_knowledge_store() is store_mod.get_knowledge_store()
        store_mod.reset_knowledge_store()
        assert store_mod.get_knowledge_store() is not None
