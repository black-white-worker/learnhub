"""生成接口：中文专业题主链路（素材注入、学科语境、英文题判废、失败降级）。"""
import json
from unittest.mock import patch, AsyncMock

import pytest
from fastapi.testclient import TestClient

from app.main import app
from app.questions import zhsource as zs
import app.api.generate as generate_mod


client = TestClient(app)
TEACHER = {"X-User-Id": "1", "X-Role": "teacher"}

GOOD_Q = {
    "stem": "用频率为 2ν 的单色光照射逸出功为 W 的金属，光电子最大初动能如何变化？",
    "options": ["按 Ek = hν - W 线性增大但不一定翻倍", "必然翻倍", "保持不变", "必然减半"],
    "answer": "A", "score": 5,
    "explanation": "依据爱因斯坦光电方程 Ek = hν - W，频率加倍后动能增加 hν，"
                   "只有逸出功为零时才恰好翻倍，因此「必然翻倍」错在忽略逸出功。",
    "focus": "光电方程",
    "basis": "依据[1]：爱因斯坦光电方程 Ek = hν - W",
}

MATERIAL = zs.Material(
    title="光电效应",
    snippet="爱因斯坦光电方程为 Ek = hν - W，其中 h 为普朗克常数，ν 为入射光频率，W 为金属逸出功。"
            "当入射光频率低于截止频率时，无论光强多大都不会有光电子逸出。",
    url="https://zh.wikipedia.org/wiki/%E5%85%89%E7%94%B5%E6%95%88%E5%BA%94",
    provider="中文维基百科",
)


def llm_response(text):
    resp = AsyncMock()
    resp.choices = [AsyncMock()]
    resp.choices[0].message.content = text
    return resp


def llm_patch(contents):
    texts = contents if isinstance(contents, list) else [contents]
    responses = [llm_response(t) for t in texts]
    instance = AsyncMock()
    mock_cls = patch("app.api.generate.AsyncOpenAI", return_value=instance)
    instance.chat.completions.create = AsyncMock(
        side_effect=responses if len(responses) > 1 else None,
        return_value=responses[0] if len(responses) == 1 else None,
    )
    return mock_cls, instance


@pytest.fixture(autouse=True)
def _clear_recent():
    generate_mod._recent.clear()
    yield
    generate_mod._recent.clear()


@pytest.fixture
def material_ok():
    return zs.MaterialResult(materials=[MATERIAL], status="ok",
                             provider="中文维基百科", query="光电效应 物理学")


def test_ai_prompt_uses_chinese_materials_subject_and_basis(material_ok):
    prompts = []
    p, instance = llm_patch(json.dumps([GOOD_Q], ensure_ascii=False))

    async def record(*args, **kwargs):
        prompts.append(kwargs["messages"][0]["content"])
        return llm_response(json.dumps([GOOD_Q], ensure_ascii=False))

    with p, patch("app.api.generate.zhsource.fetch_materials",
                  new=AsyncMock(return_value=material_ok)) as fetch:
        instance.chat.completions.create = record
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1,
                              "subject": "物理学", "grounding": True, "seed": "zh-1"},
                        headers=TEACHER)
    assert r.status_code == 200
    data = r.json()
    prompt = prompts[0]
    assert "[1]《光电效应》" in prompt and "中文维基百科" in prompt
    assert "简体中文" in prompt and "basis" in prompt and "学科语境为「物理学」" in prompt
    assert "禁止照搬" in prompt  # 随机性机制仍在
    assert data["meta"]["material_status"] == "ok"
    assert data["meta"]["materials"][0]["url"].startswith("https://zh.wikipedia.org/wiki/")
    assert data["meta"]["subject"] == "物理学" and data["meta"]["grounding"] is True
    assert data["questions"][0]["stem"].startswith("用频率")
    assert data["questions"][0]["quality_score"] >= 80
    assert fetch.await_count == 1


def test_english_question_is_discarded_and_replaced(material_ok):
    english = {
        "stem": "Which planet is known as the Red Planet in our solar system?",
        "options": ["Mars", "Venus", "Jupiter", "Mercury"], "answer": "A",
        "explanation": "Mars is red due to iron oxide.", "score": 5,
    }
    p, _ = llm_patch([json.dumps([english]), json.dumps([GOOD_Q], ensure_ascii=False)])
    with p, patch("app.api.generate.zhsource.fetch_materials",
                  new=AsyncMock(return_value=material_ok)):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1, "seed": "zh-2"},
                        headers=TEACHER)
    assert r.status_code == 200
    data = r.json()
    assert data["count"] == 1 and data["meta"]["attempts"] == 2
    assert data["discarded_count"] >= 1 and data["invalid_count"] == 0
    assert "用频率" in data["questions"][0]["stem"]
    assert "Which planet" not in json.dumps(data, ensure_ascii=False)


def test_grounding_false_skips_lookup_and_reports_disabled():
    p, _ = llm_patch(json.dumps([GOOD_Q], ensure_ascii=False))
    with p, patch("app.api.generate.zhsource.fetch_materials", new=AsyncMock()) as fetch:
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1,
                              "grounding": False, "seed": "zh-3"},
                        headers=TEACHER)
    assert r.status_code == 200
    assert fetch.await_count == 0
    assert r.json()["meta"]["material_status"] == "disabled"
    assert r.json()["questions"][0]["stem"]


