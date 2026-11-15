"""课程资源正文解析：PDF / DOCX / PPTX / TXT，输出带页码/段落信息的切片文本。

设计取舍：
- DOCX/PPTX 本质是 zip + OOXML，用标准库 zipfile + ElementTree 就能提正文，
  不引入 python-docx/python-pptx，减少镜像体积与本地依赖；
- PDF 用 pypdf（可选导入）：没装时返回 unsupported 状态而不是崩溃；
- SRT/WebVTT 字幕用标准库正则解析，连续 cue 按时间窗口合并，带时间戳溯源；
  这是视频 STT 的务实替代：有配套字幕就能灌正文，不必等 ASR；
- 视频本体 STT 尚未实现，明确返回 unsupported + TODO 文案，绝不假装解析成功；
- 任何资源最终都能产出文本（哪怕只是名称+链接），但状态会标成 fallback，
  让「哪些资料还没真正进知识库」在 kb_documents.status 里一目了然。
"""
from __future__ import annotations
import io
import os
import re
import zipfile
from dataclasses import dataclass, field
from xml.etree import ElementTree as ET

from app.core.config import settings

W_NS = "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}"

PARSED = "parsed"
FALLBACK = "fallback"
UNSUPPORTED = "unsupported"
FAILED = "failed"

_VIDEO_TODO = "视频转写（ffmpeg 抽音频 + STT）尚未实现，TODO：接入 ASR 后按时间戳切片；已有配套 SRT/VTT 字幕的资源请单独作为字幕资源上传"

# 字幕合并窗口：连续 cue 拼成一段，避免一条切片只有几个字（30 秒约等于一次讲解）
_SUBTITLE_WINDOW_SECONDS = 30
_SUBTITLE_MAX_CHARS = 500
# SRT: 00:00:01,000 --> 00:00:04,500；VTT: 00:01.000 --> 00:04.500（也允许点号毫秒）
_TIMING_RE = re.compile(
    r"(?P<start>\d{1,2}:\d{2}:\d{2}[,.]\d{3}|\d{1,2}:\d{2}[,.]\d{3})\s*-->\s*"
    r"(?P<end>\d{1,2}:\d{2}:\d{2}[,.]\d{3}|\d{1,2}:\d{2}[,.]\d{3})"
)
_TAG_RE = re.compile(r"<[^>]+>")


@dataclass
class Section:
    text: str
    meta: dict = field(default_factory=dict)


@dataclass
class ParseResult:
    status: str
    sections: list[Section] = field(default_factory=list)
    reason: str = ""

    @property
    def has_body(self) -> bool:
        return self.status == PARSED and bool(self.sections)


_KNOWN_EXTS = {"pdf", "doc", "docx", "ppt", "pptx", "txt", "md", "mp4",
               "srt", "vtt", "webvtt"}


def _ext_of(resource: dict) -> str:
    """从 name / url / object_name 里取扩展名（URL 要去掉查询串）。"""
    for candidate in (resource.get("name") or "", resource.get("url") or "",
                      resource.get("object_name") or ""):
        ext = os.path.splitext(candidate.split("?")[0])[1].lower().lstrip(".")
        if ext in _KNOWN_EXTS:
            return {"md": "txt", "mp4": "video",
                    "srt": "subtitle", "vtt": "subtitle",
                    "webvtt": "subtitle"}.get(ext, ext)
    return ""


def detect_type(resource: dict) -> str:
    """优先 DB 的 type；但 type 可能滞后/写错（实测有 .docx 文件被标成 doc）。

    doc/ppt 这类「新旧格式同名」的值，若文件后缀明确是 docx/pptx，以后缀为准。
    真正解析时还会再看文件魔数，见 _parse_office。
    """
    raw = (resource.get("type") or "").strip().lower().lstrip(".")
    if raw in {"pdf", "doc", "docx", "ppt", "pptx", "txt", "md", "text",
               "video", "mp4", "link", "url", "srt", "vtt", "webvtt", "subtitle"}:
        t = {"text": "txt", "url": "link", "mp4": "video",
             "srt": "subtitle", "vtt": "subtitle", "webvtt": "subtitle"}.get(raw, raw)
        if t in {"doc", "ppt"}:
            ext = _ext_of(resource)
            if ext in {"docx", "pptx"}:
                return ext
        return t
    return _ext_of(resource) or "unknown"


def fallback_section(resource: dict) -> Section:
    """实在没有正文时的兜底文本：保留资源名+链接，且明确标注这不是正文。"""
    name = resource.get("name") or "未命名资源"
    url = resource.get("url") or ""
    return Section(
        f"课程资源：{name}\n链接：{url}\n（提示：该资源正文未解析，知识库仅有名称与链接）",
        {"title": name, "source": url, "body": False},
    )


def _clean(text: str) -> str:
    return re.sub(r"[ \t]+", " ", (text or "").strip())


