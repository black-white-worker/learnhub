"""RAG 答疑图：严格 4 个节点。
1. rewrite  - 查询改写（结合历史把指代词补全）
2. retrieve - 向量检索 Top3 相关课件片段
3. prompt   - 拼装系统提示 + 上下文 + 历史
4. llm      - 流式调用 LLM，产出带引用的答案

不做多 Agent、不做工具循环、不做长期记忆，2 个月内做浅即可。
"""
from __future__ import annotations
import asyncio
import logging
from typing import TypedDict, List, Dict, Any, AsyncGenerator
from langgraph.graph import StateGraph, END
from openai import AsyncOpenAI

from app.core.config import settings
from app.rag.embedding import embed
from app.rag.vector_store import get_vector_store
from app.rag.session import get_history
from app.kb.search import cap_query, make_hit, rrf_fuse
from app.kb.manifest import fetch_course_manifest, video_titles
from app.kb.store import get_knowledge_store
from app.kb.tokens import lexical_score, tokenize

log = logging.getLogger(__name__)

_llm = AsyncOpenAI(
    base_url=settings.llm_base_url,
    api_key=settings.llm_api_key,
    timeout=settings.llm_timeout,
)

# 向量库延迟到第一次检索再建：ChromaStore() 会创建 PersistentClient
# （建目录、加载 sqlite），放在模块顶层等于"import 即 IO" ——
# 服务启动、单元测试、以及任何没装 chromadb 的环境都会在这里直接崩。
_store = None


def vector_store():
    global _store
    if _store is None:
        _store = get_vector_store()
    return _store


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
    # 历史先滤掉拒答轮次：改写任务本身不需要「上次没答上来」这种上下文，
    # 反而会把拒答的语气带进改写结果。
    hist = _clean_history(await get_history(state["user_id"], state["course_id"]))
    if not hist:
        return {"rewritten": state["query"]}
    # 简单改写：把最近 2 轮拼进去让 LLM 补全
    recent = "\n".join(f"{m['role']}: {m['content']}" for m in hist[-4:])
    prompt = f"对话历史：\n{recent}\n\n用户当前问题：{state['query']}\n\n请把当前问题改写为一个独立、完整的问题（保留原意，补全指代），只输出改写后的问题，不要解释。"
    resp = await _llm.chat.completions.create(
        model=settings.llm_model, messages=[{"role": "user", "content": prompt}], temperature=0.2,
    )
    return {"rewritten": resp.choices[0].message.content.strip() or state["query"]}


async def _lexical_contexts(course_id: int, query: str) -> List[Dict[str, Any]]:
    """MongoDB 中文词法兜底召回。

    pymongo 是同步客户端：用 to_thread 挪出事件循环，避免阻塞 FastAPI。
    这里任何异常都只记日志并返回空列表 —— 知识库挂了不能让答疑接口 500，
    退化成纯向量检索是可接受的降级。
    """
    tokens = tokenize(query)
    if not tokens:
        return []
    try:
        store = await asyncio.to_thread(get_knowledge_store)
        docs = await asyncio.to_thread(
            store.search_chunks, course_id, tokens, settings.retrieval_lexical_top_k)
        return [
            make_hit(d.get("text", ""), d.get("metadata") or {},
                     lexical_score(tokens, d.get("keywords") or []), "lexical")
            for d in docs
        ]
    except Exception as e:
        log.warning("知识库词法召回失败（%s），退化为纯向量检索", type(e).__name__)
        return []


NO_ANSWER_PHRASE = "资料中没有相关内容"

# 检索为空时的固定答复：不调 LLM。
# 旧实现在没有上下文时仍把「（未检索到相关资料）」塞进 system prompt 让模型自己说
# 「资料中没有相关内容」，模型还会照着指令补一个空的「【参考资料】」——
# 学生看到的就是一句光秃秃的"资料中没有相关内容【参考资料】"，既没信息量也不知道下一步做什么。
# 这里直接给可读的说明，并明确提示「可能还在入库」，把排查方向交给用户。
NO_CONTEXT_HINT = (
    "资料中没有相关内容：这门课暂时没有可检索的讲义正文。"
    "如果讲义是刚上传的，解析入库需要一点时间，可以稍后再问一次；"
    "若一直这样，请联系任课教师确认讲义已上传且格式受支持（PDF / Word / PPT）。"
    "课程视频目前只索引标题，视频里的讲解内容还没有转写，问「视频讲了什么」暂时答不了。"
)

