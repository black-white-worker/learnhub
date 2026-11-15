"""AI 出题的随机性、去重与质量校验（纯函数 + 一个进程内缓存，无外部 IO）。

背景：旧实现每次都用同一句 prompt、固定 temperature=0.7 调一次模型。
对同一个知识点，模型会稳定收敛到同一批「标准题」（例如「光电效应」永远
问截止频率 / 最大初动能），老师连点几次生成拿到的题目几乎一模一样。

这里把「变体」显式建模成四层，全部由请求 seed 驱动（同 seed 可复现，便于测试）：
  1. 命题蓝图：每题随机分配互不重复的考查角度 / 情境 / 认知层级 / 干扰项策略，
     并打散正确答案所在的字母位置；
  2. 采样参数：temperature / top_p 在合理区间内抖动；
  3. 去重：题干指纹 + 字符 bigram Jaccard + SequenceMatcher 的相似度，
     对本批次内、进程内最近 N 条、以及前端回传的上一批题干统一判重；
  4. 后处理：选项洗牌并重映射答案、结构/内容质量校验（不合格交给上层重试）。

另有一组「中文专业题」校验：中文占比 / 英文整句 / 知识点关联 / 解析充分度 /
具体条件，用于保证按知识点出的题是简体中文专业课题目，而不是通识常识题或
英文题库题。题库与 LLM 调用都不在本文件里，保证这些规则可以零依赖单测。
"""
from __future__ import annotations

import hashlib
import json
import random
import re
import secrets
import threading
from collections import defaultdict, deque
from difflib import SequenceMatcher
from typing import Iterable, List, Optional, Sequence

# ===== 随机命题蓝图素材池 =====
# 每条 = (角度名, 对模型的具体要求)。角度池越大，连续两次撞同一套组合的概率越低。
FOCUS_ANGLES: List[tuple] = [
    ("概念辨析", "辨析两个易混概念的定义、适用条件与边界，不要只考名词复述"),
    ("定量计算", "给出具体数值和单位，需要一步以上推算才能作答"),
    ("实验现象", "先描述实验装置、观测现象或测量数据，再让考生推断结论"),
    ("易错陷阱", "围绕学生最常见的错误理解设计干扰项，正确项要经得起推敲"),
    ("情境应用", "把知识点放进真实工程、生活或科研情境中，题干要给出足够背景"),
    ("条件变更", "改变一个关键条件做反事实推理（若……会怎样），不能只改数字"),
    ("图像数据", "用文字描述一张图、一条曲线或一张数据表，据其中的信息作答"),
    ("对比迁移", "与相邻知识点对比，或把结论迁移到一个新对象上"),
    ("因果解释", "要求解释现象背后的机制，而非复述结论"),
    ("过程排序", "给出被打乱的步骤或环节，判断顺序、因果或关键节点"),
]

SCENES = [
    "课堂演示", "实验室测量", "工程现场", "日常生活现象", "科研论文情境",
    "历史经典实验", "数据报表", "生产排障", "真题改编", "跨学科类比",
]

COGNITIONS = ["记忆再认", "理解释义", "应用迁移", "分析推断", "评价判断"]

DISTRACTORS = [
    "混淆相邻概念", "把充分条件当必要条件", "单位或数量级错误", "以偏概全",
    "因果倒置", "本身正确但与所问无关", "漏掉边界条件", "符号或方向写反",
]

# 题干太笼统（「下列说法正确的是」这类空壳）时只给提示，不判不合格，
# 避免误杀合法题目；上层可以据此决定是否重试。
_GENERIC_PREFIX = re.compile(r"^(?:关于|下列|以下|下列说法|下列有关|下列说法中)")
_GENERIC_HINT = re.compile(r"(?:正确|错误|不正确|的是|属于|包括)")

_PUNCT = re.compile(r"[\s\W_]+", re.UNICODE)
_OPTION_PREFIX = re.compile(r"^\s*[A-Da-d]\s*[.、。:：)）]\s*")
_ANSWER_LETTERS = "ABCD"


def new_seed() -> str:
    """生成一次出题的随机种子。前端也可显式传 seed（换一批时换新值）。"""
    return secrets.token_hex(8)


