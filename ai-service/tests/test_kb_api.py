"""知识库状态 API 与健康探针测试（对应 app/api/kb.py、app/main.py 的 /readyz）。

验收：身份头校验、参数校验、只返回聚合数字不泄露提问内容、
严格模式下明确 503、liveness 与 readiness 分离。
"""
from unittest.mock import patch

from fastapi.testclient import TestClient

from app.main import app
from app.kb.store import InMemoryKnowledgeStore, KnowledgeStoreUnavailable

client = TestClient(app)


def _store_with_data():
    s = InMemoryKnowledgeStore()
    s.upsert_document({"course_id": 1, "resource_id": 7, "status": "parsed"})
    s.log_unanswered(9, 1, "学生问了一个没讲到的问题")
    return s


class TestKbStatusAPI:
    def test_missing_user_id_is_401(self):
        r = client.get("/api/ai/kb/status?course_id=1")
        assert r.status_code == 401

    def test_invalid_course_id_is_400(self):
        r = client.get("/api/ai/kb/status?course_id=0",
                       headers={"X-User-Id": "1"})
        assert r.status_code == 400

    def test_returns_coverage_numbers(self):
        store = _store_with_data()
        with patch("app.api.kb.get_knowledge_store", return_value=store):
            r = client.get("/api/ai/kb/status?course_id=1",
                           headers={"X-User-Id": "1"})
        assert r.status_code == 200
        data = r.json()
        assert data["course_id"] == 1
        assert data["resources"] == 1 and data["parsed_resources"] == 1
        assert data["unanswered_queries"] == 1
        assert data["backend"] in ("memory", "mongodb")

    def test_never_leaks_question_text(self):
        store = _store_with_data()
        with patch("app.api.kb.get_knowledge_store", return_value=store):
            r = client.get("/api/ai/kb/status?course_id=1",
                           headers={"X-User-Id": "1"})
        assert r.status_code == 200
        assert "没讲到的问题" not in r.text
        assert "query" not in r.json()

    def test_strict_mode_unavailable_returns_503(self):
        with patch("app.api.kb.get_knowledge_store",
                   side_effect=KnowledgeStoreUnavailable("MongoDB 不可用")):
            r = client.get("/api/ai/kb/status?course_id=1",
                           headers={"X-User-Id": "1"})
        assert r.status_code == 503

    def test_unexpected_error_returns_503_not_500(self):
        with patch("app.api.kb.get_knowledge_store",
                   side_effect=RuntimeError("secret connection string")):
            r = client.get("/api/ai/kb/status?course_id=1",
                           headers={"X-User-Id": "1"})
        assert r.status_code == 503
        assert "secret connection string" not in r.text


class TestHealthProbes:
    def test_health_is_pure_liveness(self):
        r = client.get("/health")
        assert r.status_code == 200 and r.json()["status"] == "ok"

    def test_readyz_reports_backend_state(self):
        store = InMemoryKnowledgeStore(degraded=True)
        with patch("app.main.get_knowledge_store", return_value=store):
            r = client.get("/readyz")
        assert r.status_code == 200
        kb = r.json()["knowledge_base"]
        assert kb["backend"] == "memory" and kb["degraded"] is True
        assert kb["ping"] is True

    def test_readyz_unavailable_is_503_but_health_still_ok(self):
        with patch("app.main.get_knowledge_store",
                   side_effect=KnowledgeStoreUnavailable("MongoDB 不可用")):
            r = client.get("/readyz")
        assert r.status_code == 503
        assert client.get("/health").status_code == 200

    def test_probes_bypass_internal_token_but_business_routes_do_not(self, monkeypatch):
        """开启内部鉴权时探针仍可访问，业务接口无令牌必须 403。"""
        from app.core.config import settings
        monkeypatch.setattr(settings, "internal_auth_enabled", True)
        monkeypatch.setattr(settings, "internal_token", "only-go-knows")
        with patch("app.main.get_knowledge_store",
                   return_value=InMemoryKnowledgeStore()):
            assert client.get("/health").status_code == 200
            assert client.get("/readyz").status_code == 200
        assert client.get("/api/ai/kb/status?course_id=1",
                          headers={"X-User-Id": "1"}).status_code == 403
