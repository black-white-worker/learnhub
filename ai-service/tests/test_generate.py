"""AI 出题接口测试（随机性 / 去重 / 题库降级版）。

测试点：
1. 缺 X-User-Id 返回 401；不支持的题型 / count 越界返回 400
2. 正常出题返回题目列表（mock LLM），答案与选项内容自洽
3. 两次不同 seed 的 prompt 真的不一样（这是「每次都一样」的根因修复）
4. exclude_stems 会把上一批重复题判掉
5. 废题会触发换蓝图重试补齐
6. 免费题库可用时直出、不可用时自动回退 AI
"""
import json
import pytest
from unittest.mock import patch, AsyncMock
from fastapi.testclient import TestClient

from app.main import app
from app.questions.bank import BankResult
from app.questions.variety import RecentStemCache
import app.api.generate as generate_mod


client = TestClient(app)
TEACHER_HEADERS = {"X-User-Id": "1", "X-Role": "teacher"}


def llm_response(text):
    resp = AsyncMock()
    resp.choices = [AsyncMock()]
    resp.choices[0].message.content = text
    return resp


def llm_patch(contents):
    """patch 掉 AsyncOpenAI，contents 是每次 LLM 调用依次返回的文本。"""
    responses = [llm_response(c) for c in contents] if isinstance(contents, list) else [llm_response(contents)]
    instance = AsyncMock()
    mock_cls = patch("app.api.generate.AsyncOpenAI", return_value=instance)
    instance.chat.completions.create = AsyncMock(side_effect=responses if len(responses) > 1 else None,
                                                 return_value=responses[0] if len(responses) == 1 else None)
    return mock_cls, instance


@pytest.fixture(autouse=True)
def clear_recent():
    generate_mod._recent.clear()
    yield
    generate_mod._recent.clear()


def test_generate_no_user_id():
    r = client.post("/api/ai/generate", json={"knowledge": "RAG"})
    assert r.status_code == 401


def test_generate_invalid_type():
    r = client.post("/api/ai/generate", json={"knowledge": "RAG", "qtype": "invalid"},
                    headers=TEACHER_HEADERS)
    assert r.status_code == 400


def test_generate_count_out_of_range():
    r = client.post("/api/ai/generate", json={"knowledge": "RAG", "qtype": "single", "count": 20},
                    headers=TEACHER_HEADERS)
    assert r.status_code == 400


def test_generate_ai_requires_knowledge():
    r = client.post("/api/ai/generate", json={"knowledge": "  ", "source": "ai"},
                    headers=TEACHER_HEADERS)
    assert r.status_code == 400


