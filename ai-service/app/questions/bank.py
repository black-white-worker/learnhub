"""免费公开题库连接器（可插拔、可降级，主链路不强依赖）。

实测可用（2026-11-15，本机直连均返回 200，无需注册 / API Key）：
  1. Open Trivia Database  https://opentdb.com/api.php            CC BY-SA 4.0，
     `api_token.php?command=request` 的 session token 可避免重复抽到同一道题；
  2. The Trivia API        https://the-trivia-api.com/v2/questions  备用源。

诚实说明：两者都是**英文通识题**，适合课堂热身 / 通识测验，不能替代按知识点
出的专业题（例如「光电效应」）。因此题库只在老师显式选择「免费题库」来源时
直出，或按 QUESTION_BANK_INSPIRE 抽取少量题目作为命题风格样例注入 prompt；
任何网络错误、超时、限流、响应异常都返回空结果，由上层自动回退 AI 出题，
绝不把老师的「生成」按钮拖成一次失败请求。

题库内容按 CC BY-SA 4.0 使用，返回体里保留 source/provider 便于署名。
"""
from __future__ import annotations

import asyncio
import html
import json
import random
import time
from dataclasses import dataclass, field
from typing import List, Optional

import httpx

from app.core.config import settings
from app.questions.variety import parse_llm_json, shuffle_options

_DIFFICULTY = {"简单": "easy", "中等": "medium", "困难": "hard"}

# session token 是进程级缓存：OpenTDB 用它记住已经抽过的题，6 小时有效。
_token_lock = asyncio.Lock()
_session_token: str = ""
_token_at: float = 0.0


@dataclass
class BankResult:
    questions: List[dict] = field(default_factory=list)
    provider: str = ""
    status: str = "ok"            # ok / unavailable / disabled / localized
    localized: bool = False
    detail: str = ""

    @property
    def ok(self) -> bool:
        return bool(self.questions)


def _decode(text) -> str:
    return html.unescape(str(text or "")).strip()


def _difficulty_param(difficulty: str) -> str:
    return _DIFFICULTY.get(str(difficulty or "").strip(), "")


async def _reset_token() -> None:
    global _session_token, _token_at
    async with _token_lock:
        _session_token = ""
        _token_at = 0.0


async def _session_token_for(client: httpx.AsyncClient) -> str:
    """取 OpenTDB session token；拿不到就返回空串（不带 token 也能正常抽题）。"""
    global _session_token, _token_at
    if not settings.question_bank_use_token:
        return ""
    async with _token_lock:
        if _session_token and time.monotonic() - _token_at < 6 * 3600:
            return _session_token
        try:
            resp = await client.get(
                settings.question_bank_token_url,
                params={"command": "request"},
                timeout=settings.question_bank_timeout,
            )
            data = resp.json()
            _session_token = str(data.get("token") or "")
        except Exception:
            _session_token = ""
        _token_at = time.monotonic()
        return _session_token


def _choice_question(stem: str, correct: str, incorrect: List[str], rng: random.Random,
                     provider: str, category: str, score: int) -> Optional[dict]:
    opts = [_decode(correct)] + [_decode(x) for x in incorrect]
    if not _decode(stem) or any(not o for o in opts):
        return None
    q = {
        "stem": _decode(stem),
        "options": opts,
        "answer": "A",  # 正确项先放 A，再交给 shuffle_options 洗牌并重映射答案
        "score": score,
        "explanation": f"公开题库原题，分类：{category or '通识'}；答案：{_decode(correct)}",
        "focus": category or "通识",
        "source": provider,
    }
    return shuffle_options(q, rng, "single", rng.choice("ABCD"))


def _from_opentdb(results: list, rng: random.Random, score: int) -> List[dict]:
    out = []
    for item in results or []:
        if not isinstance(item, dict):
            continue
        stem = _decode(item.get("question"))
        category = _decode(item.get("category"))
        correct = _decode(item.get("correct_answer"))
        if item.get("type") == "boolean":
            out.append({
                "stem": stem,
                "options": [],
                "answer": correct.lower() == "true",
                "score": score,
                "explanation": f"公开题库原题，分类：{category or '通识'}；答案：{'对' if correct.lower() == 'true' else '错'}",
                "focus": category or "通识",
                "source": "opentdb",
            })
        else:
            q = _choice_question(stem, correct, item.get("incorrect_answers") or [], rng,
                                 "opentdb", category, score)
            if q:
                out.append(q)
    return out


def _from_trivia_api(results: list, rng: random.Random, score: int) -> List[dict]:
    out = []
    for item in results or []:
        if not isinstance(item, dict):
            continue
        question = item.get("question") or {}
        stem = question.get("text") if isinstance(question, dict) else question
        q = _choice_question(
            stem,
            item.get("correctAnswer"),
            item.get("incorrectAnswers") or [],
            rng,
            "the-trivia-api",
            _decode(item.get("category")).replace("_", " "),
            score,
        )
        if q:
            out.append(q)
    return out


