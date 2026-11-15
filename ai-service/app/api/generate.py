"""AI 出题 API（简体中文专业题 / 随机性 / 质量 / 可选通识题库版）。

老师输入知识点 → 检索中文权威资料 → 生成题目+答案+解析 → 老师审核勾选后入库。
支持题型：single（单选）、multiple（多选）、judge（判断）、fill（填空）、essay（简答）。

默认链路是**按知识点出的简体中文专业题**：先按知识点（可加学科）检索免费开放的
中文维基百科/维基教科书条目，把随机抽取的事实素材注入 prompt，要求模型据此命制
带依据、带推导解析的专业题；再做中文占比 / 专业度 / 结构校验，英文题或空壳题
判废并换蓝图补齐。资料检索失败时自动退回「纯模型中文专业出题」。

source=bank 是上一版接入的英文通识题库（Open Trivia DB / The Trivia API，
汉化后直出），只适合课堂热身，不按知识点出题；老师不显式选择就不会走这条链路。

修复「每次出题都一样」的四层机制（详见 app/questions/variety.py）：
  1. 每次请求随机 seed + 逐题随机命题蓝图（考查角度/情境/认知层级/干扰项/答案位置）；
  2. temperature / top_p 抖动 + prompt 随机编号，打破模型对固定 prompt 的收敛；
  3. 跨请求去重：进程内最近题干 + 前端回传的 exclude_stems + 本批次内相似度判重；
  4. 选项洗牌与答案重映射、质量校验、不够数量时换蓝图自动重试补齐。

另可切换 source=bank 走免费公开题库（Open Trivia DB / The Trivia API，
无需密钥），抽题 → 中文汉化 → 同一套校验；题库不可用时自动回退 AI 出题。

P0 约束沿用：只有 teacher/admin 能调；服务端做 schema 校验，废题不给入库。
"""
from __future__ import annotations

import asyncio
import contextlib
import json
import random as _random
import time

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field
from typing import List, Optional
from openai import AsyncOpenAI

from app.core.config import settings
from app.questions import bank as question_bank
from app.questions import zhsource
from app.api.jobstore import job_store, Job
from app.questions.variety import (
    RecentStemCache,
    apply_chinese_quality,
    blueprint_text,
    build_blueprint,
    dedupe_questions,
    new_seed,
    normalize_question,
    parse_llm_json,
    random_sampling,
    shuffle_options,
    validate_question,
)

router = APIRouter(prefix="/api/ai", tags=["generate"])

# 进程内最近出题记录（多副本各自独立，前端 exclude_stems 做跨副本兜底）
_recent = RecentStemCache(per_key=settings.generate_recent_window)

MAX_KNOWLEDGE_CHARS = 200
MAX_EXCLUDE_STEMS = 20


def get_user_id(req: Request) -> int:
    uid = req.headers.get("X-User-Id")
    if not uid:
        raise HTTPException(401, "missing X-User-Id")
    try:
        return int(uid)
    except ValueError:
        raise HTTPException(400, "invalid X-User-Id")


def require_teacher(req: Request):
    """校验当前用户是教师或管理员，学生不能出题。"""
    role = req.headers.get("X-Role", "")
    if role not in ("teacher", "admin"):
        raise HTTPException(403, "只有教师或管理员可以使用 AI 出题")


class GenQuestionReq(BaseModel):
    knowledge: str = ""
    difficulty: str = "中等"
    qtype: str = "single"
    count: int = 2
    score: int = 5
    # 下面都是可选：老前端不传也完全兼容
    seed: Optional[str] = None                    # 前端每次「换一批」生成新值
    exclude_stems: List[str] = Field(default_factory=list)  # 上一批题干，服务端判重
    source: str = "ai"                           # ai=中文专业题 / bank=英文通识题库（热身）
    bank_category: Optional[int] = None          # 题库分类 id（OpenTDB category）
    subject: str = ""                            # 学科语境，如「物理学」；空=按知识点自动判断
    grounding: bool = True                       # 是否检索中文权威资料作为命题依据
    job_id: Optional[str] = None                 # 客户端生成的作业号，用于刷新后断点续传