def test_generate_single_choice():
    body = json.dumps([{
        "stem": "什么是RAG？请指出下列技术中属于检索增强生成的那一项",
        "options": ["检索增强生成", "生成对抗网络", "强化学习", "递归神经网络"],
        "answer": "A", "score": 5, "explanation": "RAG 会先检索外部知识再生成答案",
        "focus": "RAG 定义",
    }], ensure_ascii=False)
    p, instance = llm_patch(body)
    with p:
        r = client.post("/api/ai/generate",
                        json={"knowledge": "RAG", "qtype": "single", "count": 1, "score": 5},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200
    data = r.json()
    assert data["qtype"] == "single"
    assert len(data["questions"]) == 1
    q = data["questions"][0]
    # 选项被洗牌后答案字母会变，但正确选项内容必须始终跟答案对应
    assert q["answer"] in "ABCD"
    assert q["options"][ord(q["answer"]) - 65] == "检索增强生成"
    assert q["explanation"]
    assert data["meta"]["seed"] and data["meta"]["strategy"]
    assert data["meta"]["attempts"] >= 1


def test_generate_prompt_changes_between_seeds():
    """同一个知识点、不同 seed：随机编号 / 蓝图 / 采样参数都不一样。"""
    body = json.dumps([{
        "stem": "在光电效应实验中，若入射光频率低于截止频率，请判断下列推断哪个成立",
        "options": ["无论光强多大都无光电子", "增大光强即可产生", "延长照射时间即可产生", "换成金属即可产生"],
        "answer": "A", "score": 5,
    }], ensure_ascii=False)
    prompts = []
    samplings = []
    p, instance = llm_patch(body)

    async def record(*args, **kwargs):
        prompts.append(kwargs["messages"][0]["content"])
        samplings.append((kwargs["temperature"], kwargs["top_p"]))
        return llm_response(body)

    with p:
        instance.chat.completions.create = record
        for seed in ("seed-aaa", "seed-bbb"):
            r = client.post("/api/ai/generate",
                            json={"knowledge": "光电效应", "qtype": "single", "count": 1, "seed": seed},
                            headers=TEACHER_HEADERS)
            assert r.status_code == 200
    # 第二次请求会把第一批题干列入禁用清单，模型若又吐同一道题会被判重并
    # 触发换蓝图重试，所以 prompt 可能是 3 条；关键是每条都不同且都带本次 seed。
    assert len(prompts) >= 2
    assert len(set(prompts)) == len(prompts)
    assert "seed-aaa" in prompts[0] and "seed-bbb" in prompts[-1]
    assert len(set(samplings)) >= 2


def test_generate_same_seed_is_reproducible():
    body = json.dumps([{
        "stem": "关于贝叶斯公式的适用条件，下列说法中哪一项是正确的",
        "options": ["需已知先验分布", "只需样本均值", "不需要任何先验", "只适用于正态分布"],
        "answer": "A", "score": 5,
    }], ensure_ascii=False)
    results = []
    p, instance = llm_patch(body)
    with p:
        for _ in range(2):
            # 清掉跨请求历史，单独验证「同 seed -> 同蓝图/同洗牌」的确定性
            generate_mod._recent.clear()
            r = client.post("/api/ai/generate",
                            json={"knowledge": "贝叶斯公式", "qtype": "single", "count": 1, "seed": "fixed"},
                            headers=TEACHER_HEADERS)
            results.append(r.json()["questions"][0])
    assert results[0] == results[1]


def test_generate_exclude_stems_removes_repeats():
    stem = "在光电效应实验中，若入射光频率低于截止频率，请判断下列推断哪个成立"
    body = json.dumps([{
        "stem": stem,
        "options": ["无论光强多大都无光电子", "增大光强即可产生", "延长照射时间即可产生", "换成金属即可产生"],
        "answer": "A", "score": 5,
    }], ensure_ascii=False)
    p, instance = llm_patch(body)
    with p:
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1,
                              "seed": "s1", "exclude_stems": [stem]},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200
    data = r.json()
    assert data["count"] == 0
    assert data["meta"]["deduped"] >= 1
    assert data["meta"]["excluded"] >= 1


def test_generate_retries_when_first_batch_is_broken():
    bad = json.dumps([{"stem": "题干太短", "options": ["A"], "answer": "A", "score": 5}])
    good = json.dumps([{
        "stem": "若把入射光频率提高到原来的两倍，光电子最大初动能如何变化？",
        "options": ["按爱因斯坦方程线性增大但不一定翻倍", "必然翻倍", "保持不变", "必然减半"],
        "answer": "A", "score": 5,
    }], ensure_ascii=False)
    p, instance = llm_patch([bad, good])
    with p:
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200
    data = r.json()
    assert data["count"] == 1 and not data["questions"][0].get("invalid")
    assert data["meta"]["attempts"] == 2
    # 补齐成功后不该再报「有 1 道题不合格」；弃掉的废题单独计数
    assert data["discarded_count"] == 1
    assert data["invalid_count"] == 0