# 拒答哨兵：提示词里**不能**出现「资料中没有相关内容」这句中文原文。
# 实测 qwen-plus 会把提示词里的这句照抄回来 —— 哪怕上下文里明明有讲义内容，
# 只要看到这句模板，它就以拒答收尾（线上现象：库里 296 个切片，学生仍然只看到
# 「资料中没有相关内容【参考资料】」）。改成英文哨兵，服务端再翻译成中文说明。
REFUSAL_SENTINEL = "NO_RELEVANT_MATERIAL"


class RefusalFilter:
    """流式过滤拒答哨兵：学生永远不该看到 NO_RELEVANT_MATERIAL 这种内部标记。

    流式输出的难点是「已经吐出去的字符收不回来」，所以哨兵前缀阶段先憋住不吐；
    一旦确定不是哨兵（出现别的字符），立刻把缓冲一次性放出去，正常答案零延迟。
    """

    def __init__(self) -> None:
        self._buf = ""
        self._decided = False

    def feed(self, delta: str) -> str:
        """返回本次应当吐给前端的文本（可能为空）。"""
        if not delta:
            return ""
        if self._decided:
            return delta
        self._buf += delta
        stripped = self._buf.strip()
        if stripped and REFUSAL_SENTINEL.startswith(stripped):
            return ""          # 还是哨兵前缀，先憋住
        self._decided = True
        out, self._buf = self._buf, ""
        return out

    def flush(self) -> str:
        """流结束时把没吐出的缓冲处理掉：整段就是哨兵（或被截断的哨兵前缀）时换成中文说明。"""
        if self._decided:
            return ""
        self._decided = True
        out, self._buf = self._buf, ""
        stripped = out.strip()
        if not stripped or REFUSAL_SENTINEL in stripped or REFUSAL_SENTINEL.startswith(stripped):
            return NO_ANSWER_PHRASE
        return out


def is_refusal(answer: str) -> bool:
    """答案是否等于拒答（哨兵或中文说明都算）。"""
    text = (answer or "").strip()
    return REFUSAL_SENTINEL in text or text.startswith(NO_ANSWER_PHRASE)


class SentinelScrubber:
    """兜底过滤：模型「先说一句，再把哨兵补在结尾」时，别把标记漏给学生。

    RefusalFilter 只处理「整段就是哨兵」的情况；实测 qwen-plus 还会输出
    「无法回答……NO_RELEVANT_MATERIAL」这种先答后拒的混合体，此时哨兵出现在
    正文中间/末尾，已经被吐出去的字符收不回来。这里按「最长后缀且是哨兵前缀」
    的方式留一点尾巴，确认不是哨兵再放行，正常文本只多几毫秒延迟。
    """

    def __init__(self) -> None:
        self._buf = ""

    def _hold(self) -> int:
        """需要留在缓冲里的尾巴长度（保证不会把哨兵切成两半吐出去）。"""
        for k in range(min(len(REFUSAL_SENTINEL), len(self._buf)), 0, -1):
            if REFUSAL_SENTINEL.startswith(self._buf[-k:]):
                return k
        return 0

    def feed(self, delta: str) -> str:
        if not delta:
            return ""
        self._buf += delta
        cut = len(self._buf) - self._hold()
        if cut <= 0:
            return ""
        out, self._buf = self._buf[:cut], self._buf[cut:]
        return out.replace(REFUSAL_SENTINEL, "")

    def flush(self) -> str:
        out, self._buf = self._buf, ""
        return out.replace(REFUSAL_SENTINEL, "")


def normalize_answer(answer: str) -> str:
    """把哨兵换成给用户看的中文说明；纯拒答时只留说明，不留空引用。"""
    text = (answer or "").strip()
    if REFUSAL_SENTINEL in text:
        return NO_ANSWER_PHRASE
    return answer


