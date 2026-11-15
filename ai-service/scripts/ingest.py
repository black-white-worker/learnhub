"""课程知识灌入：课程资料清单 → 拉原文件 → 解析正文 → 切片 → MongoDB + Chroma 双写。

用法：
  python -m scripts.ingest --course-id 1
  python -m scripts.ingest --course-id 1 --dry-run   # 不写库、不调 embedding，只预览

相比旧版修掉的问题：
0. 资料从 Go 的课程资源接口取（`app/kb/manifest.py`），不再自己查一遍 course_resources：
   页面看到的讲义和灌进知识库的必须永远是同一份，MySQL 只做接口不可达时的兜底；
1. MySQL 连接不再硬编码 localhost/root/root，全部来自 settings（本地/上线各一套环境变量）；
2. SELECT 带上 object_name / type，才能按类型真正解析正文（旧版只灌 name+url）；
3. 切片 metadata 补上 resource_id，ChromaStore.delete_by_resource 从此真正可用，
   支持「资源更新 → 先删后写」的增量重灌；
4. 每个资源的解析状态写 kb_documents.status：parsed / fallback / failed 清晰可见，
   「视频转写 TODO」不再是静默失败；
5. 原文件存 GridFS，解析策略升级后可重放，不必回 MinIO 重拉。

双写一致性：Chroma 先按 resource_id 删旧切片再 upsert（内容哈希幂等），
MongoDB 侧 replace_chunks 也是先删后写，两边都不会产生重复切片。
"""
from __future__ import annotations
import argparse
import json
import logging

from app.core.config import settings
from app.kb import manifest as kb_manifest
from app.kb.store import (
    STATUS_FAILED, STATUS_FALLBACK, STATUS_PARSED,
    InMemoryKnowledgeStore, build_chunk_doc, get_knowledge_store,
)
from app.rag.parsers import PARSED, parse_resource
from app.rag.source import fetch_resource_bytes

log = logging.getLogger(__name__)

_ENC = None
_ENCODER_TRIED = False


def _encoder():
    """tiktoken 首次 get_encoding 会联网拉 BPE 文件；离线时降级为按字符切分。

    延迟到真正切片时才加载，保证 --dry-run / 单元测试在纯净环境可跑。
    """
    global _ENC, _ENCODER_TRIED
    if _ENC is not None or _ENCODER_TRIED:
        return _ENC
    _ENCODER_TRIED = True
    try:
        import tiktoken
        _ENC = tiktoken.get_encoding("cl100k_base")
    except Exception as e:
        log.warning("tiktoken 不可用（%s），切片降级为按字符估算", type(e).__name__)
        _ENC = None
    return _ENC


def split_text(text: str, chunk_size: int | None = None,
               overlap: int | None = None) -> list[str]:
    """按 token 切片，重叠 overlap；无 tiktoken 时按字符近似（中文 1 字≈1 token）。"""
    size = chunk_size if chunk_size is not None else settings.ingest_chunk_size
    ov = overlap if overlap is not None else settings.ingest_chunk_overlap
    if not text:
        return []
    if size <= 0 or ov < 0 or ov >= size:
        raise ValueError("要求 chunk_size > 0 且 0 <= overlap < chunk_size")

    enc = _encoder()
    if enc is None:
        step = max(1, size - ov)
        pieces = [text[i:i + size] for i in range(0, len(text), step)]
        # 尾片若短于 overlap，说明它已被前一片的尾部完全覆盖，丢弃以免灌入碎片
        if len(pieces) > 1 and len(pieces[-1]) <= ov:
            pieces.pop()
        return pieces

    tokens = enc.encode(text)
    chunks, i = [], 0
    while i < len(tokens):
        piece = tokens[i:i + size]
        if len(piece) <= ov and chunks:
            break
        chunks.append(enc.decode(piece))
        i += size - ov
    return chunks


def fetch_resources(course_id: int) -> list[dict]:
    """课程讲义元数据：优先问 Go 的课程资料接口，取不到再退回 MySQL。

    优先走接口不是洁癖：接口返回的就是课程详情页看到的那份资源（私有桶下已经换成
    预签名 URL），灌库与页面永远同一口径。MySQL 只是「Go 暂时不可达」的兜底，
    保证老师传完讲义不会因为一次服务重启就永远进不了知识库。

    清单可用但为空（老师把讲义都删了）时直接返回空列表，不回退查库 ——
    否则库里残留的行会被灌回来，出现「删了还能搜到」的幽灵资料。
    """
    rows = kb_manifest.resource_rows(kb_manifest.fetch_course_manifest(course_id), course_id)
    if rows is not None:
        return rows
    return fetch_resources_from_db(course_id)