def parse_plain_text(data: bytes) -> ParseResult:
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        text = data.decode("utf-8", errors="replace")
    blocks = [_clean(b) for b in re.split(r"\n\s*\n", text)]
    sections = [Section(b, {"section": i + 1}) for i, b in enumerate(blocks) if b]
    return (ParseResult(PARSED, sections) if sections
            else ParseResult(UNSUPPORTED, [], "文本文件为空"))


def parse_pdf(data: bytes) -> ParseResult:
    if not settings.ingest_parse_pdf:
        return ParseResult(UNSUPPORTED, [], "INGEST_PARSE_PDF=false")
    try:
        from pypdf import PdfReader  # 可选依赖，未安装不崩
    except ImportError:
        return ParseResult(UNSUPPORTED, [], "pypdf 未安装（pip install pypdf 后重试）")
    try:
        reader = PdfReader(io.BytesIO(data))
        sections = []
        for i, page in enumerate(reader.pages):
            text = _clean(page.extract_text() or "")
            if text:
                sections.append(Section(text, {"page": i + 1}))
        if not sections:
            return ParseResult(UNSUPPORTED, [], "PDF 无可提取文本（可能是扫描件，需 OCR，TODO）")
        return ParseResult(PARSED, sections)
    except Exception as e:
        return ParseResult(FAILED, [], f"PDF 解析失败（{type(e).__name__}）")


def parse_docx(data: bytes) -> ParseResult:
    """DOCX = zip + word/document.xml，按 <w:p> 段落抽 <w:t>。"""
    if not settings.ingest_parse_docx:
        return ParseResult(UNSUPPORTED, [], "INGEST_PARSE_DOCX=false")
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as z:
            root = ET.fromstring(z.read("word/document.xml"))
    except (zipfile.BadZipFile, KeyError, ET.ParseError) as e:
        return ParseResult(FAILED, [], f"DOCX 结构异常（{type(e).__name__}）")
    sections = []
    for i, para in enumerate(root.iter(W_NS + "p")):
        text = _clean("".join(t.text or "" for t in para.iter(W_NS + "t")))
        if text:
            sections.append(Section(text, {"paragraph": i + 1}))
    return (ParseResult(PARSED, sections) if sections
            else ParseResult(UNSUPPORTED, [], "DOCX 无正文段落"))


def parse_pptx(data: bytes) -> ParseResult:
    """PPTX = zip + ppt/slides/slideN.xml，按幻灯片抽 <a:t>。"""
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as z:
            names = sorted(
                n for n in z.namelist() if re.match(r"ppt/slides/slide\d+\.xml$", n)
            )
            sections = []
            for name in names:
                root = ET.fromstring(z.read(name))
                texts = [el.text or "" for el in root.iter() if el.tag.endswith("}t")]
                text = _clean(" ".join(texts))
                if text:
                    n = int(re.search(r"slide(\d+)", name).group(1))
                    sections.append(Section(text, {"slide": n}))
    except (zipfile.BadZipFile, KeyError, ET.ParseError) as e:
        return ParseResult(FAILED, [], f"PPTX 结构异常（{type(e).__name__}）")
    return (ParseResult(PARSED, sections) if sections
            else ParseResult(UNSUPPORTED, [], "PPTX 无幻灯片文本"))


def _timestamp_seconds(ts: str) -> float:
    """'00:01:02,500' / '01:02.500' → 秒；异常格式返回 0，不让解析崩。"""
    parts = ts.replace(",", ".").split(":")
    try:
        if len(parts) == 3:
            return int(parts[0]) * 3600 + int(parts[1]) * 60 + float(parts[2])
        if len(parts) == 2:
            return int(parts[0]) * 60 + float(parts[1])
    except ValueError:
        pass
    return 0.0