def _clean_history(hist: List[Dict[str, str]]) -> List[Dict[str, str]]:
    """把历史里的「拒答」轮次整对丢掉。

    为什么必须丢：拒答会自我强化。第一次因为知识库空而回「资料中没有相关内容」，
    这条回答进了会话历史，下一轮模型看到自己上次都拒答了，就继续拒答 ——
    实测同一个问题，带这段历史时 qwen-plus 一律拒答，不带历史时能正常归纳。
    所以喂 prompt 的历史里不出现拒答，用户问过的问题本身也一起丢，避免出现
    「有提问没回答」的残缺对话。
    """
    out: List[Dict[str, str]] = []
    for m in hist or []:
        if m.get("role") == "assistant" and is_refusal(m.get("content", "")):
            if out and out[-1].get("role") == "user":
                out.pop()
            continue
        out.append(m)
    return out


async def _log_unanswered(state: RAGState, reason: str) -> None:
    """把「没答上来」的问题记到 kb_unanswered，供覆盖度报表定位缺哪份资料。"""
    if not settings.retrieval_log_unanswered:
        return
    try:
        store = await asyncio.to_thread(get_knowledge_store)
        q = cap_query(state.get("query", ""), settings.retrieval_max_query_chars)
        await asyncio.to_thread(
            store.log_unanswered, state["user_id"], state["course_id"], q, reason)
    except Exception as e:
        log.warning("未命中日志写入失败（%s），忽略", type(e).__name__)


async def _log_if_unanswered(state: RAGState, answer: str = "") -> None:
    """覆盖度反馈闭环：真正「没答上来」时才落 kb_unanswered。

    只看「检索为空」会漏掉最常见的情况：库里只有名称+链接的 fallback 占位切片，
    向量照样能命中，但 LLM 仍回「资料中没有相关内容」。所以三种情况都要记账：
      1. no_hit        两路检索都没结果
      2. fallback_only 只命中占位切片（metadata.body=false，说明正文没解析）
      3. llm_no_answer LLM 明确表示资料里没有相关内容
    判断放在答案产出后统一做，天然不会和检索阶段重复记账。
    """
    if not settings.retrieval_log_unanswered:
        return
    contexts = state.get("contexts") or []
    no_answer = NO_ANSWER_PHRASE in (answer or "")
    only_placeholder = bool(contexts) and all(
        (c.get("metadata") or {}).get("body") is False for c in contexts
    )
    if contexts and not no_answer and not only_placeholder:
        return
    if not contexts:
        reason = "no_hit"
    elif no_answer:
        reason = "llm_no_answer"
    else:
        reason = "fallback_only"
    await _log_unanswered(state, reason)


async def _outline_contexts(course_id: int) -> List[Dict[str, Any]]:
    """检索全空时的最后一档：把课程视频清单当上下文。

    为什么要有这一档：只有视频没有讲义的课，"视频讲了什么" 拿不到任何切片，
    旧实现直接回「资料中没有相关内容」，学生会以为课程没上传东西。视频正文要
    ASR 才能切（未实现），但标题清单至少能说清这门课有哪些内容 —— 而且提示里
    明确写了正文未转写，不让模型据此编讲解细节。

    数据来自 Go 的课程资料清单（和课程详情页同源），取不到就返回空，不影响主链路。
    """
    try:
        manifest = await asyncio.to_thread(fetch_course_manifest, course_id)
    except Exception as e:  # noqa: BLE001 —— 兜底路径绝不能把答疑打挂
        log.warning("课程 %s 视频清单获取失败（%s）", course_id, type(e).__name__)
        return []
    titles = video_titles(manifest)
    if not titles:
        return []
    listing = "；".join(titles[:20])
    more = "（以下仅列前 20 个）" if len(titles) > 20 else ""
    text = (f"这门课共有 {len(titles)} 个课程视频：{listing}。{more}"
            "视频正文尚未转写，只能提供标题清单，不能据此推断具体讲解内容。")
    return [make_hit(text, {"title": "课程视频清单", "body": False}, 1.0, "outline")]


def _outline_only(contexts: List[Dict[str, Any]] | None) -> bool:
    """上下文是否只有「课程视频清单」这一条兜底占位（没有任何真实讲义片段）。"""
    if not contexts:
        return False
    for c in contexts:
        meta = c.get("metadata") or {}
        if meta.get("body") is not False or "outline" not in (c.get("sources") or []):
            return False
    return True


