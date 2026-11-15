"""批改 worker 的"一条消息 = 一份提交"行为测试。

改造前是一条消息一道题，Go 要收 N 次回调才能拼出总分，
中间任何一次失败成绩册就是半截的。现在 worker 自己串行批完一份再回调一次，
所以这些用例盯的是：
1. 分数汇总口径（只累加批出来的题）
2. 有一题解析不出来 → 整份转人工，一个字节都不写进成绩
3. LLM 抛异常 → 走重试 / 死信，不写结果
4. 回调带 X-Internal-Token，且任务状态同步回写 Redis
5. 旧格式消息（没有 items）不能回调出一个 0 分的"完成"

全部 mock，不连 Redis、不调 LLM。
"""
import asyncio
import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.tasks import grade_worker
from app.tasks.grade_worker import (
    GradeError,
    MAX_RETRY,
    grade_items,
    handle_task,
)


def run(coro):
    return asyncio.run(coro)


def item(qid, max_score=10):
    return {
        "questionId": qid,
        "stem": f"第{qid}题题干",
        "studentAnswer": f"第{qid}题作答",
        "standardAnswer": "参考答案",
        "maxScore": max_score,
    }


def llm_ok(score):
    """伪造一次成功的 LLM 输出。"""
    return {
        "score": score,
        "maxScore": 10,
        "comment": f"给 {score} 分",
        "errorPoints": [],
        "suggestion": "继续保持",
        "parseOk": True,
    }


def llm_broken():
    return {"score": 0, "maxScore": 10, "comment": "AI批改失败，转人工: 解析失败", "errorPoints": [], "suggestion": "", "parseOk": False}


# ---- 汇总 ----

def test_grade_items_sums_all_parsed_questions():
    task = {"items": [item(1, 10), item(2, 20), item(3, 30)]}
    with patch("app.tasks.grade_worker.call_llm_grade", new_callable=AsyncMock, side_effect=[llm_ok(8), llm_ok(15), llm_ok(27)]):
        r = run(grade_items(task))
    assert r["score"] == 50
    assert r["maxScore"] == 60
    assert r["parseOk"] is True
    assert len(r["items"]) == 3
    assert r["unparsedQuestions"] == []
    assert "3 道主观题" in r["comment"]


def test_grade_items_keeps_per_question_detail():
    task = {"items": [item(7, 20)]}
    with patch("app.tasks.grade_worker.call_llm_grade", new_callable=AsyncMock, return_value={**llm_ok(12), "maxScore": 20, "errorPoints": ["漏了重排"]}):
        r = run(grade_items(task))
    one = r["items"][0]
    assert one["questionId"] == 7
    assert one["score"] == 12
    assert one["maxScore"] == 20
    assert one["errorPoints"] == ["漏了重排"]
    assert one["parseOk"] is True


def test_grade_items_unparsed_marks_whole_submission_manual():
    """有一题没解析出来：整份标记 parseOk=False，那题不给分也不计入总分。"""
    task = {"items": [item(1), item(2), item(3)]}
    with patch("app.tasks.grade_worker.call_llm_grade", new_callable=AsyncMock, side_effect=[llm_ok(9), llm_broken(), llm_ok(10)]):
        r = run(grade_items(task))
    assert r["parseOk"] is False
    assert r["unparsedQuestions"] == [2]
    # 只有 1、3 题计入：9 + 10
    assert r["score"] == 19
    assert r["items"][1]["score"] is None
    assert "第 2 题" in r["comment"]


def test_grade_items_llm_exception_raises_grade_error():
    """网络/额度异常不能吞掉：抛 GradeError 让上层重试，绝不返回半成品结果。"""
    task = {"items": [item(1), item(2)]}
    with patch("app.tasks.grade_worker.call_llm_grade", new_callable=AsyncMock, side_effect=[llm_ok(5), RuntimeError("429 rate limited")]):
        with pytest.raises(GradeError) as e:
            run(grade_items(task))
    assert "2" in str(e.value)


