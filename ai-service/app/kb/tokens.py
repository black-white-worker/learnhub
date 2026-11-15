"""轻量中英文分词 / 词法打分（零第三方依赖）。

为什么不用 MongoDB 的 $text 索引：MongoDB 自带 text index 按空格/标点切词，
对中文（没有分词边界）几乎不可用；引入 jieba/ES 又会给"本地能跑"增加依赖。
这里用「中文二元组 bigram + 英文/数字词」自己切，纯标准库，索引就是普通数组字段，
$in 查询即可，本地与上线行为一致。

注意：这是词法召回（补向量召回的漏），不是语言学意义上的中文分词，
面试时如实说明取舍即可。
"""
from __future__ import annotations
import re

# 高频虚词/疑问词：不参与召回，否则"的/是/什么"会把所有切片都命中
_STOPWORDS = {
    "的", "了", "是", "在", "和", "有", "就", "不", "都", "一", "上", "也", "很",
    "到", "说", "要", "去", "会", "着", "看", "好", "这", "那", "么", "吗", "呢",
    "吧", "啊", "请", "帮", "我", "们", "想", "问", "把", "被", "让", "给", "从",
    "对", "与", "并", "等", "个", "中", "为", "以", "及", "或", "你", "他", "它",
    "什么", "怎么", "如何", "请问", "一下", "自己", "这个", "那个", "哪些",
    "为什么", "可以", "关于", "对于", "以及", "没有", "我们",
}

_WORD_RE = re.compile(r"[a-z0-9_]+")
_CN_RUN_RE = re.compile(r"[\u4e00-\u9fff]+")
_MAX_TOKENS = 120  # 防止超长文本生成海量 token，撑爆 $in 查询


def _is_cn(token: str) -> bool:
    return all("\u4e00" <= c <= "\u9fff" for c in token)


def tokenize(text: str, max_tokens: int = _MAX_TOKENS) -> list[str]:
    """把文本切成可用于词法召回的 token 集合（已排序、去重）。

    - 英文/数字：整词保留（长度 1 的字母丢弃，数字保留）
    - 中文连续片段：长度 >= 2 时取相邻二元组 bigram；长度为 1 时取单字
    - 停用词过滤、统一小写、截断上限
    """
    if not text:
        return []
    s = text.lower()
    tokens: set[str] = set()

    for w in _WORD_RE.findall(s):
        if len(w) >= 2 or w.isdigit():
            if w not in _STOPWORDS:
                tokens.add(w)

    for run in _CN_RUN_RE.findall(s):
        if len(run) == 1:
            if run not in _STOPWORDS:
                tokens.add(run)
            continue
        for i in range(len(run) - 1):
            bigram = run[i:i + 2]
            if bigram in _STOPWORDS:
                continue
            # 只丢掉「完全由虚词组成」的二元组（如「的了」）；
            # 像「中未」「的课」这类含实词的组合必须保留，否则中文召回会被掏空。
            if all(c in _STOPWORDS for c in bigram):
                continue
            tokens.add(bigram)

    return sorted(tokens)[:max_tokens]


def token_weight(token: str) -> float:
    """中文二元组比单字/英文词更具区分度，打分时加权。"""
    return 2.0 if len(token) >= 2 and _is_cn(token) else 1.0


def lexical_score(query_tokens: list[str], chunk_tokens: list[str]) -> float:
    """词法命中得分：命中 token 的权重之和，归一化到 [0,1] 便于和向量分数并列展示。"""
    if not query_tokens or not chunk_tokens:
        return 0.0
    q = set(query_tokens)
    hit = q.intersection(set(chunk_tokens))
    if not hit:
        return 0.0
    weight = sum(token_weight(t) for t in hit)
    total = sum(token_weight(t) for t in q)
    return min(1.0, weight / total) if total else 0.0
