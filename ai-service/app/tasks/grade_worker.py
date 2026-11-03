"""作业 AI 批改：从 Redis 队列消费任务，调 LLM 打分，回调 Go 服务。

流程：
1. Go 侧学生提交作业后 LPUSH 一条 JSON 到 ai:grade:queue
2. AI 服务 BRPOP 消费，拼 Prompt 调 LLM
3. 解析 LLM 输出为结构化 JSON（score/comment/errorPoints/suggestion）
4. HTTP POST 回调 Go 服务更新成绩
5. 回调失败入死信队列 ai:grade:dead，人工排查

任务格式：
{
  "taskId": "uuid",
  "submitId": 123,
  "studentAnswer": "学生答案",
  "standardAnswer": "参考答案",
  "scoreRule": "满分20，按要点给分",
  "callbackUrl": "http://learningplatform:8888/api/assignment/submission/123/ai-callback",
  "retry": 0  // 内部用，最多重试 2 次
}
"""
import json
import time
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

请严格按以下 JSON 格式输出，不要输出任何其他内容：
{{"score": 数字, "comment": "评语", "errorPoints": ["错误1","错误2"], "suggestion": "改进建议"}}

要求：
- score 是 0-100 的整数
- comment 不超过 100 字
- errorPoints 列出学生答案中的错误点（如果有）
- suggestion 给一条具体的改进建议
"""


def parse_llm_output(text: str) -> dict:
    """从 LLM 输出中提取 JSON，失败时返回降级结果。"""
    try:
        start = text.index("{")
        end = text.rindex("}") + 1
        result = json.loads(text[start:end])
        # 校验字段
        score = int(result.get("score", 0))
        score = max(0, min(100, score))
        return {
            "score": score,
            "comment": str(result.get("comment", "")),
            "errorPoints": result.get("errorPoints", []),
            "suggestion": str(result.get("suggestion", "")),
        }
    except Exception as e:
        return {
            "score": 0,
            "comment": f"AI 批改解析失败: {e}",
            "errorPoints": [],
            "suggestion": "请老师手动批改",
        }


async def call_llm_grade(task: dict) -> dict:
    """调 LLM 批改一道题。"""
    prompt = GRADE_PROMPT.format(
        standard_answer=task.get("standardAnswer", ""),
        student_answer=task.get("studentAnswer", ""),
        score_rule=task.get("scoreRule", "满分100，按要点给分"),
    )
    resp = await _llm.chat.completions.create(
        model=settings.llm_model,
        messages=[{"role": "user", "content": prompt}],
        temperature=0.2,
    )
    return parse_llm_output(resp.choices[0].message.content or "")


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
        ok = await callback_go(task, result)
        if not ok:
            raise Exception("Go 回调返回非 200")
    except Exception as e:
        print(f"[grade_worker] 任务失败(retry={retry}): {e}")
        if retry < MAX_RETRY:
            # 重试：延迟 2 秒后重新入队
            task["retry"] = retry + 1
            time.sleep(2)
            _r.lpush(settings.grade_queue, json.dumps(task, ensure_ascii=False))
        else:
            # 死信队列
            _r.lpush("ai:grade:dead", json.dumps({
                "task": task, "error": str(e), "failedAt": time.time()
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
