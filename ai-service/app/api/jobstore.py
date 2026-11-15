"""可恢复的出题作业（单实例内存版）。

为什么需要它：
    浏览器刷新会断开 SSE，旧实现在客户端断开时直接 cancel 后台任务，
    导致进度与结果全部丢失、刷新后只能重头来过。

真实项目里长任务 UI 的通用做法（类比断点续传 / 上传任务）：
    - 每个请求带一个客户端生成的 job_id；
    - 后台任务独立运行，把所有阶段事件推给「当前订阅者」并缓冲成历史；
    - 客户端断开只移除订阅，不 kill 任务；
    - 刷新后前端用同一个 job_id 重新订阅，先回放历史（断点续传的核心），
      再继续收实时事件；作业已经跑完则直接返回缓存的 result。

单实例部署（docker compose 一个 ai-service）用进程内字典即可；
多实例才需要换成 Redis，这里不引入外部依赖，保持本地可跑。
"""
from __future__ import annotations

import asyncio
import time
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable, Optional

# 运行中的作业若超过这么久没有任何客户端连接，则取消（防止关页面后仍在烧额度）
RUNNING_IDLE_TTL = 300
# 已完成作业的结果缓存时长，过期后删除（重新生成即可）
DONE_TTL = 600
# 历史事件缓冲上限，超过丢弃最旧的（进度是低频事件，留足了）
HISTORY_LIMIT = 256


@dataclass
class Job:
    job_id: str
    status: str = "running"          # running | done | error
    history: list = field(default_factory=list)   # 已发出的 progress 事件副本
    result: Optional[dict] = None
    error: Optional[str] = None
    subscribers: set = field(default_factory=set)  # 当前 SSE 连接的队列
    last_client_at: float = field(default_factory=time.monotonic)
    created_at: float = field(default_factory=time.monotonic)
    task: Optional[asyncio.Task] = None

    def push(self, event: str, data: dict):
        """广播一条事件给所有订阅者；progress 事件额外写入历史缓冲。"""
        if event == "progress":
            self.history.append({"event": event, "data": data})
            if len(self.history) > HISTORY_LIMIT:
                self.history = self.history[-HISTORY_LIMIT:]
        for q in list(self.subscribers):
            try:
                q.put_nowait({"event": event, "data": data})
            except Exception:
                self.subscribers.discard(q)

    async def emit_progress(self, payload: dict):
        """_generate 内部 note() 调用的回调（必须是 awaitable）。"""
        payload = dict(payload)
        payload.setdefault("elapsed_ms", int((time.monotonic() - self.created_at) * 1000))
        self.push("progress", payload)

    def subscribe(self) -> "asyncio.Queue":
        q: "asyncio.Queue" = asyncio.Queue()
        self.subscribers.add(q)
        self.last_client_at = time.monotonic()
        return q

    def touch(self):
        self.last_client_at = time.monotonic()

    def detach(self, q):
        self.subscribers.discard(q)
        self.last_client_at = time.monotonic()

    def finish(self, result: dict):
        self.result = result
        self.status = "done"
        self.push("result", result)
        self.subscribers.clear()

    def fail(self, message: str):
        self.error = message[:300]
        self.status = "error"
        self.push("error", {"message": self.error})
        self.subscribers.clear()


class JobStore:
    """进程内作业注册表。get_or_create 负责真正启动后台任务。"""

    def __init__(self):
        self._jobs: dict[str, Job] = {}
        self._lock = asyncio.Lock()

    async def get_or_create(self, job_id: str, run: Callable[["Job"], Awaitable[None]]) -> Job:
        # 懒清理：顺手清掉过期的作业，避免内存堆积
        self._sweep()
        async with self._lock:
            job = self._jobs.get(job_id)
            if job is not None:
                job.touch()
                return job
            job = Job(job_id=job_id)
            self._jobs[job_id] = job
            job.task = asyncio.create_task(self._run(job, run))
            return job

    async def _run(self, job: Job, run: Callable[["Job"], Awaitable[None]]):
        try:
            # run 接收 job，自行调用 job.emit_progress 推事件，并返回最终结果
            result = await run(job)
            job.finish(result)
        except Exception as e:  # noqa: BLE001
            job.fail(f"出题失败（{type(e).__name__}）：{str(e)[:200]}")

    def _sweep(self):
        now = time.monotonic()
        expired = [
            jid for jid, j in self._jobs.items()
            if (j.status != "running" and now - j.created_at > DONE_TTL)
            or (j.status == "running" and now - j.last_client_at > RUNNING_IDLE_TTL)
        ]
        for jid in expired:
            j = self._jobs.pop(jid, None)
            if j and j.task and not j.task.done():
                j.task.cancel()


# 单例：整个进程共用一份作业表
job_store = JobStore()
