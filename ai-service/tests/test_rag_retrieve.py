"""混合检索节点测试（对应 app/graph/rag_graph.py 的 node_retrieve）。

核心验收：MongoDB / embedding 任一路故障都不能让答疑 500；
两路都有结果时 RRF 融合；两路都空时落未命中日志形成覆盖度闭环。
"""
import asyncio
from unittest.mock import AsyncMock, patch

import pytest

from app.core.config import settings
from app.graph import rag_graph
from app.kb.search import make_hit
from app.kb.store import InMemoryKnowledgeStore, build_chunk_doc


class _FakeVectorStore:
    def __init__(self, hits=(), fail=False):
        self.hits = list(hits)
        self.fail = fail
        self.queries = []

    def query(self, course_id, embedding, top_k=3):
        self.queries.append((course_id, embedding, top_k))
        if self.fail:
            raise RuntimeError("chroma down")
        return self.hits


@pytest.fixture
def store():
    s = InMemoryKnowledgeStore()
    s.replace_chunks(1, 7, [
        build_chunk_doc(1, 7, 0, "RAG 检索增强生成的原理", {"title": "第一章讲义"}),
        build_chunk_doc(1, 7, 1, "向量库与混合检索的对比", {"title": "第二章讲义"}),
        build_chunk_doc(1, 7, 2, "课程签到规则说明", {"title": "课程须知"}),
    ])
    return s


def _state(query="RAG 检索是什么", user_id=9, course_id=1):
    return {
        "user_id": user_id, "course_id": course_id, "query": query,
        "rewritten": query, "contexts": [], "messages": [], "answer": "",
    }


def _run(monkeypatch, store, vec, embed_ok=True, hybrid=True, log_unanswered=True,
         max_chars=500):
    monkeypatch.setattr(settings, "retrieval_hybrid_enabled", hybrid)
    monkeypatch.setattr(settings, "retrieval_log_unanswered", log_unanswered)
    monkeypatch.setattr(settings, "retrieval_max_query_chars", max_chars)
    monkeypatch.setattr(settings, "retrieval_vector_top_k", 3)
    monkeypatch.setattr(settings, "retrieval_lexical_top_k", 3)
    embed = AsyncMock(return_value=[0.1] * 8)
    if not embed_ok:
        embed = AsyncMock(side_effect=RuntimeError("embedding down"))
    with patch.object(rag_graph, "embed", embed), \
         patch.object(rag_graph, "vector_store", return_value=vec), \
         patch.object(rag_graph, "get_knowledge_store", return_value=store):
        return asyncio.run(rag_graph.node_retrieve(_state()))


class TestVectorOnly:
    def test_hybrid_off_returns_vector_hits_unchanged(self, monkeypatch, store):
        vec = _FakeVectorStore([("向量片段", {"title": "讲义"}, 0.2)])
        out = _run(monkeypatch, store, vec, hybrid=False)
        ctx = out["contexts"]
        assert len(ctx) == 1 and ctx[0]["text"] == "向量片段"
        assert ctx[0]["source"] == "vector"
        assert vec.queries[0][2] == 3

    def test_hybrid_off_does_not_touch_knowledge_store(self, monkeypatch, store):
        vec = _FakeVectorStore([("向量片段", {"title": "讲义"}, 0.2)])
        with patch.object(rag_graph, "get_knowledge_store",
                          side_effect=AssertionError("hybrid 关闭时不该连 Mongo")):
            monkeypatch.setattr(settings, "retrieval_hybrid_enabled", False)
            monkeypatch.setattr(settings, "retrieval_vector_top_k", 3)
            with patch.object(rag_graph, "embed", AsyncMock(return_value=[0.1])), \
                 patch.object(rag_graph, "vector_store", return_value=vec):
                out = asyncio.run(rag_graph.node_retrieve(_state()))
        assert out["contexts"]


