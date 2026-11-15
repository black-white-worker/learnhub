"""按知识点检索的**中文专业知识素材源**（免费、免密钥、可降级）。

为什么不是「现成中文题库 API」：
  调研后确认，能按知识点/学科抽中文成题的公开接口基本都要注册后拿 API Key，
  其余多为营销页；真正稳定免费、无需密钥、且按知识点可检索的中文专业内容，
  是中文维基百科与中文维基教科书（CC BY-SA 4.0）。所以这里的做法是：

  老师填知识点 → 检索对应中文条目 → 切段并按 seed 随机抽取若干条事实素材
  → 注入 LLM prompt，要求题目**必须依据素材**、输出简体中文、写解析与依据编号。

  这样得到的仍然是「按知识点出的中文专业题」，但比模型自由发挥更专业、更少编造，
  且每次抽取的素材段落不同，题目随机性也更强。

可用性与边界：
  - 2026-11-15 宿主机与 ai-service 容器内实测均 200，`variant=zh-cn` 直接返回简体；
  - 进程内缓存条目正文（默认 6 小时），每次请求只做一次外网检索，重试轮次复用缓存；
  - 超时 4s、任何异常都返回 status != ok，由 generate.py 自动退回「纯模型中文出题」，
    绝不把老师的「生成」按钮拖成一次失败请求；
  - 素材只作**事实依据**注入 prompt，不直接当题目返回；题面由模型命制，
    返回体里保留条目名与链接便于署名（CC BY-SA 4.0）。
"""
from __future__ import annotations

import random
import re
import threading
import time
from dataclasses import dataclass, field
from typing import List, Optional, Sequence
from urllib.parse import quote

import httpx

from app.core.config import settings
from app.questions.variety import chinese_ratio, normalize_stem

# ===== 数据结构 =====


@dataclass(frozen=True)
class Material:
    """一条可用于命题的中文事实素材。"""
    title: str
    snippet: str
    url: str
    provider: str
    license: str = "CC BY-SA 4.0"

    def as_dict(self) -> dict:
        return {
            "title": self.title,
            "url": self.url,
            "provider": self.provider,
            "license": self.license,
        }


@dataclass
class MaterialResult:
    materials: List[Material] = field(default_factory=list)
    status: str = "unavailable"   # ok / no_match / unavailable / disabled
    provider: str = ""
    detail: str = ""
    query: str = ""

    @property
    def ok(self) -> bool:
        return bool(self.materials)

    def as_meta(self) -> dict:
        return {
            "material_status": self.status,
            "material_provider": self.provider,
            "material_query": self.query,
            "materials": [m.as_dict() for m in self.materials],
        }


# ===== 正文清洗与切段（纯函数，可零依赖单测）=====

_TAIL_HEADINGS = ("参考文献", "参考资料", "参见", "注释", "外部链接", "延伸阅读", "参考书目", "相关条目")
_HEADING = re.compile(r"^=+\s*(.*?)\s*=+$")
_NOISE = re.compile(r"[↑^]|ISBN|ISSN|doi:|https?://|\[\d+\]|\(编辑\)|维基共享资源")
_SENTENCE_END = re.compile(r"(?<=[。！？；])")
_PRO_SIGNAL = re.compile(
    r"称为|定义|公式|方程|定律|定理|原理|实验|测量|观测|单位|等于|关系|条件|"
    r"模型|方法|效应|机制|推导|结论|假设|数据|结果表明|由.{0,6}提出|适用于"
)
_NUMBER_SIGNAL = re.compile(r"\d|＝|=|≈|≤|≥|∑|∫|Δ|λ|ν|μ|α|β|θ|%|℃")


def _keywords(knowledge: str) -> List[str]:
    """从知识点里挑出用于相关性打分的词：中文连续串 + 2-gram + 英文术语。"""
    text = str(knowledge or "").strip()
    words: List[str] = []
    for chunk in re.findall(r"[\u4e00-\u9fff]+", text):
        if len(chunk) >= 2:
            words.append(chunk)
        words.extend(chunk[i:i + 2] for i in range(len(chunk) - 1))
    words.extend(w.lower() for w in re.findall(r"[A-Za-z]{3,}", text))
    # 长词权重更高，去重后交给打分函数
    return list(dict.fromkeys(words))


