"""HTTP API：
- POST /api/ai/chat          SSE 流式答疑
- POST /api/ai/generate      AI 出题（老师用，返回题目给老师审核）
学习总结在 summary.py，会话管理在 session.py。
"""
from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel
from typing import Optional
import json
import redis

from app.core.config import settings
from app.graph.rag_graph import graph, stream_answer
from app.rag.session import append_turn, get_history

router = APIRouter(prefix="/api/ai", tags=["ai"])
_r = redis.from_url(settings.redis_url, decode_responses=True)


# 简单鉴权：从 Authorization 头解析 user_id（Go 网关已经验过 JWT，这里信任 header）
def get_user_id(req: Request) -> int:
    uid = req.headers.get("X-User-Id")
    if not uid:
        raise HTTPException(401, "missing X-User-Id")
    return int(uid)


class ChatReq(BaseModel):
    query: str
    course_id: int
    session_id: Optional[str] = None  # 预留，Demo 用 user_id+course_id 做 key


@router.post("/chat")
async def chat(body: ChatReq, req: Request):
    user_id = get_user_id(req)
    # 限流：每天每用户 N 次
    key = f"ai:limit:u{user_id}:{__import__('datetime').date.today()}"
    cnt = _r.incr(key)
    if cnt == 1:
        _r.expire(key, 86400)
    if cnt > settings.daily_token_limit:
        raise HTTPException(429, "今日 AI 调用已达上限")

    state = {"user_id": user_id, "course_id": body.course_id, "query": body.query}

    async def event_stream():
        answer_parts = []
        try:
            gen = stream_answer(state)
            async for chunk in gen:
                # 从 chunk 里提取 delta 拼接完整答案
                if chunk.startswith("data: ") and '"delta"' in chunk:
                    try:
                        d = json.loads(chunk[6:])
                        if "delta" in d:
                            answer_parts.append(d["delta"])
                    except Exception:
                        pass
                yield chunk
        except Exception as e:
            yield f"data: {json.dumps({'error': str(e)}, ensure_ascii=False)}\n\n"
        finally:
            # 写完历史
            if answer_parts:
                append_turn(user_id, body.course_id, body.query, "".join(answer_parts))

    return StreamingResponse(event_stream(), media_type="text/event-stream", headers={
        "Cache-Control": "no-cache",
        "X-Accel-Buffering": "no",
    })


class GenQuestionReq(BaseModel):
    knowledge: str
    difficulty: str = "中等"
    qtype: str = "essay"  # essay / fill
    count: int = 2


@router.post("/generate")
async def gen_question(body: GenQuestionReq, req: Request):
    """AI 出题：返回题目+参考答案，老师审核后再入库。"""
    from openai import AsyncOpenAI
    llm = AsyncOpenAI(base_url=settings.llm_base_url, api_key=settings.llm_api_key, timeout=settings.llm_timeout)
    prompt = (
        f"根据知识点【{body.knowledge}】，难度：{body.difficulty}，"
        f"生成{body.count}道{body.qtype=='essay'?'简答':'填空'}题，输出严格JSON数组："
        '[{"question":"题目","answer":"参考答案"}]，不要额外文字。'
    )
    resp = await llm.chat.completions.create(
        model=settings.llm_model, messages=[{"role": "user", "content": prompt}], temperature=0.7,
    )
    text = resp.choices[0].message.content.strip()
    # 简单提取 JSON
    try:
        start = text.index("[")
        end = text.rindex("]") + 1
        questions = json.loads(text[start:end])
    except Exception:
        questions = [{"question": text, "answer": ""}]
    return {"questions": questions}
