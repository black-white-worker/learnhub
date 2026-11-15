"""中文专业素材源单测：清洗、相关性、随机抽样、prompt、缓存与降级。

全部用假 HTTP client，不打外网；settings 的开关用 monkeypatch 临时打开。
"""
import json
import re

import pytest

from app.questions import zhsource as zs
from app.questions.variety import (
    assess_chinese_quality,
    chinese_ratio,
    validate_question,
    apply_chinese_quality,
)


@pytest.fixture(autouse=True)
def _enable_and_clean(monkeypatch):
    monkeypatch.setattr(zs.settings, "zh_material_enabled", True)
    monkeypatch.setattr(zs.settings, "zh_material_wikibooks_enabled", True)
    zs.clear_cache()
    yield
    zs.clear_cache()


def wiki_payload(title="光电效应", extract="", extra=None):
    pages = [{"title": title, "extract": extract}]
    if extra:
        pages.append(extra)
    return {"query": {"pages": pages}}


LONG_ZH = (
    "光电效应是指光束照射物体时会使其发射出电子的物理效应，发射出来的电子称为光电子。"
    "1905年爱因斯坦提出光量子假说，认为光的能量并非连续分布，而是与光的频率成正比。"
)
LONG_ZH_2 = (
    "爱因斯坦光电方程为 Ek = hν - W，其中 h 为普朗克常数，ν 为入射光频率，W 为金属逸出功。"
    "当入射光频率低于截止频率时，无论光强多大、照射时间多长，都不会有光电子逸出。"
)
LONG_ZH_3 = (
    "密立根通过精确测量遏止电压与入射光频率的关系，验证了爱因斯坦光电方程的线性关系，"
    "并由此测定了普朗克常数；实验数据显示斜率约为 6.6×10^-34 J·s。"
)


class FakeResp:
    def __init__(self, data):
        self._data = data

    def json(self):
        return self._data


class FakeClient:
    """按 URL 返回预置 payload；也可让某个源抛异常模拟超时。"""

    def __init__(self, wiki=None, books=None, wiki_error=False, books_error=False,
                 wiki_intro=None, books_intro=None):
        self.wiki = wiki
        self.books = books
        self.wiki_intro = wiki_intro
        self.books_intro = books_intro
        self.wiki_error = wiki_error
        self.books_error = books_error
        self.calls = []

    async def get(self, url, params=None, timeout=None, headers=None):
        self.calls.append((url, dict(params or {})))
        is_intro = (params or {}).get("exintro") == "1"
        empty = {"query": {"pages": []}}
        if "wikibooks" in url:
            if self.books_error:
                raise TimeoutError("wikibooks timeout")
            data = self.books_intro if is_intro and self.books_intro is not None else self.books
            return FakeResp(data if data is not None else empty)
        if self.wiki_error:
            raise TimeoutError("wiki timeout")
        data = self.wiki_intro if is_intro and self.wiki_intro is not None else self.wiki
        return FakeResp(data if data is not None else empty)


# ===== 中文与专业度纯函数 =====

def test_chinese_ratio_and_english_detection():
    assert chinese_ratio("光电效应中光电子的最大初动能") > 0.9
    assert chinese_ratio("Which planet is known as the Red Planet?") == 0.0
    report = assess_chinese_quality(
        {"stem": "Which planet is known as the Red Planet?", "options": ["Mars", "Venus", "Jupiter", "Mercury"],
         "answer": "A", "explanation": "Mars is red."},
        "光电效应", "single",
    )
    assert report["invalid"] and "中文" in report["reason"]


def test_professional_chinese_question_passes_with_high_score():
    q = {
        "stem": "用频率为 2ν 的单色光照射逸出功为 W 的金属，光电子最大初动能如何变化？",
        "options": ["按 Ek = hν - W 线性增大但不一定翻倍", "必然翻倍", "保持不变", "必然减半"],
        "answer": "A",
        "explanation": "依据爱因斯坦光电方程 Ek = hν - W，ν 加倍后动能增加 hν，"
                       "只有 W=0 时才会翻倍，故 B 错在忽略逸出功。",
        "focus": "光电方程",
        "basis": "依据[1]：爱因斯坦光电方程",
    }
    report = assess_chinese_quality(q, "光电效应", "single", require_basis=True)
    assert not report["invalid"]
    assert report["quality_score"] >= 88
    assert report["warnings"] == []
    out = apply_chinese_quality(validate_question(q, "single"), "光电效应", "single", True)
    assert not out.get("invalid") and out["quality_score"] >= 88


def test_quality_warnings_for_weak_or_traditional_questions():
    report = assess_chinese_quality(
        {"stem": "关于這個知識點，下列说法正确的是", "options": ["甲", "乙", "丙", "丁"],
         "answer": "A", "explanation": "略"},
        "光电效应", "single",
    )
    assert not report["invalid"]  # 内容偏弱只提示，不判废
    joined = " ".join(report["warnings"])
    assert "繁体" in joined and "解析" in joined and "笼统" in joined and "关联弱" in joined


