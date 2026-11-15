"""免费公开题库连接器单测：解析、HTML 解码、token 防重、备用源、降级、汉化。

全部用注入的 FakeClient，不发真实网络请求。
"""
import json
from unittest.mock import AsyncMock

import pytest

from app.questions import bank


class FakeResponse:
    def __init__(self, data):
        self._data = data

    def json(self):
        return self._data


class FakeClient:
    def __init__(self, handler):
        self.handler = handler
        self.calls = []

    async def get(self, url, params=None, timeout=None):
        self.calls.append((url, dict(params or {})))
        return self.handler(url, params or {})


def opentdb_item(question="Which planet is &quot;red&quot;?", correct="Mars"):
    return {
        "type": "multiple", "difficulty": "medium", "category": "Science &amp; Nature",
        "question": question, "correct_answer": correct,
        "incorrect_answers": ["Venus", "Jupiter", "Mercury"],
    }


@pytest.fixture(autouse=True)
def reset_bank_state(monkeypatch):
    monkeypatch.setattr(bank, "_session_token", "")
    monkeypatch.setattr(bank, "_token_at", 0.0)
    monkeypatch.setattr(bank.settings, "question_bank_enabled", True)
    monkeypatch.setattr(bank.settings, "question_bank_use_token", False)
    monkeypatch.setattr(bank.settings, "question_bank_localize", True)


@pytest.mark.asyncio
async def test_opentdb_multiple_choice_is_parsed_and_answer_matches_option():
    data = {"response_code": 0, "results": [opentdb_item()]}
    client = FakeClient(lambda url, params: FakeResponse(data))
    result = await bank.fetch_random_questions(1, "single", "中等", 5, client=client)

    assert result.ok and result.provider == "opentdb"
    assert result.status == "ok"
    q = result.questions[0]
    assert "red" in q["stem"] and "&quot;" not in q["stem"]  # HTML 实体已解码
    assert q["focus"] == "Science & Nature" and "&amp;" not in q["focus"]
    assert len(q["options"]) == 4
    assert q["options"][ord(q["answer"]) - 65] == "Mars"
    assert q["score"] == 5 and q["source"] == "opentdb"
    assert client.calls[0][1]["type"] == "multiple"
    assert client.calls[0][1]["difficulty"] == "medium"


@pytest.mark.asyncio
async def test_opentdb_boolean_becomes_judge_question():
    data = {"response_code": 0, "results": [{
        "type": "boolean", "category": "Science: Computers",
        "question": "HTML stands for Hyper Text Markup Language.",
        "correct_answer": "True", "incorrect_answers": ["False"],
    }]}
    client = FakeClient(lambda url, params: FakeResponse(data))
    result = await bank.fetch_random_questions(1, "judge", "简单", 3, client=client)
    assert result.ok
    q = result.questions[0]
    assert q["answer"] is True and q["options"] == [] and q["score"] == 3
    assert client.calls[0][1]["type"] == "boolean"
    assert client.calls[0][1]["difficulty"] == "easy"


@pytest.mark.asyncio
async def test_falls_back_to_second_free_provider():
    def handler(url, params):
        if "opentdb.com/api.php" in url:
            return FakeResponse({"response_code": 5, "results": []})  # 限流
        return FakeResponse([{
            "category": "science_and_nature", "type": "text_choice",
            "question": {"text": "What gas do plants absorb?"},
            "correctAnswer": "Carbon dioxide",
            "incorrectAnswers": ["Oxygen", "Nitrogen", "Helium"],
        }])

    client = FakeClient(handler)
    result = await bank.fetch_random_questions(1, "single", "中等", 5, client=client)
    assert result.ok and result.provider == "the-trivia-api"
    assert result.questions[0]["options"][ord(result.questions[0]["answer"]) - 65] == "Carbon dioxide"
    assert len(client.calls) == 2


@pytest.mark.asyncio
async def test_judge_mode_does_not_mix_in_choice_questions():
    calls = []

    def handler(url, params):
        calls.append(url)
        raise RuntimeError("network down")

    client = FakeClient(handler)
    result = await bank.fetch_random_questions(1, "judge", "中等", 5, client=client)
    assert not result.ok and result.status == "unavailable"
    assert len(calls) == 1  # 判断题没有备用源可用，不混题型