def split_snippets(extract: str, max_chars: int = 420) -> List[str]:
    """把条目正文切成可用于 prompt 的中文段落：去掉尾部参考章节、目录行、引用残片。"""
    lines: List[str] = []
    for raw in str(extract or "").splitlines():
        s = raw.strip()
        if not s:
            continue
        m = _HEADING.match(s)
        if m:
            if any(h in m.group(1) for h in _TAIL_HEADINGS):
                break
            continue  # 普通小标题不作为事实素材
        if len(s) < 24 or _NOISE.search(s):
            continue
        # 超长行按句号再切，避免一次把整章塞进 prompt
        if len(s) > max_chars * 2:
            buf = ""
            for sent in _SENTENCE_END.split(s):
                if not sent.strip():
                    continue
                if len(buf) + len(sent) > max_chars and buf:
                    lines.append(buf.strip())
                    buf = ""
                buf += sent
                if len(buf) >= max_chars:
                    lines.append(buf.strip())
                    buf = ""
            if buf.strip():
                lines.append(buf.strip())
        else:
            lines.append(s[:max_chars * 2])
    # 太短、几乎是英文/表格的段落不适合作为中文命题依据
    return [s for s in lines if 40 <= len(s) and chinese_ratio(s) >= 0.45]


def score_snippet(snippet: str, knowledge: str, keywords: Optional[Sequence[str]] = None) -> int:
    """越高越适合拿来命制专业题：贴知识点、有数值/公式、有学科表述。"""
    kws = list(keywords if keywords is not None else _keywords(knowledge))
    low = snippet.lower()
    score = 0
    score += 4 * sum(1 for k in kws if len(k) >= 3 and k in low)
    score += 1 * sum(1 for k in kws if len(k) == 2 and k in snippet)
    if _NUMBER_SIGNAL.search(snippet):
        score += 2
    if _PRO_SIGNAL.search(snippet):
        score += 2
    if 90 <= len(snippet) <= 420:
        score += 1
    if chinese_ratio(snippet) < 0.6:
        score -= 2
    return score


def select_materials(docs: Sequence[dict], knowledge: str, seed: str,
                     limit: int = 3, max_chars: int = 420) -> List[Material]:
    """从候选条目里按 seed 随机抽取素材段落（同 seed 可复现，不同 seed 换素材）。

    先按相关性打分取候选池，再加随机扰动抽样：既不跑题，又不会因为
    「每次都取 top3」而让同一知识点的题目收敛到同样的三句话。
    """
    rng = random.Random(f"{seed}:zh-material")
    kws = _keywords(knowledge)
    candidates: List[tuple] = []
    for doc in docs:
        snippets = split_snippets(doc.get("extract", ""), max_chars)
        for idx, snip in enumerate(snippets):
            score = score_snippet(snip, knowledge, kws) + rng.random() * 2.5
            candidates.append((score, doc, idx, snip))
    if not candidates:
        return []
    candidates.sort(key=lambda x: x[0], reverse=True)
    pool = candidates[:max(limit * 4, 8)]
    # 尽量让素材来自不同条目，避免一批题全押在同一段话上
    picked: List[tuple] = []
    used_titles: set = set()
    for round_no in range(limit):
        fresh = [c for c in pool if c[1].get("title") not in used_titles and c not in picked]
        choice_pool = fresh or [c for c in pool if c not in picked]
        if not choice_pool:
            break
        item = choice_pool[rng.randrange(len(choice_pool))]
        picked.append(item)
        used_titles.add(item[1].get("title"))
    return [
        Material(
            title=str(doc.get("title") or "中文条目"),
            snippet=snip,
            url=f"{doc.get('view_url', '')}{quote(str(doc.get('title') or ''), safe='')}",
            provider=str(doc.get("provider") or "中文维基百科"),
            license=str(doc.get("license") or "CC BY-SA 4.0"),
        )
        for _, doc, _, snip in picked
    ]


