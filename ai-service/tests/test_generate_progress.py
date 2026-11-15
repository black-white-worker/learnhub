"""AI 出题「阶段进度流」测试。

浏览器带 Accept: text/event-stream 调 /api/ai/generate 时，服务端逐条推送真实
阶段事件（准备蓝图 / 检索资料 / 模型命制 / 质量校验 / 完成），最后一条 result
携带与普通 JSON 接口完全一致的结果。旧调用不带 Accept，仍是一次性 JSON。

这里全部 mock LLM 与外网，不烧额度、不联网。
"""
import json
from unittest.mock import AsyncMock, patch

import pytest
from fastapi.testclient import TestClient

from app.main import app
import app.api.generate as generate_mod

client = TestClient(app)
TEACHER_HEADERS = {"X-User-Id": "1", "X-Role": "teacher", "Accept": "text/event-stream"}


def llm_response(text):
    resp = AsyncMock()
    resp.choices = [AsyncMock()]
    resp.choices[0].message.content = text
    return resp


def llm_patch(text):
    instance = AsyncMock()
    instance.chat.completions.create = AsyncMock(return_value=llm_response(text))
    return patch("app.api.generate.AsyncOpenAI", return_value=instance)


def good_questions(n=1):
    return json.dumps([{
        "stem": f"在硅的禁带宽度为 1.12 eV 的半导体中，入射光子能量为 {2 + i} eV 时，"
                f"关于内光电效应的载流子产生，下列判断正确的是？",
        "options": ["产生电子空穴对", "只产生空穴", "不产生任何载流子", "发生原子核嬗变"],
        "answer": "A", "score": 5,
        "explanation": "光子能量大于禁带宽度时会激发价带电子跃迁到导带，形成电子空穴对；"
                       "能量不足时不会，选项 C 是常见误解。",
        "focus": "内光电效应条件", "basis": "[1] 光电效应",
    } for i in range(n)], ensure_ascii=False)


def parse_sse(text):
    """把 TestClient 拿到的整段 SSE 文本解析成 [(event, data_dict)]。"""
    events = []
    for block in str(text or "").replace("\r\n", "\n").split("\n\n"):
        if not block.strip():
            continue
        event, data = "message", []
        for line in block.splitlines():
            if line.startswith("event:"):
                event = line[6:].strip()
            elif line.startswith("data:"):
                data.append(line[5:].lstrip())
            elif line.startswith(":"):
                pass  # 心跳 / 注释
        raw = "\n".join(data)
        if raw:
            events.append((event, json.loads(raw)))
    return events


@pytest.fixture(autouse=True)
def clear_recent():
    generate_mod._recent.clear()
    yield
    generate_mod._recent.clear()


def test_stream_returns_real_stages_and_final_result():
    with llm_patch(good_questions()):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "内光电效应", "qtype": "single", "count": 1,
                              "score": 5, "grounding": True, "seed": "stream-1"},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200
    assert "text/event-stream" in r.headers["content-type"]

    evts = parse_sse(r.text)
    progress = [d for e, d in evts if e == "progress"]
    stages = [d["stage"] for d in progress]
    assert stages[0] == "prepare"
    assert "materials" in stages and "compose" in stages and "verify" in stages
    assert stages[-1] == "done"
    # 每条进度都要有人话说明和耗时，前端才有东西可显示
    assert all("detail" in d and "elapsed_ms" in d for d in progress)

    results = [d for e, d in evts if e == "result"]
    assert len(results) == 1
    assert results[0]["count"] == 1 and results[0]["questions"][0]["stem"]
    assert results[0]["qtype"] == "single"
    assert not [d for e, d in evts if e == "error"]


def test_stream_result_matches_json_api_shape():
    """同一个请求走流式与走 JSON，结果结构必须一致（前端可无缝回退）。"""
    payload = {"knowledge": "内光电效应", "qtype": "single", "count": 1,
               "score": 5, "grounding": False, "seed": "shape-1"}
    with llm_patch(good_questions()):
        plain = client.post("/api/ai/generate", json=payload,
                            headers={"X-User-Id": "1", "X-Role": "teacher"})
    with llm_patch(good_questions()):
        streamed = client.post("/api/ai/generate", json=payload, headers=TEACHER_HEADERS)

    assert plain.status_code == 200
    assert "application/json" in plain.headers["content-type"]
    result = next(d for e, d in parse_sse(streamed.text) if e == "result")
    assert set(result) == set(plain.json())
    assert result["meta"]["seed"] == plain.json()["meta"]["seed"]


def test_stream_error_event_on_llm_failure():
    instance = AsyncMock()
    instance.chat.completions.create = AsyncMock(side_effect=RuntimeError("upstream down"))
    with patch("app.api.generate.AsyncOpenAI", return_value=instance):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "内光电效应", "qtype": "single", "count": 1,
                              "grounding": False, "seed": "stream-err"},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200  # 流已经建立，错误信息走事件而不是 HTTP 状态码
    errors = [d for e, d in parse_sse(r.text) if e == "error"]
    assert len(errors) == 1 and errors[0]["message"]
    assert "出题失败" in errors[0]["message"] or "AI 出题服务" in errors[0]["message"]


def test_stream_bad_params_still_http_400():
    """参数校验必须在建流之前完成，否则浏览器只能收到一个 200 空流。"""
    r = client.post("/api/ai/generate", json={"knowledge": "x", "qtype": "nope"},
                    headers=TEACHER_HEADERS)
    assert r.status_code == 400


@pytest.mark.asyncio
async def test_emit_payloads_have_ordered_stages():
    """直接调核心流程注入 emit：阶段顺序与耗时递增，且不依赖 HTTP/SSE。"""
    events = []

    async def emit(payload):
        events.append(payload)

    class R:
        headers = {"X-User-Id": "1", "X-Role": "teacher"}

    body = generate_mod.GenQuestionReq(
        knowledge="内光电效应", qtype="single", count=1, score=5, grounding=False,
    )
    with llm_patch(good_questions()):
        result = await generate_mod._generate(body, R(), emit)

    stages = [e["stage"] for e in events]
    assert stages[0] == "prepare" and stages[-1] == "done"
    assert "compose" in stages and "verify" in stages
    assert result["count"] == 1
    assert events[-1]["count"] == 1 and events[-1]["attempts"] >= 1


def test_sse_event_format():
    line = generate_mod._sse_event("progress", {"stage": "compose", "detail": "中文·测试"})
    assert line.startswith("event: progress\ndata: ")
    assert line.endswith("\n\n")
    assert json.loads(line.split("data: ", 1)[1].strip())["detail"] == "中文·测试"
