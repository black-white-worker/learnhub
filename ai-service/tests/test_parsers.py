"""文档解析层测试（对应 app/rag/parsers.py）。

用标准库现场构造 DOCX/PPTX 字节，模拟 pypdf 缺失与扫描件，
重点验证「解析不出来必须标 fallback/unsupported，不能假装成功」。
"""
import io
import sys
import types
import zipfile
from unittest.mock import patch

from app.rag import parsers


def _docx_bytes(paragraphs):
    body = "".join(
        f"<w:p><w:r><w:t>{p}</w:t></w:r></w:p>" for p in paragraphs
    )
    xml = ('<?xml version="1.0"?>'
           '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">'
           f"<w:body>{body}</w:body></w:document>")
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("word/document.xml", xml)
    return buf.getvalue()


def _pptx_bytes(slides):
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        for i, text in enumerate(slides, start=1):
            xml = ('<?xml version="1.0"?>'
                   '<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" '
                   'xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">'
                   f"<a:t>{text}</a:t></p:sld>")
            z.writestr(f"ppt/slides/slide{i}.xml", xml)
    return buf.getvalue()


def _fake_pdf_module(page_texts):
    mod = types.ModuleType("pypdf")

    class _Page:
        def __init__(self, text):
            self._text = text

        def extract_text(self):
            return self._text

    class _Reader:
        def __init__(self, buf):
            self.pages = [_Page(t) for t in page_texts]

    mod.PdfReader = _Reader
    return mod


class TestDetectType:
    def test_from_db_type(self):
        assert parsers.detect_type({"type": "PDF", "name": "x"}) == "pdf"

    def test_from_filename_when_type_missing(self):
        assert parsers.detect_type({"type": "", "name": "讲义.pdf"}) == "pdf"

    def test_from_url_with_query_string(self):
        assert parsers.detect_type({"name": "无后缀", "url": "http://x/a.docx?sign=abc"}) == "docx"

    def test_unknown(self):
        assert parsers.detect_type({"name": "无后缀", "url": ""}) == "unknown"

    def test_doc_type_with_docx_filename_prefers_suffix(self):
        """实测数据：文件名 NOTE2.DOCX，DB 里 type 却写成 doc。"""
        r = {"name": "NOTE2.DOCX", "type": "doc",
             "object_name": "course/4/resources/x.docx"}
        assert parsers.detect_type(r) == "docx"


class TestOfficeByContent:
    """Office 解析按文件魔数分发，type 字段写错也能解析出正文。"""

    def test_doc_type_but_real_docx_is_parsed(self):
        r = parsers.parse_resource(
            {"name": "NOTE2.DOCX", "type": "doc", "url": "http://x/a.docx"},
            _docx_bytes(["检索增强生成 RAG"]),
        )
        assert r.status == parsers.PARSED
        assert r.sections[0].text == "检索增强生成 RAG"
        assert r.sections[0].meta["resource_type"] == "docx"

    def test_pptx_content_with_wrong_type_is_parsed(self):
        r = parsers.parse_resource(
            {"name": "课件.ppt", "type": "doc", "url": "http://x/a.pptx"},
            _pptx_bytes(["第一页 RAG"]),
        )
        assert r.status == parsers.PARSED
        assert r.sections[0].meta["slide"] == 1

    def test_legacy_binary_is_honestly_fallback(self):
        r = parsers.parse_resource({"name": "老课件.doc", "type": "doc"},
                                   b"\xd0\xcf\x11\xe0 OLE binary")
        assert r.status == parsers.FALLBACK
        assert "老式" in r.reason

    def test_zip_without_office_xml_is_fallback(self):
        buf = io.BytesIO()
        with zipfile.ZipFile(buf, "w") as z:
            z.writestr("other.txt", "hello")
        r = parsers.parse_resource({"name": "x.docx", "type": "docx"}, buf.getvalue())
        assert r.status == parsers.FALLBACK
        assert "没找到" in r.reason


class TestPlainText:
    def test_splits_paragraphs_with_section_meta(self):
        r = parsers.parse_plain_text("第一段讲 RAG\n\n第二段讲向量库".encode())
        assert r.status == parsers.PARSED
        assert len(r.sections) == 2
        assert r.sections[1].meta["section"] == 2

    def test_empty_text_is_unsupported(self):
        r = parsers.parse_plain_text(b"")
        assert r.status == parsers.UNSUPPORTED

    def test_invalid_utf8_does_not_crash(self):
        r = parsers.parse_plain_text(b"\xff\xfe bad")
        assert r.status == parsers.PARSED


