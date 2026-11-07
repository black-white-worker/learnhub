"""AI 出题 API。

老师输入知识点，AI 生成题目+答案，老师审核后再入库。
支持题型：single（单选）、multiple（多选）、judge（判断）、fill（填空）、essay（简答）。

P0 修复：
- 角色校验：只有 teacher/admin 能调
- 服务端 schema 校验：单选4选项/答案在A-D、多选≥2、判断归一化true/false
"""
from fastapi import APIRouter, HTTPException, Request
from pydantic import BaseModel
from typing import List, Optional
import json
from openai import AsyncOpenAI

from app.core.config import settings

router = APIRouter(prefix="/api/ai", tags=["generate"])


def get_user_id(req: Request) -> int:
    uid = req.headers.get("X-User-Id")
    if not uid:
        raise HTTPException(401, "missing X-User-Id")
    return int(uid)


def require_teacher(req: Request):
    """校验当前用户是教师或管理员，学生不能出题。"""
    role = req.headers.get("X-Role", "")
    if role not in ("teacher", "admin"):
        raise HTTPException(403, "只有教师或管理员可以使用 AI 出题")


class GenQuestionReq(BaseModel):
    knowledge: str
    difficulty: str = "中等"
    qtype: str = "single"
    count: int = 2
    score: int = 5


TYPE_PROMPTS = {
    "single": '生成{count}道单选题，每题4个选项(A/B/C/D)，标注正确答案。输出JSON数组：[{{"stem":"题干","options":["选项A","选项B","选项C","选项D"],"answer":"A","score":{score}}}]',
    "multiple": '生成{count}道多选题，每题4个选项，答案可能有2-4个。输出JSON数组：[{{"stem":"题干","options":["选项A","选项B","选项C","选项D"],"answer":"ABD","score":{score}}}]',
    "judge": '生成{count}道判断题，答案只有true或false。输出JSON数组：[{{"stem":"题干","answer":true,"score":{score}}}]',
    "fill": '生成{count}道填空题，答案是简短关键词。输出JSON数组：[{{"stem":"题干（用____表示空）","answer":"参考答案","score":{score}}}]',
    "essay": '生成{count}道简答题，给参考答案要点。输出JSON数组：[{{"stem":"题干","answer":"参考答案要点","score":{score}}}]',
}


def validate_questions(questions: list, qtype: str) -> list:
    """服务端校验 AI 生成的题目结构，不合格的标记 invalid。"""
    valid = []
    for q in questions:
        if qtype == "single":
            opts = q.get("options", [])
            ans = q.get("answer", "")
            if len(opts) != 4 or ans not in ("A", "B", "C", "D"):
                q["invalid"] = True
                q["invalid_reason"] = "单选必须4选项且答案在A-D内"
        elif qtype == "multiple":
            ans = q.get("answer", "")
            if len(ans) < 2:
                q["invalid"] = True
                q["invalid_reason"] = "多选答案至少2个"
        elif qtype == "judge":
            ans = q.get("answer")
            # 归一化：对/正确/true → True，错/错误/false → False
            if isinstance(ans, str):
                if ans in ("对", "正确", "true", "True"):
                    q["answer"] = True
                elif ans in ("错", "错误", "false", "False"):
                    q["answer"] = False
        valid.append(q)
    return valid


@router.post("/generate")
async def gen_question(body: GenQuestionReq, req: Request):
    """AI 出题。"""
    get_user_id(req)
    require_teacher(req)  # 角色校验

    if body.qtype not in TYPE_PROMPTS:
        raise HTTPException(400, f"不支持的题型: {body.qtype}")
    if body.count < 1 or body.count > 10:
        raise HTTPException(400, "count 必须在 1-10 之间")

    prompt = (
        f"根据知识点【{body.knowledge}】，难度：{body.difficulty}。"
        + TYPE_PROMPTS[body.qtype].format(count=body.count, score=body.score)
        + "。只输出 JSON 数组，不要任何解释文字。"
    )

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

    try:
        start = text.index("[")
        end = text.rindex("]") + 1
        questions = json.loads(text[start:end])
    except Exception:
        return {"questions": [{"stem": text, "answer": "", "score": body.score}], "raw": True}

    # 服务端校验
    questions = validate_questions(questions, body.qtype)
    invalid_count = sum(1 for q in questions if q.get("invalid"))

    return {
        "questions": questions,
        "qtype": body.qtype,
        "count": len(questions),
        "invalid_count": invalid_count,
    }
