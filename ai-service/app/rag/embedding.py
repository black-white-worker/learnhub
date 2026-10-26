"""Embedding：走 OpenAI 兼容接口，本地用 ollama 跑 bge-m3。"""
from openai import OpenAI
from app.core.config import settings

_client = OpenAI(base_url=settings.embedding_base_url, api_key=settings.embedding_model or "not-needed")


def embed(text: str) -> list[float]:
    """单条文本向量化。"""
    resp = _client.embeddings.create(model=settings.embedding_model, input=text)
    return resp.data[0].embedding


def embed_batch(texts: list[str]) -> list[list[float]]:
    resp = _client.embeddings.create(model=settings.embedding_model, input=texts)
    return [d.embedding for d in resp.data]