TYPE_PROMPTS = {
    "single": '生成{count}道单选题，每题4个选项(A/B/C/D)，标注正确答案。输出JSON数组：[{{"stem":"题干","options":["选项A","选项B","选项C","选项D"],"answer":"A","score":{score}}}]',
    "multiple": '生成{count}道多选题，每题4个选项，答案可能有2-4个。输出JSON数组：[{{"stem":"题干","options":["选项A","选项B","选项C","选项D"],"answer":"ABD","score":{score}}}]',
    "judge": '生成{count}道判断题，答案只有true或false。输出JSON数组：[{{"stem":"题干","answer":true,"score":{score}}}]',
    "fill": '生成{count}道填空题，答案是简短关键词。输出JSON数组：[{{"stem":"题干（用____表示空）","answer":"参考答案","score":{score}}}]',
    "essay": '生成{count}道简答题，给参考答案要点。输出JSON数组：[{{"stem":"题干","answer":"参考答案要点","score":{score}}}]',
}

QUALITY_RULES = """【通用质量要求 · 简体中文专业题】
- 这是**学科专业课**的题目，不是通识常识测验：必须考该知识点的定义、公式、条件、机制、
  实验现象、数据推断或典型易错点，达到该学科课程考核的难度与术语规范；
- 全部输出使用简体中文（题干、选项、解析、考查点、依据），禁止整句英文与繁体；
  必要的英文缩写/符号可括注，但不得用英文替代中文表述；
- 每题一个对象，键名统一用英文：stem（题干）、options（选项数组）、answer（答案）、score（分值）、
  explanation（解析）、focus（考查点）、basis（命题依据，见下方资料要求）；
- stem 必须给出具体信息（条件、数值、单位、现象、公式或机制），禁止「关于X，下列说法正确的是」
  这类没有信息量的空壳题干，也禁止把通用常识包装成专业题；
- 选择题恰好 4 个选项，干扰项必须来自该知识点的真实易错理解（概念混淆、条件遗漏、
  单位或数量级错误、因果倒置等），不得出现「以上都对/以上都不对」，不得有两个同义选项；
- answer 只写选项字母（单选如 "C"，多选如 "ABD"），判断题写 true/false，禁止写成 "C. xxx" 连写；
- explanation 不少于 20 字，要写出推理依据（所用公式/条件/机制）并说明主要干扰项错在哪，
  不要写「答案为 C」这类字母引用（服务端会重排选项）；
- focus 写本题考查的具体考点，4-12 字；score 统一写 {score}；
- 数字、单位、条件必须自洽，不要编造不存在的定律、常数或实验数据；
- 各题正确选项的字母位置要分散，不要全部落在同一个字母。"""


def build_generation_prompt(knowledge: str, difficulty: str, qtype: str, count: int,
                            score: int, plan: list, banned: list, nonce: str,
                            inspiration: str = "", subject: str = "",
                            materials_block: str = "", retry_hint: str = "") -> str:
    """组装 prompt。随机编号 + 蓝图 + 中文素材 + 禁用清单是每次都不一样的四个来源。

    retry_hint 把上一轮被判废的具体原因回灌给模型，重试才不是「换个种子碰运气」。
    """
    subject_line = f"学科语境为「{subject}」，请使用该学科的规范术语、符号与惯例。" if subject else ""
    parts = [
        f"你是该学科的资深命题专家。请围绕知识点【{knowledge}】命制 {count} 道"
        f"难度为「{difficulty}」的简体中文专业题。{subject_line}",
        f"本次命题随机编号：{nonce}。这是全新的一次命题，禁止照搬该知识点最常见的默认例题，"
        "也不要把上一批题目换个说法再写一遍。",
        blueprint_text(plan),
    ]
    if materials_block:
        parts.append(materials_block)
    else:
        # 无外部资料时，别让模型把「依据」编造成一本不存在的教材或论文
        parts.append(
            "【无外部资料时的依据要求】本次未检索中文资料，basis 字段只写本题所依据的"
            "定律 / 公式 / 原理 / 公认史实名称，不要杜撰书名、论文、文献出处或实验数据。"
        )
    parts.extend([
        QUALITY_RULES.format(score=score),
        TYPE_PROMPTS[qtype].format(count=count, score=score),
    ])
    if inspiration:
        parts.append(inspiration)
    if banned:
        lines = "\n".join(f"  {i + 1}. {str(s).strip()[:100]}" for i, s in enumerate(banned[:12]))
        parts.append("【禁止重复】这些题目本系统近期已经出过，必须换考查角度、数据或情境，不得改写后重复出现：\n" + lines)
    if retry_hint:
        parts.append(
            "【上一轮未通过校验，本轮必须修正】服务端把上一批题目判废，原因如下：\n"
            f"  {retry_hint}\n"
            "请严格按上面的字段、选项数量、答案格式与简体中文要求重新命制，"
            "不要重复上一轮的写法。"
        )
    parts.append("只输出 JSON 数组，不要任何解释文字，不要 markdown 代码块。")
    return "\n\n".join(parts)


