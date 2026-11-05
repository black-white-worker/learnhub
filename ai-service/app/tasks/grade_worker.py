"""作业 AI 批改：从 Redis 队列消费任务，调 LLM 打分，回调 Go 服务。

P0 修复：
- 分数口径：请求带 maxScore，prompt 按 maxScore 打分，clamp 到 [0, maxScore]
- 解析容错：正则提取数字，兼容 "85分"/"85.0" 等格式
- 解析失败不回调，进人工队列（绝不写 0 分进成绩册）
- time.sleep 改 asyncio.sleep，不阻塞事件循环
"""
import json
import re
import asyncio
import httpx
import redis
from openai import AsyncOpenAI
from app.core.config import settings

_r = redis.from_url(settings.redis_url, decode_responses=True)
_llm = AsyncOpenAI(
    base_url=settings.llm_base_url,
    api_key=settings.llm_api_key,
    timeout=settings.llm_timeout,
)

MAX_RETRY = 2


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
    """从 LLM 输出中提取 JSON，失败时返回 None（不回调，进人工队列）。"""
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
        # 解析失败：返回标记，不写 0 分，不回调
        return {
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
    async with httpx.AsyncClient(timeout=5) as client:
        resp = await client.post(callback, json=payload)
        return resp.status_code == 200


async def handle_task(task: dict):
    """处理一条批改任务，含重试和死信。"""
    retry = task.get("retry", 0)
    try:
        result = await call_llm_grade(task)

        # 解析失败：不回调，进人工队列
        if not result.get("parseOk"):
            print(f"[grade_worker] 解析失败，进人工队列: {task.get('taskId')}")
            _r.lpush("ai:grade:manual", json.dumps({
                "task": task, "error": result.get("error"), "raw": result.get("raw")
            }, ensure_ascii=False))
            return

        ok = await callback_go(task, result)
        if not ok:
            raise Exception("Go 回调返回非 200")
    except Exception as e:
        print(f"[grade_worker] 任务失败(retry={retry}): {e}")
        if retry < MAX_RETRY:
            task["retry"] = retry + 1
            await asyncio.sleep(2)  # 不阻塞事件循环
            _r.lpush(settings.grade_queue, json.dumps(task, ensure_ascii=False))
        else:
            _r.lpush("ai:grade:dead", json.dumps({
                "task": task, "error": str(e), "failedAt": asyncio.get_event_loop().time()
            }, ensure_ascii=False))
            print(f"[grade_worker] 任务进入死信队列: {task.get('taskId')}")


async def consume_loop():
    """后台协程：阻塞式 BRPOP 消费。"""
    print("[grade_worker] 启动 AI 批改消费者...")
    while True:
        item = _r.brpop(settings.grade_queue, timeout=0)
        if not item:
            continue
        try:
            task = json.loads(item[1])
            await handle_task(task)
        except Exception as e:
            print(f"[grade_worker] 任务处理异常: {e}")
