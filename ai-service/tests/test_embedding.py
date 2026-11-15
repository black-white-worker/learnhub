"""Embedding 分批测试（对应 app/rag/embedding.py）。

回归的是这次线上问题：DashScope text-embedding-v3 单批上限 10 条，
旧实现把整份讲义（87 / 209 条切片）一次请求发出去 → 400 →
ingest 判定该资源失败 → 知识库为空 → 答疑只能回「资料中没有相关内容」。
"""
import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.core.config import settings
from app.rag import embedding as emb


def _resp(count: int):
    """构造一个 OpenAI embeddings 响应替身：第 i 条向量是 [i, i]。"""
    r = MagicMock()
    r.data = [MagicMock(embedding=[float(i), float(i)]) for i in range(count)]
    return r


def _client(calls: list, fail_times: int = 0):
    """假客户端：记录每批入参，前 fail_times 次抛错。"""
    c = MagicMock()
    state = {"fail": fail_times}

    def create(model, input):  # noqa: A002 —— 跟 SDK 的签名保持一致
        if state["fail"] > 0:
            state["fail"] -= 1
            raise RuntimeError("temporary 500")
        calls.append(list(input))
        return _resp(len(input))

    c.embeddings.create.side_effect = create
    return c


class TestEmbedBatch:
    def test_splits_into_batches_of_ten(self, monkeypatch):
        calls: list = []
        monkeypatch.setattr(emb, "_sync_client", _client(calls))
        texts = [f"片段{i}" for i in range(25)]

        vectors = emb.embed_batch(texts)

        assert [len(c) for c in calls] == [10, 10, 5]
        assert len(vectors) == 25
        # 每批的向量按批内下标返回（0..n-1），批次顺序拼接后必须与入参一一对应，
        # 否则 Chroma 的 ids / documents / embeddings 三个数组会错位
        assert [v[0] for v in vectors] == [float(i % 10) for i in range(25)]

    def test_batch_size_cannot_exceed_provider_limit(self, monkeypatch):
        """配置写成 50 也不会越界：服务端硬上限是 10。"""
        calls: list = []
        monkeypatch.setattr(emb, "_sync_client", _client(calls))
        monkeypatch.setattr(settings, "embedding_batch_size", 50)

        emb.embed_batch([f"t{i}" for i in range(12)])

        assert [len(c) for c in calls] == [10, 2]

    def test_respects_smaller_configured_batch(self, monkeypatch):
        calls: list = []
        monkeypatch.setattr(emb, "_sync_client", _client(calls))
        monkeypatch.setattr(settings, "embedding_batch_size", 3)

        emb.embed_batch([f"t{i}" for i in range(7)])

        assert [len(c) for c in calls] == [3, 3, 1]

    def test_empty_input_makes_no_request(self, monkeypatch):
        calls: list = []
        monkeypatch.setattr(emb, "_sync_client", _client(calls))

        assert emb.embed_batch([]) == []
        assert calls == []

    def test_retries_transient_failure(self, monkeypatch):
        calls: list = []
        monkeypatch.setattr(emb, "_sync_client", _client(calls, fail_times=1))
        monkeypatch.setattr(emb.time, "sleep", lambda _s: None)

        vectors = emb.embed_batch(["只有一条"])

        assert len(vectors) == 1
        assert len(calls) == 1

    def test_raises_after_retries_exhausted(self, monkeypatch):
        calls: list = []
        monkeypatch.setattr(emb, "_sync_client", _client(calls, fail_times=99))
        monkeypatch.setattr(emb.time, "sleep", lambda _s: None)

        with pytest.raises(RuntimeError):
            emb.embed_batch(["会一直失败"])

    def test_count_mismatch_fails_loudly(self, monkeypatch):
        """服务端少返向量时必须报错，不能把后面的切片写进错的向量位。"""
        c = MagicMock()
        c.embeddings.create.return_value = _resp(1)
        monkeypatch.setattr(emb, "_sync_client", c)

        with pytest.raises(RuntimeError):
            emb.embed_batch(["a", "b"])


class TestSingleEmbed:
    def test_embed_uses_async_client(self, monkeypatch):
        c = MagicMock()
        c.embeddings.create = AsyncMock(return_value=_resp(1))
        monkeypatch.setattr(emb, "_async_client", c)

        assert asyncio.run(emb.embed("问题")) == [0.0, 0.0]
