"""Embedding：走 DashScope OpenAI 兼容接口（text-embedding-v3）。

- embed()      异步：查询时在 async 路由里调用，不阻塞事件循环
- embed_batch() 同步：数据灌入脚本用，一次性批量向量化
"""
from openai import OpenAI, AsyncOpenAI
from app.core.config import settings

# DashScope 的 LLM 和 Embedding 共用同一个 API key
_sync_client = OpenAI(
    base_url=settings.embedding_base_url,
    api_key=settings.llm_api_key,
)
_async_client = AsyncOpenAI(
    base_url=settings.embedding_base_url,
    api_key=settings.llm_api_key,
)


async def embed(text: str) -> list[float]:
    """单条文本向量化（异步，供 RAG 检索时调用）。"""
    resp = await _async_client.embeddings.create(model=settings.embedding_model, input=text)
    return resp.data[0].embedding


def embed_batch(texts: list[str]) -> list[list[float]]:
    """批量向量化（同步，供 ingest 脚本调用）。"""
    resp = _sync_client.embeddings.create(model=settings.embedding_model, input=texts)
    return [d.embedding for d in resp.data]
