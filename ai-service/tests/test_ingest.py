"""知识灌入脚本测试（对应 scripts/ingest.py）。

覆盖：连接配置化、按类型真解析、resource_id 补齐、双写幂等、状态可观测、
单资源失败隔离、dry-run 完全只读、视频 TODO 如实标注。
"""
import io
import sys
import types
import zipfile
from unittest.mock import MagicMock, patch

import pytest

from app.kb.store import (
    STATUS_FAILED, STATUS_FALLBACK, STATUS_PARSED, InMemoryKnowledgeStore,
)
from app.rag import parsers
from scripts import ingest


def _docx_bytes(paragraphs):
    body = "".join(f"<w:p><w:r><w:t>{p}</w:t></w:r></w:p>" for p in paragraphs)
    xml = ('<?xml version="1.0"?>'
           '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">'
           f"<w:body>{body}</w:body></w:document>")
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("word/document.xml", xml)
    return buf.getvalue()


class FakeVectorStore:
    def __init__(self):
        self.calls = []
        self.upserts = []

    def delete_by_resource(self, course_id, resource_id):
        self.calls.append(("delete", course_id, resource_id))

    def upsert(self, course_id, chunks, metadatas):
        self.calls.append(("upsert", course_id))
        self.upserts.append((list(chunks), [dict(m) for m in metadatas]))


@pytest.fixture(autouse=True)
def _offline_encoder(monkeypatch):
    """强制走离线字符切分，测试不需要 tiktoken / 不联网。"""
    monkeypatch.setitem(sys.modules, "tiktoken", None)
    monkeypatch.setattr(ingest, "_ENC", None)
    monkeypatch.setattr(ingest, "_ENCODER_TRIED", False)


@pytest.fixture
def resource():
    return {
        "id": 7, "course_id": 1, "name": "第一章讲义.docx",
        "url": "http://minio/course/a.docx", "object_name": "course/1/a.docx",
        "type": "docx", "size": 1024,
    }


@pytest.fixture
def docx_fetcher():
    data = _docx_bytes(["RAG 检索增强生成的原理", "向量库与混合检索"])
    return lambda r: (data, "")


class TestSplitText:
    def test_empty(self):
        assert ingest.split_text("") == []

    def test_invalid_params(self):
        with pytest.raises(ValueError):
            ingest.split_text("text", chunk_size=0)
        with pytest.raises(ValueError):
            ingest.split_text("text", chunk_size=10, overlap=10)
        with pytest.raises(ValueError):
            ingest.split_text("text", chunk_size=10, overlap=-1)

    def test_offline_fallback_splits_with_overlap(self):
        chunks = ingest.split_text("abcdefghij", chunk_size=4, overlap=1)
        assert chunks == ["abcd", "defg", "ghij"]

    def test_token_path_with_fake_encoder(self, monkeypatch):
        mod = types.ModuleType("tiktoken")

        class _Enc:
            def encode(self, text):
                return list(text)

            def decode(self, toks):
                return "".join(toks)

        mod.get_encoding = lambda name: _Enc()
        monkeypatch.setitem(sys.modules, "tiktoken", mod)
        monkeypatch.setattr(ingest, "_ENC", None)
        monkeypatch.setattr(ingest, "_ENCODER_TRIED", False)
        assert ingest.split_text("abcdefghij", 4, 1) == ["abcd", "defg", "ghij"]


class TestFetchResources:
    def test_no_hardcoded_credentials_and_selects_needed_columns(self, monkeypatch):
        from app.core.config import settings
        # 这条钉的是「Go 清单接口不可用时的兜底」：清单拿不到才允许查库
        monkeypatch.setattr(
            ingest.kb_manifest, "fetch_course_manifest",
            lambda cid, use_cache=True: None)
        conn = MagicMock()
        cur = conn.cursor.return_value.__enter__.return_value
        cur.fetchall.return_value = [{"id": 1}]
        monkeypatch.setattr(settings, "mysql_password", "from-settings")
        with patch("pymysql.connect", return_value=conn) as m:
            rows = ingest.fetch_resources(42)
        assert rows == [{"id": 1}]
        assert m.call_args.kwargs == settings.mysql_conn_kwargs
        sql, params = cur.execute.call_args.args
        assert "object_name" in sql and "type" in sql
        assert "localhost/root/root" not in sql
        assert params == (42,)
        conn.close.assert_called_once()


