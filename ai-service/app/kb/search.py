"""检索结果归一化 + RRF 融合（纯函数，无 IO，便于单测）。

RRF（Reciprocal Rank Fusion）：不依赖两路分数的量纲，只依赖排名，
score = Σ 1/(k + rank)。向量余弦距离和词法权重完全不可比，用 RRF 最稳。
"""
from __future__ import annotations
import hashlib
from typing import Iterable


def chunk_key(text: str) -> str:
    """切片唯一键：用文本内容哈希，让向量结果与 Mongo 切片能对齐去重。"""
    return hashlib.sha1((text or "").encode("utf-8")).hexdigest()[:20]


def cap_query(query: str, max_chars: int) -> str:
    """落库前截断，避免把整段对话/超长输入写进 MongoDB。"""
    q = (query or "").strip()
    return q[:max_chars] if max_chars > 0 else q


def make_hit(text: str, metadata: dict | None = None, score: float = 0.0,
             source: str = "vector") -> dict:
    """统一两路召回的 hit 结构，供 prompt 节点直接消费。"""
    return {
        "key": chunk_key(text),
        "text": text,
        "metadata": dict(metadata or {}),
        "score": float(score),
        "source": source,
        "sources": [source],
    }


def rrf_fuse(*ranked_lists: Iterable[dict], k: int = 60,
             top_k: int | None = None) -> list[dict]:
    """融合多路有序召回结果，按 RRF 分数降序返回。

    同一 key 出现在多路时：合并来源、保留较高原始分、补齐缺失的 metadata 字段。
    """
    if k <= 0:
        raise ValueError("rrf k must be positive")
    scores: dict[str, float] = {}
    merged: dict[str, dict] = {}

    for lst in ranked_lists:
        for rank, hit in enumerate(lst or [], start=1):
            key = hit.get("key") or chunk_key(hit.get("text", ""))
            scores[key] = scores.get(key, 0.0) + 1.0 / (k + rank)
            srcs = list(hit.get("sources") or [hit.get("source", "unknown")])
            if key not in merged:
                merged[key] = {
                    "key": key,
                    "text": hit.get("text", ""),
                    "metadata": dict(hit.get("metadata") or {}),
                    "score": float(hit.get("score", 0.0)),
                    "sources": list(srcs),
                }
            else:
                m = merged[key]
                m["score"] = max(m["score"], float(hit.get("score", 0.0)))
                for src in srcs:
                    if src not in m["sources"]:
                        m["sources"].append(src)
                for field, value in (hit.get("metadata") or {}).items():
                    m["metadata"].setdefault(field, value)

    out = []
    for key, hit in merged.items():
        hit["rrf"] = scores[key]
        hit["source"] = "+".join(hit["sources"])
        out.append(hit)
    out.sort(key=lambda h: (-h["rrf"], -h["score"]))
    return out[:top_k] if top_k else out
