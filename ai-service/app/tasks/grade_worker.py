"""作业 AI 批改：从 Redis 队列消费任务，调 LLM 打分，回调 Go 服务。

Go 侧 homework-rpc 学生提交后 LPUSH 一条 JSON 到 ai:grade:queue，
格式：{"taskId","submitId","studentAnswer","standardAnswer","scoreRule","callbackUrl"}。
AI 服务 BRPOP 消费，拼 Prompt，LLM 输出结构化 JSON，HTTP POST 回调 callbackUrl。
"""
import json
import httpx
import redis
from openai import AsyncOpenAI
from app.core.config import settings

_r = redis.from_url(settings.redis_url, decode_responses=True)
_llm = AsyncOpenAI(base_url=settings.llm_base_url, api_key=settings.llm_api_key, timeout=settings.llm_timeout)


GRADE_PROMPT = """你是一名教学助教，批改学生简答题。
参考答案：{standard_answer}
学生答案：{student_answer}
评分规则：{score_rule}
输出严格 JSON 格式，不要额外文字：
{{"score": 分数int, "comment": "评语", "errorPoints": ["错误1","错误2"], "suggestion": "改进建议"}}
"""


async def handle_task(task: dict):
    prompt = GRADE_PROMPT.format(
        standard_answer=task.get("standardAnswer", ""),
        student_answer=task.get("studentAnswer", ""),
        score_rule=task.get("scoreRule", "满分100，按要点给分"),
    )
    try:
        resp = await _llm.chat.completions.create(
            model=settings.llm_model,
            messages=[{"role": "user", "content": prompt}],
            temperature=0.2,
        )
        text = resp.choices[0].message.content.strip()
        # 提取 JSON
        start = text.index("{")
        end = text.rindex("}") + 1
        result = json.loads(text[start:end])
    except Exception as e:
        result = {"score": 0, "comment": f"AI 批改失败: {e}", "errorPoints": [], "suggestion": "请老师手动批改"}

    # 回调 Go 服务
    callback = task.get("callbackUrl") or f"{settings.go_callback_base}/api/assignment/submission/{task['submitId']}/ai-callback"
    async with httpx.AsyncClient(timeout=5) as client:
        try:
            await client.post(callback, json={
                "taskId": task.get("taskId"),
                "submitId": task.get("submitId"),
                **result,
            })
        except Exception as e:
            # 回调失败记日志，任务入死信队列
            _r.lpush("ai:grade:dead", json.dumps({"task": task, "error": str(e)}, ensure_ascii=False))


async def consume_loop():
    """后台协程：阻塞式 BRPOP 消费。"""
    print("[grade_worker] 启动 AI 批改消费者...")
    while True:
        # BRPOP 阻塞 0 秒（一直等），拿到 (queue, item)
        item = _r.brpop(settings.grade_queue, timeout=0)
        if not item:
            continue
        try:
            task = json.loads(item[1])
            await handle_task(task)
        except Exception as e:
            print(f"[grade_worker] 任务处理失败: {e}")
