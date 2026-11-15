"""课程资料清单：向 Go 主服务要「这门课到底有哪些讲义 / 视频」。

为什么不继续让 ingest 自己查 MySQL：
`course_resources` 的唯一对外口径是课程资源接口（详情页那排「预览 / 下载 / 删除」
就是它的返回）。ai-service 再直连一次库，等于把口径复制了一份 —— 私有桶开预签名、
资源类型映射改了、资源被删了，只要有一处没同步，学生看到的就是
「页面上明明有讲义，AI 却说资料里没有相关内容」。

所以改成问 Go 要清单（`GET /api/internal/course/:courseId/kb-manifest`，
X-Internal-Token 鉴权）。查不到（Go 重启中 / 本地没配内部令牌）时返回 None，
由调用方退回直读 MySQL —— 灌库可以慢，但不能因为一次 HTTP 失败就整门课不灌。

刻意不做的事：这里不缓存教学口径以外的信息，也不把预签名 URL 写进日志。
"""
from __future__ import annotations

import json
import logging
import time
import urllib.request

from app.core.config import settings

log = logging.getLogger(__name__)

# course_id -> (取回时刻, 清单)。一次答疑会连续问几轮，每轮都打 Go 没必要。
_cache: dict[int, tuple[float, dict]] = {}


def enabled() -> bool:
    """没配内部令牌 / 回调地址时一律不发请求（本地调试、单元测试走兜底路径）。"""
    return bool((settings.go_callback_base or "").strip()) and bool(
        (settings.internal_token or "").strip()
    )


def clear_cache(course_id: int | None = None) -> None:
    """灌库/删资源后调用，让下一次答疑立刻看到最新清单。"""
    if course_id is None:
        _cache.clear()
    else:
        _cache.pop(course_id, None)


def _manifest_url(course_id: int) -> str:
    base = (settings.go_callback_base or "").strip().rstrip("/")
    if "://" not in base:
        base = "http://" + base
    return f"{base}/api/internal/course/{course_id}/kb-manifest"


def fetch_course_manifest(course_id: int, use_cache: bool = True) -> dict | None:
    """取清单；任何失败都返回 None（调用方兜底），不往主流程抛异常。"""
    if course_id <= 0 or not enabled():
        return None
    now = time.monotonic()
    if use_cache:
        hit = _cache.get(course_id)
        if hit and now - hit[0] < settings.manifest_cache_ttl:
            return hit[1]

    req = urllib.request.Request(
        _manifest_url(course_id),
        headers={
            "X-Internal-Token": settings.internal_token,
            "Accept": "application/json",
            "User-Agent": "learningplatform-ai/1.0",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=settings.manifest_timeout) as resp:
            data = json.loads(resp.read(1 << 20).decode("utf-8"))
    except Exception as e:
        # 只记异常类型：清单里是预签名地址，原文不能进日志
        log.warning("课程 %s 资料清单获取失败（%s），改用数据库兜底", course_id, type(e).__name__)
        return None
    if not isinstance(data, dict):
        log.warning("课程 %s 资料清单结构异常，改用数据库兜底", course_id)
        return None
    _cache[course_id] = (now, data)
    return data


def resource_rows(manifest: dict | None, course_id: int) -> list[dict] | None:
    """清单里的讲义 → ingest 的行结构（列名与旧 SQL 查询保持一致）。

    返回 None = 清单不可用（去查库）；返回 [] = 这门课确实没有讲义（不用查库）。
    这两种情况必须区分：把「清单空」当成「没取到」会让刚清空讲义的课程
    又被库里残留数据灌回来。
    """
    if manifest is None:
        return None
    rows: list[dict] = []
    for item in manifest.get("resources") or []:
        if not isinstance(item, dict):
            continue
        try:
            rid = int(item.get("id") or 0)
        except (TypeError, ValueError):
            continue
        if rid <= 0:
            continue
        rows.append({
            "id": rid,
            "course_id": int(manifest.get("course_id") or course_id),
            "name": item.get("name") or "",
            "url": item.get("url") or "",
            "object_name": item.get("object_name") or "",
            "type": item.get("type") or "",
            "size": int(item.get("size") or 0),
        })
    return rows


def video_titles(manifest: dict | None) -> list[str]:
    """课程视频标题。视频正文要 ASR 才能切，当前只能给标题清单。"""
    if not manifest:
        return []
    titles: list[str] = []
    for item in manifest.get("videos") or []:
        if not isinstance(item, dict):
            continue
        name = str(item.get("name") or "").strip()
        if name:
            titles.append(name)
    return titles