class TestLexicalFallback:
    def test_empty_vector_is_rescued_by_mongo_chunks(self, monkeypatch, store):
        out = _run(monkeypatch, store, _FakeVectorStore([]), embed_ok=True)
        ctx = out["contexts"]
        assert ctx and all(c["source"] == "lexical" for c in ctx)
        assert "RAG" in ctx[0]["text"] or "检索" in ctx[0]["text"]

    def test_embedding_failure_does_not_crash(self, monkeypatch, store):
        out = _run(monkeypatch, store, _FakeVectorStore(fail=True), embed_ok=False)
        assert out["contexts"]     # 词法兜底仍给出上下文

    def test_knowledge_store_failure_degrades_to_pure_vector(self, monkeypatch, store):
        monkeypatch.setattr(settings, "retrieval_hybrid_enabled", True)
        vec = _FakeVectorStore([("向量片段", {"title": "讲义"}, 0.2)])
        with patch.object(rag_graph, "embed", AsyncMock(return_value=[0.1])), \
             patch.object(rag_graph, "vector_store", return_value=vec), \
             patch.object(rag_graph, "get_knowledge_store",
                          side_effect=RuntimeError("mongo down")):
            out = asyncio.run(rag_graph.node_retrieve(_state()))
        assert len(out["contexts"]) == 1 and out["contexts"][0]["text"] == "向量片段"


class TestRRFFusion:
    def test_same_chunk_in_both_lists_is_deduped_and_ranked_first(self, monkeypatch, store):
        shared = "RAG 检索增强生成的原理"
        vec = _FakeVectorStore([(shared, {"title": "第一章讲义"}, 0.1),
                                ("向量库与混合检索的对比", {"page": 2}, 0.3)])
        out = _run(monkeypatch, store, vec)
        ctx = out["contexts"]
        assert ctx[0]["text"] == shared
        assert set(ctx[0]["sources"]) == {"vector", "lexical"}
        assert ctx[0]["metadata"]["title"] == "第一章讲义"
        assert sum(1 for c in ctx if c["text"] == shared) == 1

    def test_top_k_respected(self, monkeypatch, store):
        out = _run(monkeypatch, store, _FakeVectorStore([
            (f"v{i}", {}, 0.1 * i) for i in range(3)]))
        assert len(out["contexts"]) <= 3


def _log(monkeypatch, store, contexts=(), answer="", enabled=True, max_chars=500):
    """直接驱动答案产出后的闭环判断。"""
    monkeypatch.setattr(settings, "retrieval_log_unanswered", enabled)
    monkeypatch.setattr(settings, "retrieval_max_query_chars", max_chars)
    state = _state()
    state["contexts"] = list(contexts)
    with patch.object(rag_graph, "get_knowledge_store", return_value=store):
        asyncio.run(rag_graph._log_if_unanswered(state, answer))
    return state


class TestUnansweredLoop:
    """覆盖三种「真没答上来」：检索空 / 只命中占位切片 / LLM 明确说没有。"""

    def test_no_hit_is_logged(self, monkeypatch):
        s = InMemoryKnowledgeStore()
        _log(monkeypatch, s)
        assert s.coverage(1)["unanswered_queries"] == 1
        assert s._unanswered[0]["reason"] == "no_hit"

    def test_fallback_only_contexts_are_logged(self, monkeypatch):
        """向量命中了名称+链接占位切片，正文其实没解析 —— 这是最常见的情况。"""
        s = InMemoryKnowledgeStore()
        ctx = [make_hit("课程资源：讲义", {"title": "讲义", "body": False}, 0.5)]
        _log(monkeypatch, s, contexts=ctx, answer="这是讲义链接")
        assert s.coverage(1)["unanswered_queries"] == 1
        assert s._unanswered[0]["reason"] == "fallback_only"

    def test_llm_no_answer_phrase_is_logged_even_with_real_contexts(self, monkeypatch):
        s = InMemoryKnowledgeStore()
        ctx = [make_hit("RAG 检索增强生成", {"title": "讲义", "body": True}, 0.9)]
        _log(monkeypatch, s, contexts=ctx, answer="资料中没有相关内容")
        assert s.coverage(1)["unanswered_queries"] == 1
        assert s._unanswered[0]["reason"] == "llm_no_answer"

    def test_real_answer_with_real_contexts_does_not_log(self, monkeypatch):
        s = InMemoryKnowledgeStore()
        ctx = [make_hit("RAG 检索增强生成", {"title": "讲义", "body": True}, 0.9)]
        _log(monkeypatch, s, contexts=ctx, answer="RAG 是检索增强生成")
        assert s.coverage(1)["unanswered_queries"] == 0

    def test_query_is_capped_before_logging(self, monkeypatch):
        s = InMemoryKnowledgeStore()
        _log(monkeypatch, s, max_chars=5)
        assert len(s._unanswered[0]["query"]) == 5

    def test_logging_can_be_disabled(self, monkeypatch):
        s = InMemoryKnowledgeStore()
        _log(monkeypatch, s, enabled=False)
        assert s.coverage(1)["unanswered_queries"] == 0

    def test_store_failure_does_not_raise(self, monkeypatch):
        monkeypatch.setattr(settings, "retrieval_log_unanswered", True)
        with patch.object(rag_graph, "get_knowledge_store",
                          side_effect=RuntimeError("mongo down")):
            asyncio.run(rag_graph._log_if_unanswered(_state(), ""))