class TestFetchResourcesFromCourseApi:
    """灌库数据来源必须与课程详情页一致：先问 Go 的课程资料接口，MySQL 只兜底。"""

    def test_manifest_rows_are_used_and_mysql_is_not_touched(self, monkeypatch):
        payload = {"course_id": 42, "resources": [{
            "id": 6, "name": "实验报告.docx", "url": "http://minio/presigned",
            "object_name": "course/42/resources/r6.docx", "type": "docx", "size": 884,
        }]}
        monkeypatch.setattr(
            ingest.kb_manifest, "fetch_course_manifest",
            lambda cid, use_cache=True: payload)
        with patch("pymysql.connect") as m:
            rows = ingest.fetch_resources(42)
        assert rows == [{
            "id": 6, "course_id": 42, "name": "实验报告.docx",
            "url": "http://minio/presigned",
            "object_name": "course/42/resources/r6.docx",
            "type": "docx", "size": 884,
        }]
        m.assert_not_called()

    def test_empty_manifest_means_no_material_not_db_fallback(self, monkeypatch):
        """清单可用但为空 = 老师把讲义删了：不能再去库里把残留行灌回来。"""
        monkeypatch.setattr(
            ingest.kb_manifest, "fetch_course_manifest",
            lambda cid, use_cache=True: {"course_id": 42, "resources": []})
        with patch("pymysql.connect") as m:
            assert ingest.fetch_resources(42) == []
        m.assert_not_called()

    def test_manifest_failure_falls_back_to_db(self, monkeypatch):
        monkeypatch.setattr(
            ingest.kb_manifest, "fetch_course_manifest",
            lambda cid, use_cache=True: None)
        conn = MagicMock()
        cur = conn.cursor.return_value.__enter__.return_value
        cur.fetchall.return_value = [{"id": 9}]
        with patch("pymysql.connect", return_value=conn):
            assert ingest.fetch_resources(42) == [{"id": 9}]


class TestBuildChunks:
    def test_metadata_has_resource_id_and_keeps_page(self, resource):
        sections = [parsers.Section("检索增强生成", {"page": 3, "title": "讲义"})]
        chunks, metas = ingest.build_chunks(1, resource, sections)
        assert chunks == ["检索增强生成"]
        assert metas[0]["resource_id"] == 7
        assert metas[0]["course_id"] == 1
        assert metas[0]["page"] == 3
        assert metas[0]["title"] == "第一章讲义.docx"

    def test_multiple_sections(self, resource):
        sections = [parsers.Section("A", {"page": 1}), parsers.Section("B", {"page": 2})]
        chunks, metas = ingest.build_chunks(1, resource, sections)
        assert len(chunks) == 2
        assert [m["page"] for m in metas] == [1, 2]


