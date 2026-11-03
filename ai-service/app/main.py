"""AI 服务主入口：FastAPI + 后台批改消费者。

启动：uvicorn app.main:app --host 0.0.0.0 --port 8000
只跑在 docker 内网，不对外暴露；外部请求由 Go 主服务转发。
"""
import asyncio
from contextlib import asynccontextmanager
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.api.chat import router as chat_router
from app.api.generate import router as generate_router
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

app.include_router(chat_router)
app.include_router(generate_router)


@app.get("/health")
async def health():
    return {"status": "ok", "service": "ai-service", "port": settings.port}


# gRPC 服务在 grpc_server/ 下单独起一个 goroutine，端口 50051
# 为了简单，Demo 阶段先只跑 HTTP，gRPC 骨架放着后续练习微服务时打开。