def materials_prompt(materials: Sequence[Material], knowledge: str = "",
                     subject: str = "") -> str:
    """把素材写成模型必须遵守的命题依据块。"""
    if not materials:
        return ""
    lines = [
        "【中文权威资料 · 本次命题的事实依据】",
        f"以下简体中文资料按知识点「{knowledge}」检索自免费开放的中文百科/教科书条目"
        "（CC BY-SA 4.0），请据此命制专业题，而不是凭印象编通用常识题：",
    ]
    for i, m in enumerate(materials, 1):
        lines.append(f"[{i}]《{m.title}》（{m.provider}）：{m.snippet}")
    subject_line = f"学科语境：{subject}。" if subject else "学科语境：按知识点所属学科自行判断。"
    lines.extend([
        "【专业性与语言要求（必须全部满足）】",
        f"- {subject_line}使用该学科的规范中文术语、符号与单位，达到该学科课程考核水平；",
        "- 每题必须至少落实上述一条资料中的具体事实、公式、条件、机制或数据，"
        "并在 JSON 里额外输出 basis 字段，写清依据编号与一句话理由（如 \"依据[1]：爱因斯坦光电方程…\"）；",
        "- 所有输出（题干、选项、解析、考查点、依据）一律使用**简体中文**，禁止整句英文、禁止繁体；"
        "必要的英文缩写/专有名词可括注，但不得替代中文表述；",
        "- 不得编造资料中不存在的常数、定律、实验数据或文献；定量题要给出自洽的已知量与单位；",
        "- 解析必须写出推理依据（公式、条件或机制）并说明主要干扰项错在哪里，不少于 20 字，"
        "禁止只写「答案为 X」；",
        "- 禁止出成「下列说法正确的是」式空壳题、通识常识题或脑筋急转弯。",
    ])
    return "\n".join(lines)


# ===== 检索（带缓存、超时、双源降级）=====

_SOURCES = (
    ("中文维基百科", "zh_wiki_api_url", "zh_wiki_view_url"),
    ("中文维基教科书", "zh_wikibooks_api_url", "zh_wikibooks_view_url"),
)

_doc_cache: dict = {}
_cache_lock = threading.Lock()
_CACHE_MAX = 120


def _cache_get(key: str):
    with _cache_lock:
        item = _doc_cache.get(key)
        if not item:
            return None
        at, docs = item
        if time.monotonic() - at > settings.zh_material_cache_ttl:
            _doc_cache.pop(key, None)
            return None
        return docs


def _cache_put(key: str, docs: list) -> None:
    with _cache_lock:
        if len(_doc_cache) >= _CACHE_MAX:
            _doc_cache.pop(next(iter(_doc_cache)))
        _doc_cache[key] = (time.monotonic(), docs)


def clear_cache() -> None:
    with _cache_lock:
        _doc_cache.clear()


def _docs_from_payload(data: dict, provider: str, view_url: str) -> List[dict]:
    pages = (data or {}).get("query", {}).get("pages", [])
    if isinstance(pages, dict):          # formatversion=1 的兼容
        pages = list(pages.values())
    out = []
    for page in pages or []:
        if not isinstance(page, dict):
            continue
        extract = str(page.get("extract") or "")
        if len(extract) < 80:
            continue
        out.append({
            "title": str(page.get("title") or ""),
            "extract": extract,
            "provider": provider,
            "view_url": view_url,
            "license": "CC BY-SA 4.0",
        })
    return out