async def node_retrieve(state: RAGState) -> dict:
    """混合检索：Chroma 语义 Top-K + MongoDB 中文词法兜底，RRF 融合。

    单独一路向量召回在知识稀疏时容易漏，词法召回补的是「讲义里明明有这个词」的漏；
    embedding 服务抖动时也能靠词法兜底给出上下文。
    """
    vector_hits: List[Dict[str, Any]] = []
    try:
        emb = await embed(state["rewritten"])
        raw = vector_store().query(
            state["course_id"], emb, top_k=settings.retrieval_vector_top_k)
        vector_hits = [make_hit(t, m, s, "vector") for t, m, s in raw]
    except Exception as e:
        # embedding / Chroma 故障时不清空主链路，交给词法兜底
        log.warning("向量召回失败（%s），改用知识库词法兜底", type(e).__name__)

    if settings.retrieval_hybrid_enabled:
        lexical_hits = await _lexical_contexts(state["course_id"], state["rewritten"])
        contexts = rrf_fuse(vector_hits, lexical_hits,
                            k=settings.retrieval_rrf_k,
                            top_k=settings.retrieval_vector_top_k)
    else:
        contexts = rrf_fuse(vector_hits, k=settings.retrieval_rrf_k,
                            top_k=settings.retrieval_vector_top_k)

    if not contexts:
        # 两路都空：退一档用课程视频清单兜底（见 _outline_contexts）
        contexts = await _outline_contexts(state["course_id"])

    # 未命中日志在答案产出后统一判断（见 _log_if_unanswered），这里不重复记账
    return {"contexts": contexts}


async def node_prompt(state: RAGState) -> dict:
    """拼装 messages：系统提示 + 检索上下文 + 历史 + 用户问题。

    提示词的每一条都是踩坑换来的，改动前务必跑 test_rag_retrieve 的
    TestRefusalSentinel（提示词里出现某些字面量会让 qwen-plus 直接拒答）：
      1. 片段带资料名（`[片段1 · 第一章讲义.docx]`）—— 模型能说清依据，前端也能对上引用；
      2. 概括性问题（「这门课讲了什么」）要求按片段归纳，不能因为片段不完整就拒答。
         旧提示只有一句「没有相关内容就说没有」，模型遇到泛指问题一律回
         「资料中没有相关内容【参考资料】」—— 库里明明有资料，学生却什么都看不到；
      3. **提示词里不能出现「资料中没有相关内容」这类中文拒答原文，也不能提「【参考资料】」**。
         实测这两串字面量都会被 qwen-plus 照抄回答案里，即使上下文完全相关；
         拒答改用英文哨兵 REFUSAL_SENTINEL，引用由服务端展示，不写进提示词。
    """
    ctx_text = "\n\n".join(
        f"[片段{i+1} · {(c['metadata'] or {}).get('title') or '课程资料'}] {c['text']}"
        for i, c in enumerate(state["contexts"])
    ) or "（未检索到相关资料）"

    # 只有视频清单这一档上下文时，走另一套提示词：
    # 通用提示词里「片段与问题无关就输出哨兵」会让模型对着清单拒答
    # （实测输出「无法回答视频内容……NO_RELEVANT_MATERIAL」），
    # 而这门课其实有视频，学生需要的是「有哪些视频 + 为什么答不了具体内容」。
    if _outline_only(state.get("contexts")):
        sys = (
            "你是课程答疑助教。这名学生选的课程目前只有视频，视频正文还没有转写成文字，"
            "下面给出的是课程视频清单（只有标题）。请按以下要求回答：\n"
            "1. 先说清这门课有几个视频、标题分别是什么；\n"
            "2. 再说明视频里的讲解内容尚未转写，无法据此回答具体知识点，"
            "建议参看课程讲义，或请任课教师补充讲义 / 字幕；\n"
            "3. 不要编造视频内容，也不要输出任何英文标记。\n\n"
            f"课程视频清单：\n{ctx_text}"
        )
    else:
        sys = (
            "你是课程答疑助教，只依据下面给出的课程资料片段回答学生问题，不编造资料之外的内容。\n"
            "1. 片段能支持回答时就直接作答，结尾用一句话说明依据哪份资料。\n"
            "2. 学生问「这门课/这一章讲了什么」这类概括性问题时，按片段归纳主要知识点；"
            "片段只是节选，不完整不代表没有相关内容，不要因此拒答。\n"
            f"3. 只有片段与问题确实无关时，才只输出 {REFUSAL_SENTINEL} 这一个词，不要输出别的内容。\n\n"
            f"课程资料：\n{ctx_text}"
        )
    hist = _clean_history(await get_history(state["user_id"], state["course_id"]))
    messages = [{"role": "system", "content": sys}]
    messages.extend(hist)
    messages.append({"role": "user", "content": state["query"]})
    return {"messages": messages}


