"""AI 服务配置：所有环境变量走 pydantic-settings，docker-compose 里注入。"""
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")

    # HTTP
    host: str = "0.0.0.0"
    port: int = 8000

    # LLM：通义千问 DashScope OpenAI 兼容模式
    llm_base_url: str = "https://dashscope.aliyuncs.com/compatible-mode/v1"
    llm_api_key: str = ""
    llm_model: str = "qwen-plus"
    llm_timeout: int = 60

    # Embedding：通义 text-embedding-v3
    embedding_base_url: str = "https://dashscope.aliyuncs.com/compatible-mode/v1"
    embedding_model: str = "text-embedding-v3"

    # 向量库：chroma 本地目录
    vector_db_dir: str = "./data/chroma"
    vector_db_type: str = "chroma"  # 将来换 milvus 只改这里

    # Redis（会话历史 + 异步批改队列）
    redis_url: str = "redis://localhost:6379/0"
    session_ttl: int = 3600 * 24 * 7  # 会话保留 7 天
    session_max_rounds: int = 5       # 只留最近 5 轮
    grade_queue: str = "ai:grade:queue"

    # Go 主服务回调地址（docker 内网）
    go_callback_base: str = "http://learningplatform:8888"

    # MongoDB（存会话/任务记录，Demo 用；也可以换 SQLite）
    mongodb_url: str = "mongodb://localhost:27017"
    mongodb_db: str = "ai_service"

    # gRPC 端口（给 Go 微服务调用）
    grpc_port: int = 50051

    # 限流：每个用户每天 AI 调用上限
    daily_token_limit: int = 100


settings = Settings()