def test_chinese_stem_with_many_tech_abbreviations_is_not_misjudged_as_english():
    """计算机等专业题的题干天然夹带 CPU / cache / pipeline 等英文缩写，
    旧规则「英文词 >= 6 就判英文整句」会把地道中文专业题整批判废。"""
    stem = ("某RISC-V架构的CPU在400 MHz主频下运行，cache命中率为95%，"
            "若pipeline每周期可发射两条整数指令，且GPU不参与该整数运算，"
            "内存访存延迟为10 ns，则在不考虑分支预测失败与结构冒险的情况下，"
            "其IPC实测上限最接近下列哪一项？")
    q = {
        "stem": stem,
        "options": ["约 0.5 条/周期", "约 2.0 条/周期", "约 4.0 条/周期", "约 8.0 条/周期"],
        "answer": "B",
        "explanation": "单周期双发射且无结构冒险时，每周期最多提交 2 条指令，"
                       "故 IPC 上限为 2.0；其余选项分别对应单发射、四发射与八发射的错误假设。",
        "focus": "流水线发射宽度与 IPC",
    }
    assert chinese_ratio(stem) >= 0.5  # 确实是中文为主的题干
    assert len(re.findall(r"[A-Za-z][A-Za-z'’-]{1,}", stem)) >= 6  # 又确实夹带不少英文术语
    report = assess_chinese_quality(q, "计算机体系结构", "single")
    assert not report["invalid"], report["reason"]
    out = apply_chinese_quality(validate_question(q, "single"), "计算机体系结构", "single")
    assert not out.get("invalid")


def test_english_options_are_rejected_but_formula_options_are_not():
    en = assess_chinese_quality(
        {"stem": "请选择下列正确的中文表述", "options": ["Mars", "Venus", "Jupiter", "Mercury"],
         "answer": "A", "explanation": "火星是红色的行星"}, "太阳系", "single")
    assert en["invalid"]
    formula = assess_chinese_quality(
        {"stem": "该算法在最坏情况下的时间复杂度是下列哪一项", "options": ["O(n)", "O(n log n)", "O(n²)", "O(2ⁿ)"],
         "answer": "B", "explanation": "归并排序每层合并为线性时间，共有 log n 层，因此总复杂度为 O(n log n)。"},
        "归并排序时间复杂度", "single")
    assert not formula["invalid"]


# ===== 正文清洗 / 打分 / 抽样 =====

def test_split_snippets_drops_tail_sections_noise_and_english():
    text = "\n".join([
        LONG_ZH,
        "== 历史 ==",
        LONG_ZH_2,
        "^ Smith, J. (2001). ISBN 978-1-234",
        "== 参考文献 ==",
        "This section lists English references only, with no Chinese content at all.",
        LONG_ZH_3,
    ])
    parts = zs.split_snippets(text)
    assert len(parts) >= 2
    assert all(chinese_ratio(p) >= 0.45 for p in parts)
    assert all("ISBN" not in p and "参考文献" not in p for p in parts)
    assert all("This section lists" not in p for p in parts)


def test_score_prefers_relevant_numeric_snippets():
    relevant = zs.score_snippet(LONG_ZH_2, "光电效应")
    generic = zs.score_snippet(
        "在课堂教学中，教师可以根据学生的反馈调整讲解节奏，适当安排讨论与练习时间，"
        "以便学生更好地理解所学内容并提升课堂参与度。", "光电效应")
    assert relevant > generic


def test_select_materials_is_seed_driven_and_uses_multiple_docs():
    docs = [
        {"title": "光电效应", "extract": "\n".join([LONG_ZH, LONG_ZH_2, LONG_ZH_3]),
         "provider": "中文维基百科", "view_url": "https://zh.wikipedia.org/wiki/"},
        {"title": "内光电效应", "extract": "\n".join([LONG_ZH_2, LONG_ZH]),
         "provider": "中文维基百科", "view_url": "https://zh.wikipedia.org/wiki/"},
    ]
    a = zs.select_materials(docs, "光电效应", "seed-1", limit=3)
    b = zs.select_materials(docs, "光电效应", "seed-1", limit=3)
    c = zs.select_materials(docs, "光电效应", "seed-2", limit=3)
    assert a == b
    assert len(a) == 3
    assert [m.snippet for m in a] != [m.snippet for m in c]
    assert all(m.url.startswith("https://zh.wikipedia.org/wiki/") for m in a)
    assert all(m.provider == "中文维基百科" for m in a)