async def _fetch_docs(client: httpx.AsyncClient, query: str) -> tuple:
    """按知识点检索条目正文，返回 (条目列表, 失败源数, 成功响应数)。

    区分「检索成功但没有相关条目(no_match)」和「网络全部失败(unavailable)」，
    前端提示才不会把断网说成「没有这个知识点」。
    """
    docs: List[dict] = []
    failures = 0
    responses = 0
    for idx, (provider, api_key, view_key) in enumerate(_SOURCES):
        if idx and not settings.zh_material_wikibooks_enabled:
            continue
        if idx and docs:      # 主源命中就不打第二次外网请求
            continue
        params = {
            "action": "query",
            "format": "json",
            "formatversion": "2",
            "generator": "search",
            "gsrsearch": query,
            "gsrlimit": settings.zh_material_max_docs,
            "gsrnamespace": 0,
            "prop": "extracts",
            "explaintext": "1",
            "exsectionformat": "plain",
            "exchars": settings.zh_material_extract_chars,
            "variant": "zh-cn",      # 关键：直接拿简体中文，不让繁体混进 prompt
            "redirects": "1",
        }
        # TextExtracts 的全文 extract 一次只给一个页面（exlimit>1 时其余页面为空），
        # 所以补一次「多条目简体导言」请求，把素材覆盖面扩到相邻条目，
        # 让同一知识点的不同批次能押中不同的专业侧面。
        intro_params = {k: v for k, v in params.items()
                        if k not in ("exchars", "exsectionformat")}
        intro_params.update({"exintro": "1", "exlimit": "max"})
        seen = {d["title"] for d in docs}
        view_url = getattr(settings, view_key)
        for attempt_params in (params, intro_params):
            try:
                resp = await client.get(
                    getattr(settings, api_key), params=attempt_params,
                    timeout=settings.zh_material_timeout,
                    headers={"User-Agent": "LearningPlatform-AI/1.0 (classroom question generation)"},
                )
                data = resp.json()
                responses += 1
                for doc in _docs_from_payload(data, provider, view_url):
                    if doc["title"] in seen:
                        continue
                    seen.add(doc["title"])
                    docs.append(doc)
            except Exception:
                failures += 1
                continue
    return docs, failures, responses


async def fetch_materials(knowledge: str, subject: str = "", seed: str = "",
                          limit: int = 0, client=None) -> MaterialResult:
    """检索并按 seed 抽样中文专业素材。永不抛异常；失败返回 status != ok。"""
    knowledge = str(knowledge or "").strip()[:120]
    subject = str(subject or "").strip()[:40]
    if not settings.zh_material_enabled:
        return MaterialResult(status="disabled", detail="中文资料检索已关闭")
    if not knowledge:
        return MaterialResult(status="no_match", detail="未填写知识点")

    limit = max(1, min(int(limit or settings.zh_material_max_snippets), 5))
    # 学科不拼进主查询：实测「光电效应 物理学」会召回「量子力学」这类宽泛条目，
    # 反而稀释专业性。学科只用于 prompt 语境，以及查不到时做消歧补查。
    query = knowledge
    key = f"{normalize_stem(query)[:100]}"
    docs = _cache_get(key)
    failures, responses = 0, 0
    if docs is None:
        try:
            if client is not None:
                docs, failures, responses = await _fetch_docs(client, query)
            else:
                async with httpx.AsyncClient(timeout=settings.zh_material_timeout) as c:
                    docs, failures, responses = await _fetch_docs(c, query)
        except Exception as e:
            return MaterialResult(status="unavailable", query=query,
                                  detail=f"检索失败（{type(e).__name__}）")
        # 纯知识点没命中时，加上学科语境做一次消歧补查（如「苹果 植物学」）
        if not docs and subject:
            try:
                broadened = f"{knowledge} {subject}"
                if client is not None:
                    docs, f2, r2 = await _fetch_docs(client, broadened)
                else:
                    async with httpx.AsyncClient(timeout=settings.zh_material_timeout) as c:
                        docs, f2, r2 = await _fetch_docs(c, broadened)
                failures += f2
                responses += r2
            except Exception:
                pass
        if docs:
            _cache_put(key, docs)

    if not docs:
        if failures and not responses:
            return MaterialResult(status="unavailable", query=query,
                                  detail="中文资料源不可达，已回退纯模型出题")
        return MaterialResult(status="no_match", query=query, detail="没有检索到相关中文条目")
    materials = select_materials(docs, knowledge, seed, limit,
                                 settings.zh_material_snippet_chars)
    if not materials:
        return MaterialResult(status="no_match", query=query, detail="条目正文没有可用的中文段落")
    return MaterialResult(
        materials=materials,
        status="ok",
        provider=" / ".join(sorted({m.provider for m in materials})),
        query=query,
    )
