"""pytest 全局配置：测试环境不连真实 Redis/LLM。

在 import app 之前把环境变量设好，避免连到 docker 里的 Redis。
"""
import os

# 测试环境用假 Redis（本地不存在，测试里会 mock）
os.environ.setdefault("REDIS_URL", "redis://localhost:6379/15")
os.environ.setdefault("LLM_API_KEY", "test-key")
os.environ.setdefault("VECTOR_DB_DIR", "./data/test_chroma")
# 测试时关闭内部 token 校验，否则 TestClient 所有请求都被 403
os.environ.setdefault("INTERNAL_AUTH_ENABLED", "false")
# 知识库测试一律走内存实现：不连真实 MongoDB、不依赖容器状态
os.environ.setdefault("MONGODB_ENABLED", "false")
os.environ.setdefault("MONGODB_STRICT", "false")
os.environ.setdefault("INGEST_DRY_RUN", "false")
# 出题测试绝不连外网：题库风格注入默认关闭，题库单测自己用 fake client 注入。
# 要覆盖 ai-service/.env 里的值，这里直接赋值而不是 setdefault。
os.environ["QUESTION_BANK_INSPIRE"] = "false"
os.environ["QUESTION_BANK_ENABLED"] = "true"
# 中文资料检索同理：单测默认关闭，素材链路的单测用 fake client / monkeypatch 单独打开。
os.environ["ZH_MATERIAL_ENABLED"] = "false"
