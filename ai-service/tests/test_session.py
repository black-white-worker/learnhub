"""会话管理 API 测试。

测试点：
1. 缺 X-User-Id 返回 401
2. course_id <= 0 返回 400
3. 查历史：mock Redis 返回固定消息列表
4. 清空会话：mock Redis delete 被调用
"""
import pytest
from unittest.mock import patch, AsyncMock, MagicMock
from fastapi.testclient import TestClient

from app.main import app


client = TestClient(app)


def test_history_no_user_id():
    r = client.get("/api/ai/session/history?course_id=1")
    assert r.status_code == 401


def test_history_invalid_course():
    r = client.get("/api/ai/session/history?course_id=0", headers={"X-User-Id": "1"})
    assert r.status_code == 400


def test_history_with_data():
    fake_history = [
        {"role": "user", "content": "什么是RAG"},
        {"role": "assistant", "content": "RAG是检索增强生成"},
    ]
    with patch("app.api.session.get_history", new_callable=AsyncMock, return_value=fake_history):
        r = client.get(
            "/api/ai/session/history?course_id=4",
            headers={"X-User-Id": "26"},
        )
        assert r.status_code == 200
        data = r.json()
        assert data["user_id"] == 26
        assert data["course_id"] == 4
        assert len(data["messages"]) == 2
        assert data["rounds"] == 1


def test_history_empty():
    with patch("app.api.session.get_history", new_callable=AsyncMock, return_value=[]):
        r = client.get(
            "/api/ai/session/history?course_id=4",
            headers={"X-User-Id": "26"},
        )
        assert r.status_code == 200
        assert r.json()["rounds"] == 0


def test_clear_session():
    with patch("app.api.session.clear", new_callable=AsyncMock) as mock_clear:
        r = client.delete(
            "/api/ai/session?course_id=4",
            headers={"X-User-Id": "26"},
        )
        assert r.status_code == 200
        assert r.json()["deleted"] is True
        mock_clear.assert_called_once_with(26, 4)


def test_clear_invalid_course():
    r = client.delete("/api/ai/session?course_id=-1", headers={"X-User-Id": "1"})
    assert r.status_code == 400
