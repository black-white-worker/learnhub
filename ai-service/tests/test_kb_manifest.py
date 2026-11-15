"""课程资料清单（app/kb/manifest.py）测试。

清单是「页面上的讲义 = 灌进知识库的讲义」这条等价关系的唯一保证，
所以这几条契约必须钉住：没令牌不发请求、空清单不等于取不到、字段映射别漂。
"""
from app.core.config import settings
from app.kb import manifest


class _FakeResp:
    def __init__(self, payload: bytes):
        self._payload = payload

    def read(self, n: int = -1) -> bytes:
        return self._payload

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


def test_disabled_without_internal_token(monkeypatch):
    """本地调试 / 未接 AI 网关：不该往外发请求，直接让调用方走兜底。"""
    monkeypatch.setattr(settings, "internal_token", "")
    monkeypatch.setattr(settings, "go_callback_base", "http://backend:8888")
    assert manifest.enabled() is False

    def boom(*a, **kw):
        raise AssertionError("没配令牌时不该发 HTTP 请求")

    monkeypatch.setattr(manifest.urllib.request, "urlopen", boom)
    manifest.clear_cache()
    assert manifest.fetch_course_manifest(7) is None


def test_fetch_calls_course_api_with_token_and_caches(monkeypatch):
    monkeypatch.setattr(settings, "internal_token", "tok-123")
    monkeypatch.setattr(settings, "go_callback_base", "http://backend:8888/")
    monkeypatch.setattr(settings, "manifest_cache_ttl", 60)
    calls = []

    def fake_urlopen(req, timeout=None):
        calls.append(req.full_url)
        assert "tok-123" in list(req.headers.values()), "内部令牌必须随请求发出"
        return _FakeResp(b'{"course_id":7,"resources":[],"videos":[]}')

    monkeypatch.setattr(manifest.urllib.request, "urlopen", fake_urlopen)
    manifest.clear_cache()

    first = manifest.fetch_course_manifest(7)
    second = manifest.fetch_course_manifest(7)
    assert first["course_id"] == 7 and second == first
    assert calls == ["http://backend:8888/api/internal/course/7/kb-manifest"], "第二次应命中缓存"

    # 强制绕过缓存（灌库前会 clear_cache）必须重新请求
    manifest.clear_cache(7)
    manifest.fetch_course_manifest(7)
    assert len(calls) == 2


def test_network_error_degrades_to_none(monkeypatch):
    monkeypatch.setattr(settings, "internal_token", "tok")
    monkeypatch.setattr(settings, "go_callback_base", "http://backend:8888")

    def exploding(req, timeout=None):
        raise TimeoutError("timed out")

    monkeypatch.setattr(manifest.urllib.request, "urlopen", exploding)
    manifest.clear_cache()
    assert manifest.fetch_course_manifest(7) is None


def test_resource_rows_maps_columns_and_skips_invalid():
    payload = {"course_id": 7, "resources": [
        {"id": 6, "name": "讲义.docx", "url": "u", "object_name": "o/r6.docx",
         "type": "docx", "size": 884},
        {"id": 0, "name": "id 非法"},
        {"name": "没有 id"},
        "不是 dict",
    ]}
    rows = manifest.resource_rows(payload, 7)
    assert [r["id"] for r in rows] == [6]
    assert rows[0]["object_name"] == "o/r6.docx"
    assert rows[0]["course_id"] == 7
    assert rows[0]["type"] == "docx"


def test_resource_rows_none_is_unknown_not_empty():
    """None = 没取到（去查库）；[] = 确实没有（别查库，否则删掉的讲义会被灌回来）。"""
    assert manifest.resource_rows(None, 7) is None
    assert manifest.resource_rows({"resources": []}, 7) == []


def test_video_titles():
    payload = {"videos": [{"name": "第一讲 Vue3 起步"}, {"name": "  "}, {}]}
    assert manifest.video_titles(payload) == ["第一讲 Vue3 起步"]
    assert manifest.video_titles(None) == []