@pytest.mark.asyncio
async def test_all_sources_failing_never_raises():
    client = FakeClient(lambda url, params: (_ for _ in ()).throw(TimeoutError("timeout")))
    result = await bank.fetch_random_questions(2, "single", "中等", 5, client=client)
    assert not result.ok
    assert result.status == "unavailable" and result.detail


@pytest.mark.asyncio
async def test_disabled_bank_short_circuits(monkeypatch):
    monkeypatch.setattr(bank.settings, "question_bank_enabled", False)
    client = FakeClient(lambda url, params: FakeResponse({"response_code": 0, "results": [opentdb_item()]}))
    result = await bank.fetch_random_questions(1, "single", "中等", 5, client=client)
    assert result.status == "disabled" and not client.calls


@pytest.mark.asyncio
async def test_session_token_is_retried_when_expired(monkeypatch):
    monkeypatch.setattr(bank.settings, "question_bank_use_token", True)
    seen = []

    def handler(url, params):
        seen.append((url, dict(params)))
        if "api_token" in url:
            return FakeResponse({"response_code": 0, "token": "TOKEN123"})
        if params.get("token") == "TOKEN123":
            return FakeResponse({"response_code": 3, "results": []})  # token 失效
        return FakeResponse({"response_code": 0, "results": [opentdb_item()]})

    client = FakeClient(handler)
    result = await bank.fetch_random_questions(1, "single", "中等", 5, client=client)
    assert result.ok
    api_calls = [(u, p) for u, p in seen if "api.php" in u]
    assert api_calls[0][1].get("token") == "TOKEN123"
    assert "token" not in api_calls[1][1]
    assert bank._session_token == ""  # 已重置，下次重新申请


@pytest.mark.asyncio
async def test_localize_translates_text_but_keeps_answer_mapping():
    questions = [{
        "stem": "Which planet is red?", "options": ["Mars", "Venus", "Jupiter", "Mercury"],
        "answer": "A", "source": "opentdb",
    }]
    translated = json.dumps([{"q": "哪颗行星是红色的？", "opts": ["火星", "金星", "木星", "水星"]}], ensure_ascii=False)
    llm = AsyncMock()
    llm.chat.completions.create = AsyncMock(return_value=_resp(translated))

    assert await bank.localize_questions(questions, llm) is True
    assert questions[0]["stem"] == "哪颗行星是红色的？"
    assert questions[0]["options"][ord(questions[0]["answer"]) - 65] == "火星"
    assert questions[0]["localized"] is True


@pytest.mark.asyncio
async def test_localize_failure_keeps_english_original():
    questions = [{"stem": "Which planet is red?", "options": ["Mars", "Venus"], "answer": "A"}]
    llm = AsyncMock()
    llm.chat.completions.create = AsyncMock(return_value=_resp("抱歉，我无法翻译"))
    assert await bank.localize_questions(questions, llm) is False
    assert questions[0]["stem"] == "Which planet is red?"


@pytest.mark.asyncio
async def test_localize_rejects_length_mismatch():
    questions = [{"stem": "Q1", "options": ["a", "b"], "answer": "A"},
                 {"stem": "Q2", "options": ["a", "b"], "answer": "B"}]
    llm = AsyncMock()
    llm.chat.completions.create = AsyncMock(return_value=_resp('[{"q":"题1","opts":["甲","乙"]}]'))
    assert await bank.localize_questions(questions, llm) is False
    assert questions[0]["stem"] == "Q1"


@pytest.mark.asyncio
async def test_inspiration_disabled_by_default_and_injects_when_enabled(monkeypatch):
    monkeypatch.setattr(bank.settings, "question_bank_inspire", False)
    text, status = await bank.fetch_inspiration(2, "seed")
    assert text == "" and status == "disabled"

    monkeypatch.setattr(bank.settings, "question_bank_inspire", True)
    sample = bank.BankResult(questions=[{
        "stem": "What is the capital of France?", "options": ["Paris", "Rome", "Madrid", "Berlin"],
        "answer": "A",
    }], provider="opentdb")
    monkeypatch.setattr(bank, "fetch_random_questions", AsyncMock(return_value=sample))
    text, status = await bank.fetch_inspiration(2, "seed")
    assert "What is the capital of France?" in text
    assert "禁止照抄" in text
    assert status == "opentdb"


def _resp(text):
    r = AsyncMock()
    r.choices = [AsyncMock()]
    r.choices[0].message.content = text
    return r
