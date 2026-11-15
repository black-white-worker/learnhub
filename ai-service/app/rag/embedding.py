"""Embedding：走 DashScope OpenAI 兼容接口（text-embedding-v3）。

- embed()      异步：查询时在 async 路由里调用，不阻塞事件循环
- embed_batch() 同步：数据灌入脚本用，一次性批量向量化

**为什么必须分批**：DashScope 的 embeddings 接口对单次请求的 input 条数有硬上限
（text-embedding-v3 为 10），整篇讲义切片成 87 / 209 条时一把梭会直接
`400 InvalidParameter: batch size is invalid, it should not be larger than 10`，
而 ingest 把这个异常当作「该资源灌入失败」，于是知识库永远是空的 ——
答疑检索不到任何片段，只能回「资料中没有相关内容」。
这里按 EMBEDDING_BATCH_SIZE（默认 10）切开，顺序拼接，保证
`embed_batch(texts)[i]` 恒对应 `texts[i]`。
"""
import logging
import time

from openai import OpenAI, AsyncOpenAI

from app.core.config import settings

log = logging.getLogger(__name__)

# DashScope 的 LLM 和 Embedding 共用同一个 API key
_sync_client = OpenAI(
    base_url=settings.embedding_base_url,
    api_key=settings.llm_api_key,
)
_async_client = AsyncOpenAI(
    base_url=settings.embedding_base_url,
    api_key=settings.llm_api_key,
)

# 单批失败后的重试次数（网络抖动 / 偶发 5xx，重试比整门课重灌便宜得多）
_RETRIES = 2


async def embed(text: str) -> list[float]:
    """单条文本向量化（异步，供 RAG 检索时调用）。"""
    resp = await _async_client.embeddings.create(model=settings.embedding_model, input=text)
    return resp.data[0].embedding


def _batch_size() -> int:
    """单批条数；服务端上限 10，配置写大了也不会越界。"""
    return max(1, min(int(settings.embedding_batch_size or 10), 10))


def embed_batch(texts: list[str]) -> list[list[float]]:
    """批量向量化（同步，供 ingest 脚本调用），内部自动按批切分。

    空输入返回空列表；分批后顺序与入参严格一致（Chroma 的 ids/documents/
    embeddings 三个数组靠下标对齐，错位会写进错误的向量）。
    """
    if not texts:
        return []

    size = _batch_size()
    out: list[list[float]] = []
    for start in range(0, len(texts), size):
        batch = texts[start:start + size]
        out.extend(_embed_one_batch(batch))
    if len(out) != len(texts):
        # 服务端少返向量时宁可失败，也不能悄悄把后面的切片错位写入
        raise RuntimeError(f"embedding 返回数量不匹配：期望 {len(texts)}，实际 {len(out)}")
    return out


def _embed_one_batch(batch: list[str]) -> list[list[float]]:
    """发一批（≤10 条），失败重试；最终失败把异常抛给调用方记 failed 状态。"""
    last: Exception | None = None
    for attempt in range(_RETRIES + 1):
        try:
            resp = _sync_client.embeddings.create(
                model=settings.embedding_model, input=batch)
            vectors = [d.embedding for d in resp.data]
            if len(vectors) != len(batch):
                raise RuntimeError(
                    f"embedding 返回数量不匹配：期望 {len(batch)}，实际 {len(vectors)}")
            return vectors
        except Exception as e:  # noqa: BLE001 —— 重试后仍失败才向上抛
            last = e
            if attempt < _RETRIES:
                log.warning("embedding 第 %d 批失败（%s），重试 %d/%d",
                            len(batch), type(e).__name__, attempt + 1, _RETRIES)
                time.sleep(0.5 * (attempt + 1))
    raise last  # type: ignore[misc]
