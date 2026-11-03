"""AI 出题接口测试。

测试点：
1. 缺 X-User-Id 返回 401
2. 不支持的题型返回 400
3. count 越界返回 400
4. 正常出题返回题目列表（mock LLM）
"""
import pytest
from unittest.mock import patch, AsyncMock
from fastapi.testclient import TestClient

from app.main import app


client = TestClient(app)


def test_generate_no_user_id():
    r = client.post("/api/ai/generate", json={"knowledge": "RAG"})
    assert r.status_code == 401


def test_generate_invalid_type():
    r = client.post(
        "/api/ai/generate",
        json={"knowledge": "RAG", "qtype": "invalid"},
        headers={"X-User-Id": "1"},
    )
    assert r.status_code == 400


def test_generate_count_out_of_range():
    r = client.post(
        "/api/ai/generate",
        json={"knowledge": "RAG", "qtype": "single", "count": 20},
        headers={"X-User-Id": "1"},
    )
    assert r.status_code == 400


def test_generate_single_choice():
    fake_llm = AsyncMock()
    fake_llm.choices = [AsyncMock()]
    fake_llm.choices[0].message.content = (
        '[{"stem":"什么是RAG","options":["检索增强生成","生成对抗网络","强化学习","递归神经网络"],"answer":"A","score":5}]'
    )

    with patch("app.api.generate.AsyncOpenAI") as mock_cls:
        mock_instance = AsyncMock()
        mock_instance.chat.completions.create = AsyncMock(return_value=fake_llm)
        mock_cls.return_value = mock_instance

        r = client.post(
            "/api/ai/generate",
            json={"knowledge": "RAG", "qtype": "single", "count": 1, "score": 5},
            headers={"X-User-Id": "1"},
        )
        assert r.status_code == 200
        data = r.json()
        assert data["qtype"] == "single"
        assert len(data["questions"]) == 1
        assert data["questions"][0]["answer"] == "A"


def test_generate_llm_returns_raw_text():
    """LLM 返回非 JSON 时降级返回原始文本。"""
    fake_llm = AsyncMock()
    fake_llm.choices = [AsyncMock()]
    fake_llm.choices[0].message.content = "这不是JSON"

    with patch("app.api.generate.AsyncOpenAI") as mock_cls:
        mock_instance = AsyncMock()
        mock_instance.chat.completions.create = AsyncMock(return_value=fake_llm)
        mock_cls.return_value = mock_instance

        r = client.post(
            "/api/ai/generate",
            json={"knowledge": "RAG", "qtype": "essay", "count": 1},
            headers={"X-User-Id": "1"},
        )
        assert r.status_code == 200
        assert r.json().get("raw") is True