def validate_questions(questions: list, qtype: str, score: int = 5) -> list:
    """兼容旧调用名的批量校验入口。"""
    return [validate_question(q, qtype, score) for q in questions]


async def _call_llm(llm, prompt: str, sampling: dict, max_tokens: int = 0) -> str:
    kwargs = dict(
        model=settings.llm_model,
        messages=[{"role": "user", "content": prompt}],
        temperature=sampling["temperature"],
        top_p=sampling["top_p"],
    )
    # 给一个合理上限，避免模型把解析写得过长、拖慢整次出题
    if max_tokens and max_tokens > 0:
        kwargs["max_tokens"] = max_tokens
    resp = await llm.chat.completions.create(**kwargs)
    return (resp.choices[0].message.content or "").strip()


def _postprocess(questions: list, qtype: str, score: int, plan: list, seed: str,
                 attempt: int, knowledge: str = "", require_basis: bool = False,
                 enforce_chinese: bool = True) -> tuple:
    """归一化 → 结构校验 → 中文专业度校验 → 选项洗牌。返回 (合格题, 废题)。"""
    rng = _random.Random(f"{seed}:shuffle:{attempt}")
    good, bad = [], []
    for i, raw in enumerate(questions[:max(len(plan), 10)]):
        q = validate_question(normalize_question(raw), qtype, score)
        if not q.get("invalid") and enforce_chinese and settings.generate_require_chinese:
            q = apply_chinese_quality(q, knowledge, qtype, require_basis)
        if q.get("invalid"):
            bad.append(q)
            continue
        slot = plan[i]["answer_slot"] if i < len(plan) else None
        good.append(shuffle_options(q, rng, qtype, slot))
    return good, bad


def _make_llm():
    return AsyncOpenAI(
        base_url=settings.llm_base_url,
        api_key=settings.llm_api_key,
        timeout=settings.llm_timeout,
    )


@router.post("/generate")
async def gen_question(body: GenQuestionReq, req: Request):
    """AI 出题入口。

    同一个接口两种返回形态，由 Accept 决定：
      - 普通 JSON（默认，旧前端 / 脚本调用完全不变）：处理完一次性返回题目；
      - Accept: text/event-stream：先逐条推送真实阶段进度，最后一条 result 带完整题目。
    中文专业出题要检索资料 + 调模型 + 可能重试，实测 25~90 秒；进度流让老师
    看到「现在卡在哪一步」，而不是对着一个转圈遮罩猜是不是死了。
    """
    get_user_id(req)
    require_teacher(req)
    _check_params(body)
    if "text/event-stream" in (req.headers.get("accept") or "").lower():
        return await _progress_stream(body, req)
    return await _generate(body, req)


def _check_params(body: GenQuestionReq):
    if body.qtype not in TYPE_PROMPTS:
        raise HTTPException(400, f"不支持的题型: {body.qtype}")
    if body.count < 1 or body.count > 10:
        raise HTTPException(400, "count 必须在 1-10 之间")


