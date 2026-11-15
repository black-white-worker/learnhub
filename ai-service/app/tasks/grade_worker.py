"""作业 AI 批改：从 Redis 队列消费任务，调 LLM 打分，回调 Go 服务。

任务粒度：**一条队列消息 = 一份作业提交**（items 里是这份提交的全部主观题）。
以前一条消息一道题，Go 要收 N 次回调才能拼出总分，中间任何一次失败成绩册就是半截的。

P0 修复：
- 分数口径：请求带 maxScore，prompt 按 maxScore 打分，clamp 到 [0, maxScore]
- 解析容错：正则提取数字，兼容 "85分"/"85.0" 等格式
- 解析失败不回调，进人工队列（绝不写 0 分进成绩册）
- time.sleep 改 asyncio.sleep，不阻塞事件循环
- Redis 客户端改异步，brpop/lpush 全部 await
"""
import json
import re
import asyncio
import httpx
import redis.asyncio as aioredis
from openai import AsyncOpenAI
from app.core.config import settings
from app.tasks import grade_store

_r = aioredis.from_url(settings.redis_url, decode_responses=True)
_llm = AsyncOpenAI(
    base_url=settings.llm_base_url,
    api_key=settings.llm_api_key,
    timeout=settings.llm_timeout,
)

MAX_RETRY = 2
# 单份提交的题目上限，和 api/grade.py 保持一致
MAX_ITEMS = 30


GRADE_PROMPT = """你是一名教学助教，批改学生简答题。
参考答案：{standard_answer}
学生答案：{student_answer}
评分规则：{score_rule}
本题满分：{max_score} 分

请严格按以下 JSON 格式输出，不要输出任何其他内容：
{{"score": 数字, "comment": "评语", "errorPoints": ["错误1","错误2"], "suggestion": "改进建议"}}

要求：
- score 是 0 到 {max_score} 之间的整数
- comment 不超过 100 字
- errorPoints 列出学生答案中的错误点（如果有）
- suggestion 给一条具体的改进建议
"""


def extract_score(raw) -> int:
    """从各种格式提取分数：85 / "85分" / "85.0" / 85.5 → int。"""
    if isinstance(raw, int):
        return raw
    if isinstance(raw, float):
        return int(raw)
    # 字符串：正则提取数字
    m = re.search(r"(\d+(?:\.\d+)?)", str(raw))
    if m:
        return int(float(m.group(1)))
    return 0


def parse_llm_output(text: str, max_score: int = 100) -> dict:
    """从 LLM 输出中提取 JSON。
    成功：返回完整批改结果，parseOk=True。
    失败：返回 score=0 + comment 提示，parseOk=False，调用方据此进人工队列。
    """
    try:
        start = text.index("{")
        end = text.rindex("}") + 1
        result = json.loads(text[start:end])
        score = extract_score(result.get("score", 0))
        # clamp 到 [0, max_score]
        score = max(0, min(max_score, score))
        return {
            "score": score,
            "maxScore": max_score,
            "comment": str(result.get("comment", "")),
            "errorPoints": result.get("errorPoints", []),
            "suggestion": str(result.get("suggestion", "")),
            "parseOk": True,
        }
    except Exception as e:
        # 解析失败：返回 score=0 + comment 标记，parseOk=False
        # 调用方检查 parseOk=False 时不回调 Go，进人工队列
        return {
            "score": 0,
            "maxScore": max_score,
            "comment": f"AI批改失败，转人工: 解析失败",
            "errorPoints": [],
            "suggestion": "",
            "parseOk": False,
            "error": f"解析失败: {e}",
            "raw": text[:200],
        }


async def call_llm_grade(task: dict) -> dict:
    """调 LLM 批改一道题。"""
    max_score = task.get("maxScore", 100)
    prompt = GRADE_PROMPT.format(
        standard_answer=task.get("standardAnswer", ""),
        student_answer=task.get("studentAnswer", ""),
        score_rule=task.get("scoreRule", f"满分{max_score}，按要点给分"),
        max_score=max_score,
    )
    resp = await _llm.chat.completions.create(
        model=settings.llm_model,
        messages=[{"role": "user", "content": prompt}],
        temperature=0.2,
    )
    return parse_llm_output(resp.choices[0].message.content or "", max_score)


async def callback_go(task: dict, result: dict) -> bool:
    """回调 Go 服务，成功返回 True。"""
    callback = task.get("callbackUrl") or \
        f"{settings.go_callback_base}/api/assignment/submission/{task['submitId']}/ai-callback"
    payload = {
        "taskId": task.get("taskId"),
        "submitId": task.get("submitId"),
        **result,
    }
    headers = {"X-Internal-Token": settings.internal_token} if settings.internal_token else {}
    async with httpx.AsyncClient(timeout=5) as client:
        resp = await client.post(callback, json=payload, headers=headers)
        return resp.status_code == 200


