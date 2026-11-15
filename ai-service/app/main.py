"""AI 服务主入口：FastAPI。

启动：uvicorn app.main:app --host 0.0.0.0 --port 8000
只跑在 docker 内网，不对外暴露；外部请求由 Go 主服务转发。

安全：所有 /api/ai/* 请求必须带 X-Internal-Token 头，
防止绕过 Go 网关直连 8000 端口伪造身份。
"""
import asyncio
from contextlib import asynccontextmanager
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware

from app.api.chat import router as chat_router
from app.api.generate import router as generate_router
from app.api.session import router as session_router
from app.api.grade import router as grade_router
from app.api.kb import router as kb_router
from app.core.config import settings
from app.kb.store import KnowledgeStoreUnavailable, get_knowledge_store


# 内部调用令牌：和 Go 代理侧的 AiService.InternalToken 同值，从环境变量注入。
# 留空且又打开了校验开关 = 所有请求都会被拒（故意的：宁可拒绝服务，
# 也不能"忘了配密钥"就退化成不校验身份）。
async def internal_token_middleware(request: Request, call_next):
    """校验 X-Internal-Token，只放行 Go 代理 / Go 后端转发的请求。"""
    # 测试环境或探针不校验：/health 与 /readyz 都是容器/编排内部调用，
    # 拿不到 X-Internal-Token；不给它们放行会让就绪探针永远 403。
    # 业务接口（含 /api/ai/kb/status）仍必须带令牌。
    if not settings.internal_auth_enabled or request.url.path in ("/health", "/readyz"):
        return await call_next(request)

    token = request.headers.get("X-Internal-Token", "")
    if not settings.internal_token or token != settings.internal_token:
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

# 先加 CORS（外层），再加内部 token 中间件（内层）
# 这样 OPTIONS 预检请求能被 CORS 正常处理
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
app.include_router(session_router)
app.include_router(generate_router)
# 主观题批改入口：只给 Go 后端调，不进 Go 的浏览器代理路由表
app.include_router(grade_router)
# 知识库覆盖度查询（Go 代理白名单里新增 /api/ai/kb/status）
app.include_router(kb_router)


@app.get("/health")
async def health():
    """liveness：只检查进程存活，绝不因为依赖挂了就让容器重启。"""
    return {"status": "ok", "service": "ai-service", "port": settings.port}


@app.get("/readyz")
async def readyz():
    """readiness：暴露知识库后端与降级状态，供上线做就绪探针 / 告警。

    和 /health 分开是刻意的：MongoDB 抖动时服务仍可用（自动降级），
    不该被 liveness 判死重启；但 readiness 要明确显示 degraded=true。
    """
    try:
        store = await asyncio.to_thread(get_knowledge_store)
        ok = await asyncio.to_thread(store.ping)
    except KnowledgeStoreUnavailable as e:
        from fastapi.responses import JSONResponse
        return JSONResponse(status_code=503, content={"status": "unavailable", "detail": str(e)})
    except Exception as e:
        from fastapi.responses import JSONResponse
        return JSONResponse(
            status_code=503,
            content={"status": "unavailable", "detail": f"知识库不可用（{type(e).__name__}）"},
        )
    return {
        "status": "ok" if ok else "unavailable",
        "knowledge_base": {
            "backend": store.backend,
            "degraded": store.degraded,
            "ping": ok,
        },
    }