class TestStreamAnswerLoop:
    def test_empty_retrieval_answers_without_calling_llm(self, monkeypatch):
        """检索为空时不该调 LLM：旧实现会得到一句空的「资料中没有相关内容【参考资料】」。"""
        monkeypatch.setattr(settings, "retrieval_log_unanswered", True)
        monkeypatch.setattr(settings, "retrieval_max_query_chars", 500)
        s = InMemoryKnowledgeStore()
        llm_create = AsyncMock(side_effect=AssertionError("没有上下文时不该调 LLM"))

        async def consume():
            return [c async for c in rag_graph.stream_answer(_state())]

        with patch.object(rag_graph, "node_rewrite",
                          AsyncMock(return_value={"rewritten": "这一章讲了什么"})), \
             patch.object(rag_graph, "node_retrieve",
                          AsyncMock(return_value={"contexts": []})), \
             patch.object(rag_graph._llm.chat.completions, "create", llm_create), \
             patch.object(rag_graph, "get_knowledge_store", return_value=s):
            chunks = asyncio.run(consume())

        joined = "".join(chunks)
        # 前端靠 delta 拼正文：必须给出可读说明，而不是空转
        assert rag_graph.NO_ANSWER_PHRASE in joined
        assert "解析入库" in joined          # 告诉用户「可能还在入库」，而不是只说不知道
        assert '"done": true' in joined
        assert s.coverage(1)["unanswered_queries"] == 1
        assert s._unanswered[0]["reason"] == "no_hit"

    def test_node_llm_without_contexts_skips_llm(self, monkeypatch):
        monkeypatch.setattr(settings, "retrieval_log_unanswered", True)
        s = InMemoryKnowledgeStore()
        state = _state()
        state["messages"] = [{"role": "user", "content": "问"}]
        with patch.object(rag_graph._llm.chat.completions, "create",
                          AsyncMock(side_effect=AssertionError("不该调 LLM"))), \
             patch.object(rag_graph, "get_knowledge_store", return_value=s):
            result = asyncio.run(rag_graph.node_llm(state))
        assert result["answer"] == rag_graph.NO_CONTEXT_HINT
        assert s.coverage(1)["unanswered_queries"] == 1

    def test_sse_end_logs_no_answer(self, monkeypatch):
        """真实答疑路径：LLM 回「资料中没有相关内容」后必须写覆盖度日志。"""
        monkeypatch.setattr(settings, "retrieval_log_unanswered", True)
        monkeypatch.setattr(settings, "retrieval_max_query_chars", 500)
        s = InMemoryKnowledgeStore()
        ctx = [make_hit("课程资源：讲义", {"title": "讲义", "body": False}, 0.5)]

        class _Stream:
            def __aiter__(self):
                async def gen():
                    for text in ("资料", "中没有相关内容"):
                        ch = AsyncMock()
                        ch.choices = [AsyncMock()]
                        ch.choices[0].delta.content = text
                        yield ch
                return gen()

        async def consume():
            return [c async for c in rag_graph.stream_answer(_state())]

        with patch.object(rag_graph, "node_rewrite",
                          AsyncMock(return_value={"rewritten": "讲义讲了什么"})), \
             patch.object(rag_graph, "node_retrieve",
                          AsyncMock(return_value={"contexts": ctx})), \
             patch.object(rag_graph, "node_prompt",
                          AsyncMock(return_value={"messages": []})), \
             patch.object(rag_graph._llm.chat.completions, "create",
                          AsyncMock(return_value=_Stream())), \
             patch.object(rag_graph, "get_knowledge_store", return_value=s):
            chunks = asyncio.run(consume())
        assert any('"done"' in c for c in chunks)
        assert s.coverage(1)["unanswered_queries"] == 1


