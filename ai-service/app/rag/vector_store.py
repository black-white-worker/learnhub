"""向量库抽象层：当前用 Chroma，将来换 Milvus 只改这一个文件。

接口约定：
- upsert(course_id, chunks: list[str], metadatas: list[dict])
- query(course_id, embedding: list[float], top_k=3) -> list[(text, metadata, score)]
"""
from __future__ import annotations
import os
from typing import List, Dict, Any
from app.core.config import settings


class VectorStore:
    def upsert(self, course_id: int, chunks: List[str], metadatas: List[Dict[str, Any]]):
        raise NotImplementedError

    def query(self, course_id: int, embedding: List[float], top_k: int = 3):
        raise NotImplementedError


class ChromaStore(VectorStore):
    def __init__(self):
        import chromadb
        os.makedirs(settings.vector_db_dir, exist_ok=True)
        self._client = chromadb.PersistentClient(path=settings.vector_db_dir)

    def _coll(self, course_id: int):
        # 每个课程一个 collection，物理隔离 A/B 课程资料
        name = f"course_{course_id}"
        return self._client.get_or_create_collection(name=name, metadata={"hnsw:space": "cosine"})

    def upsert(self, course_id: int, chunks: List[str], metadatas: List[Dict[str, Any]]):
        coll = self._coll(course_id)
        ids = [f"c{abs(hash((course_id, i))) % (10**16)}" for i in range(len(chunks))]
        coll.upsert(ids=ids, documents=chunks, metadatas=metadatas)

    def query(self, course_id: int, embedding: List[float], top_k: int = 3):
        coll = self._coll(course_id)
        res = coll.query(query_embeddings=[embedding], n_results=top_k)
        out = []
        if not res["documents"]:
            return out
        for doc, meta, dist in zip(res["documents"][0], res["metadatas"][0], res["distances"][0]):
            out.append((doc, meta, dist))
        return out


def get_vector_store() -> VectorStore:
    # 工厂：将来 settings.vector_db_type == "milvus" 时返回 MilvusStore
    return ChromaStore()