def test_grade_items_rejects_legacy_message_without_items():
    """旧格式（单题字段）没有 items：报 GradeError，不能回调一个 0 分的"完成"。"""
    legacy = {"questionId": 1, "studentAnswer": "x", "maxScore": 10}
    with pytest.raises(GradeError):
        run(grade_items(legacy))


def test_grade_items_caps_at_max_items():
    task = {"items": [item(i) for i in range(1, 41)]}
    mock = AsyncMock(side_effect=[llm_ok(5) for _ in range(grade_worker.MAX_ITEMS)])
    with patch("app.tasks.grade_worker.call_llm_grade", new=mock):
        r = run(grade_items(task))
    assert len(r["items"]) == grade_worker.MAX_ITEMS
    assert mock.await_count == grade_worker.MAX_ITEMS


# ---- handle_task 的状态机 ----

def make_task(**over):
    t = {
        "taskId": "t" + "0" * 29,
        "submitId": 99,
        "retry": 0,
        "items": [item(1), item(2)],
        **over,
    }
    return t


def mocks():
    """_r（Redis）、grade_store、callback_go 三件套。"""
    r = MagicMock()
    r.lpush = AsyncMock()
    store = MagicMock()
    store.update_task = AsyncMock()
    store.STATUS_PROCESSING = "processing"
    store.STATUS_DONE = "done"
    store.STATUS_MANUAL = "manual"
    store.STATUS_FAILED = "failed"
    return r, store


def test_handle_task_success_writes_done_and_calls_back_once():
    task = make_task()
    r, store = mocks()
    with patch("app.tasks.grade_worker._r", r), \
         patch("app.tasks.grade_worker.grade_store", store), \
         patch("app.tasks.grade_worker.grade_items", new_callable=AsyncMock, return_value={"score": 18, "maxScore": 20, "comment": "不错", "parseOk": True, "items": [], "unparsedQuestions": []}), \
         patch("app.tasks.grade_worker.callback_go", new_callable=AsyncMock, return_value=True) as cb:
        run(handle_task(task))

    assert cb.await_count == 1
    # 状态流转：processing → done（且 done 带结果）
    statuses = [c.kwargs.get("status") for c in store.update_task.await_args_list]
    assert statuses == ["processing", "done"]
    assert store.update_task.await_args_list[1].kwargs["result"]["score"] == 18
    r.lpush.assert_not_called()


def test_handle_task_unparsed_does_not_call_back():
    """AI 没批出来：一个字节都不回调，进人工队列，状态标 manual。"""
    task = make_task()
    r, store = mocks()
    result = {"score": 0, "maxScore": 20, "comment": "第 2 题请人工批改", "parseOk": False, "items": [], "unparsedQuestions": [2]}
    with patch("app.tasks.grade_worker._r", r), \
         patch("app.tasks.grade_worker.grade_store", store), \
         patch("app.tasks.grade_worker.grade_items", new_callable=AsyncMock, return_value=result), \
         patch("app.tasks.grade_worker.callback_go", new_callable=AsyncMock) as cb:
        run(handle_task(task))

    assert cb.await_count == 0
    assert r.lpush.call_args.args[0] == "ai:grade:manual"
    assert store.update_task.await_args_list[-1].kwargs["status"] == "manual"


def test_handle_task_llm_error_retries_then_dead_letters():
    """回调没成功 → 重试，重试次数用尽进死信，并把状态标 failed。"""
    task = make_task()
    r, store = mocks()
    with patch("app.tasks.grade_worker._r", r), \
         patch("app.tasks.grade_worker.grade_store", store), \
         patch("app.tasks.grade_worker.grade_items", new_callable=AsyncMock, side_effect=GradeError("429 rate limited")), \
         patch("asyncio.sleep", new_callable=AsyncMock):
        # 第 1 次失败：retry 0 → 重新入队，retry=1
        run(handle_task(task))
        assert r.lpush.call_args.args[0] == grade_worker.settings.grade_queue
        assert json.loads(r.lpush.call_args.args[1])["retry"] == 1

        r.lpush.reset_mock()
        # 用尽重试：最后一次进死信 + 状态 failed
        task["retry"] = MAX_RETRY
        run(handle_task(task))
        assert r.lpush.call_args.args[0] == "ai:grade:dead"
        assert store.update_task.await_args_list[-1].kwargs["status"] == "failed"
        assert "429" in store.update_task.await_args_list[-1].kwargs["error"]