def test_material_unavailable_still_generates_chinese_questions():
    unavailable = zs.MaterialResult(status="unavailable", detail="超时", query="光电效应")
    p, _ = llm_patch(json.dumps([GOOD_Q], ensure_ascii=False))
    with p, patch("app.api.generate.zhsource.fetch_materials",
                  new=AsyncMock(return_value=unavailable)):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1, "seed": "zh-4"},
                        headers=TEACHER)
    assert r.status_code == 200
    data = r.json()
    assert data["meta"]["material_status"] == "unavailable"
    assert data["meta"]["materials"] == []
    assert data["count"] == 1 and data["questions"][0]["quality_score"] >= 80


def test_prompt_forbids_fabricated_sources_without_materials():
    """资料不可用时不能让模型把 basis 编成一本不存在的教材。"""
    prompts = []
    p, instance = llm_patch(json.dumps([GOOD_Q], ensure_ascii=False))

    async def record(*args, **kwargs):
        prompts.append(kwargs["messages"][0]["content"])
        return llm_response(json.dumps([GOOD_Q], ensure_ascii=False))

    unavailable = zs.MaterialResult(status="unavailable", detail="超时")
    with p, patch("app.api.generate.zhsource.fetch_materials",
                  new=AsyncMock(return_value=unavailable)):
        instance.chat.completions.create = record
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1, "seed": "zh-5"},
                        headers=TEACHER)
    assert r.status_code == 200
    assert "不要杜撰书名" in prompts[0]
    assert "简体中文" in prompts[0]


def test_legacy_payload_keeps_working_and_is_chinese_checked():
    p, _ = llm_patch(json.dumps([GOOD_Q], ensure_ascii=False))
    with p:
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1},
                        headers=TEACHER)
    assert r.status_code == 200
    data = r.json()
    assert data["count"] == 1
    assert data["meta"]["subject"] == ""
    assert "quality_score" in data["questions"][0]


def test_bank_mode_is_optional_and_not_grounded():
    """英文通识题库仍可显式选择，但不会掺进中文专业题链路。"""
    from app.questions.bank import BankResult
    english = {
        "stem": "Which planet is known as the Red Planet?",
        "options": ["Mars", "Venus", "Jupiter", "Mercury"], "answer": "A",
        "score": 5, "focus": "Astronomy", "source": "opentdb",
    }
    p, _ = llm_patch("[]")
    with p, patch("app.api.generate.question_bank.fetch_random_questions",
                  new=AsyncMock(return_value=BankResult(questions=[english], provider="opentdb"))), \
            patch("app.api.generate.question_bank.localize_questions",
                  new=AsyncMock(return_value=False)), \
            patch("app.api.generate.zhsource.fetch_materials", new=AsyncMock()) as fetch:
        r = client.post("/api/ai/generate",
                        json={"knowledge": "", "qtype": "single", "count": 1, "source": "bank"},
                        headers=TEACHER)
    assert r.status_code == 200
    assert r.json()["meta"]["source"] == "bank"
    assert r.json()["meta"]["material_status"] == "disabled"
    assert fetch.await_count == 0


def test_retry_prompt_feeds_back_previous_rejection_reasons(material_ok):
    """重试不能只是换个随机种子碰运气，要把上一轮被判废的原因回灌给模型。"""
    broken = {"stem": "短", "options": ["甲", "乙", "丙", "丁"], "answer": "A", "score": 5}
    p, instance = llm_patch([json.dumps([broken]), json.dumps([GOOD_Q], ensure_ascii=False)])
    with p, patch("app.api.generate.zhsource.fetch_materials",
                  new=AsyncMock(return_value=material_ok)):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1,
                              "grounding": True, "seed": "zh-retry-hint"},
                        headers=TEACHER)
    assert r.status_code == 200
    calls = instance.chat.completions.create.await_args_list
    assert len(calls) == 2
    first = calls[0].kwargs["messages"][0]["content"]
    second = calls[1].kwargs["messages"][0]["content"]
    assert "上一轮未通过校验" not in first
    assert "上一轮未通过校验" in second
    assert "题干缺失或过短" in second
    assert r.json()["discarded_count"] == 1 and r.json()["invalid_count"] == 0


def test_all_rejects_falls_back_with_explicit_hint():
    """反复判废时不能给老师一堆静默的灰卡片，必须说明不可入库及下一步怎么做。"""
    broken = {"stem": "短", "options": ["甲", "乙", "丙", "丁"], "answer": "A", "score": 5}
    p, _ = llm_patch(json.dumps([broken], ensure_ascii=False))
    with p, patch("app.api.generate.zhsource.fetch_materials",
                  new=AsyncMock(return_value=zs.MaterialResult(status="no_match"))):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1,
                              "grounding": True, "seed": "zh-all-bad"},
                        headers=TEACHER)
    assert r.status_code == 200
    data = r.json()
    assert data["invalid_count"] == data["count"] == 1
    # 每轮模型都只返回那一道废题：2 轮主体生成 + 4 轮严格补全，逐轮判废都要计数，
    # 否则「明明一直没合格却显示 discarded=2」会误导排查。
    meta = data["meta"]
    assert meta["attempts"] == 6
    assert meta["fill_attempts"] == 4
    assert data["discarded_count"] == meta["attempts"] == 6
    assert all(q.get("invalid") for q in data["questions"])
    assert "不可勾选入库" in meta["hint"]
