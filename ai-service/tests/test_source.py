"""原文件获取层测试（对应 app/rag/source.py）。

重点：本地目录映射的路径穿越防护、URL 协议白名单、大小上限、
以及「minio SDK 没装也不能阻断 URL 兜底」。
"""
import io
from unittest.mock import patch

from app.rag import source


class _Resp(io.BytesIO):
    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False


class TestLocalPath:
    def test_maps_object_name_under_root(self, tmp_path, monkeypatch):
        (tmp_path / "course" / "a.pdf").parent.mkdir(parents=True)
        (tmp_path / "course" / "a.pdf").write_bytes(b"pdf-bytes")
        monkeypatch.setattr(source.settings, "ingest_local_file_root", str(tmp_path))
        data, why = source.fetch_resource_bytes({"object_name": "course/a.pdf", "url": ""})
        assert data == b"pdf-bytes" and why == ""

    def test_path_traversal_is_rejected(self, tmp_path, monkeypatch):
        outside = tmp_path.parent / "secret.txt"
        outside.write_bytes(b"secret")
        monkeypatch.setattr(source.settings, "ingest_local_file_root", str(tmp_path))
        assert source._local_path("../secret.txt") is None

    def test_missing_file_falls_through(self, tmp_path, monkeypatch):
        monkeypatch.setattr(source.settings, "ingest_local_file_root", str(tmp_path))
        data, why = source.fetch_resource_bytes({"object_name": "nope.pdf", "url": ""})
        assert data is None and why


class TestViaUrl:
    def test_only_http_https_allowed(self):
        data, why = source._via_url("file:///etc/passwd")
        assert data is None and "协议" in why

    def test_downloads_with_timeout_and_size_cap(self, monkeypatch):
        monkeypatch.setattr(source.settings, "ingest_fetch_timeout", 3)
        with patch.object(source.urllib.request, "urlopen",
                          return_value=_Resp(b"hello")) as m:
            data, why = source._via_url("http://minio/course/a.pdf")
        assert data == b"hello" and why == ""
        assert m.call_args.kwargs["timeout"] == 3

    def test_oversize_file_rejected(self, monkeypatch):
        monkeypatch.setattr(source.settings, "ingest_max_file_bytes", 4)
        with patch.object(source.urllib.request, "urlopen",
                          return_value=_Resp(b"12345")):
            data, why = source._via_url("http://minio/course/a.pdf")
        assert data is None and "超过" in why

    def test_network_failure_returns_reason_not_exception(self):
        with patch.object(source.urllib.request, "urlopen",
                          side_effect=OSError("connection refused")):
            data, why = source._via_url("http://minio/course/a.pdf")
        assert data is None and "URL 拉取失败" in why
        assert "connection refused" not in why   # 不把底层异常原文带出去


class TestMinioOptional:
    def test_missing_sdk_does_not_block_url_fallback(self, monkeypatch):
        monkeypatch.setattr(source.settings, "minio_access_key", "ak")
        monkeypatch.setattr(source.settings, "minio_secret_key", "sk")
        with patch.dict("sys.modules", {"minio": None}), \
             patch.object(source, "_via_url", return_value=(b"from-url", "")) as url_mock:
            data, why = source.fetch_resource_bytes(
                {"object_name": "course/a.pdf", "url": "http://x/a.pdf"})
        assert data == b"from-url"
        assert url_mock.called

    def test_no_ak_skips_minio_entirely(self, monkeypatch):
        monkeypatch.setattr(source.settings, "minio_access_key", "")
        data, why = source._via_minio("course/a.pdf")
        assert data is None and why == ""


class TestFetchAllPathsFail:
    def test_returns_none_with_reason(self, monkeypatch):
        monkeypatch.setattr(source.settings, "ingest_local_file_root", "")
        monkeypatch.setattr(source.settings, "minio_access_key", "")
        with patch.object(source, "_via_url", return_value=(None, "URL 拉取失败")):
            data, why = source.fetch_resource_bytes({"url": "http://x/a.pdf"})
        assert data is None and "URL 拉取失败" in why
