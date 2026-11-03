"""AI 服务主入口：FastAPI + 后台批改消费者。

启动：uvicorn app.main:app --host 0.0.0.0 --port 8000
只跑在 docker 内网，不对外暴露；外部请求由 Go 主服务转发。

路由总览：
- POST /api/ai/chat            SSE 流式答疑（LangGraph 4 节点）
- POST /api/ai/summary         学习总结（提炼考点）
- POST /api/ai/generate        AI 出题（老师用，多题型）
- GET  /api/ai/session/history 查对话历史
- DELETE /api/ai/session       清空对话
- GET  /health                 健康检查
后台：Redis 队列消费作业批改任务
"""
import asyncio
from contextlib import asynccontextmanager
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.api.chat import router as chat_router
from app.api.summary import router as summary_router
from app.api.generate import router as generate_router
from app.api.session import router as session_router
from app.tasks.grade_worker import consume_loop
from app.core.config import settings


@asynccontextmanager
async def lifespan(app: FastAPI):
    # 启动：起一个后台协程消费 Redis 批改队列
    task = asyncio.create_task(consume_loop())
    yield
    task.cancel()


app = FastAPI(title="LearningPlatform AI Service", version="0.1.0", lifespan=lifespan)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

# 注册所有路由
app.include_router(chat_router)
app.include_router(summary_router)
app.include_router(generate_router)
app.include_router(session_router)


@app.get("/health")
async def health():
    return {"status": "ok", "service": "ai-service", "port": settings.port}