def _spread(rng: random.Random, pool: Sequence, count: int) -> list:
    """从池子里取 count 个，尽量不重复、且相邻两项不撞。count 超池子大小时循环补齐。"""
    out: list = []
    while len(out) < count:
        batch = list(pool)
        rng.shuffle(batch)
        if out and batch and batch[0] == out[-1] and len(batch) > 1:
            batch[0], batch[1] = batch[1], batch[0]
        out.extend(batch)
    return out[:count]


def _answer_slots(rng: random.Random, count: int) -> List[str]:
    """把正确答案位置均匀打散到 A/B/C/D，避免一整批题答案全是 C。"""
    slots: List[str] = []
    letters = list(_ANSWER_LETTERS)
    while len(slots) < count:
        batch = letters[:]
        rng.shuffle(batch)
        if slots and batch and batch[0] == slots[-1] and len(batch) > 1:
            batch[0], batch[1] = batch[1], batch[0]
        slots.extend(batch)
    return slots[:count]


def build_blueprint(count: int, seed: str, qtype: str = "single", difficulty: str = "中等") -> List[dict]:
    """为一批题生成逐题命题约束。同 seed 结果完全一致，不同 seed 高度不同。"""
    rng = random.Random(f"{seed}:blueprint:{qtype}:{difficulty}")
    angles = _spread(rng, FOCUS_ANGLES, count)
    scenes = _spread(rng, SCENES, count)
    cognitions = _spread(rng, COGNITIONS, count)
    distractors = _spread(rng, DISTRACTORS, count)
    slots = _answer_slots(rng, count)
    choice = qtype in ("single", "multiple")
    plan = []
    for i in range(count):
        plan.append({
            "index": i + 1,
            "focus": angles[i][0],
            "focus_hint": angles[i][1],
            "scene": scenes[i],
            "cognition": cognitions[i],
            "distractor": distractors[i],
            "answer_slot": slots[i] if choice else None,
            "variant": f"{i + 1:02d}-{rng.randrange(1000, 9999)}",
        })
    return plan


def blueprint_text(plan: Sequence[dict]) -> str:
    """把蓝图写成模型能直接遵守的中文约束。"""
    lines = ["【本批次随机命题蓝图】每道题必须严格按下表各行的角度出题，"
             "同一批内不得出现设问方式雷同的两道题："]
    for p in plan:
        slot = f"；正确选项安排在 {p['answer_slot']} 位" if p.get("answer_slot") else ""
        lines.append(
            f"- 第{p['index']}题（变体 {p['variant']}）：考查角度「{p['focus']}」（{p['focus_hint']}）；"
            f"情境「{p['scene']}」；认知层级「{p['cognition']}」；"
            f"干扰项主要采用「{p['distractor']}」{slot}。"
        )
    return "\n".join(lines)


def random_sampling(seed: str) -> dict:
    """采样参数抖动。范围贴着 qwen/dashscope 的可用区间，不用会破坏 JSON 的极端值。"""
    rng = random.Random(f"{seed}:sampling")
    return {
        "temperature": round(rng.uniform(0.82, 1.0), 2),
        "top_p": round(rng.uniform(0.88, 0.97), 2),
    }


# ===== 题干相似度 / 去重 =====

def normalize_stem(text) -> str:
    """去掉空白与标点、统一大小写后用于比对（中文按字，不做分词）。"""
    return _PUNCT.sub("", str(text or "")).lower()


def stem_fingerprint(text) -> str:
    return hashlib.sha1(normalize_stem(text).encode("utf-8")).hexdigest()[:16]


def _bigrams(s: str) -> set:
    return {s[i:i + 2] for i in range(len(s) - 1)} or {s}


