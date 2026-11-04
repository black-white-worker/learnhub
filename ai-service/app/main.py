"""AI 服务主入口：FastAPI。

启动：uvicorn app.main:app --host 0.0.0.0 --port 8000
只跑在 docker 内网，不对外暴露；外部请求由 Go 主服务转发。

安全：所有 /api/ai/* 请求必须带 X-Internal-Token 头，
防止绕过 Go 网关直连 8000 端口伪造身份。
"""
from contextlib import asynccontextmanager
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware

from app.api.chat import router as chat_router
from app.api.summary import router as summary_router
from app.api.generate import router as generate_router
from app.api.session import router as session_router
from app.core.config import settings

# 内部调用令牌：和 Go 代理侧的 internalToken 保持一致
INTERNAL_TOKEN = "ai-internal-secret-2026"


async def internal_token_middleware(request: Request, call_next):
    """校验 X-Internal-Token，只放行 Go 代理转发的请求。"""
    # /health 不校验（docker healthcheck 用）
    if request.url.path == "/health":
        return await call_next(request)

    token = request.headers.get("X-Internal-Token", "")
    if token != INTERNAL_TOKEN:
        from fastapi.responses import JSONResponse
        return JSONResponse(
            status_code=403,
            content={"detail": "forbidden: invalid internal token"},
        )
    return await call_next(request)


@asynccontextmanager
async def lifespan(app: FastAPI):
    # API 进程不启动批改消费者（拆到 ai-worker 独立进程）
    yield


app = FastAPI(title="LearningPlatform AI Service", version="0.2.0", lifespan=lifespan)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

# 内部 token 校验中间件
app.middleware("http")(internal_token_middleware)

# 注册所有路由
app.include_router(chat_router)
app.include_router(summary_router)
app.include_router(generate_router)
app.include_router(session_router)


@app.get("/health")
async def health():
    return {"status": "ok", "service": "ai-service", "port": settings.port}
