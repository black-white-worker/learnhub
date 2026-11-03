"""AI 出题 API。

老师输入知识点，AI 生成题目+答案，老师审核后再入库。
支持题型：single（单选）、multiple（多选）、judge（判断）、fill（填空）、essay（简答）。

输出格式严格 JSON，前端拿到后直接渲染成题目卡片。
"""
from fastapi import APIRouter, HTTPException, Request
from pydantic import BaseModel
from typing import List, Optional
import json

from app.core.config import settings

router = APIRouter(prefix="/api/ai", tags=["generate"])


def get_user_id(req: Request) -> int:
    uid = req.headers.get("X-User-Id")
    if not uid:
        raise HTTPException(401, "missing X-User-Id")
    return int(uid)


class GenQuestionReq(BaseModel):
    knowledge: str           # 知识点
    difficulty: str = "中等"  # 简单/中等/困难
    qtype: str = "single"    # single/multiple/judge/fill/essay
    count: int = 2           # 生成几道题
    score: int = 5           # 每题分值


# 题型对应的 prompt 模板
TYPE_PROMPTS = {
    "single": '生成{count}道单选题，每题4个选项(A/B/C/D)，标注正确答案。输出JSON数组：[{"stem":"题干","options":["选项A","选项B","选项C","选项D"],"answer":"A","score":{score}}]',
    "multiple": '生成{count}道多选题，每题4个选项，答案可能有2-4个。输出JSON数组：[{"stem":"题干","options":["选项A","选项B","选项C","选项D"],"answer":"ABD","score":{score}}]',
    "judge": '生成{count}道判断题，答案只有"对"或"错"。输出JSON数组：[{"stem":"题干","answer":"对","score":{score}}]',
    "fill": '生成{count}道填空题，答案是简短关键词。输出JSON数组：[{"stem":"题干（用____表示空）","answer":"参考答案","score":{score}}]',
    "essay": '生成{count}道简答题，给参考答案要点。输出JSON数组：[{"stem":"题干","answer":"参考答案要点","score":{score}}]',
}


@router.post("/generate")
async def gen_question(body: GenQuestionReq, req: Request):
    """AI 出题。"""
    get_user_id(req)  # 鉴权

    if body.qtype not in TYPE_PROMPTS:
        raise HTTPException(400, f"不支持的题型: {body.qtype}，可选: {list(TYPE_PROMPTS.keys())}")
    if body.count < 1 or body.count > 10:
        raise HTTPException(400, "count 必须在 1-10 之间")

    prompt_template = TYPE_PROMPTS[body.qtype]
    prompt = (
        f"根据知识点【{body.knowledge}】，难度：{body.difficulty}。"
        + prompt_template.format(count=body.count, score=body.score)
        + "。只输出 JSON 数组，不要任何解释文字。"
    )

    from openai import AsyncOpenAI
    llm = AsyncOpenAI(
        base_url=settings.llm_base_url,
        api_key=settings.llm_api_key,
        timeout=settings.llm_timeout,
    )
    resp = await llm.chat.completions.create(
        model=settings.llm_model,
        messages=[{"role": "user", "content": prompt}],
        temperature=0.7,
    )
    text = resp.choices[0].message.content.strip()

    # 提取 JSON 数组
    try:
        start = text.index("[")
        end = text.rindex("]") + 1
        questions = json.loads(text[start:end])
    except Exception:
        # 降级：返回原始文本让老师手动处理
        return {"questions": [{"stem": text, "answer": "", "score": body.score}], "raw": True}

    return {"questions": questions, "qtype": body.qtype, "count": len(questions)}
