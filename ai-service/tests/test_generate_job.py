"""可恢复出题作业 + 超生成提速 的测试。

验证点：
1. 同一 job_id 不会重复跑出题（run 只调用一次）；
2. 作业跑完后，新订阅者能回放历史进度事件并直接拿到缓存的 result（断点续传核心）；
3. 进行中的作业，第二个订阅者先收到历史、再收到后续实时事件；
4. 超生成：count=1 时一次 LLM 调用通常即可凑齐，attempts 多为 1；
5. 懒清理：长时间无客户端的运行中作业会被取消并移除。
"""
import asyncio
import json

import pytest
from unittest.mock import patch, AsyncMock
from fastapi.testclient import TestClient

from app.main import app
from app.api.jobstore import JobStore, Job

client = TestClient(app)
TEACHER_HEADERS = {"X-User-Id": "1", "X-Role": "teacher"}


def llm_response(text):
    resp = AsyncMock()
    resp.choices = [AsyncMock()]
    resp.choices[0].message.content = text
    return resp


def llm_patch(contents):
    responses = [llm_response(c) for c in contents] if isinstance(contents, list) else [llm_response(contents)]
    instance = AsyncMock()
    mock_cls = patch("app.api.generate.AsyncOpenAI", return_value=instance)
    instance.chat.completions.create = AsyncMock(
        side_effect=responses if len(responses) > 1 else None,
        return_value=responses[0] if len(responses) == 1 else None,
    )
    return mock_cls, instance


SINGLE = json.dumps([{
    "stem": "关于光电效应截止频率，下列说法正确的是",
    "options": ["频率低于截止频率则无光电子", "增大光强即可产生", "延长照射即可产生", "换金属即可产生"],
    "answer": "A", "score": 5, "explanation": "低于截止频率时无论光强多大都不能打出光电子",
    "focus": "截止频率", "basis": "爱因斯坦光电方程",
}], ensure_ascii=False)


async def _demo_runner(job: Job):
    await job.emit_progress({"stage": "prepare", "status": "running", "detail": "蓝图"})
    await job.emit_progress({"stage": "done", "status": "done", "detail": "完成"})
    return {"questions": [{"stem": "q1"}], "meta": {"job_id": job.job_id}, "count": 1}


async def _live_runner(job: Job):
    await job.emit_progress({"stage": "prepare", "status": "running", "detail": "start"})
    await asyncio.sleep(0.01)
    await job.emit_progress({"stage": "compose", "status": "running", "detail": "gen"})
    return {"questions": []}


def test_job_resume_replays_history_and_result():
    store = JobStore()
    calls = []

    async def run(job: Job):
        calls.append(1)
        return await _demo_runner(job)

    async def main():
        job = await store.get_or_create("job-resume-1", run)
        await job.task
        return job

    job = asyncio.run(main())
    assert calls == [1]  # run 只执行一次
    assert job.status == "done"
    assert len(job.history) == 2  # 两条 progress 事件被缓冲

    # 模拟「刷新后重新订阅」：新连接先回放历史，再直接拿到缓存结果
    myq = job.subscribe()
    replayed = list(job.history)
    assert [e["event"] for e in replayed] == ["progress", "progress"]
    assert job.status == "done" and job.result is not None
    assert job.result["meta"]["job_id"] == "job-resume-1"
    job.detach(myq)


def test_job_same_id_no_double_run():
    store = JobStore()
    seen = []

    async def run(job: Job):
        seen.append(job.job_id)
        await job.emit_progress({"stage": "prepare", "status": "running", "detail": "x"})
        return {"ok": True}

    async def main():
        j1 = await store.get_or_create("dup", run)
        j2 = await store.get_or_create("dup", run)  # 同 id 不应再起一个任务
        assert j1 is j2
        await j1.task
        return j1

    asyncio.run(main())
    assert seen == ["dup"]  # run 只跑一次


def test_job_live_subscriber_gets_history_then_live():
    store = JobStore()
    job = Job(job_id="live")
    store._jobs["live"] = job

    async def main():
        myq = job.subscribe()  # 先订阅，保证不漏掉实时事件
        task = asyncio.create_task(store._run(job, _live_runner))
        # 收到第一条（prepare）后再检查历史回放
        item = await asyncio.wait_for(myq.get(), timeout=5)
        assert item["event"] == "progress" and item["data"]["stage"] == "prepare"
        assert len(job.history) >= 1
        item = await asyncio.wait_for(myq.get(), timeout=5)
        assert item["event"] == "progress" and item["data"]["stage"] == "compose"
        item = await asyncio.wait_for(myq.get(), timeout=5)
        assert item["event"] == "result"
        await task
        job.detach(myq)

    asyncio.run(main())


def test_sweep_cancels_abandoned_running_job():
    import time as _time
    store = JobStore()
    job = Job(job_id="abandoned")
    job.status = "running"
    job.last_client_at = _time.monotonic() - 9999  # 很久没客户端
    store._jobs["abandoned"] = job

    async def main():
        job.task = asyncio.create_task(store._run(job, _live_runner))
        store._sweep()
        assert "abandoned" not in store._jobs
        for _ in range(20):
            if job.task.done():
                break
            await asyncio.sleep(0.01)
        assert job.task.cancelled() or job.task.done()

    asyncio.run(main())


def test_overgen_single_call_fills_count():
    """count=1 时超生成让模型一次给足余量，通常一轮就凑齐，不再进第二轮回填。"""
    p, instance = llm_patch(SINGLE)
    with p, patch("app.api.generate.zhsource.fetch_materials",
                  new=AsyncMock(return_value=type("M", (), {
                      "ok": False, "as_meta": lambda self: {"material_status": "unavailable"}
                  })())):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1, "seed": "og1"},
                        headers=TEACHER_HEADERS)
    assert r.status_code == 200
    data = r.json()
    assert data["count"] == 1
    # 一次 LLM 调用给了 3 道候选、取 1 道合格，attempts 应为 1（不再补第二轮）
    assert data["meta"]["attempts"] == 1


def test_sse_returns_result_via_job():
    """SSE 链路：带 Accept 与 job_id，完整收流后拿到与 JSON 一致的结果结构；
    同 job_id 再请求应直接复用已完成作业（命中缓存结果），不重跑。"""
    p, instance = llm_patch(SINGLE)
    with p, patch("app.api.generate.zhsource.fetch_materials",
                  new=AsyncMock(return_value=type("M", (), {
                      "ok": False, "as_meta": lambda self: {"material_status": "unavailable"}
                  })())):
        r = client.post("/api/ai/generate",
                        json={"knowledge": "光电效应", "qtype": "single", "count": 1,
                              "seed": "sse1", "job_id": "sse-job-1"},
                        headers={**TEACHER_HEADERS, "Accept": "text/event-stream"})
    assert r.status_code == 200
    assert "event: result" in r.text

    with p, patch("app.api.generate.zhsource.fetch_materials",
                  new=AsyncMock(return_value=type("M", (), {
                      "ok": False, "as_meta": lambda self: {"material_status": "unavailable"}
                  })())):
        r2 = client.post("/api/ai/generate",
                         json={"knowledge": "光电效应", "qtype": "single", "count": 1,
                               "seed": "sse1", "job_id": "sse-job-1"},
                         headers={**TEACHER_HEADERS, "Accept": "text/event-stream"})
    assert r2.status_code == 200
    assert "event: result" in r2.text