def test_materials_prompt_requires_simplified_chinese_and_basis():
    materials = [
        zs.Material("光电效应", LONG_ZH_2, "https://zh.wikipedia.org/wiki/光电效应", "中文维基百科")
    ]
    text = zs.materials_prompt(materials, "光电效应", "物理学")
    assert "[1]《光电效应》" in text and "CC BY-SA 4.0" in text
    assert "简体中文" in text and "basis" in text and "物理学" in text
    assert zs.materials_prompt([], "x") == ""


# ===== 检索、缓存、降级 =====

@pytest.mark.asyncio
async def test_fetch_materials_uses_simplified_chinese_docs_and_caches():
    client = FakeClient(wiki=wiki_payload(extract="\n".join([LONG_ZH, LONG_ZH_2, LONG_ZH_3])))
    result = await zs.fetch_materials("光电效应", "物理学", "s1", client=client)
    assert result.ok and result.status == "ok"
    assert result.provider == "中文维基百科"
    assert result.materials and all(chinese_ratio(m.snippet) > 0.5 for m in result.materials)
    assert result.as_meta()["materials"][0]["url"].startswith("https://zh.wikipedia.org/wiki/")
    assert all(p.get("variant") == "zh-cn" for _, p in client.calls)

    # 第二次命中缓存，不再打外网；不同 seed 会换素材
    calls = len(client.calls)
    result2 = await zs.fetch_materials("光电效应", "物理学", "s2", client=client)
    assert len(client.calls) == calls
    assert result2.ok


@pytest.mark.asyncio
async def test_fallback_to_wikibooks_when_main_source_fails():
    client = FakeClient(
        wiki_error=True,
        books=wiki_payload(title="物理实验教程", extract="\n".join([LONG_ZH_2, LONG_ZH_3])),
    )
    result = await zs.fetch_materials("光电效应", "", "s1", client=client)
    assert result.ok and result.provider == "中文维基教科书"
    # 每个源会发「全文 + 多条目导言」两次请求；主源失败后才会打备用源
    assert len(client.calls) >= 2


@pytest.mark.asyncio
async def test_materials_come_from_multiple_related_articles():
    """全文只给一个页面，补一次多条目导言请求后素材应覆盖不同条目。"""
    full = wiki_payload("光电效应", "\n".join([LONG_ZH, LONG_ZH_2, LONG_ZH_3]))
    intro = {"query": {"pages": [
        {"title": "内光电效应", "extract": LONG_ZH_2 +
         "内光电效应发生在半导体内部，光子激发电子空穴对而改变电导率，与外光电效应的逸出功条件不同。"},
        {"title": "光生伏打效应", "extract": LONG_ZH_3 +
         "在PN结自建场作用下光生载流子分离形成光电压，开路电压与辐照度的对数相关。"},
    ]}}
    client = FakeClient(wiki=full, wiki_intro=intro)
    result = await zs.fetch_materials("光电效应", "", "seed-multi", client=client)
    assert result.ok
    assert len({m.title for m in result.materials}) >= 2
    assert any(p.get("exintro") == "1" for _, p in client.calls)
    assert all(p.get("variant") == "zh-cn" for _, p in client.calls)


@pytest.mark.asyncio
async def test_all_sources_down_is_unavailable_and_never_raises():
    client = FakeClient(wiki_error=True, books_error=True)
    result = await zs.fetch_materials("光电效应", "", "s1", client=client)
    assert result.status == "unavailable" and not result.ok
    assert result.as_meta()["material_status"] == "unavailable"


@pytest.mark.asyncio
async def test_disabled_and_empty_knowledge_short_circuit(monkeypatch):
    monkeypatch.setattr(zs.settings, "zh_material_enabled", False)
    client = FakeClient(wiki=wiki_payload(extract=LONG_ZH))
    assert (await zs.fetch_materials("光电效应", client=client)).status == "disabled"
    assert client.calls == []
    monkeypatch.setattr(zs.settings, "zh_material_enabled", True)
    assert (await zs.fetch_materials("", client=client)).status == "no_match"


@pytest.mark.asyncio
async def test_no_matching_pages_reports_no_match():
    result = await zs.fetch_materials("一个完全不存在的知识点XYZ", "", "s1",
                                      client=FakeClient(wiki={"query": {"pages": []}}))
    assert result.status == "no_match" and not result.ok


@pytest.mark.asyncio
async def test_subject_is_only_used_as_disambiguation_fallback():
    client = FakeClient(wiki={"query": {"pages": []}})
    result = await zs.fetch_materials("冷门知识点", "高等量子力学", "s1", client=client)
    assert result.status == "no_match"
    queries = [p.get("gsrsearch") for _, p in client.calls]
    # 先按知识点精确检索，没命中才加学科补查
    assert queries and queries[0] == "冷门知识点"
    assert any(q and "高等量子力学" in q for q in queries)