def fetch_resources_from_db(course_id: int) -> list[dict]:
    """从 MySQL 读课程资源元数据；连接参数全部来自 settings，源码无凭据字面量。"""
    import pymysql  # 延迟导入：dry-run / 单元测试不需要装 pymysql
    conn = pymysql.connect(**settings.mysql_conn_kwargs)
    try:
        with conn.cursor(pymysql.cursors.DictCursor) as cur:
            cur.execute(
                "SELECT id, course_id, name, url, object_name, type, size "
                "FROM course_resources WHERE course_id=%s ORDER BY id ASC",
                (course_id,),
            )
            return list(cur.fetchall())
    finally:
        conn.close()


def build_chunks(course_id: int, resource: dict, sections: list) -> tuple[list[str], list[dict]]:
    """把解析出的段落切成片段，并补齐 resource_id 等检索/溯源元数据。"""
    chunks: list[str] = []
    metas: list[dict] = []
    for sec in sections:
        for piece in split_text(sec.text):
            meta = dict(sec.meta)
            meta.update({
                "course_id": course_id,
                "resource_id": resource["id"],
                "title": resource.get("name") or meta.get("title") or "未命名资源",
                "source": resource.get("url") or "",
            })
            chunks.append(piece)
            metas.append(meta)
    return chunks, metas


def _file_key(course_id: int, resource: dict) -> str:
    part = (resource.get("object_name") or resource.get("name") or "file").strip("/")
    return f"c{course_id}/r{resource['id']}/{part}"


def ingest_course(course_id: int, dry_run: bool | None = None,
                  resources: list[dict] | None = None,
                  store=None, vector=None, fetcher=fetch_resource_bytes) -> dict:
    """灌入一门课，返回统计。测试可注入 resources/store/vector/fetcher。"""
    dry = settings.ingest_dry_run if dry_run is None else dry_run
    # dry-run 必须完全只读：连 Mongo 都不连，纯离线跑（本地无依赖也能预览）
    if store is None:
        store = InMemoryKnowledgeStore() if dry else get_knowledge_store()
    if vector is None and not dry:
        from app.rag.vector_store import get_vector_store  # 延迟：dry-run 不碰 chromadb
        vector = get_vector_store()
    rows = resources if resources is not None else fetch_resources(course_id)

    stats = {"course_id": course_id, "dry_run": dry, "resources": len(rows),
             "parsed": 0, "fallback": 0, "failed": 0, "chunks": 0, "files": 0,
             "backend": store.backend, "degraded": store.degraded}
    run_id = None if dry else store.start_ingestion(course_id, {"dry_run": dry})

    for row in rows:
        rid = row["id"]
        try:
            data, fetch_reason = fetcher(row)
            result = parse_resource(row, data)
            chunks, metas = build_chunks(course_id, row, result.sections)
            status = STATUS_PARSED if result.status == PARSED else STATUS_FALLBACK
            reason = "；".join(x for x in (fetch_reason, result.reason) if x)
            file_key = _file_key(course_id, row)

            if not dry:
                chunk_docs = [build_chunk_doc(course_id, rid, i, ch, metas[i])
                              for i, ch in enumerate(chunks)]
                store.replace_chunks(course_id, rid, chunk_docs)
                # 先删旧向量再写：资源更新不会留下过期切片
                vector.delete_by_resource(course_id, rid)
                vector.upsert(course_id, chunks, metas)
                if data is not None and result.status == PARSED:
                    store.put_file(file_key, data, {
                        "course_id": course_id, "resource_id": rid,
                        "name": row.get("name") or "",
                    })
                    stats["files"] += 1

            if not dry:
                store.upsert_document({
                    "course_id": course_id,
                    "resource_id": rid,
                    "name": row.get("name") or "",
                    "url": row.get("url") or "",
                    "type": (row.get("type") or "").lower(),
                    "status": status,
                    "body_parsed": result.status == PARSED,
                    "chunk_count": len(chunks),
                    "file_key": file_key if (data is not None and result.status == PARSED) else "",
                    "reason": reason,
                })
            stats["parsed" if status == STATUS_PARSED else "fallback"] += 1
            stats["chunks"] += len(chunks)
        except Exception as e:
            # 单个资源失败不拖垮整门课；状态写 failed，覆盖度报表能看见
            log.warning("课程 %s 资源 %s 灌入失败（%s）", course_id, rid, type(e).__name__)
            stats["failed"] += 1
            if not dry:
                store.upsert_document({
                    "course_id": course_id, "resource_id": rid,
                    "name": row.get("name") or "", "status": STATUS_FAILED,
                    "body_parsed": False, "chunk_count": 0,
                    "reason": f"灌入异常（{type(e).__name__}）",
                })

    if not dry:
        store.finish_ingestion(run_id, {"status": "done", **stats})
    return stats


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    ap = argparse.ArgumentParser(description="课程知识灌入")
    ap.add_argument("--course-id", type=int, required=True)
    ap.add_argument("--dry-run", action="store_true", help="只预览，不写库、不调 embedding")
    args = ap.parse_args()
    print(json.dumps(
        ingest_course(args.course_id,
                      dry_run=args.dry_run or settings.ingest_dry_run),
        ensure_ascii=False, indent=2))