class TestRefusalSentinel:
    """拒答哨兵：提示词里不能出现中文拒答原文（qwen-plus 会照抄），流里也不能漏出来。"""

    def test_prompt_does_not_contain_chinese_refusal_phrase(self):
        """回归线上问题：提示词里写了「资料中没有相关内容」，模型就照抄着拒答。"""
        state = _state()
        state["contexts"] = [make_hit("软间隔SVM 的铰链损失", {"title": "实验八.docx"}, 0.9)]
        with patch.object(rag_graph, "get_history", AsyncMock(return_value=[])):
            result = asyncio.run(rag_graph.node_prompt(state))
        sys_msg = result["messages"][0]["content"]
        assert rag_graph.NO_ANSWER_PHRASE not in sys_msg
        # 「【参考资料】」同样会把模型带偏：写了它，模型就连拒答一起照抄这个标题
        assert "【参考资料】" not in sys_msg
        assert rag_graph.REFUSAL_SENTINEL in sys_msg

    def test_filter_holds_sentinel_prefix_until_decided(self):
        f = rag_graph.RefusalFilter()
        # 前缀阶段一个字符都不许漏给学生
        assert f.feed("NO_") == ""
        assert f.feed("RELEVANT") == ""
        assert f.flush() == rag_graph.NO_ANSWER_PHRASE

    def test_filter_releases_normal_answer_immediately(self):
        f = rag_graph.RefusalFilter()
        assert f.feed("这门课讲了") == "这门课讲了"
        assert f.feed("软间隔 SVM。") == "软间隔 SVM。"
        assert f.flush() == ""

    def test_filter_releases_buffer_when_not_sentinel(self):
        f = rag_graph.RefusalFilter()
        assert f.feed("N") == ""          # 疑似哨兵前缀，先憋住
        assert f.feed("o，这门课") == "No，这门课"   # 确认不是哨兵，缓冲一次吐出
        assert f.flush() == ""

    def test_normalize_answer_replaces_sentinel(self):
        assert rag_graph.normalize_answer(" NO_RELEVANT_MATERIAL ") == rag_graph.NO_ANSWER_PHRASE
        assert rag_graph.normalize_answer("正常答案") == "正常答案"

    def test_non_stream_refusal_gets_no_dangling_reference_heading(self):
        state = _state()
        state["contexts"] = [make_hit("软间隔SVM", {"title": "实验八.docx"}, 0.9)]
        state["messages"] = [{"role": "user", "content": "问"}]
        fake_resp = AsyncMock()
        fake_resp.choices = [AsyncMock()]
        fake_resp.choices[0].message.content = rag_graph.REFUSAL_SENTINEL
        with patch.object(rag_graph._llm.chat.completions, "create",
                          AsyncMock(return_value=fake_resp)), \
             patch.object(rag_graph, "get_knowledge_store", return_value=InMemoryKnowledgeStore()):
            result = asyncio.run(rag_graph.node_llm(state))
        assert result["answer"] == rag_graph.NO_ANSWER_PHRASE


    def test_history_with_refusal_is_dropped_from_prompt(self):
        """拒答自我强化：历史里还留着上次的「没答上来」，模型下一轮会继续拒答。"""
        hist = [
            {"role": "user", "content": "这门课讲了什么"},
            {"role": "assistant", "content": "资料中没有相关内容【参考资料】"},
            {"role": "user", "content": "软间隔是什么"},
            {"role": "assistant", "content": "软间隔允许部分样本落在间隔内。"},
        ]
        cleaned = rag_graph._clean_history(hist)
        assert [m["role"] for m in cleaned] == ["user", "assistant"]
        assert cleaned[0]["content"] == "软间隔是什么"

    def test_history_with_sentinel_is_dropped_too(self):
        hist = [{"role": "user", "content": "问"},
                {"role": "assistant", "content": rag_graph.REFUSAL_SENTINEL}]
        assert rag_graph._clean_history(hist) == []

    def test_prompt_uses_cleaned_history(self):
        hist = [{"role": "user", "content": "这门课讲了什么"},
                {"role": "assistant", "content": rag_graph.NO_ANSWER_PHRASE}]
        state = _state()
        state["contexts"] = [make_hit("软间隔SVM", {"title": "实验八.docx"}, 0.9)]
        with patch.object(rag_graph, "get_history", AsyncMock(return_value=hist)):
            result = asyncio.run(rag_graph.node_prompt(state))
        roles = [m["role"] for m in result["messages"]]
        assert roles == ["system", "user"]      # 拒答那一对整对丢掉，只剩本轮提问