async def _fetch_with_client(client: httpx.AsyncClient, amount: int, qtype: str,
                             difficulty: str, score: int, category: Optional[int],
                             rng: random.Random) -> BankResult:
    params = {
        "amount": max(1, min(amount, 50)),
        "type": "boolean" if qtype == "judge" else "multiple",
    }
    diff = _difficulty_param(difficulty)
    if diff:
        params["difficulty"] = diff
    if category:
        params["category"] = category

    token = await _session_token_for(client)
    if token:
        params["token"] = token
    try:
        resp = await client.get(settings.question_bank_url, params=params,
                                timeout=settings.question_bank_timeout)
        data = resp.json()
        # 3/4 = token 失效或题库抽尽：重置 token 后不带 token 再试一次
        if data.get("response_code") in (3, 4) and token:
            await _reset_token()
            params.pop("token", None)
            resp = await client.get(settings.question_bank_url, params=params,
                                    timeout=settings.question_bank_timeout)
            data = resp.json()
        if data.get("response_code") == 0:
            questions = _from_opentdb(data.get("results") or [], rng, score)
            if questions:
                return BankResult(questions=questions, provider="opentdb")
    except Exception as e:
        # 记录但不外抛：继续走备用源
        opentdb_error = type(e).__name__
    else:
        opentdb_error = f"response_code={data.get('response_code')}"

    # 备用源只有选择题；判断题模式不混用题型，直接回退 AI
    if qtype == "judge":
        return BankResult(status="unavailable", detail=f"opentdb: {opentdb_error}")

    try:
        resp = await client.get(
            settings.question_bank_fallback_url,
            params={"limit": max(1, min(amount, 50))},
            timeout=settings.question_bank_timeout,
        )
        data = resp.json()
        questions = _from_trivia_api(data if isinstance(data, list) else [], rng, score)
        if questions:
            return BankResult(questions=questions, provider="the-trivia-api")
        return BankResult(status="unavailable", detail="题库返回空结果")
    except Exception as e:
        return BankResult(status="unavailable", detail=f"题库不可用（{type(e).__name__}）")


async def fetch_random_questions(amount: int, qtype: str = "single", difficulty: str = "中等",
                                 score: int = 5, category: Optional[int] = None,
                                 seed: str = "", client=None) -> BankResult:
    """抽一批随机题。永不抛异常：不可用返回 status=unavailable，上层据此回退 AI。"""
    if not settings.question_bank_enabled:
        return BankResult(status="disabled", detail="免费题库已关闭")
    amount = max(1, min(int(amount or 1), settings.question_bank_max_items))
    rng = random.Random(f"{seed}:bank")
    try:
        if client is not None:
            return await _fetch_with_client(client, amount, qtype, difficulty, score, category, rng)
        async with httpx.AsyncClient(timeout=settings.question_bank_timeout) as c:
            return await _fetch_with_client(c, amount, qtype, difficulty, score, category, rng)
    except Exception as e:
        return BankResult(status="unavailable", detail=f"题库不可用（{type(e).__name__}）")


LOCALIZE_PROMPT = """你是双语命题助理。把下面 JSON 数组里的英文题目翻译成通顺、地道的简体中文。

严格要求：
- 只输出一个 JSON 数组，长度、顺序与输入完全一致；
- 每项形如 {"q":"中文题干","opts":["中文选项1", ...]}；opts 数量必须与输入一致（判断题为 []）；
- 保留数字、单位、公式、专有名词（人名/地名/作品名按通行译法），不得增补、删减或改写题意；
- 不要输出答案，不要解释，不要输出 markdown 代码块。

待翻译题目：
"""


async def localize_questions(questions: List[dict], llm, model: str = "") -> bool:
    """把英文题库题批量翻译成中文。失败/长度不符时保留英文原文。

    只让模型翻译文本，不让模型碰答案：正确项索引由代码在洗牌时维护，
    所以翻译绝不可能把答案翻错。
    """
    if not questions or not settings.question_bank_localize:
        return False
    payload = [{"q": q.get("stem", ""), "opts": q.get("options") or []} for q in questions]
    try:
        resp = await llm.chat.completions.create(
            model=model or settings.llm_model,
            messages=[{"role": "user", "content": LOCALIZE_PROMPT + json.dumps(payload, ensure_ascii=False)}],
            temperature=0.2,
        )
        text = (resp.choices[0].message.content or "").strip()
        items = parse_llm_json(text)
        if not items or len(items) != len(questions):
            return False
        translated = []
        for src, item in zip(questions, items):
            q = str(item.get("q") or "").strip()
            opts = item.get("opts") or []
            expected = src.get("options") or []
            if not q or not isinstance(opts, list) or len(opts) != len(expected):
                return False
            if expected and any(not str(o).strip() for o in opts):
                return False
            translated.append((q, [str(o).strip() for o in opts]))
        for src, (q, opts) in zip(questions, translated):
            src["stem"] = q
            if opts:
                src["options"] = opts
            src["localized"] = True
        return True
    except Exception:
        return False


INSPIRE_HEADER = "【公开题库命题风格样例】以下是免费公开题库的英文样例，只用于参考设问方式与干扰项设计；禁止照抄题目内容，禁止输出英文题干："


async def fetch_inspiration(count: int = 2, seed: str = "") -> tuple:
    """给 AI 主链路注入少量「风格样例」，让每次出题的表述模板也不同。

    默认关闭（QUESTION_BANK_INSPIRE=false）：会多一次外网请求，通识英文题对
    专业知识点帮助有限。打开后失败也不影响主链路。
    """
    if not settings.question_bank_enabled or not settings.question_bank_inspire:
        return "", "disabled"
    result = await fetch_random_questions(count, "single", "中等", 5, seed=f"{seed}:inspire")
    if not result.ok:
        return "", result.status
    lines = [INSPIRE_HEADER]
    for q in result.questions[:count]:
        opts = q.get("options") or []
        wrong = [o for i, o in enumerate(opts) if chr(65 + i) != q.get("answer")]
        lines.append(f"- 题干：{q.get('stem')}；干扰项写法：{' / '.join(wrong[:3])}")
    return "\n".join(lines), result.provider