def test_handle_task_callback_non_200_is_retried():
    """Go 返回非 200（比如回调鉴权失败）也要重试，否则结果永远躺在 Redis 里。"""
    task = make_task()
    r, store = mocks()
    with patch("app.tasks.grade_worker._r", r), \
         patch("app.tasks.grade_worker.grade_store", store), \
         patch("app.tasks.grade_worker.grade_items", new_callable=AsyncMock, return_value={"score": 10, "maxScore": 10, "parseOk": True, "items": [], "comment": "", "unparsedQuestions": []}), \
         patch("app.tasks.grade_worker.callback_go", new_callable=AsyncMock, return_value=False), \
         patch("asyncio.sleep", new_callable=AsyncMock):
        run(handle_task(task))
    assert json.loads(r.lpush.call_args.args[1])["retry"] == 1


def test_handle_task_without_task_id_still_works():
    """老消息没有 taskId（改造前进队的）：不能因为回写状态而崩。"""
    task = make_task()
    task.pop("taskId")
    r, store = mocks()
    with patch("app.tasks.grade_worker._r", r), \
         patch("app.tasks.grade_worker.grade_store", store), \
         patch("app.tasks.grade_worker.grade_items", new_callable=AsyncMock, return_value={"score": 10, "maxScore": 10, "parseOk": True, "items": [], "comment": "", "unparsedQuestions": []}), \
         patch("app.tasks.grade_worker.callback_go", new_callable=AsyncMock, return_value=True):
        run(handle_task(task))
    store.update_task.assert_not_awaited()


# ---- 回调细节 ----

def test_callback_go_sends_internal_token_and_default_url():
    """回调必须带 X-Internal-Token：Go 那边没配令牌时一律 403，两边值来自同一个环境变量。"""
    task = {"taskId": "t1", "submitId": 99}
    result = {"score": 18, "maxScore": 20, "comment": "不错", "parseOk": True, "items": []}
    captured = {}

    class FakeResp:
        status_code = 200

    class FakeClient:
        def __init__(self, *a, **kw):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *exc):
            return False

        async def post(self, url, json=None, headers=None):
            captured["url"] = url
            captured["json"] = json
            captured["headers"] = headers or {}
            return FakeResp()

    with patch("app.tasks.grade_worker.httpx.AsyncClient", FakeClient), \
         patch("app.tasks.grade_worker.settings") as st:
        st.go_callback_base = "http://backend:8888"
        st.internal_token = "shared-secret"
        ok = run(grade_worker.callback_go(task, result))

    assert ok is True
    assert captured["url"] == "http://backend:8888/api/assignment/submission/99/ai-callback"
    assert captured["headers"]["X-Internal-Token"] == "shared-secret"
    assert captured["json"]["taskId"] == "t1"
    assert captured["json"]["score"] == 18
    assert captured["json"]["parseOk"] is True


def test_callback_go_uses_task_callback_url_when_given():
    task = {"taskId": "t1", "submitId": 99, "callbackUrl": "http://lp-backend:8888/custom/callback"}
    captured = {}

    class FakeResp:
        status_code = 200

    class FakeClient:
        def __init__(self, *a, **kw):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *exc):
            return False

        async def post(self, url, json=None, headers=None):
            captured["url"] = url
            return FakeResp()

    with patch("app.tasks.grade_worker.httpx.AsyncClient", FakeClient):
        run(grade_worker.callback_go(task, {"score": 1, "parseOk": True}))
    assert captured["url"] == "http://lp-backend:8888/custom/callback"


def test_callback_go_non_200_returns_false():
    task = {"taskId": "t1", "submitId": 99}

    class FakeResp:
        status_code = 403

    class FakeClient:
        def __init__(self, *a, **kw):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *exc):
            return False

        async def post(self, url, json=None, headers=None):
            return FakeResp()

    with patch("app.tasks.grade_worker.httpx.AsyncClient", FakeClient):
        assert run(grade_worker.callback_go(task, {"score": 1, "parseOk": True})) is False