def similarity(a, b) -> float:
    """0~1 的题干相似度：先用便宜的 bigram Jaccard 粗筛，再算编辑序列相似度。"""
    na, nb = normalize_stem(a), normalize_stem(b)
    if not na or not nb:
        return 0.0
    if na == nb:
        return 1.0
    # 长度差太大基本不可能是同一道题（也避免对长文本做 O(n²) 的 SequenceMatcher）
    if abs(len(na) - len(nb)) / max(len(na), len(nb)) > 0.45:
        return 0.0
    ja, jb = _bigrams(na), _bigrams(nb)
    jac = len(ja & jb) / len(ja | jb)
    if jac < 0.35:
        return jac
    # 一方是另一方的片段（模型把旧题截短/加了个前缀）也算重复
    if na in nb or nb in na:
        return max(0.9, 0.6 + 0.4 * jac)
    seq = SequenceMatcher(None, na, nb).ratio()
    return 0.6 * seq + 0.4 * jac


def is_similar(a, b, threshold: float = 0.82) -> bool:
    return similarity(a, b) >= threshold


def dedupe_questions(questions: Iterable[dict], banned_stems: Iterable = (),
                     threshold: float = 0.82) -> tuple:
    """去掉与禁用列表（上一批 / 历史 / 本批已收题目）相似的题，返回 (kept, dropped)。"""
    seen = [str(s) for s in banned_stems if str(s or "").strip()]
    kept: List[dict] = []
    dropped = 0
    for q in questions:
        stem = str(q.get("stem") or "") if isinstance(q, dict) else ""
        if stem and any(is_similar(stem, s, threshold) for s in seen):
            dropped += 1
            continue
        kept.append(q)
        if stem:
            seen.append(stem)
    return kept, dropped


class RecentStemCache:
    """进程内最近出题记录：按「知识点+题型+难度」隔离，防止连点生成拿到旧题。

    只存题干文本，不存答案；容量有上限，重启即清空。多副本部署时各副本独立，
    但前端每次都会回传上一批题干（exclude_stems），重复仍能被拦住。
    """

    def __init__(self, per_key: int = 60, max_keys: int = 200):
        self.per_key = per_key
        self.max_keys = max_keys
        self._data = defaultdict(lambda: deque(maxlen=per_key))
        self._lock = threading.Lock()

    @staticmethod
    def key_for(knowledge: str, qtype: str, difficulty: str) -> str:
        return f"{normalize_stem(knowledge)[:80]}|{qtype}|{difficulty}"

    def snapshots(self, key: str) -> List[str]:
        with self._lock:
            return list(self._data.get(key, ()))

    def add(self, key: str, stems: Iterable) -> None:
        items = [str(s).strip()[:300] for s in stems if str(s or "").strip()]
        if not items:
            return
        with self._lock:
            if key not in self._data and len(self._data) >= self.max_keys:
                self._data.pop(next(iter(self._data)))
            self._data[key].extend(items)

    def clear(self) -> None:
        with self._lock:
            self._data.clear()


# ===== 结构归一化与解析 =====

def strip_option_prefix(text) -> str:
    return _OPTION_PREFIX.sub("", str(text or "")).strip()


def normalize_question(q: dict) -> dict:
    """把模型偶发的畸形结构归一成 {stem, options, answer, score, explanation}。

    兼容中文键名、options 为逗号/JSON 字符串或对象、answer 写成「C. xxx」连写、
    选项自带「A. 」前缀等；解析字段兼容 解析/explanation/analysis。
    """
    if not isinstance(q, dict):
        return {}
    out = dict(q)
    cn_map = {
        "题干": "stem", "题目": "stem", "问题": "stem", "question": "stem",
        "选项": "options", "choices": "options", "opts": "options",
        "答案": "answer", "correct_answer": "answer", "correctAnswer": "answer",
        "分数": "score", "分值": "score",
        "解析": "explanation", "analysis": "explanation", "说明": "explanation",
        "考点": "focus", "考查点": "focus",
    }
    for cn, en in cn_map.items():
        if cn in out and not out.get(en):
            out[en] = out.pop(cn)

    opts = out.get("options")
    if isinstance(opts, str):
        s = opts.strip()
        if s.startswith("["):
            try:
                parsed = json.loads(s)
                if isinstance(parsed, list):
                    opts = parsed
                elif isinstance(parsed, dict):
                    opts = [parsed[k] for k in sorted(parsed)]
            except Exception:
                opts = None
        if not isinstance(opts, list):
            opts = [o for o in re.split(r"[,，;；\n]+", s) if o.strip()]
    elif isinstance(opts, dict):
        opts = [opts[k] for k in sorted(opts)]
    if isinstance(opts, list):
        out["options"] = [
            strip_option_prefix(o.get("text", o.get("label", o.get("value", ""))) if isinstance(o, dict) else o)
            for o in opts
        ]
    elif opts is not None:
        out["options"] = []

    ans = out.get("answer")
    if isinstance(ans, str) and isinstance(out.get("options"), list) and out["options"]:
        m = re.match(r"^\s*([A-Da-d](?:\s*[,，]\s*[A-Da-d])*)\s*[.、。:：]?\s*.+$", ans.strip())
        if m:
            out["answer"] = m.group(1).replace(" ", "").replace(",", "").replace("，", "").upper()
    return out