def parse_subtitle(data: bytes) -> ParseResult:
    """SRT / WebVTT 字幕 → 带时间戳的正文切片。

    视频本体的 STT 仍是 TODO；老师若已上传配套字幕，正文就能真实进知识库，
    比只灌「名称+链接」的 fallback 占位切片有用得多，且零新增依赖。
    连续 cue 按时间窗口合并，避免一条切片只有几个字；VTT 的 NOTE/STYLE 块与
    内联标签（<c>/<v>）会被跳过，不会污染正文。
    """
    if not settings.ingest_parse_subtitle:
        return ParseResult(UNSUPPORTED, [], "INGEST_PARSE_SUBTITLE=false")
    try:
        text = data.decode("utf-8-sig")
    except UnicodeDecodeError:
        text = data.decode("utf-8", errors="replace")
    lines = text.replace("\r\n", "\n").split("\n")

    cues: list[tuple[float, str, str, str]] = []  # (start_sec, start, end, text)
    i = 0
    while i < len(lines):
        line = lines[i].strip()
        i += 1
        if not line:
            continue
        upper = line.upper()
        if upper.startswith("WEBVTT"):
            continue
        if upper.startswith(("NOTE", "STYLE", "REGION")):
            # 元数据块到空行结束，整块跳过（里面可能含形似时间轴的内容）
            while i < len(lines) and lines[i].strip():
                i += 1
            continue
        m = _TIMING_RE.search(line)
        if not m:
            continue
        start, end = m.group("start"), m.group("end")
        buf: list[str] = []
        while i < len(lines) and lines[i].strip():
            raw = _TAG_RE.sub("", lines[i].strip())
            if raw:
                buf.append(raw)
            i += 1
        body = _clean(" ".join(buf))
        if body:
            cues.append((_timestamp_seconds(start), start, end, body))
    if not cues:
        return ParseResult(UNSUPPORTED, [], "字幕里没有可解析的时间轴与文本")

    def _section(start_ts: str, end_ts: str, parts: list[str]) -> Section:
        return Section(" ".join(parts), {
            "timestamp": start_ts, "start": start_ts, "end": end_ts,
            "cue_count": len(parts),
        })

    sections: list[Section] = []
    win_start, cur_start, cur_end, parts = cues[0][0], cues[0][1], cues[0][2], [cues[0][3]]
    for start_sec, start, end, body in cues[1:]:
        too_long = len(" ".join(parts)) >= _SUBTITLE_MAX_CHARS
        if start_sec - win_start <= _SUBTITLE_WINDOW_SECONDS and not too_long:
            parts.append(body)
            cur_end = end
        else:
            sections.append(_section(cur_start, cur_end, parts))
            win_start, cur_start, cur_end, parts = start_sec, start, end, [body]
    sections.append(_section(cur_start, cur_end, parts))
    return ParseResult(PARSED, sections)


def parse_video(data: bytes | None) -> ParseResult:
    """TODO：ffmpeg 抽音频 → ASR → 带时间戳字幕。当前如实返回 unsupported。"""
    if not settings.ingest_parse_video:
        return ParseResult(UNSUPPORTED, [], _VIDEO_TODO)
    return ParseResult(UNSUPPORTED, [], _VIDEO_TODO + "（开关已打开但实现仍缺失）")


def _parse_office(data: bytes | None) -> ParseResult:
    """Word/PPT 统一按文件内容分发，不盲信 DB 的 type。

    OOXML（.docx/.pptx）本质是 zip，魔数 PK；老式 .doc/.ppt 是 OLE 复合文档。
    实测库里存在「文件名 NOTE2.DOCX、type 却写成 doc」的数据，按 type 判会直接丢掉正文。
    """
    if not data:
        return ParseResult(UNSUPPORTED, [], "未取到 Office 文件字节")
    if data[:2] != b"PK":
        return ParseResult(UNSUPPORTED, [], "老式 doc/ppt 二进制格式未支持（建议先转 DOCX/PPTX/PDF）")
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as z:
            names = set(z.namelist())
    except (zipfile.BadZipFile, OSError):
        return ParseResult(FAILED, [], "Office 文件不是有效 OOXML 压缩包")
    if "word/document.xml" in names:
        if not settings.ingest_parse_docx:
            return ParseResult(UNSUPPORTED, [], "INGEST_PARSE_DOCX=false")
        return parse_docx(data)
    if any(n.startswith("ppt/slides/") and n.endswith(".xml") for n in names):
        return parse_pptx(data)
    return ParseResult(UNSUPPORTED, [], "OOXML 包里没找到 word/document.xml 或 ppt/slides")


def parse_resource(resource: dict, data: bytes | None) -> ParseResult:
    """统一入口：按类型解析；拿不到正文时返回 fallback（一条名称+链接切片）。"""
    rtype = detect_type(resource)
    name = resource.get("name") or "未命名资源"

    if rtype == "video":
        result = parse_video(data)
    elif rtype == "pdf":
        result = parse_pdf(data) if data else ParseResult(UNSUPPORTED, [], "未取到 PDF 文件字节")
    elif rtype in {"doc", "docx", "ppt", "pptx"}:
        result = _parse_office(data)
    elif rtype in {"txt", "md"}:
        result = parse_plain_text(data) if data else ParseResult(UNSUPPORTED, [], "未取到文本文件字节")
    elif rtype == "subtitle":
        result = parse_subtitle(data) if data else ParseResult(UNSUPPORTED, [], "未取到字幕文件字节")
    else:
        result = ParseResult(UNSUPPORTED, [], f"未知资源类型 {rtype}")

    # 附加资源级元信息到每个 section，供引用溯源
    for sec in result.sections:
        sec.meta.setdefault("title", name)
        sec.meta.setdefault("source", resource.get("url") or "")
        sec.meta.setdefault("body", True)
        sec.meta.setdefault("resource_type", rtype)

    if result.has_body:
        return result
    # 正文没解析出来：降级成名称+链接，但状态标 fallback，覆盖度报表能看见
    return ParseResult(FALLBACK, [fallback_section(resource)],
                       result.reason or "正文未解析，仅灌入名称与链接")