async def _generate(body: GenQuestionReq, req: Request, emit=None) -> dict:
    """出题主流程。emit 是可选 async 回调，SSE 模式下注入阶段事件。"""
    qtype = body.qtype
    score = body.score if body.score and body.score > 0 else 5
    difficulty = body.difficulty if body.difficulty in ("简单", "中等", "困难") else "中等"
    knowledge = str(body.knowledge or "").strip()[:MAX_KNOWLEDGE_CHARS]
    source = body.source if body.source in ("ai", "bank") else "ai"
    seed = (body.seed or "").strip()[:64] or new_seed()
    exclude = [str(s or "").strip()[:300] for s in body.exclude_stems[:MAX_EXCLUDE_STEMS] if str(s or "").strip()]
    # 学科只作 prompt 语境与检索限定词，压成单行、限长，避免被塞进指令注入
    subject = " ".join(str(body.subject or "").split())[:24]
    grounding = bool(body.grounding) and source == "ai"

    async def note(stage: str, status: str = "running", detail: str = "",
                   label: str = "", **extra):
        """发一条阶段进度。emit=None（普通 JSON 调用）时零开销直接跳过。"""
        if emit is None:
            return
        payload = {"stage": stage, "status": status, "detail": detail, **extra}
        if label:
            payload["label"] = label
        await emit(payload)

    type_label = {"single": "单选题", "multiple": "多选题", "judge": "判断题",
                  "fill": "填空题", "essay": "简答题"}.get(qtype, qtype)
    await note("prepare", "running", f"{body.count} 道{type_label} · {difficulty}")
    await note("prepare", "done", f"已分配 {body.count} 组互不重复的考查角度与答案位置")

    cache_key = RecentStemCache.key_for(knowledge, qtype, difficulty)
    banned = _recent.snapshots(cache_key) + exclude
    llm = _make_llm()

    meta = {
        "seed": seed,
        "source": source,
        "strategy": [],
        "attempts": 0,
        "deduped": 0,
        "excluded": len(banned),
        "bank_status": "",
        "bank_provider": "",
        "localized": False,
        "subject": subject,
        "grounding": grounding,
        "material_status": "disabled" if not grounding else "",
        "material_provider": "",
        "materials": [],
        "job_id": body.job_id or "",
    }
    # 一次让模型多生成几道（留余量），优先一轮凑齐，避免动辄进入第二轮回填；
    # 题库模式仍是按 count 抽，不参与超生成。
    gen_count = body.count
    if source == "ai":
        gen_count = min(10, body.count + settings.generate_overgen_slack)

    # ===== 模式一：免费公开题库 =====
    # 题库只有单选 / 判断两种，所以 multiple 自动按单选处理，返回 qtype 由前端接管。
    if source == "bank":
        bank_qtype = "judge" if qtype == "judge" else "single"
        await note("materials", "skipped", "通识题库模式不检索中文资料")
        await note("compose", "running", "正在从免费公开题库抽随机题", label="抽取通识题并汉化")
        result = await question_bank.fetch_random_questions(
            body.count, bank_qtype, difficulty, score, body.bank_category, seed
        )
        meta["bank_status"] = result.status
        meta["bank_provider"] = result.provider
        if result.ok:
            await note("compose", "running", f"抽到 {len(result.questions)} 题，正在译成中文")
            meta["localized"] = await question_bank.localize_questions(result.questions, llm)
            await note("compose", "done", "汉化完成" if meta["localized"] else "汉化不可用，展示英文原文")
            # 通识题库是英文原题 + 汉化，不走「中文专业题」硬校验（汉化失败只提示）
            prepared, bad = _postprocess(
                result.questions, bank_qtype, score,
                build_blueprint(len(result.questions), seed, bank_qtype, difficulty), seed, 0,
                enforce_chinese=False,
            )
            prepared, dropped = dedupe_questions(prepared, banned, settings.generate_similarity_threshold)
            meta["deduped"] = dropped
            meta["strategy"] = [q.get("focus") or "通识" for q in prepared]
            _recent.add(cache_key, [q.get("stem", "") for q in prepared])
            # 题库抽题也可能因排重不足请求数：补抽直到凑齐或达到上限
            bank_tries = 0
            while len(prepared) < body.count and bank_tries < 3:
                bank_tries += 1
                extra = await question_bank.fetch_random_questions(
                    body.count, bank_qtype, difficulty, score, body.bank_category,
                    f"{seed}-b{bank_tries}",
                )
                if not extra.ok:
                    break
                more, _ = _postprocess(
                    extra.questions, bank_qtype, score,
                    build_blueprint(len(extra.questions), f"{seed}-b{bank_tries}", bank_qtype, difficulty),
                    seed, 0, enforce_chinese=False,
                )
                more, extra_dropped = dedupe_questions(
                    more, banned + [q.get("stem", "") for q in prepared],
                    settings.generate_similarity_threshold,
                )
                prepared.extend(more)
                meta["deduped"] += extra_dropped
            shown = prepared[:body.count]
            await note("verify", "done", f"合格 {len(shown)} 题 · 判废 {len(bad)} · 排重 {dropped}")
            await note("done", "done", f"共 {len(shown)} 题", count=len(shown), attempts=1)
            return {
                "questions": shown,
                "qtype": bank_qtype,
                "count": len(shown),
                "invalid_count": sum(1 for q in shown if q.get("invalid")),
                "discarded_count": len(bad),
                "meta": meta,
            }
        # 题库不可用 → 下面自动回退 AI 出题，并在 meta 里如实说明
        meta["source"] = "ai"
        source = "ai"  # 局部变量同步改掉，否则下面的「严格补全」会被误跳过
        meta["bank_status"] = f"fallback: {result.status} {result.detail}".strip()
        await note("compose", "running", "题库不可用，已回退按知识点出中文专业题",
                   label="命制专业题")
        if not knowledge:
            knowledge = "通识常识与科学基础"

    # ===== 模式二：AI 按知识点出题（默认，也是题库失败后的兜底）=====
    if not knowledge:
        raise HTTPException(400, "请填写知识点")

    inspiration = ""
    if settings.question_bank_enabled and settings.question_bank_inspire:
        inspiration, provider = await question_bank.fetch_inspiration(2, seed)
        meta["bank_provider"] = provider or meta["bank_provider"]

    collected: list = []
    invalid_all: list = []
    dropped_total = 0
    last_plan: list = []
    last_sampling: dict = {}
    attempts = 0
    retry_hint = ""

    for attempt in range(1, max(1, settings.generate_max_attempts) + 1):
        attempts = attempt
        attempt_seed = f"{seed}-{attempt}"
        plan = build_blueprint(gen_count, attempt_seed, qtype, difficulty)
        sampling = random_sampling(attempt_seed)
        last_plan, last_sampling = plan, sampling
        max_attempts = max(1, settings.generate_max_attempts)

        # 按知识点检索中文权威资料，并按本轮 seed 换一批素材段落。
        # 条目正文有缓存，重试轮次不再打外网；失败/无匹配时退回纯模型中文专业出题。
        materials_block = ""
        if grounding:
            if attempt == 1:
                await note("materials", "running", "正在检索中文维基百科 / 维基教科书")
            material = await zhsource.fetch_materials(knowledge, subject, attempt_seed)
            if material.ok:
                materials_block = zhsource.materials_prompt(material.materials, knowledge, subject)
                meta.update(material.as_meta())
                if attempt == 1:
                    titles = "、".join(m.title for m in material.materials[:3])
                    await note("materials", "done", f"命中 {len(material.materials)} 段素材：{titles}")
            elif not meta.get("materials"):
                meta.update(material.as_meta())
                if attempt == 1:
                    await note("materials", "skipped", "资料源不可达或未匹配，改用模型的学科知识出题")
        elif attempt == 1:
            await note("materials", "skipped", "未开启「引用中文资料」")

        angles = "、".join(p["focus"] for p in plan[:3])
        await note("compose", "running",
                   f"第 {attempt}/{max_attempts} 轮生成中 · 考查角度 {angles}")
        prompt = build_generation_prompt(
            knowledge, difficulty, qtype, gen_count, score, plan,
            banned + [q.get("stem", "") for q in collected], f"{seed}/{attempt}",
            inspiration, subject, materials_block, retry_hint,
        )
        try:
            text = await _call_llm(llm, prompt, sampling, max_tokens=max(900, gen_count * 360))
        except Exception as e:
            if collected:
                break  # 已经够用就别因为第二次调用失败丢掉结果
            raise HTTPException(503, f"AI 出题服务暂不可用（{type(e).__name__}）")

        await note("compose", "running", f"第 {attempt} 轮模型已返回，正在校验结构与中文专业度")
        parsed = parse_llm_json(text)
        if parsed is None:
            # 模型没按格式返回：保留原文交给前端提示「建议重新生成」
            if not collected and attempt >= max_attempts:
                await note("done", "done", "模型未按题目格式返回，已展示原文", count=1, attempts=attempt)
                return {"questions": [{"stem": text, "answer": "", "score": score}],
                        "raw": True, "meta": meta}
            continue

        good, bad = _postprocess(
            parsed, qtype, score, plan, seed, attempt,
            knowledge=knowledge, require_basis=bool(materials_block),
        )
        invalid_all.extend(bad)
        # 把废题原因回灌到下一轮 prompt：重试要针对问题改，而不是只换随机种子。
        reasons = [str(q.get("invalid_reason") or "").strip() for q in bad]
        retry_hint = "；".join(dict.fromkeys(r for r in reasons if r))[:300]
        good, dropped = dedupe_questions(
            good, banned + [q.get("stem", "") for q in collected],
            settings.generate_similarity_threshold,
        )
        dropped_total += dropped
        collected.extend(good)
        await note("verify", "done",
                   f"合格 {len(good)} · 判废 {len(bad)} · 排重 {dropped} · 已凑 {len(collected)}/{body.count}")
        if len(collected) >= body.count:
            break

    # ===== 严格补全（修复「出题数量 < 请求数量」）=====
    # 去重 / 判废可能把多生成的余量吃光，导致最终题数少于请求数。这里按缺口继续
    # 补生成，直到凑齐 body.count 或达到补生成上限，保证「请求的题数 = 实际返回的题数」。
    # 每轮用新 seed（蓝图 / 采样 / 素材全换）并把已收题 + 历史禁用题干一起拦，避免补出重复题。
    MAX_FILL_ATTEMPTS = 4
    fill_attempts = 0
    while source != "bank" and len(collected) < body.count and fill_attempts < MAX_FILL_ATTEMPTS:
        fill_attempts += 1
        remaining = body.count - len(collected)
        # 补生成也留余量：优先一轮补齐缺口
        fill_count = min(10, remaining + settings.generate_overgen_slack)
        fill_seed = f"{seed}-fill-{fill_attempts}"
        plan = build_blueprint(fill_count, fill_seed, qtype, difficulty)
        sampling = random_sampling(fill_seed)
        fill_banned = banned + [q.get("stem", "") for q in collected]
        materials_block = ""
        if grounding:
            material = await zhsource.fetch_materials(knowledge, subject, fill_seed)
            if material.ok:
                materials_block = zhsource.materials_prompt(material.materials, knowledge, subject)
            meta.update(material.as_meta())
        angles = "、".join(p["focus"] for p in plan[:3])
        await note("compose", "running",
                   f"补生成第 {fill_attempts} 轮 · 缺口 {remaining} 题 · 考查角度 {angles}")
        prompt = build_generation_prompt(
            knowledge, difficulty, qtype, fill_count, score, plan,
            fill_banned, f"{seed}/fill-{fill_attempts}", inspiration, subject,
            materials_block, retry_hint,
        )
        try:
            text = await _call_llm(llm, prompt, sampling, max_tokens=max(900, fill_count * 360))
        except Exception:
            break  # 补生成调用失败就别硬凑，以现有题数收尾，不卡死
        parsed = parse_llm_json(text)
        if parsed is None:
            continue
        good, bad = _postprocess(
            parsed, qtype, score, plan, seed, fill_attempts,
            knowledge=knowledge, require_basis=bool(materials_block),
        )
        invalid_all.extend(bad)
        reasons = [str(q.get("invalid_reason") or "").strip() for q in bad]
        retry_hint = "；".join(dict.fromkeys(r for r in reasons if r))[:300]
        good, dropped = dedupe_questions(good, fill_banned, settings.generate_similarity_threshold)
        dropped_total += dropped
        collected.extend(good)
        await note("verify", "done",
                   f"补生成第 {fill_attempts} 轮 · 合格 {len(good)} · 已凑 {len(collected)}/{body.count}")

    questions = collected[:body.count]
    # 两轮都拿不到合格题时，把废题原样带回去（前端会置灰），但必须明确告知不可入库，
    # 不能让老师面对一堆灰卡片却不知道发生了什么。
    fallback_used = False
    if not questions and invalid_all:
        questions = invalid_all[:body.count]
        fallback_used = True

    _recent.add(cache_key, [q.get("stem", "") for q in questions])
    # invalid_count 只统计「最终展示给老师的题里」不合格的数量（前端据此置灰）；
    # 重试过程中被换掉的废题单独记 discarded_count，避免明明补齐了还弹「有 N 道题不合格」。
    invalid_shown = sum(1 for q in questions if q.get("invalid"))
    meta.update({
        "attempts": attempts + fill_attempts,
        "fill_attempts": fill_attempts,
        "deduped": dropped_total,
        "discarded_count": len(invalid_all),
        "strategy": [p["focus"] for p in last_plan][:len(questions)],
        "temperature": last_sampling.get("temperature"),
        "top_p": last_sampling.get("top_p"),
    })
    if fallback_used:
        meta["hint"] = ("本批题目未通过质量校验，已原样展示但不可勾选入库；"
                        "建议点「换一批」，或把知识点写得更具体（如「爱因斯坦光电方程与截止频率」）")
    elif not questions:
        # 去重把整批都判掉时，别让老师面对一片空白却不知道发生了什么
        meta["hint"] = "模型连续返回了重复或不合格的题目，建议再点一次「换一批」，或把知识点写得更具体"
    elif len(questions) < body.count:
        # 补生成仍凑不齐（极少见）：如实告知实际题数，不谎报已凑齐
        meta["hint"] = (f"本次未能凑齐请求的 {body.count} 题，已返回实际得到的 {len(questions)} 题；"
                        "建议点「换一批」再试一次")
    await note("done", "done", f"共 {len(questions)} 题 · {attempts + fill_attempts} 轮命制",
               count=len(questions), attempts=attempts + fill_attempts, discarded=len(invalid_all))
    return {
        "questions": questions,
        "qtype": qtype,
        "count": len(questions),
        "invalid_count": invalid_shown,
        "discarded_count": len(invalid_all),
        "meta": meta,
    }