def parse_llm_json(text) -> Optional[List[dict]]:
    """从模型输出里抠出 JSON 数组；兼容 markdown 代码块、前后解释文字、尾逗号。"""
    s = str(text or "").strip()
    s = re.sub(r"^```(?:json)?\s*", "", s, flags=re.IGNORECASE)
    s = re.sub(r"\s*```\s*$", "", s)
    start, end = s.find("["), s.rfind("]")
    if start < 0 or end <= start:
        return None
    raw = s[start:end + 1]
    for candidate in (raw, re.sub(r",\s*([}\]])", r"\1", raw)):
        try:
            data = json.loads(candidate)
        except Exception:
            continue
        if isinstance(data, dict):
            data = data.get("questions") or data.get("data")
        if not isinstance(data, list):
            return None
        return [x for x in data if isinstance(x, dict)]
    return None


# ===== 选项洗牌与答案重映射 =====

def shuffle_options(q: dict, rng: random.Random, qtype: str = "single",
                    target_letter: Optional[str] = None) -> dict:
    """打乱选项并同步重写答案字母。

    这是「每次都不一样」最直观的一层：即便模型吐出完全相同的题，
    选项顺序和正确答案位置也会换，且同批次答案位置由蓝图均匀分配。
    answer 本身不合法 / 选项不足时原样返回，绝不制造错题。
    """
    if not isinstance(q, dict) or qtype not in ("single", "multiple"):
        return q
    opts = q.get("options")
    if not isinstance(opts, list) or len(opts) < 2:
        return q
    out = dict(q)
    opts = [strip_option_prefix(o) for o in opts]
    n = len(opts)
    ans = str(out.get("answer") or "").upper()

    if qtype == "single":
        if ans not in _ANSWER_LETTERS[:n]:
            return out
        idx = _ANSWER_LETTERS.index(ans)
        correct = opts[idx]
        others = opts[:idx] + opts[idx + 1:]
        rng.shuffle(others)
        slot = _ANSWER_LETTERS.index(target_letter) if target_letter in _ANSWER_LETTERS[:n] else rng.randrange(n)
        others.insert(slot, correct)
        out["options"] = others
        out["answer"] = _ANSWER_LETTERS[slot]
        return out

    # 多选：把每个正确项映射到洗牌后的新位置，字母重新排序
    letters = sorted({c for c in re.findall(r"[A-D]", ans) if _ANSWER_LETTERS.index(c) < n})
    if len(letters) < 2:
        return out
    order = list(range(n))
    rng.shuffle(order)
    position = {old: new for new, old in enumerate(order)}
    out["options"] = [opts[i] for i in order]
    out["answer"] = "".join(_ANSWER_LETTERS[position[_ANSWER_LETTERS.index(c)]] for c in letters)
    # 洗牌后字母可能乱序，统一升序（平台按 "A,C" 判分）
    out["answer"] = "".join(sorted(out["answer"]))
    return out


# ===== 质量校验 =====

def _judge_bool(value):
    if isinstance(value, bool):
        return value
    s = str(value or "").strip().lower()
    if s in ("对", "正确", "是", "true", "t", "1", "√", "yes", "y"):
        return True
    if s in ("错", "错误", "否", "false", "f", "0", "×", "x", "no", "n"):
        return False
    return None


def _positive_score(value, fallback: int) -> int:
    try:
        n = int(float(value))
        if n > 0:
            return n
    except (TypeError, ValueError):
        pass
    return fallback


