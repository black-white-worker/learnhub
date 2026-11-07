"""学习总结 API。

P0 修复：
- 输出改结构化 JSON（mastered/weak/suggestions/sources），前端能渲染成卡片
- LLM 错误统一信封，不直接 500 冒泡
- content 超长截断时返回 truncated: true 提示
"""
from fastapi import APIRouter, HTTPException, Request
from pydantic import BaseModel
from typing import Optional, List
import httpx
from openai import AsyncOpenAI

from app.core.config import settings

router = APIRouter(prefix="/api/ai", tags=["summary"])

MAX_CONTENT = 3000


def get_user_id(req: Request) -> int:
    uid = req.headers.get("X-User-Id")
    if not uid:
        raise HTTPException(401, "missing X-User-Id")
    try:
        return int(uid)
    except ValueError:
        raise HTTPException(400, "invalid X-User-Id")


class SummaryReq(BaseModel):
    content: Optional[str] = ""
    course_id: Optional[int] = 0


SUMMARY_PROMPT = """你是课程学习助教。请根据下面学生的学习笔记/材料，输出严格 JSON：

{{
  "mastered": ["已掌握的考点1", "已掌握的考点2"],
  "weak": [{{"point": "薄弱知识点", "evidence": "从哪看出来的"}}],
  "suggestions": ["建议1", "建议2"],
  "sources": ["来源1", "来源2"]
}}

只输出 JSON，不要额外文字。

学习材料：
{content}
"""


async def fetch_notes_from_go(user_id: int, course_id: int) -> str:
    """调 Go 后端拉笔记，失败返回空字符串。"""
    go_base = settings.go_callback_base
    async with httpx.AsyncClient(timeout=5) as client:
        try:
            resp = await client.get(
                f"{go_base}/api/course/note/summary/{course_id}",
                headers={"X-User-Id": str(user_id)},
            )
            if resp.status_code != 200:
                return ""
            data = resp.json()
            notes = data.get("notes", [])
            if not notes:
                return ""
            parts = []
            for n in notes:
                ts = n.get("timestampSeconds", 0)
                parts.append(f"[视频{n.get('videoId','?')} {ts//60}分{ts%60}秒] {n.get('content','')}")
            return "\n".join(parts)
        except Exception:
            return ""


@router.post("/summary")
async def summarize(body: SummaryReq, req: Request):
    """生成学习总结。"""
    user_id = get_user_id(req)

    materials = ""
    truncated = False

    if body.course_id and body.course_id > 0:
        notes_text = await fetch_notes_from_go(user_id, body.course_id)
        if notes_text:
            materials += f"学生笔记：\n{notes_text}\n\n"

    if body.content:
        if len(body.content) > MAX_CONTENT:
            truncated = True
            materials += f"补充材料（已截断）：\n{body.content[:MAX_CONTENT]}\n"
        else:
            materials += f"补充材料：\n{body.content}\n"

    if not materials.strip():
        raise HTTPException(400, "没有可总结的学习材料，请传 content 或 course_id")

    prompt = SUMMARY_PROMPT.format(content=materials[:4000])

    try:
        llm = AsyncOpenAI(
            base_url=settings.llm_base_url,
            api_key=settings.llm_api_key,
            timeout=settings.llm_timeout,
        )
        resp = await llm.chat.completions.create(
            model=settings.llm_model,
            messages=[{"role": "user", "content": prompt}],
            temperature=0.3,
        )
        text = resp.choices[0].message.content or ""

        # 尝试解析 JSON，失败降级为纯文本字符串
        import json
        try:
            start = text.index("{")
            end = text.rindex("}") + 1
            summary = json.loads(text[start:end])
        except Exception:
            summary = text  # 解析失败直接返回原始文本，前端可直接展示

        return {
            "summary": summary,
            "course_id": body.course_id,
            "truncated": truncated,
            "material_length": len(materials),
        }
    except Exception as e:
        # 统一错误信封
        raise HTTPException(status_code=503, detail=f"AI 服务暂不可用: {str(e)[:100]}")
