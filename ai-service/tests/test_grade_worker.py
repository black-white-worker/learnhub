"""作业批改 worker 测试。

测试点：
1. parse_llm_output 正常 JSON 解析
2. parse_llm_output 带额外文字的 JSON 提取
3. parse_llm_output 非法 JSON 降级
4. score 越界被 clamp 到 0-100
"""
import pytest
from app.tasks.grade_worker import parse_llm_output


def test_parse_normal_json():
    text = '{"score": 80, "comment": "回答不错", "errorPoints": ["概念混淆"], "suggestion": "再复习第三章"}'
    r = parse_llm_output(text)
    assert r["score"] == 80
    assert "不错" in r["comment"]
    assert len(r["errorPoints"]) == 1


def test_parse_json_with_extra_text():
    """LLM 输出带前后多余文字时能正确提取 JSON。"""
    text = '好的，这是批改结果：\n{"score": 75, "comment": "基本正确", "errorPoints": [], "suggestion": "注意细节"}\n以上。'
    r = parse_llm_output(text)
    assert r["score"] == 75
    assert "基本正确" in r["comment"]


def test_parse_invalid_json():
    """非法 JSON 降级为 0 分。"""
    text = "这不是JSON"
    r = parse_llm_output(text)
    assert r["score"] == 0
    assert "解析失败" in r["comment"]


def test_score_clamp_high():
    """score 超过 100 被截断。"""
    text = '{"score": 150, "comment": "x", "errorPoints": [], "suggestion": ""}'
    r = parse_llm_output(text)
    assert r["score"] == 100


def test_score_clamp_low():
    """score 负数被截断为 0。"""
    text = '{"score": -10, "comment": "x", "errorPoints": [], "suggestion": ""}'
    r = parse_llm_output(text)
    assert r["score"] == 0


def test_score_non_int():
    """score 不是数字时不崩溃。"""
    text = '{"score": "abc", "comment": "x", "errorPoints": [], "suggestion": ""}'
    r = parse_llm_output(text)
    assert r["score"] == 0
