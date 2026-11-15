"""主观题批改 HTTP 入口（只给 Go 后端调，不对浏览器开放）。

链路：
    老师点「AI 初评」
      → Go 从库里拼载荷（题干/参考答案/学生作答/分值都是服务端事实，不信前端传的）
      → POST /api/ai/grade（带 X-Internal-Token）
      → 这里写任务状态 + LPUSH 进 ai:grade:queue，立刻返回 taskId
      → ai-worker 消费、调 LLM、回调 Go 落库
      → 前端拿 taskId 轮 GET /api/ai/task/{taskId} 看进度

为什么异步队列而不是同步返回：一份作业几道简答题，LLM 串行要跑十几秒，
同步接口会被 nginx / axios 的超时掐断，而且老师盯着转圈等的体验更差。

为什么不挂进 Go 的 AI 代理路由表：见 backend/internal/handler/ai_proxy.go 的注释
—— 批改烧 LLM 额度、还会往成绩册写"AI 初评"，入口必须只有服务端能碰。
"""
import uuid
import json
from typing import List

from fastapi import APIRouter, HTTPException, Request
from pydantic import BaseModel, Field
import redis.asyncio as aioredis

from app.core.config import settings
from app.tasks import grade_store

router = APIRouter(prefix="/api/ai", tags=["grade"])
_r = aioredis.from_url(settings.redis_url, decode_responses=True)

# 一次批改最多几道题：超过基本是误传（或者有人在拿我们接口批量刷额度）
MAX_ITEMS = 30
# 单题分值上限：LLM 按 maxScore 打分，异常大的分值直接拒
MAX_ITEM_SCORE = 100


def get_user_id(req: Request) -> int:
    uid = req.headers.get("X-User-Id")
    if not uid:
        raise HTTPException(401, "missing X-User-Id")
    try:
        return int(uid)
    except ValueError:
        raise HTTPException(400, "invalid X-User-Id")


def require_teacher(req: Request):
    """批改只能由老师/管理员发起。

    Go 那边已经校验过一次了，这里再校验一次是纵深防御：
    万一内部令牌泄漏，拿着它也不能随便给别人的作业刷分。
    """
    role = req.headers.get("X-Role", "")
    if role not in ("teacher", "admin"):
        raise HTTPException(403, "只有教师或管理员可以发起 AI 批改")


class GradeItem(BaseModel):
    """一道主观题的批改输入。字段名与 Go 侧的 JSON 保持一致（camelCase）。"""

    questionId: int
    stem: str = ""
    studentAnswer: str = ""
    standardAnswer: str = ""
    maxScore: int = Field(default=10, ge=0, le=MAX_ITEM_SCORE)
    scoreRule: str = ""


class GradeReq(BaseModel):
    submitId: int = Field(gt=0)
    assignmentId: int = 0
    courseId: int = 0
    studentId: int = 0
    items: List[GradeItem] = Field(default_factory=list)
    # Go 侧可以先建好任务记录再入队（taskId 由调用方给定），
    # 这样 worker 的回调不可能早于 Go 的落库发生 —— 否则极快的批改会把
    # Go 刚写好的 done 结果覆盖回 processing。留空时这里自己生成。
    taskId: str = ""


def _normalize_task_id(raw: str) -> str:
    """调用方给的 taskId 只接受 hex 风格，别的字符一律当非法输入拒掉。

    这个 id 会拼进 Redis key（ai:grade:task:{id}），留花括号/空格进来就能写坏别人的键。
    """
    tid = (raw or "").strip()
    if not tid:
        return uuid.uuid4().hex
    if len(tid) > 64 or not all(c in "0123456789abcdefABCDEF" for c in tid):
        raise HTTPException(400, "taskId 只允许 64 位以内的十六进制字符串")
    return tid.lower()


@router.post("/grade")
async def enqueue_grade(body: GradeReq, req: Request):
    """入队一次批改，立刻返回 taskId（不等 LLM）。"""
    get_user_id(req)
    require_teacher(req)

    if not body.items:
        raise HTTPException(400, "没有需要批改的主观题")
    if len(body.items) > MAX_ITEMS:
        raise HTTPException(400, f"一次最多批改 {MAX_ITEMS} 道题，当前 {len(body.items)} 道")

    task_id = _normalize_task_id(body.taskId)
    payload = body.model_dump()
    payload["taskId"] = task_id
    await grade_store.create_task(task_id, payload)

    job = {**payload, "taskId": task_id, "retry": 0}
    await _r.lpush(settings.grade_queue, json.dumps(job, ensure_ascii=False))

    return {
        "taskId": task_id,
        "submitId": body.submitId,
        "status": grade_store.STATUS_PENDING,
        "itemCount": len(body.items),
    }


@router.get("/task/{task_id}")
async def task_status(task_id: str, req: Request):
    """查批改任务进度 / 结果。

    结果同时会由 worker 回调写进 Go 的成绩相关表；这里是"进度视图"，
    前端拿它决定要不要继续轮询，Go 拿它兜底修复丢掉的回调。
    """
    get_user_id(req)
    task = await grade_store.get_task(task_id)
    if not task:
        raise HTTPException(404, "批改任务不存在或已过期")
    return task
