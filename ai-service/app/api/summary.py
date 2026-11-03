"""学习总结 API。

两种模式：
1. content 模式：前端直接传学习材料文本，AI 提炼考点
2. course_id 模式：ai-service 调 Go 后端拉该课程下当前用户的笔记，
   基于笔记内容生成个性化学习总结

输出结构化：3 个核心考点 + 薄弱点提示 + 下一步学习建议。
"""
from fastapi import APIRouter, Depends, HTTPException, Request
from pydantic import BaseModel
from typing import Optional
import httpx

from app.core.config import settings

router = APIRouter(prefix="/api/ai", tags=["summary"])


# 简单鉴权：Go 网关已验 JWT，这里信任 X-User-Id
def get_user_id(req: Request) -> int:
    uid = req.headers.get("X-User-Id")
    if not uid:
        raise HTTPException(401, "missing X-User-Id")
    try:
        return int(uid)
    except ValueError:
        raise HTTPException(400, "invalid X-User-Id")


class SummaryReq(BaseModel):
    """学习总结请求。

    content 和 course_id 至少传一个：
    - content 非空：直接总结传入的文本
    - course_id > 0：从 Go 后端拉该课程笔记做总结
    两个都传时以 course_id 为主，content 作为补充材料。
    """
    content: Optional[str] = ""
    course_id: Optional[int] = 0
    user_id: Optional[int] = 0  # 服务端从 header 取，前端不用传


SUMMARY_PROMPT = """你是课程学习助教。请根据下面学生的学习笔记/材料，完成三件事：

1. 提炼 3 个核心考点（每个考点一句话说清楚）
2. 指出 1-2 个可能的薄弱点（从笔记里看出来理解不深的地方）
3. 给出 2 条具体的下一步学习建议

输出格式严格如下（不要额外文字）：
【核心考点】
1. ...
2. ...
3. ...

【薄弱点】
- ...

【学习建议】
- ...
- ...

学习材料：
{content}
"""


async def fetch_notes_from_go(user_id: int, course_id: int) -> str:
    """调 Go 后端拉该用户在某课程下的所有笔记，拼成文本。"""
    go_base = settings.go_callback_base
    async with httpx.AsyncClient(timeout=5) as client:
        try:
            # Go 侧接口：GET /api/course/note/:courseId/:videoId 是按视频查的，
            # 这里用一个汇总接口（如果没有就返回空）
            resp = await client.get(
                f"{go_base}/api/course/note/summary/{course_id}",
                headers={"X-User-Id": str(user_id)},
            )
            if resp.status_code != 200:
                return ""
            data = resp.json()
            # data 形如 {"notes": [{"videoId":1,"timestampSeconds":30,"content":"..."}]}
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

    # 收集材料：优先从 Go 拉笔记，再拼上前端直接传的 content
    materials = ""
    if body.course_id and body.course_id > 0:
        notes_text = await fetch_notes_from_go(user_id, body.course_id)
        if notes_text:
            materials += f"学生在本课程的笔记：\n{notes_text}\n\n"
    if body.content:
        materials += f"补充学习材料：\n{body.content[:3000]}\n"

    if not materials.strip():
        raise HTTPException(400, "没有可总结的学习材料，请传 content 或 course_id")

    prompt = SUMMARY_PROMPT.format(content=materials[:4000])

    from openai import AsyncOpenAI
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

    return {
        "summary": text,
        "course_id": body.course_id,
        "material_length": len(materials),
    }
