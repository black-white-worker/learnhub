"""pytest 全局配置：测试环境不连真实 Redis/LLM。

在 import app 之前把环境变量设好，避免连到 docker 里的 Redis。
"""
import os

# 测试环境用假 Redis（本地不存在，测试里会 mock）
os.environ.setdefault("REDIS_URL", "redis://localhost:6379/15")
os.environ.setdefault("LLM_API_KEY", "test-key")
os.environ.setdefault("VECTOR_DB_DIR", "./data/test_chroma")