def _is_generic_stem(stem: str) -> bool:
    """短、以「关于/下列说法」开头、且只问对错，没有具体信息量。"""
    return (
        len(stem) < 34
        and bool(_GENERIC_PREFIX.match(stem))
        and bool(_GENERIC_HINT.search(stem))
    )


def validate_question(q: dict, qtype: str, score: int = 5) -> dict:
    """服务端题目体检：结构不合格标 invalid（前端不可勾选），内容偏弱只给 warning。"""
    out = dict(q) if isinstance(q, dict) else {}
    stem = str(out.get("stem") or "").strip()
    out["stem"] = stem
    out["score"] = _positive_score(out.get("score"), score)
    out.setdefault("explanation", "")

    # 题干里写死「选 A」的题在选项洗牌后会自相矛盾，只提示、不判废
    if qtype in ("single", "multiple") and _has_letter_reference(stem):
        out["quality_warning"] = "题干引用了选项字母，建议换一批"

    reason = None
    if len(stem) < 8:
        reason = "题干缺失或过短"
    elif qtype in ("single", "multiple"):
        opts = out.get("options")
        if not isinstance(opts, list) or len(opts) != 4:
            reason = "选择题必须恰好 4 个选项"
        elif any(not str(o).strip() for o in opts):
            reason = "存在空选项"
        elif len({str(o).strip() for o in opts}) != 4:
            reason = "选项内容重复"
        elif qtype == "single":
            ans = str(out.get("answer") or "").strip().upper()
            if ans not in _ANSWER_LETTERS:
                reason = "单选答案必须是 A-D 之一"
            else:
                out["answer"] = ans
        else:
            letters = sorted({c for c in re.findall(r"[A-D]", str(out.get("answer") or "").upper())})
            if len(letters) < 2:
                reason = "多选答案至少 2 个选项"
            else:
                out["answer"] = "".join(letters)
    elif qtype == "judge":
        v = _judge_bool(out.get("answer"))
        if v is None:
            reason = "判断题答案必须是 对/错"
        else:
            out["answer"] = v
    else:  # essay / fill 以及未知题型的兜底
        if not str(out.get("answer") or "").strip():
            reason = "缺少参考答案"

    if reason:
        out["invalid"] = True
        out["invalid_reason"] = reason
    else:
        out.pop("invalid", None)
        out.pop("invalid_reason", None)
        if _is_generic_stem(stem):
            out["quality_warning"] = "题干较笼统，建议换一批或补充具体情境"
    return out


def _has_letter_reference(stem: str) -> bool:
    """题干若明确写「选项 A」「应选 C」之类，洗牌后就会与答案冲突。"""
    return bool(re.search(r"(?:选项|应选|答案选|正确答案是?)\s*[A-D](?![A-Za-z])", stem, re.IGNORECASE))


# ===== 中文专业题校验 =====

_CJK = re.compile(r"[\u4e00-\u9fff]")
_ASCII_WORD = re.compile(r"[A-Za-z][A-Za-z'’-]{1,}")
# 只放「繁体专用字」，避免把「为/与/关系/条件」这些简繁同形字误判成繁体
_TRADITIONAL = re.compile(r"[體發現實數驗據關應該與個為認說鍵條頻電壓學變測產係]")
_SPECIFIC_SIGNAL = re.compile(
    r"\d|[=＝≈≤≥<>∑∫Δλμνπθ]|[A-Za-z]{2,}|若|当|當|假设|假設|已知|测得|測得|"
    r"实验|實驗|数据|數據|单位|單位|公式|方程|条件|條件|变化|變化|原因|机制|機制|"
    r"如图|如圖|下表|下列哪|哪个|哪個|推断|推斷"
)
_DEPTH_SIGNAL = re.compile(r"\d|＝|=|≈|≤|≥|∑|∫|[A-Za-z]{2,}|公式|方程|机制|機制|条件|條件|依据|根據|因为|因為|所以|导致|導致")


def chinese_ratio(text) -> float:
    """中文字符占非空白字符的比例。用于识别「英文题库题没汉化成功」。"""
    s = re.sub(r"\s+", "", str(text or ""))
    if not s:
        return 0.0
    return len(_CJK.findall(s)) / len(s)


