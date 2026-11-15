"""课程资源原文件获取（ingest 用）。

三条路径，按优先级：
1. INGEST_LOCAL_FILE_ROOT 本地目录映射（本地调试/离线灌库，带路径穿越防护）
2. MinIO SDK 按 object_name 拉取（上线私有桶正解；minio SDK 为可选依赖，没装不阻断）
3. 资源 URL 的 HTTP GET（公开桶 / 预签名 URL；只放行 http/https）

任何一步失败都不抛异常到主流程，返回 (None, reason) 让 ingest 记 fallback 状态：
知识灌入可以「不完整」，但不能「静默失败」。日志里不写 URL（预签名 URL 带签名）。
"""
from __future__ import annotations
import os
import urllib.parse
import urllib.request

from app.core.config import settings


def _local_path(object_name: str) -> str | None:
    """把 object_name 映射到本地调试目录，并阻止 ../ 路径穿越。"""
    root = settings.ingest_local_file_root
    if not root or not object_name:
        return None
    root_abs = os.path.abspath(root)
    candidate = os.path.abspath(os.path.join(root_abs, object_name))
    try:
        if os.path.commonpath([root_abs, candidate]) != root_abs:
            return None
    except ValueError:  # 不同盘符
        return None
    return candidate if os.path.isfile(candidate) else None


def _via_local(object_name: str) -> bytes | None:
    p = _local_path(object_name)
    if not p:
        return None
    with open(p, "rb") as f:
        data = f.read(settings.ingest_max_file_bytes + 1)
    return None if len(data) > settings.ingest_max_file_bytes else data


def _via_minio(object_name: str) -> tuple[bytes | None, str]:
    """可选依赖：装了 minio SDK 且配了 AK 时优先按对象名取，适配私有桶。"""
    if not (settings.minio_access_key and object_name):
        return None, ""
    try:
        from minio import Minio  # type: ignore
    except ImportError:
        return None, "minio SDK 未安装（私有桶上线建议安装）"
    try:
        client = Minio(
            settings.minio_endpoint,
            access_key=settings.minio_access_key,
            secret_key=settings.minio_secret_key,
            secure=settings.minio_secure,
        )
        resp = client.get_object(settings.minio_bucket, object_name)
        try:
            data = resp.read(settings.ingest_max_file_bytes + 1)
        finally:
            resp.close()
            resp.release_conn()
        if len(data) > settings.ingest_max_file_bytes:
            return None, "文件超过 INGEST_MAX_FILE_BYTES"
        return data, ""
    except Exception as e:
        # 不拼 str(e)：MinIO 异常里可能带 endpoint/凭据
        return None, f"MinIO 拉取失败（{type(e).__name__}）"


def _via_url(url: str) -> tuple[bytes | None, str]:
    if not url:
        return None, "资源没有 URL"
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme not in ("http", "https"):
        return None, "URL 协议不是 http/https"
    try:
        req = urllib.request.Request(
            url, headers={"User-Agent": "learningplatform-ingest/1.0"})
        with urllib.request.urlopen(req, timeout=settings.ingest_fetch_timeout) as resp:
            data = resp.read(settings.ingest_max_file_bytes + 1)
        if len(data) > settings.ingest_max_file_bytes:
            return None, "文件超过 INGEST_MAX_FILE_BYTES"
        return data, ""
    except Exception as e:
        return None, f"URL 拉取失败（{type(e).__name__}）"


def fetch_resource_bytes(resource: dict) -> tuple[bytes | None, str]:
    """返回 (文件字节, 失败原因)。data 为 None 表示没拿到，调用方应记 fallback。"""
    object_name = (resource.get("object_name") or "").strip()
    url = (resource.get("url") or "").strip()
    reasons: list[str] = []

    try:
        data = _via_local(object_name)
        if data is not None:
            return data, ""
    except Exception as e:
        reasons.append(f"本地文件读取失败（{type(e).__name__}）")

    if settings.minio_access_key:
        data, why = _via_minio(object_name)
        if data is not None:
            return data, ""
        if why:
            reasons.append(why)

    data, why = _via_url(url)
    if data is not None:
        return data, ""
    if why:
        reasons.append(why)

    return None, "；".join(reasons) or "未配置任何可用的文件获取方式"