class TestDownstreamCompatibility:
    def test_prompt_node_still_consumes_mixed_contexts(self, monkeypatch, store):
        out = _run(monkeypatch, store, _FakeVectorStore([]))
        state = _state()
        state["contexts"] = out["contexts"]
        with patch.object(rag_graph, "get_history", AsyncMock(return_value=[])):
            result = asyncio.run(rag_graph.node_prompt(state))
        sys_msg = result["messages"][0]["content"]
        assert "课程资料：" in sys_msg
        assert any(c["text"] in sys_msg for c in out["contexts"])

    def test_llm_node_builds_references_from_mixed_hits(self, monkeypatch, store):
        out = _run(monkeypatch, store, _FakeVectorStore([]))
        state = _state()
        state["contexts"] = out["contexts"]
        state["messages"] = [{"role": "user", "content": "问"}]
        fake_resp = AsyncMock()
        fake_resp.choices = [AsyncMock()]
        fake_resp.choices[0].message.content = "答案"
        with patch.object(rag_graph._llm.chat.completions, "create",
                          AsyncMock(return_value=fake_resp)):
            result = asyncio.run(rag_graph.node_llm(state))
        assert "【参考资料】" in result["answer"]


class TestVideoOutlineFallback:
    """只有视频没有讲义的课：两路检索都空时用课程视频清单兜底。

    线上现象：学生问「视频讲了什么」，课里明明有视频，却只得到
    「资料中没有相关内容」。视频正文要 ASR 才能切（未实现），但标题清单
    可以说清这门课有哪些内容，比直接判「没有资料」有用。
    """

    def _run_outline(self, monkeypatch, store, manifest):
        monkeypatch.setattr(settings, "retrieval_hybrid_enabled", True)
        monkeypatch.setattr(settings, "retrieval_vector_top_k", 3)
        with patch.object(rag_graph, "fetch_course_manifest", return_value=manifest), \
             patch.object(rag_graph, "embed", AsyncMock(return_value=[0.1] * 8)), \
             patch.object(rag_graph, "vector_store", return_value=_FakeVectorStore([])), \
             patch.object(rag_graph, "get_knowledge_store", return_value=store):
            return asyncio.run(rag_graph.node_retrieve(_state(query="视频讲了什么")))

    def test_video_titles_become_context(self, monkeypatch, store):
        manifest = {"course_id": 1, "resources": [],
                    "videos": [{"name": "第一讲 Vue3 起步"}, {"name": "第二讲 组件通信"}]}
        out = self._run_outline(monkeypatch, store, manifest)
        ctx = out["contexts"]
        assert len(ctx) == 1
        assert "第一讲 Vue3 起步" in ctx[0]["text"]
        assert "第二讲 组件通信" in ctx[0]["text"]
        # 明确标注「不是正文」：前端/日志据此区分占位上下文，别让模型据此编讲解细节
        assert ctx[0]["metadata"]["body"] is False
        assert ctx[0]["metadata"]["title"] == "课程视频清单"
        assert ctx[0]["source"] == "outline"

    def test_no_videos_keeps_context_empty(self, monkeypatch, store):
        out = self._run_outline(monkeypatch, store, {"course_id": 1, "videos": []})
        assert out["contexts"] == []

    def test_manifest_unavailable_keeps_context_empty(self, monkeypatch, store):
        out = self._run_outline(monkeypatch, store, None)
        assert out["contexts"] == []

    def test_outline_ignored_when_real_chunks_hit(self, monkeypatch, store):
        """有讲义时不该拿视频清单凑上下文（清单只是最后一档兜底）。"""
        manifest = {"course_id": 1, "videos": [{"name": "第一讲 Vue3 起步"}]}
        monkeypatch.setattr(settings, "retrieval_hybrid_enabled", True)
        monkeypatch.setattr(settings, "retrieval_vector_top_k", 3)
        with patch.object(rag_graph, "fetch_course_manifest", return_value=manifest) as m, \
             patch.object(rag_graph, "embed", AsyncMock(return_value=[0.1] * 8)), \
             patch.object(rag_graph, "vector_store",
                          return_value=_FakeVectorStore([("向量片段", {"title": "讲义"}, 0.2)])), \
             patch.object(rag_graph, "get_knowledge_store", return_value=store):
            out = asyncio.run(rag_graph.node_retrieve(_state()))
        assert all(c["source"] != "outline" for c in out["contexts"])
        m.assert_not_called()