class TestIngestCourse:
    def test_parses_real_body_and_writes_both_stores(self, resource, docx_fetcher):
        store, vec = InMemoryKnowledgeStore(), FakeVectorStore()
        stats = ingest.ingest_course(1, dry_run=False, resources=[resource],
                                     store=store, vector=vec, fetcher=docx_fetcher)
        assert stats["parsed"] == 1 and stats["chunks"] == 2 and stats["files"] == 1
        doc = store.list_documents(1)[0]
        assert doc["status"] == STATUS_PARSED and doc["body_parsed"] is True
        assert doc["chunk_count"] == 2 and doc["file_key"]
        assert store.get_file(doc["file_key"]) is not None
        assert store.coverage(1)["chunks"] == 2
        chunks, metas = vec.upserts[0]
        assert len(chunks) == 2
        assert all(m["resource_id"] == 7 and m["course_id"] == 1 for m in metas)

    def test_deletes_old_vectors_before_upsert(self, resource, docx_fetcher):
        vec = FakeVectorStore()
        ingest.ingest_course(1, dry_run=False, resources=[resource],
                             store=InMemoryKnowledgeStore(), vector=vec,
                             fetcher=docx_fetcher)
        assert vec.calls[0] == ("delete", 1, 7)
        assert vec.calls[1] == ("upsert", 1)

    def test_fallback_when_file_unavailable(self, resource):
        resource = dict(resource, type="pdf", name="第一章讲义.pdf")
        store, vec = InMemoryKnowledgeStore(), FakeVectorStore()
        stats = ingest.ingest_course(1, dry_run=False, resources=[resource],
                                     store=store, vector=vec,
                                     fetcher=lambda r: (None, "拉不到文件"))
        assert stats["fallback"] == 1 and stats["parsed"] == 0
        doc = store.list_documents(1)[0]
        assert doc["status"] == STATUS_FALLBACK and doc["body_parsed"] is False
        assert "拉不到文件" in doc["reason"]
        assert doc["file_key"] == ""
        assert vec.upserts[0][1][0]["body"] is False

    def test_rerun_is_idempotent(self, resource, docx_fetcher):
        store, vec = InMemoryKnowledgeStore(), FakeVectorStore()
        for _ in range(2):
            ingest.ingest_course(1, dry_run=False, resources=[resource],
                                 store=store, vector=vec, fetcher=docx_fetcher)
        assert store.coverage(1)["chunks"] == 2       # 不翻倍
        assert len(store.list_documents(1)) == 1
        assert [c[0] for c in vec.calls] == ["delete", "upsert", "delete", "upsert"]

    def test_failure_isolated_to_one_resource(self, resource, docx_fetcher):
        bad = dict(resource, id=8, name="坏资源.docx")

        def flaky(r):
            if r["id"] == 8:
                raise RuntimeError("boom")
            return docx_fetcher(r)

        store, vec = InMemoryKnowledgeStore(), FakeVectorStore()
        stats = ingest.ingest_course(1, dry_run=False, resources=[bad, resource],
                                     store=store, vector=vec, fetcher=flaky)
        assert stats["failed"] == 1 and stats["parsed"] == 1
        docs = {d["resource_id"]: d for d in store.list_documents(1)}
        assert docs[8]["status"] == STATUS_FAILED
        assert "boom" not in docs[8]["reason"]     # 异常原文不落库
        assert docs[7]["status"] == STATUS_PARSED

    def test_empty_course(self):
        stats = ingest.ingest_course(9, dry_run=True, resources=[], store=None)
        assert stats["resources"] == 0 and stats["chunks"] == 0

    def test_video_resource_is_honestly_marked(self, resource):
        video = dict(resource, type="mp4", name="第一课.mp4")
        store, vec = InMemoryKnowledgeStore(), FakeVectorStore()
        stats = ingest.ingest_course(1, dry_run=False, resources=[video],
                                     store=store, vector=vec,
                                     fetcher=lambda r: (b"fake-video", ""))
        assert stats["fallback"] == 1
        doc = store.list_documents(1)[0]
        assert doc["status"] == STATUS_FALLBACK and doc["body_parsed"] is False
        assert "TODO" in doc["reason"]


class TestDryRun:
    def test_dry_run_is_completely_read_only(self, resource, docx_fetcher):
        """不连 Mongo、不构造 Chroma、不写任何集合，只返回统计。"""
        with patch.object(ingest, "get_knowledge_store",
                          side_effect=AssertionError("dry-run 不该连 Mongo")):
            stats = ingest.ingest_course(1, dry_run=True, resources=[resource],
                                         fetcher=docx_fetcher)
        assert stats["dry_run"] is True and stats["chunks"] == 2

    def test_dry_run_does_not_mutate_supplied_store(self, resource, docx_fetcher):
        store, vec = InMemoryKnowledgeStore(), FakeVectorStore()
        ingest.ingest_course(1, dry_run=True, resources=[resource],
                             store=store, vector=vec, fetcher=docx_fetcher)
        assert store.list_documents(1) == []
        assert store.coverage(1)["chunks"] == 0
        assert vec.calls == []
