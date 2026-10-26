"""RAG 答疑图：严格 4 个节点。
1. rewrite  - 查询改写（结合历史把指代词补全）
2. retrieve - 向量检索 Top3 相关课件片段
3. prompt   - 拼装系统提示 + 上下文 + 历史
4. llm      - 流式调用 LLM，产出带引用的答案

不做多 Agent、不做工具循环、不做长期记忆，2 个月内做浅即可。
"""
from __future__ import annotations
from typing import TypedDict, List, Dict, Any, AsyncGenerator
from langgraph.graph import StateGraph, END
from openai import AsyncOpenAI

from app.core.config import settings
from app.rag.embedding import embed
from app.rag.vector_store import get_vector_store
from app.rag.session import get_history

_llm = AsyncOpenAI(
    base_url=settings.llm_base_url,
    api_key=settings.llm_api_key,
    timeout=settings.llm_timeout,
)
_store = get_vector_store()


class RAGState(TypedDict):
    user_id: int
    course_id: int
    query: str
    # 节点产物
    rewritten: str
    contexts: List[Dict[str, Any]]   # [{text, metadata, score}]
    messages: List[Dict[str, str]]
    answer: str


async def node_rewrite(state: RAGState) -> dict:
    """结合会话历史，把 '它是什么' 这种指代补全成完整问题。"""
    hist = get_history(state["user_id"], state["course_id"])
    if not hist:
        return {"rewritten": state["query"]}
    # 简单改写：把最近 2 轮拼进去让 LLM 补全
    recent = "\n".join(f"{m['role']}: {m['content']}" for m in hist[-4:])
    prompt = f"对话历史：\n{recent}\n\n用户当前问题：{state['query']}\n\n请把当前问题改写为一个独立、完整的问题（保留原意，补全指代），只输出改写后的问题，不要解释。"
    resp = await _llm.chat.completions.create(
        model=settings.llm_model, messages=[{"role": "user", "content": prompt}], temperature=0.2,
    )
    return {"rewritten": resp.choices[0].message.content.strip() or state["query"]}


async def node_retrieve(state: RAGState) -> dict:
    """向量检索 Top3。"""
    emb = embed(state["rewritten"])
    hits = _store.query(state["course_id"], emb, top_k=3)
    contexts = [{"text": t, "metadata": m, "score": s} for t, m, s in hits]
    return {"contexts": contexts}


async def node_prompt(state: RAGState) -> dict:
    """拼装 messages：系统提示 + 检索上下文 + 历史 + 用户问题。"""
    ctx_text = "\n\n".join(
        f"[片段{i+1}] {c['text']}" for i, c in enumerate(state["contexts"])
    ) or "（未检索到相关资料）"
    sys = (
        "你是课程答疑助教。只能基于下面给出的课程资料回答学生问题。"
        "如果资料里没有相关内容，直接说'资料中没有相关内容'，不要编造。"
        "答案末尾用【参考资料】列出引用到的片段标题。\n\n"
        f"课程资料：\n{ctx_text}"
    )
    hist = get_history(state["user_id"], state["course_id"])
    messages = [{"role": "system", "content": sys}]
    messages.extend(hist)
    messages.append({"role": "user", "content": state["query"]})
    return {"messages": messages}


async def node_llm(state: RAGState) -> dict:
    """普通（非流式）调用，用于非流式场景。流式走 stream_answer。"""
    resp = await _llm.chat.completions.create(
        model=settings.llm_model, messages=state["messages"], temperature=0.3,
    )
    answer = resp.choices[0].message.content or ""
    # 追加引用
    refs = [c["metadata"].get("title", "未知章节") for c in state["contexts"] if c["metadata"].get("title")]
    if refs:
        answer += "\n\n【参考资料】" + "、".join(refs)
    return {"answer": answer}


# 构建图
def build_graph():
    g = StateGraph(RAGState)
    g.add_node("rewrite", node_rewrite)
    g.add_node("retrieve", node_retrieve)
    g.add_node("prompt", node_prompt)
    g.add_node("llm", node_llm)
    g.set_entry_point("rewrite")
    g.add_edge("rewrite", "retrieve")
    g.add_edge("retrieve", "prompt")
    g.add_edge("prompt", "llm")
    g.add_edge("llm", END)
    return g.compile()


graph = build_graph()


async def stream_answer(state: RAGState) -> AsyncGenerator[str, None]:
    """SSE 流式：先跑 rewrite/retrieve/prompt，再流式吐 LLM token，最后追引用。"""
    s = dict(state)
    s.update(await node_rewrite(s))
    s.update(await node_retrieve(s))
    s.update(await node_prompt(s))

    # 先吐一段 system 事件：告知引用来源
    refs = [c["metadata"].get("title", "") for c in s["contexts"] if c["metadata"].get("title")]
    if refs:
        yield f"event: refs\ndata: {__import__('json').dumps(refs, ensure_ascii=False)}\n\n"

    # 流式吐答案
    stream = await _llm.chat.completions.create(
        model=settings.llm_model, messages=s["messages"], temperature=0.3, stream=True,
    )
    full = []
    async for chunk in stream:
        delta = chunk.choices[0].delta.content
        if delta:
            full.append(delta)
            yield f"data: {__import__('json').dumps({'delta': delta}, ensure_ascii=False)}\n\n"

    # 结束事件
    yield f"data: {__import__('json').dumps({'done': True, 'answer': ''.join(full)}, ensure_ascii=False)}\n\n"
