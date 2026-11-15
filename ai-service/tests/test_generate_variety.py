"""出题随机性 / 质量规则的纯函数单测（不碰网络、不碰 LLM）。"""
import random

from app.questions.variety import (
    RecentStemCache,
    blueprint_text,
    build_blueprint,
    dedupe_questions,
    is_similar,
    normalize_question,
    normalize_stem,
    parse_llm_json,
    random_sampling,
    shuffle_options,
    stem_fingerprint,
    validate_question,
)


# ===== 随机蓝图 =====

def test_blueprint_same_seed_reproducible_different_seed_varies():
    a = build_blueprint(5, "seed-1", "single", "中等")
    b = build_blueprint(5, "seed-1", "single", "中等")
    c = build_blueprint(5, "seed-2", "single", "中等")
    assert a == b
    assert [p["focus"] for p in a] != [p["focus"] for p in c]

    combos = {
        tuple((p["focus"], p["scene"], p["cognition"], p["distractor"]) for p in build_blueprint(4, f"s{i}"))
        for i in range(8)
    }
    assert len(combos) >= 7


def test_blueprint_no_repeated_focus_and_spread_answer_slots():
    plan = build_blueprint(8, "spread", "single")
    assert len({p["focus"] for p in plan}) == 8
    slots = [p["answer_slot"] for p in plan]
    assert len(set(slots)) >= 3
    assert all(slots[i] != slots[i + 1] for i in range(len(slots) - 1))
    assert all(p["answer_slot"] is None for p in build_blueprint(3, "x", "judge"))


def test_blueprint_text_contains_each_question_constraint():
    plan = build_blueprint(2, "text", "single")
    text = blueprint_text(plan)
    for p in plan:
        assert p["focus"] in text and p["scene"] in text
        assert p["answer_slot"] in text


def test_random_sampling_stays_in_safe_range_and_varies():
    values = {random_sampling(f"s{i}")["temperature"] for i in range(12)}
    assert len(values) > 1
    for i in range(12):
        s = random_sampling(f"s{i}")
        assert 0.8 <= s["temperature"] <= 1.0
        assert 0.85 <= s["top_p"] <= 0.98


# ===== 去重 =====

def test_normalize_stem_and_fingerprint():
    assert normalize_stem(" 关于 光电效应，下列说法正确的是：") == normalize_stem("关于光电效应下列说法正确的是")
    assert stem_fingerprint("同一道题") == stem_fingerprint("同一道题！")


def test_similarity_catches_rewritten_duplicates_but_not_unrelated():
    a = "在光电效应实验中，当入射光频率低于某金属的截止频率时，下列说法正确的是"
    b = "在光电效应实验中，当入射光频率低于某金属的截止频率时，下列说法中正确的是"
    c = "用频率为2ν的单色光照射金属表面，测得光电子最大初动能的变化"
    assert is_similar(a, a)
    assert is_similar(a, b)
    assert not is_similar(a, c)


def test_dedupe_questions_against_history_and_within_batch():
    qs = [
        {"stem": "在光电效应实验中，当入射光频率低于截止频率时会发生什么？"},
        {"stem": "在光电效应实验中，当入射光频率低于截止频率时会发生什么？"},
        {"stem": "用频率为2ν的单色光照射金属，最大初动能如何变化？"},
    ]
    kept, dropped = dedupe_questions(qs, ["在光电效应实验中，当入射光频率低于截止频率时会发生什么？"])
    assert len(kept) == 1 and dropped == 2
    assert kept[0]["stem"] == qs[2]["stem"]


# ===== 归一化 / 解析 =====

def test_normalize_question_handles_cn_keys_string_options_and_glued_answer():
    q = normalize_question({
        "题干": "CPU 中存放操作数地址的是？",
        "选项": '["A. 程序计数器（PC）","B. 指令寄存器（IR）","C. 地址寄存器（AR）","D. 数据寄存器（DR）"]',
        "答案": "C. 地址寄存器（AR）",
        "解析": "AR 专门存放地址",
        "考点": "CPU 寄存器",
    })
    assert q["stem"].startswith("CPU")
    assert q["options"][2] == "地址寄存器（AR）"
    assert q["answer"] == "C"
    assert q["explanation"] == "AR 专门存放地址"
    assert q["focus"] == "CPU 寄存器"