def _knowledge_terms(knowledge: str) -> List[str]:
    text = str(knowledge or "").strip().lower()
    terms = re.findall(r"[A-Za-z]{3,}", text)
    for chunk in re.findall(r"[\u4e00-\u9fff]{2,}", text):
        terms.append(chunk)
        if len(chunk) >= 4:
            terms.extend(chunk[i:i + 2] for i in range(len(chunk) - 1))
    return list(dict.fromkeys(terms))


def assess_chinese_quality(q: dict, knowledge: str = "", qtype: str = "single",
                           require_basis: bool = False) -> dict:
    """中文专业题体检。硬伤（非中文）判 invalid 交给上层重试，其余只记提示。

    返回 {"invalid", "reason", "warnings", "quality_score"}，不修改入参。
    注意数学/编程题的选项可能纯符号，所以「选项是否中文」只在同时出现英文单词时才判废，
    避免误杀「O(n log n)」这类合法选项。
    """
    q = q if isinstance(q, dict) else {}
    stem = str(q.get("stem") or "").strip()
    options = q.get("options") if isinstance(q.get("options"), list) else []
    explanation = str(q.get("explanation") or q.get("解析") or "").strip()
    warnings: List[str] = []
    reason = ""

    stem_ratio = chinese_ratio(stem)
    if stem and stem_ratio < 0.32:
        reason = "题干不是中文（疑似未汉化的英文题）"
    elif stem and stem_ratio < 0.5 and len(_ASCII_WORD.findall(stem)) >= 8:
        # 计算机 / 物理等学科题干天然带 CPU、RISC-V、MHz、cache 等英文缩写，
        # 只数英文单词会把地道的中文专业题误杀。必须「中文占比也偏低」才判英文题。
        reason = "题干为英文整句"
    elif options and chinese_ratio("".join(str(o) for o in options)) < 0.2 \
            and sum(1 for o in options if _ASCII_WORD.search(str(o))) >= 2:
        # 至少两个英文词组才算「英文题库选项」；O(n log n) 这类公式选项放行
        reason = "选项不是中文（疑似未汉化的英文题）"

    if not reason:
        if _TRADITIONAL.search(stem):
            warnings.append("含繁体字，已建议模型输出简体")
        if not _SPECIFIC_SIGNAL.search(stem):
            warnings.append("题干缺少具体条件或数据")
        if _is_generic_stem(stem):
            warnings.append("题干较笼统，建议换一批")
        if knowledge and not any(t in (stem + " ".join(map(str, options)) + explanation).lower()
                                 for t in _knowledge_terms(knowledge)):
            warnings.append("与知识点关联弱")
        if len(explanation) < 18:
            warnings.append("解析不充分（缺依据或推理）")
        elif not _DEPTH_SIGNAL.search(explanation):
            warnings.append("解析未写出推理依据")
        if require_basis and not str(q.get("basis") or q.get("依据") or "").strip():
            warnings.append("未标注资料依据")

    score = 100 - 12 * len(warnings)
    if reason:
        score = 0
    elif _DEPTH_SIGNAL.search(explanation):
        score = min(100, score + 5)
    return {
        "invalid": bool(reason),
        "reason": reason,
        "warnings": warnings,
        "quality_score": max(0, score),
    }


def apply_chinese_quality(q: dict, knowledge: str = "", qtype: str = "single",
                          require_basis: bool = False) -> dict:
    """把中文专业题体检结果写进题目（保留已有的结构校验结论）。"""
    out = dict(q) if isinstance(q, dict) else {}
    if out.get("invalid"):
        return out
    report = assess_chinese_quality(out, knowledge, qtype, require_basis)
    if report["invalid"]:
        out["invalid"] = True
        out["invalid_reason"] = report["reason"]
        out["quality_score"] = 0
        return out
    out["quality_score"] = report["quality_score"]
    if report["warnings"]:
        out["quality_notes"] = report["warnings"]
        # 前端原本只展示一个 quality_warning，挑最值得提醒的一条
        if not out.get("quality_warning"):
            out["quality_warning"] = report["warnings"][0]
    return out
