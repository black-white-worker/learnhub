"""知识库灌库/清理接口与后台执行器测试（对应 app/api/kb.py、app/kb/pipeline.py）。

回归背景：讲义上传后没有任何东西把内容写进知识库，`scripts/ingest.py` 只能手工跑，
于是答疑永远检索为空、只会回「资料中没有相关内容」。这里锁定三条契约：
  1. 灌库/清理必须带可信身份头（内部令牌），否则 401；
  2. 灌库立刻返回（后台线程解析），重复提交同一门课不排队；
  3. 删除讲义要把切片和向量一起清掉，不能留幽灵资料。
"""
from unittest.mock import patch

from fastapi.testclient import TestClient

from app.kb import pipeline
from app.main import app

client = TestClient(app)


class TestIngestEndpoint:
    def test_requires_trusted_identity(self):
        r = client.post("/api/ai/kb/ingest", json={"course_id": 1})
        assert r.status_code == 401

    def test_invalid_course_id_is_400(self):
        r = client.post("/api/ai/kb/ingest", json={"course_id": 0},
                        headers={"X-Internal-Token": "t"})
        assert r.status_code == 400

    def test_invalid_resource_id_is_400(self):
        r = client.post("/api/ai/kb/ingest", json={"course_id": 1, "resource_id": -3},
                        headers={"X-Internal-Token": "t"})
        assert r.status_code == 400

    def test_accepts_and_returns_immediately(self):
        with patch.object(pipeline, "start_ingest", return_value=pipeline.STARTED) as start, \
             patch.object(pipeline, "is_running", return_value=True), \
             patch.object(pipeline, "pending_count", return_value=0):
            r = client.post("/api/ai/kb/ingest",
                            json={"course_id": 7, "resource_id": 9},
                            headers={"X-User-Id": "5"})
        assert r.status_code == 200
        body = r.json()
        assert body["accepted"] is True and body["running"] is True
        assert body["queued"] is False
        assert body["course_id"] == 7 and body["resource_id"] == 9
        start.assert_called_once_with(7, 9)

    def test_busy_course_queues_instead_of_dropping(self):
        """老师连传两份讲义：第二份必须排队，不能被静默跳过。"""
        with patch.object(pipeline, "start_ingest", return_value=pipeline.QUEUED), \
             patch.object(pipeline, "is_running", return_value=True), \
             patch.object(pipeline, "pending_count", return_value=1):
            r = client.post("/api/ai/kb/ingest", json={"course_id": 7},
                            headers={"X-User-Id": "5"})
        assert r.status_code == 200
        body = r.json()
        assert body["accepted"] is True and body["queued"] is True
        assert body["pending"] == 1
        assert "排队" in body["reason"]


class TestPurgeEndpoint:
    def test_requires_trusted_identity(self):
        r = client.post("/api/ai/kb/purge", json={"course_id": 1, "resource_id": 2})
        assert r.status_code == 401

    def test_invalid_ids_are_400(self):
        r = client.post("/api/ai/kb/purge", json={"course_id": 1, "resource_id": 0},
                        headers={"X-Internal-Token": "t"})
        assert r.status_code == 400

    def test_purges_chunks_and_vectors(self):
        with patch.object(pipeline, "purge_resource",
                          return_value={"course_id": 1, "resource_id": 2,
                                        "chunks_cleared": True, "vectors_cleared": True}) as purge:
            r = client.post("/api/ai/kb/purge", json={"course_id": 1, "resource_id": 2},
                            headers={"X-Internal-Token": "t"})
        assert r.status_code == 200
        assert r.json()["chunks_cleared"] is True
        purge.assert_called_once_with(1, 2)


class TestPipeline:
    def test_second_submission_is_queued_and_then_run(self, monkeypatch):
        """并发灌同一门课会让 Chroma 的「先删后写」互相踩踏：串行 + 排队不丢任务。"""
        import threading
        import time

        gate = threading.Event()
        calls: list[tuple[int, int | None]] = []

        def slow(course_id, resource_id=None):
            calls.append((course_id, resource_id))
            gate.wait(2)
            return {}

        monkeypatch.setattr(pipeline, "run_ingest", slow)
        monkeypatch.setattr(pipeline, "_running", {})
        monkeypatch.setattr(pipeline, "_pending", {})

        assert pipeline.start_ingest(7, 9) == pipeline.STARTED
        # 第二次提交（换了资源）不能丢：排队，等前一个跑完自动接上
        assert pipeline.start_ingest(7, 10) == pipeline.QUEUED
        assert pipeline.pending_count(7) == 1

        gate.set()
        for _ in range(300):
            if not pipeline.is_running(7):
                break
            time.sleep(0.01)
        assert calls == [(7, 9), (7, 10)]
        assert pipeline.pending_count(7) == 0

    def test_run_ingest_filters_single_resource(self):
        rows = [{"id": 6, "course_id": 7, "name": "a.docx"},
                {"id": 7, "course_id": 7, "name": "b.docx"}]
        with patch("scripts.ingest.fetch_resources", return_value=rows), \
             patch("scripts.ingest.ingest_course", return_value={"chunks": 3}) as ingest:
            stats = pipeline.run_ingest(7, 7)
        assert stats["chunks"] == 3
        passed = ingest.call_args.kwargs["resources"]
        assert [r["id"] for r in passed] == [7]

    def test_run_ingest_whole_course_passes_none(self):
        with patch("scripts.ingest.fetch_resources", return_value=[]), \
             patch("scripts.ingest.ingest_course", return_value={"chunks": 0}) as ingest:
            pipeline.run_ingest(7)
        assert ingest.call_args.kwargs["resources"] is None

    def test_missing_resource_is_skipped_without_touching_store(self):
        with patch("scripts.ingest.fetch_resources", return_value=[]), \
             patch("scripts.ingest.ingest_course") as ingest:
            stats = pipeline.run_ingest(7, 999)
        assert stats.get("skipped") is True
        ingest.assert_not_called()

    def test_purge_clears_both_stores(self):
        store = type("S", (), {"delete_resource": lambda self, c, r: setattr(self, "hit", (c, r))})()
        vec = type("V", (), {"delete_by_resource": lambda self, c, r: setattr(self, "hit", (c, r))})()
        with patch("app.kb.store.get_knowledge_store", return_value=store), \
             patch("app.rag.vector_store.get_vector_store", return_value=vec):
            out = pipeline.purge_resource(3, 4)
        assert out["chunks_cleared"] and out["vectors_cleared"]
        assert store.hit == (3, 4) and vec.hit == (3, 4)

    def test_purge_failure_does_not_raise(self):
        with patch("app.kb.store.get_knowledge_store", side_effect=RuntimeError("mongo down")), \
             patch("app.rag.vector_store.get_vector_store", side_effect=RuntimeError("chroma down")):
            out = pipeline.purge_resource(3, 4)
        assert out["chunks_cleared"] is False and out["vectors_cleared"] is False