def test_parse_llm_json_handles_code_fence_prose_and_trailing_comma():
    payload = '[{"stem":"题干","answer":"A",},]'
    assert parse_llm_json(f"```json\n{payload}\n```") == [{"stem": "题干", "answer": "A"}]
    assert parse_llm_json(f"好的，题目如下：\n{payload}\n希望有帮助")[0]["stem"] == "题干"
    assert parse_llm_json("这不是 JSON") is None
    assert parse_llm_json('{"questions":[{"stem":"x"}]}') == [{"stem": "x"}]


# ===== 选项洗牌 =====

def test_shuffle_single_keeps_correct_option_and_honours_target_slot():
    q = {"stem": "1+1=?", "options": ["0", "1", "2", "3"], "answer": "C"}
    out = shuffle_options(q, random.Random(3), "single", "A")
    assert out["answer"] == "A"
    assert out["options"][0] == "2"
    assert sorted(out["options"]) == sorted(q["options"])
    assert q["answer"] == "C"  # 不修改入参


def test_shuffle_multiple_remaps_all_correct_letters():
    q = {"stem": "多选题", "options": ["甲", "乙", "丙", "丁"], "answer": "AC"}
    out = shuffle_options(q, random.Random(7), "multiple")
    assert len(out["answer"]) == 2
    letters = [ord(c) - 65 for c in out["answer"]]
    assert {out["options"][i] for i in letters} == {"甲", "丙"}
    assert sorted(out["options"]) == sorted(q["options"])


def test_shuffle_invalid_answer_is_left_alone():
    q = {"stem": "x", "options": ["甲", "乙"], "answer": "??"}
    assert shuffle_options(q, random.Random(1), "single")["answer"] == "??"


# ===== 质量校验 =====

def test_validate_single_choice_ok_and_broken():
    ok = validate_question(
        {"stem": "若入射光频率低于截止频率，下列推断哪一项成立？",
         "options": ["无光电子产生", "增大光强即可", "延长照射即可", "换金属即可"],
         "answer": "a", "score": 0}, "single", 5)
    assert ok["answer"] == "A" and ok["score"] == 5 and not ok.get("invalid")

    broken = validate_question({"stem": "下列说法正确的是", "options": ["甲", "甲", "丙", "丁"], "answer": "E"}, "single")
    assert broken["invalid"]
    assert "选项内容重复" in broken["invalid_reason"]
    assert validate_question(
        {"stem": "下列说法正确的是", "options": ["甲", "乙", "丙", "丁"], "answer": "E"}, "single"
    )["invalid_reason"].startswith("单选答案")


def test_validate_judge_normalizes_aliases():
    for raw, expected in [("对", True), ("错误", False), (True, True), ("FALSE", False), ("√", True)]:
        q = validate_question({"stem": "光速在真空中恒定不变", "answer": raw}, "judge")
        assert q["answer"] is expected and not q.get("invalid")
    assert validate_question({"stem": "一个判断题题干", "answer": "也许"}, "judge")["invalid"]


def test_validate_flags_generic_stem_as_warning_not_invalid():
    q = validate_question(
        {"stem": "关于光电效应，下列说法正确的是：", "options": ["甲", "乙", "丙", "丁"], "answer": "A"},
        "single")
    assert not q.get("invalid")
    assert "笼统" in q.get("quality_warning", "")


def test_validate_rejects_empty_essay_answer_and_missing_stem():
    assert validate_question({"stem": "简述光电效应方程", "answer": ""}, "essay")["invalid"]
    assert validate_question({"stem": "", "answer": "A", "options": ["甲", "乙", "丙", "丁"]}, "single")["invalid"]


def test_recent_cache_capacity_and_key_isolation():
    cache = RecentStemCache(per_key=3)
    key = cache.key_for("知识点", "single", "中等")
    cache.add(key, [f"题干{i}" for i in range(5)])
    assert len(cache.snapshots(key)) == 3
    assert cache.snapshots(cache.key_for("知识点", "single", "困难")) == []
    cache.clear()
    assert cache.snapshots(key) == []
