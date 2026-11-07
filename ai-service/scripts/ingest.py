"""一次性数据灌入脚本：从 MySQL 读课程章节文本，切片、向量化、写 Chroma。

用法：
  python -m scripts.ingest --course-id 1
  课程文档更新时再跑一次增量同步。

切片策略：按 500 token 切，重叠 50 token（用 tiktoken 粗算）。
"""
import argparse
import tiktoken
import pymysql
from app.rag.vector_store import get_vector_store
from app.core.config import settings

ENC = tiktoken.get_encoding("cl100k_base")


def split_text(text: str, chunk_size: int = 500, overlap: int = 50):
    tokens = ENC.encode(text)
    chunks = []
    i = 0
    while i < len(tokens):
        piece = tokens[i:i + chunk_size]
        chunks.append(ENC.decode(piece))
        i += chunk_size - overlap
    return chunks


def ingest_course(course_id: int):
    conn = pymysql.connect(host="localhost", user="root", password="root", database="learningplatform", charset="utf8mb4")
    try:
        with conn.cursor(pymysql.cursors.DictCursor) as cur:
            # 读课程资源文本（这里只读 resources 表里的 name/简介；PDF 解析留 TODO）
            cur.execute("SELECT id, name, url FROM course_resources WHERE course_id=%s", (course_id,))
            rows = cur.fetchall()
    finally:
        conn.close()

    store = get_vector_store()
    all_chunks, all_metas = [], []
    for r in rows:
        # TODO: 接 PyPDFLoader / DocxLoader 解析 PDF/Word 正文
        # 现在先用资源名+url 占位，后续补文档解析
        text = f"课程资源：{r['name']}\n链接：{r['url']}"
        for c in split_text(text):
            all_chunks.append(c)
            all_metas.append({"title": r["name"], "source": r["url"], "course_id": course_id})

    if not all_chunks:
        print(f"[ingest] course {course_id} 没有可灌入的资源")
        return

    # ChromaStore.upsert 内部会调 embed_batch，这里只传文本和元数据即可
    store.upsert(course_id, all_chunks, all_metas)
    print(f"[ingest] course {course_id} 灌入 {len(all_chunks)} 个切片")


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--course-id", type=int, required=True)
    args = ap.parse_args()
    ingest_course(args.course_id)
