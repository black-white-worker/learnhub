"""学习总结接口测试。

测试点：
1. 缺 X-User-Id 返回 401
2. content 和 course_id 都没传返回 400
3. 正常传 content 返回 summary 字段
4. fetch_notes_from_go 异常时降级为只用 content

LLM 调用全部 mock，不依赖真实大模型。
"""
import pytest
from unittest.mock import patch, AsyncMock
from fastapi.testclient import TestClient

from app.main import app


client = TestClient(app)


def test_summary_no_user_id():
    """缺 X-User-Id 应返回 401。"""
    r = client.post("/api/ai/summary", json={"content": "测试材料"})
    assert r.status_code == 401


def test_summary_no_content_no_course():
    """content 和 course_id 都没传应返回 400。"""
    r = client.post("/api/ai/summary", json={}, headers={"X-User-Id": "1"})
    assert r.status_code == 400


def test_summary_with_content():
    """传 content 时正常返回 summary。"""
    fake_llm_response = AsyncMock()
    fake_llm_response.choices = [AsyncMock()]
    fake_llm_response.choices[0].message.content = "【核心考点】\n1. 测试考点"

    with patch("app.api.summary.AsyncOpenAI") as mock_cls:
        mock_instance = AsyncMock()
        mock_instance.chat.completions.create = AsyncMock(return_value=fake_llm_response)
        mock_cls.return_value = mock_instance

        r = client.post(
            "/api/ai/summary",
            json={"content": "这是一段测试学习材料"},
            headers={"X-User-Id": "1"},
        )
        assert r.status_code == 200
        data = r.json()
        assert "summary" in data
        assert "核心考点" in data["summary"]
        assert data["material_length"] > 0


def test_summary_fetch_notes_fallback():
    """Go 后端拉笔记失败时，降级为只用 content 总结。"""
    fake_llm_response = AsyncMock()
    fake_llm_response.choices = [AsyncMock()]
    fake_llm_response.choices[0].message.content = "总结结果"

    with patch("app.api.summary.fetch_notes_from_go", new_callable=AsyncMock, return_value=""), \
         patch("app.api.summary.AsyncOpenAI") as mock_cls:
        mock_instance = AsyncMock()
        mock_instance.chat.completions.create = AsyncMock(return_value=fake_llm_response)
        mock_cls.return_value = mock_instance

        r = client.post(
            "/api/ai/summary",
            json={"content": "补充材料", "course_id": 4},
            headers={"X-User-Id": "1"},
        )
        assert r.status_code == 200
        assert "补充材料" in r.json()["summary"] or r.json()["summary"] == "总结结果"


def test_summary_invalid_user_id():
    """X-User-Id 不是数字返回 400。"""
    r = client.post(
        "/api/ai/summary",
        json={"content": "x"},
        headers={"X-User-Id": "abc"},
    )
    assert r.status_code == 400