async def node_llm(state: RAGState) -> dict:
    """普通（非流式）调用，用于非流式场景。流式走 stream_answer。"""
    if not state.get("contexts"):
        # 没有上下文就别让模型自由发挥：既省一次调用，也避免它硬编一个空引用
        await _log_unanswered(state, "no_hit")
        return {"answer": NO_CONTEXT_HINT}
    resp = await _llm.chat.completions.create(
        model=settings.llm_model, messages=state["messages"], temperature=0.3,
    )
    answer = normalize_answer(resp.choices[0].message.content or "")
    # 追加引用
    refs = [c["metadata"].get("title", "未知章节") for c in state["contexts"] if c["metadata"].get("title")]
    if refs and not is_refusal(answer):
        answer += "\n\n【参考资料】" + "、".join(refs)
    await _log_if_unanswered(state, answer)
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
    """SSE 流式：先跑 rewrite/retrieve/prompt，再流式吐 LLM token，最后追引用。

    检索为空时直接吐固定说明并结束（不调 LLM）：既避免模型自由发挥，
    也让「资料还没入库」这件事对用户可见，而不是一句空的「【参考资料】」。
    """
    s = dict(state)
    s.update(await node_rewrite(s))
    s.update(await node_retrieve(s))

    if not s.get("contexts"):
        yield f"data: {__import__('json').dumps({'delta': NO_CONTEXT_HINT}, ensure_ascii=False)}\n\n"
        yield f"data: {__import__('json').dumps({'done': True, 'answer': NO_CONTEXT_HINT}, ensure_ascii=False)}\n\n"
        await _log_unanswered(s, "no_hit")
        return

    s.update(await node_prompt(s))

    # 先吐一段 system 事件：告知引用来源
    refs = [c["metadata"].get("title", "") for c in s["contexts"] if c["metadata"].get("title")]
    if refs:
        yield f"event: refs\ndata: {__import__('json').dumps(refs, ensure_ascii=False)}\n\n"

    # 流式吐答案：两道过滤
    #   1. RefusalFilter —— 整段就是哨兵时直接换成中文说明；
    #   2. SentinelScrubber —— 「先说一句、结尾补个哨兵」的混合体，尾巴留几字符再放行。
    stream = await _llm.chat.completions.create(
        model=settings.llm_model, messages=s["messages"], temperature=0.3, stream=True,
    )
    full = []
    guard = RefusalFilter()
    scrub = SentinelScrubber()

    def emit(piece: str) -> str:
        return scrub.feed(piece) if piece else ""

    async for chunk in stream:
        delta = chunk.choices[0].delta.content
        if delta:
            piece = emit(guard.feed(delta))
            if piece:
                full.append(piece)
                yield f"data: {__import__('json').dumps({'delta': piece}, ensure_ascii=False)}\n\n"
    for piece in (emit(guard.flush()), scrub.flush()):
        if piece:
            full.append(piece)
            yield f"data: {__import__('json').dumps({'delta': piece}, ensure_ascii=False)}\n\n"

    # 结束事件
    full_answer = "".join(full)
    yield f"data: {__import__('json').dumps({'done': True, 'answer': full_answer}, ensure_ascii=False)}\n\n"
    # SSE 结束后才判断「是否真没答上来」，覆盖 no_hit / fallback_only / llm_no_answer
    await _log_if_unanswered(s, full_answer)