class TestSentinelScrubber:
    """模型「先答一句再补哨兵」时，标记不能漏给学生（实测 qwen-plus 会这样收尾）。"""

    def _drive(self, text, step=3):
        scrub = rag_graph.SentinelScrubber()
        parts = [scrub.feed(text[i:i + step]) for i in range(0, len(text), step)]
        parts.append(scrub.flush())
        return "".join(parts)

    def test_sentinel_at_end_is_removed(self):
        text = "视频正文未转写，答不了具体内容。" + rag_graph.REFUSAL_SENTINEL
        out = self._drive(text)
        assert out == "视频正文未转写，答不了具体内容。"
        assert "NO_RELEVANT" not in out

    def test_sentinel_in_middle_is_removed(self):
        text = "前" + rag_graph.REFUSAL_SENTINEL + "后"
        assert self._drive(text, step=1) == "前后"

    def test_plain_text_is_not_delayed(self):
        """正常中文不做任何扣留（只有哨兵前缀才憋住），答疑不能因此变慢。"""
        scrub = rag_graph.SentinelScrubber()
        assert scrub.feed("你好") == "你好"

    def test_sentinel_prefix_is_held_until_decided(self):
        scrub = rag_graph.SentinelScrubber()
        assert scrub.feed("NO_REL") == ""                       # 可能是哨兵开头，先憋住
        assert scrub.feed("，普通内容") == "NO_REL，普通内容"     # 确定不是哨兵，立刻放行


class TestOutlineOnlyPrompt:
    def test_video_only_context_uses_dedicated_prompt(self):
        state = _state(query="视频讲了什么")
        state["contexts"] = [make_hit(
            "这门课共有 1 个课程视频：第一讲 Vue3 起步。",
            {"title": "课程视频清单", "body": False}, 1.0, "outline")]
        with patch.object(rag_graph, "get_history", AsyncMock(return_value=[])):
            out = asyncio.run(rag_graph.node_prompt(state))
        sys_msg = out["messages"][0]["content"]
        assert "课程视频清单" in sys_msg
        assert "转写" in sys_msg
        # 这条路径不许出现拒答哨兵：模型看到就会照着拒答，把「有哪些视频」也一起答没了
        assert rag_graph.REFUSAL_SENTINEL not in sys_msg

    def test_mixed_context_keeps_general_prompt(self):
        state = _state()
        state["contexts"] = [make_hit("讲义正文", {"title": "第一章讲义"}, 0.1, "vector")]
        with patch.object(rag_graph, "get_history", AsyncMock(return_value=[])):
            out = asyncio.run(rag_graph.node_prompt(state))
        assert "课程资料：" in out["messages"][0]["content"]