def _sse_event(event: str, data: dict) -> str:
    """JSON 里不会有裸换行（json.dumps 会转义），所以单条 data: 一行足够。"""
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"


def new_job_id() -> str:
    """服务端兜底生成作业号；真实场景由前端用 crypto.randomUUID 生成并带回。"""
    import uuid
    return uuid.uuid4().hex


async def _progress_stream(body: GenQuestionReq, req: Request) -> StreamingResponse:
    """按 job_id 复用 / 新建可恢复作业，把阶段事件流转成 SSE 流。

    - 同一 job_id 第二次进来（刷新后）不会重跑出题，而是 attach 到正在跑的作业，
      先回放历史进度、再续收实时事件；作业已结束则直接返回缓存结果；
    - 客户端断开只移除订阅，不取消后台任务，所以刷新不会丢失进度、也不会多烧一次额度。
    """
    job_id = (body.job_id or "").strip()[:64] or new_job_id()

    async def run(job: Job):
        return await _generate(body, req, job.emit_progress)

    job = await job_store.get_or_create(job_id, run)

    return StreamingResponse(_stream_job(job), media_type="text/event-stream", headers={
        "Cache-Control": "no-cache",
        "Connection": "keep-alive",
        "X-Accel-Buffering": "no",
        "X-Job-Id": job_id,
    })


async def _stream_job(job: Job):
    """把一个作业的事件流推给当前这个 SSE 连接。

    断点续传核心：连上后先把历史进度回放一遍，再续收实时事件；作业已结束
    则直接把缓存的结果推出去，客户端立刻拿到题目。
    """
    yield ": connected\n\n"
    myq = job.subscribe()
    try:
        for item in list(job.history):
            yield _sse_event(item["event"], item["data"])
        if job.status == "done" and job.result is not None:
            yield _sse_event("result", job.result)
            return
        if job.status == "error" and job.error is not None:
            yield _sse_event("error", {"message": job.error})
            return
        while True:
            try:
                item = await asyncio.wait_for(myq.get(), timeout=5)
            except asyncio.TimeoutError:
                yield ": keep-alive\n\n"
                continue
            yield _sse_event(item["event"], item["data"])
            if item["event"] in ("result", "error"):
                break
    finally:
        job.detach(myq)