class TestDocxAndPptx:
    def test_docx_paragraphs(self):
        r = parsers.parse_docx(_docx_bytes(["检索增强生成", "向量库原理"]))
        assert r.status == parsers.PARSED
        assert [s.text for s in r.sections] == ["检索增强生成", "向量库原理"]
        assert r.sections[0].meta["paragraph"] == 1

    def test_docx_empty_document(self):
        r = parsers.parse_docx(_docx_bytes([]))
        assert r.status == parsers.UNSUPPORTED

    def test_bad_zip_is_failed_not_crash(self):
        r = parsers.parse_docx(b"not a zip")
        assert r.status == parsers.FAILED

    def test_pptx_slides(self):
        r = parsers.parse_pptx(_pptx_bytes(["第一页 RAG", "第二页 Chroma"]))
        assert r.status == parsers.PARSED
        assert [s.meta["slide"] for s in r.sections] == [1, 2]

    def test_pptx_notes_not_counted(self):
        buf = io.BytesIO()
        with zipfile.ZipFile(buf, "w") as z:
            z.writestr("ppt/notesSlides/notesSlide1.xml", "<a:t>讲稿</a:t>")
        assert parsers.parse_pptx(buf.getvalue()).status == parsers.UNSUPPORTED


class TestPdf:
    def test_pages_get_page_meta(self):
        with patch.dict(sys.modules, {"pypdf": _fake_pdf_module(["第一页 RAG", "第二页 向量"])}):
            r = parsers.parse_pdf(b"fake-pdf")
        assert r.status == parsers.PARSED
        assert [s.meta["page"] for s in r.sections] == [1, 2]

    def test_missing_pypdf_is_unsupported(self):
        with patch.dict(sys.modules, {"pypdf": None}):
            r = parsers.parse_pdf(b"fake-pdf")
        assert r.status == parsers.UNSUPPORTED
        assert "pypdf" in r.reason

    def test_scanned_pdf_no_text(self):
        with patch.dict(sys.modules, {"pypdf": _fake_pdf_module(["", ""])}):
            r = parsers.parse_pdf(b"fake-pdf")
        assert r.status == parsers.UNSUPPORTED and "OCR" in r.reason

    def test_can_be_disabled_by_config(self, monkeypatch):
        monkeypatch.setattr(parsers.settings, "ingest_parse_pdf", False)
        assert parsers.parse_pdf(b"x").status == parsers.UNSUPPORTED


class TestVideoTodoHonesty:
    def test_default_off_and_reason_mentions_todo(self):
        r = parsers.parse_video(b"fake")
        assert r.status == parsers.UNSUPPORTED
        assert "TODO" in r.reason and "转写" in r.reason

    def test_even_when_switch_on_it_stays_unsupported(self, monkeypatch):
        monkeypatch.setattr(parsers.settings, "ingest_parse_video", True)
        r = parsers.parse_video(b"fake")
        assert r.status == parsers.UNSUPPORTED
        assert "实现仍缺失" in r.reason


class TestParseResourceFallback:
    def test_no_bytes_falls_back_with_name(self):
        r = parsers.parse_resource(
            {"name": "第三章讲义.pdf", "url": "http://x/a.pdf", "type": "pdf"}, None)
        assert r.status == parsers.FALLBACK
        assert "第三章讲义" in r.sections[0].text
        assert r.sections[0].meta["body"] is False
        assert "未取到" in r.reason

    def test_old_office_format_marked_unsupported_then_fallback(self):
        r = parsers.parse_resource({"name": "老课件.doc", "type": "doc"}, b"xx")
        assert r.status == parsers.FALLBACK
        assert "老式" in r.reason

    def test_success_attaches_resource_metadata(self):
        r = parsers.parse_resource(
            {"name": "讲义.docx", "url": "http://x/a.docx", "type": "docx"},
            _docx_bytes(["RAG 检索增强生成"]),
        )
        assert r.status == parsers.PARSED
        sec = r.sections[0]
        assert sec.meta["title"] == "讲义.docx"
        assert sec.meta["source"] == "http://x/a.docx"
        assert sec.meta["resource_type"] == "docx"
        assert sec.meta["body"] is True

    def test_fallback_section_never_claims_body(self):
        s = parsers.fallback_section({"name": "x", "url": "u"})
        assert s.meta["body"] is False