async def grade_items(task: dict) -> dict:
    """逐题批改，汇总成分数 + 总评。"""
    items = (task.get("items") or [])[:MAX_ITEMS]
    if not items:
        # 旧格式消息（一道题一条）或载荷被截断：宁可乐死在队列里，
        # 也不能回调一个 score=0 的"批改完成"——成绩册会把它当成真批过。
        raise GradeError("任务里没有 items，无法批改（可能是旧版队列消息）")
    results = []
    score_sum = 0
    max_sum = 0
    unparsed = []
    for it in items:
        try:
            one = await call_llm_grade(it)
        except Exception as e:
            # LLM/网络异常：整份退回队列重试，绝不把半截结果写进成绩
            raise GradeError(f"第 {it.get('questionId')} 题批改失败: {e}") from e
        max_sum += int(it.get("maxScore", 100) or 0)
        if not one.get("parseOk"):
            unparsed.append(it.get("questionId"))
            results.append({
                "questionId": it.get("questionId"),
                "score": None,
                "maxScore": it.get("maxScore", 100),
                "comment": one.get("comment", "AI 批改失败，转人工"),
                "errorPoints": [],
                "suggestion": "",
                "parseOk": False,
            })
            continue
        score_sum += int(one.get("score", 0) or 0)
        results.append({
            "questionId": it.get("questionId"),
            "score": one.get("score"),
            "maxScore": one.get("maxScore"),
            "comment": one.get("comment", ""),
            "errorPoints": one.get("errorPoints", []),
            "suggestion": one.get("suggestion", ""),
            "parseOk": True,
        })

    return {
        "score": score_sum,
        "maxScore": max_sum,
        "items": results,
        # 只要有一道题没解析出来，这份就不该自动落成绩册
        "parseOk": not unparsed,
        "unparsedQuestions": unparsed,
        "comment": build_comment(results, unparsed),
    }


def build_comment(results: list, unparsed: list) -> str:
    """把逐题评语拼成一段总评（老师一眼能看完，不用逐题点开）。"""
    parts = [f"AI 初评：{len(results)} 道主观题"]
    if unparsed:
        parts.append(f"其中第 {'、'.join(str(q) for q in unparsed)} 题 AI 未能给出分数，请人工批改")
    ok = [r for r in results if r.get("parseOk")]
    if ok:
        got = sum(1 for r in ok if (r.get("score") or 0) >= (r.get("maxScore") or 0))
        parts.append(f"满分题 {got}/{len(ok)} 道")
    return "；".join(parts)


async def handle_task(task: dict):
    """处理一条批改任务（一份提交），含重试、死信和任务状态回写。"""
    task_id = task.get("taskId")
    retry = task.get("retry", 0)
    if task_id:
        await grade_store.update_task(task_id, status=grade_store.STATUS_PROCESSING, retry=retry)
    try:
        result = await grade_items(task)

        # 有题没解析出来：不回调，进人工队列（成绩册一个字节都不写）
        if not result.get("parseOk"):
            print(f"[grade_worker] 解析失败，进人工队列: {task_id}")
            if task_id:
                await grade_store.update_task(
                    task_id, status=grade_store.STATUS_MANUAL, result=result
                )
            await _r.lpush("ai:grade:manual", json.dumps({
                "task": task, "error": f"未解析题号 {result.get('unparsedQuestions')}"
            }, ensure_ascii=False))
            return

        ok = await callback_go(task, result)
        if not ok:
            raise GradeError("Go 回调返回非 200")
        if task_id:
            await grade_store.update_task(task_id, status=grade_store.STATUS_DONE, result=result)
    except Exception as e:
        print(f"[grade_worker] 任务失败(retry={retry}): {e}")
        if retry < MAX_RETRY:
            task["retry"] = retry + 1
            await asyncio.sleep(2)  # 不阻塞事件循环
            await _r.lpush(settings.grade_queue, json.dumps(task, ensure_ascii=False))
        else:
            loop = asyncio.get_running_loop()
            await _r.lpush("ai:grade:dead", json.dumps({
                "task": task, "error": str(e), "failedAt": loop.time()
            }, ensure_ascii=False))
            if task_id:
                await grade_store.update_task(
                    task_id, status=grade_store.STATUS_FAILED, error=str(e)[:300]
                )
            print(f"[grade_worker] 任务进入死信队列: {task_id}")


class GradeError(Exception):
    """批改过程中的可重试错误。"""


async def consume_loop():
    """后台协程：异步 BRPOP 消费。"""
    print("[grade_worker] 启动 AI 批改消费者...")
    while True:
        item = await _r.brpop(settings.grade_queue, timeout=0)
        if not item:
            continue
        try:
            task = json.loads(item[1])
            await handle_task(task)
        except Exception as e:
            print(f"[grade_worker] 任务处理异常: {e}")