def test_generate_fills_missing_count():
    """严格补全：主两轮凑不齐时继续按缺口补生成，保证「请求 3 题 = 返回 3 题」。

    复现线上问题「出题数量和最后结果不一样」：
    第 1 轮只回 2 道合格题，第 2 轮又回同样 2 道（全被排重），
    没有补全逻辑时最终只返回 2 题；补全后应恰好凑齐 3 题。
    """
    a = json.dumps([
        {"stem": "在光电效应实验中用频率为 1.2e15 Hz 的光照射钾金属，测得遏止电压为 1.0 V，其逸出功约为多少？",
         "options": ["约 2.3 电子伏特", "约 1.0 电子伏特", "约 3.3 电子伏特", "约 4.3 电子伏特"],
         "answer": "A", "score": 5,
         "explanation": "由爱因斯坦光电方程 hν = W + eU，代入数据可算出逸出功约 2.3 eV。"},
        {"stem": "若光强加倍而频率不变，单位时间到达阴极的光子数目如何变化？",
         "options": ["加倍", "减半", "不变", "变为四倍"], "answer": "A", "score": 5,
         "explanation": "光强正比于光子通量，频率不变时光子能量不变，光子数加倍。"},
    ], ensure_ascii=False)
    b = json.dumps([
        {"stem": "用波长 400 纳米的紫光照射某金属发生光电效应，遏止电压 0.85 V，该金属截止频率最接近多少？",
         "options": ["约 1.1×10¹⁵ 赫兹", "约 5.5×10¹⁴ 赫兹", "约 2.2×10¹⁵ 赫兹", "约 3.0×10¹⁴ 赫兹"],
         "answer": "A", "score": 5,
         "explanation": "先由波长算光子能量约 3.1 eV，减去动能 e·U 得逸出功，再由 W = hν0 求截止频率。"},
        {"stem": "光电效应中饱和电流的大小主要取决于下列哪个物理量？",
         "options": ["入射光强", "入射光频率", "金属种类", "外加正向电压"], "answer": "A", "score": 5,
         "explanation": "饱和电流由单位时间逸出的光电子数决定，频率不变时与光强成正比。"},
    ], ensure_ascii=False)
    p, instance = llm_patch([a, a, b])
    with p:
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 3, "seed": "fill-1"},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200
    data = r.json()
    # 请求 3 题 → 必须返回 3 题，且题干两两不重复
    assert data["count"] == 3
    assert len(data["questions"]) == 3
    assert len({q["stem"] for q in data["questions"]}) == 3
    assert data["meta"]["fill_attempts"] >= 1
    assert data["invalid_count"] == 0


def test_generate_llm_returns_raw_text():
    """LLM 一直返回非 JSON 时降级返回原始文本。"""
    p, instance = llm_patch("这不是JSON")
    with p:
        r = client.post("/api/ai/generate",
                        json={"knowledge": "RAG", "qtype": "essay", "count": 1},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200
    assert r.json().get("raw") is True


def test_bank_source_returns_bank_questions():
    bank_questions = [{
        "stem": "Which planet is known as the Red Planet?",
        "options": ["Mars", "Venus", "Jupiter", "Mercury"],
        "answer": "A", "score": 5, "focus": "Astronomy", "source": "opentdb",
    }]
    p, instance = llm_patch("[]")
    with p, patch("app.api.generate.question_bank.fetch_random_questions",
                  new=AsyncMock(return_value=BankResult(questions=bank_questions, provider="opentdb"))), \
            patch("app.api.generate.question_bank.localize_questions", new=AsyncMock(return_value=True)):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "", "qtype": "multiple", "count": 1, "source": "bank"},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200
    data = r.json()
    # 题库没有多选题，自动按单选返回并让前端用服务端 qtype 渲染
    assert data["qtype"] == "single"
    assert data["count"] == 1
    assert data["meta"]["bank_provider"] == "opentdb"
    assert data["meta"]["localized"] is True


def test_bank_unavailable_falls_back_to_ai():
    body = json.dumps([{
        "stem": "请判断下列哪一项属于通识科学常识中的正确表述",
        "options": ["水在标准大气压下约100℃沸腾", "水的沸点是0℃", "沸点与气压无关", "水在真空中沸点最高"],
        "answer": "A", "score": 5,
    }], ensure_ascii=False)
    p, instance = llm_patch(body)
    with p, patch("app.api.generate.question_bank.fetch_random_questions",
                  new=AsyncMock(return_value=BankResult(status="unavailable", detail="timeout"))):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "", "qtype": "single", "count": 1, "source": "bank"},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200
    data = r.json()
    assert data["meta"]["source"] == "ai"
    assert "fallback" in data["meta"]["bank_status"]
    assert data["count"] == 1


def test_recent_cache_isolates_keys():
    cache = RecentStemCache(per_key=2)
    k1 = cache.key_for("光电效应", "single", "中等")
    k2 = cache.key_for("  光电效应  ", "single", "中等")
    k3 = cache.key_for("光电效应", "judge", "中等")
    assert k1 == k2 != k3
    cache.add(k1, ["题干1", "题干2", "题干3"])
    assert len(cache.snapshots(k1)) == 2
    assert cache.snapshots(k3) == []
