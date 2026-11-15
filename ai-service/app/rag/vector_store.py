"""向量库抽象层：当前用 Chroma。

关键修复：写入时显式传 embeddings（用 DashScope text-embedding-v3），
和查询时用同一个模型，避免维度不匹配。
"""
from __future__ import annotations
import os
import hashlib
from typing import List, Dict, Any
from app.core.config import settings
from app.rag.embedding import embed_batch


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
        # collection 名加 _v1 版本号，改 embedding 模型时改版本号即可
        name = f"course_{course_id}_v1"
        return self._client.get_or_create_collection(
            name=name,
            metadata={"hnsw:space": "cosine"}
        )

    def upsert(self, course_id: int, chunks: List[str], metadatas: List[Dict[str, Any]]):
        """写入时先 embed，再显式传 embeddings，和查询走同一模型。"""
        if not chunks:
            return
        coll = self._coll(course_id)
        embeddings = embed_batch(chunks)
        # ID 必须含 resource_id + 切片序号：旧版只用「课程:文本」做哈希，
        # 同一课程里两段相同文字、或不同资源引用同一段文字会互相覆盖，
        # 切片数悄悄变少；也让 delete_by_resource 的「先删后写」有稳定可辨的粒度。
        ids = [
            hashlib.md5(
                (f"{course_id}:"
                 f"{(metadatas[i] if i < len(metadatas) else {}).get('resource_id', '')}"
                 f":{i}:{ch}").encode()
            ).hexdigest()[:24]
            for i, ch in enumerate(chunks)
        ]
        coll.upsert(
            ids=ids,
            documents=chunks,
            metadatas=metadatas,
            embeddings=embeddings,
        )

    def delete_by_resource(self, course_id: int, resource_id: int):
        """按资源 ID 删除所有切片（资源更新时先删后写）。"""
        coll = self._coll(course_id)
        coll.delete(where={"resource_id": resource_id})

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
    return ChromaStore()
