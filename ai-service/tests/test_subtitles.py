"""字幕解析测试（对应 app/rag/parsers.py 的 parse_subtitle / detect_type）。

重点：SRT 与 WebVTT 两种格式、时间戳溯源、时间窗口合并、
元数据块与内联标签不污染正文、解析不出来必须诚实标 unsupported/fallback。
"""
from app.rag import parsers

SRT = """1
00:00:01,000 --> 00:00:04,500
今天讲检索增强生成 RAG

2
00:00:05,000 --> 00:00:09,000
RAG 分为索引检索与生成三步
"""

VTT = """WEBVTT

NOTE 这是讲解备注，不应进正文

STYLE
::cue { color: yellow; }

00:00:01.000 --> 00:00:04.500
<c>第一章 向量库</c>

00:00:05.000 --> 00:00:09.000
<v Teacher>混合检索用 RRF 融合</v>
"""


class TestSrt:
    def test_parses_body_with_timestamp_and_cue_count(self):
        r = parsers.parse_subtitle(SRT.encode())
        assert r.status == parsers.PARSED
        assert len(r.sections) == 1
        sec = r.sections[0]
        assert "今天讲检索增强生成 RAG" in sec.text
        assert "RAG 分为索引检索与生成三步" in sec.text
        assert sec.meta["start"] == "00:00:01,000"
        assert sec.meta["timestamp"] == "00:00:01,000"
        assert sec.meta["end"] == "00:00:09,000"
        assert sec.meta["cue_count"] == 2

    def test_short_mmss_format(self):
        data = "01:02.000 --> 01:05.000\n短格式时间戳\n".encode()
        r = parsers.parse_subtitle(data)
        assert r.status == parsers.PARSED
        assert r.sections[0].meta["start"] == "01:02.000"

    def test_bom_is_handled(self):
        r = parsers.parse_subtitle(b"\xef\xbb\xbf" + SRT.encode())
        assert r.status == parsers.PARSED


class TestWebVtt:
    def test_skips_note_style_and_inline_tags(self):
        r = parsers.parse_subtitle(VTT.encode())
        assert r.status == parsers.PARSED
        text = r.sections[0].text
        assert "第一章 向量库" in text
        assert "混合检索用 RRF 融合" in text
        for noise in ("NOTE", "讲解备注", "STYLE", "color", "<c>", "<v", "Teacher"):
            assert noise not in text

    def test_vtt_timestamps_use_dot_millis(self):
        r = parsers.parse_subtitle(VTT.encode())
        assert r.sections[0].meta["start"] == "00:00:01.000"


class TestWindowMerging:
    def test_gap_over_30s_starts_new_section(self):
        data = ("1\n00:00:01,000 --> 00:00:02,000\n第一段\n\n"
                "2\n00:01:05,000 --> 00:01:08,000\n第二段\n").encode()
        r = parsers.parse_subtitle(data)
        assert r.status == parsers.PARSED
        assert len(r.sections) == 2
        assert [s.meta["timestamp"] for s in r.sections] == [
            "00:00:01,000", "00:01:05,000"]

    def test_very_long_cue_group_is_split_by_chars(self):
        long = "检索增强生成 " * 120  # 远超 500 字符
        data = (f"1\n00:00:01,000 --> 00:00:10,000\n{long}\n\n"
                "2\n00:00:11,000 --> 00:00:14,000\n后续内容\n").encode()
        r = parsers.parse_subtitle(data)
        assert r.status == parsers.PARSED
        assert len(r.sections) == 2
        assert "后续内容" in r.sections[1].text


class TestHonestFailures:
    def test_empty_is_unsupported(self):
        r = parsers.parse_subtitle(b"")
        assert r.status == parsers.UNSUPPORTED
        assert "没有可解析" in r.reason

    def test_text_without_timing_is_unsupported(self):
        r = parsers.parse_subtitle("只是一段没有时间轴的文字".encode())
        assert r.status == parsers.UNSUPPORTED

    def test_malformed_timing_does_not_crash(self):
        data = "1\n99:99:99,xxx --> bad\n正文\n".encode()
        assert parsers.parse_subtitle(data).status == parsers.UNSUPPORTED

    def test_invalid_utf8_does_not_crash(self):
        r = parsers.parse_subtitle(b"\xff\xfe" + SRT.encode())
        assert r.status == parsers.PARSED

    def test_switch_off_returns_unsupported(self, monkeypatch):
        monkeypatch.setattr(parsers.settings, "ingest_parse_subtitle", False)
        r = parsers.parse_subtitle(SRT.encode())
        assert r.status == parsers.UNSUPPORTED
        assert "INGEST_PARSE_SUBTITLE" in r.reason


class TestDetectAndDispatch:
    def test_srt_and_vtt_suffixes(self):
        assert parsers.detect_type({"name": "课件.srt", "type": ""}) == "subtitle"
        assert parsers.detect_type({"name": "课件.VTT", "type": ""}) == "subtitle"

    def test_db_type_subtitle_and_url_query(self):
        assert parsers.detect_type({"type": "subtitle", "name": "x"}) == "subtitle"
        assert parsers.detect_type(
            {"name": "无后缀", "url": "http://x/a.srt?sign=abc"}) == "subtitle"

    def test_parse_resource_attaches_resource_meta(self):
        r = parsers.parse_resource(
            {"name": "第一课字幕.srt", "url": "http://x/a.srt", "type": "srt"},
            SRT.encode(),
        )
        assert r.status == parsers.PARSED
        sec = r.sections[0]
        assert sec.meta["title"] == "第一课字幕.srt"
        assert sec.meta["resource_type"] == "subtitle"
        assert sec.meta["body"] is True
        assert "timestamp" in sec.meta

    def test_no_bytes_falls_back_and_never_claims_body(self):
        r = parsers.parse_resource(
            {"name": "第一课字幕.vtt", "url": "http://x/a.vtt", "type": "vtt"}, None)
        assert r.status == parsers.FALLBACK
        assert r.sections[0].meta["body"] is False
        assert "未取到" in r.reason
