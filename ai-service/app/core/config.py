"""AI 服务配置：所有环境变量走 pydantic-settings，docker-compose 里注入。

设计约束（本地可跑 / 上线可配）：
1. 任何基础设施默认值都必须是「本机不带依赖也能启动」的值，源码不留密码字面量；
2. 生产用环境变量覆盖（docker 服务名、认证、连接池、TLS、严格模式）；
3. MongoDB 不可用时默认自动降级到内存知识库，不影响答疑主链路；
   上线把 MONGODB_STRICT=true，连不上直接失败，避免「以为灌进去了其实没有」。
"""
from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")

    # HTTP
    host: str = "0.0.0.0"
    port: int = 8000

    # LLM：通义千问 DashScope OpenAI 兼容模式
    llm_base_url: str = "https://dashscope.aliyuncs.com/compatible-mode/v1"
    llm_api_key: str = Field(default="", repr=False)
    llm_model: str = "qwen-plus"
    llm_timeout: int = 60

    # ===== AI 出题：随机性与质量 =====
    # 一次生成最多调几次 LLM（第一次不够 / 有废题时换随机蓝图重试补齐）
    generate_max_attempts: int = 2
    # 单次让模型多生成几道（留余量），优先一轮凑齐，减少进入第二轮回填的概率，
    # 从而把一次出题从「可能两轮 ~70s」压到「通常一轮 ~40s」。上限受题型 4 选项与
    # 单次 token 限制，最终生成数 = min(10, count + slack)。
    generate_overgen_slack: int = 2
    # 每个「知识点+题型+难度」在进程内记住多少条最近题干，用于跨请求去重
    generate_recent_window: int = 60
    # 题干相似度阈值（0~1，越高越宽松）；0.82 能抓住「换了个标点/加了前缀」的重复题
    generate_similarity_threshold: float = 0.82

    # ===== 中文专业知识素材（按知识点出题的事实依据，免费免密钥）=====
    # 中文维基百科 / 中文维基教科书，CC BY-SA 4.0，variant=zh-cn 直接返回简体。
    # 检索失败 / 无匹配时自动退回「纯模型中文专业出题」，不影响生成。
    zh_material_enabled: bool = True
    zh_material_timeout: float = 4.0
    zh_material_max_docs: int = 4         # 每次检索最多取几个条目
    zh_material_max_snippets: int = 3     # 注入 prompt 的素材段落数
    zh_material_snippet_chars: int = 420  # 单段最大字数
    zh_material_extract_chars: int = 6000  # 单条目正文截断长度
    zh_material_cache_ttl: int = 21600    # 条目正文缓存 6 小时
    zh_material_wikibooks_enabled: bool = True   # 主源无结果时用维基教科书补查
    zh_wiki_api_url: str = "https://zh.wikipedia.org/w/api.php"
    zh_wiki_view_url: str = "https://zh.wikipedia.org/wiki/"
    zh_wikibooks_api_url: str = "https://zh.wikibooks.org/w/api.php"
    zh_wikibooks_view_url: str = "https://zh.wikibooks.org/wiki/"
    # 中文 / 专业度硬校验（英文整句题干判废并触发补齐）
    generate_require_chinese: bool = True

    # ===== 免费公开题库（可选增强，失败一律降级回 AI 出题）=====
    # 两个源都免费、无需密钥：Open Trivia DB（CC BY-SA 4.0）与 The Trivia API。
    # enabled 只控制「能力开关」；是否使用由请求里的 source=bank / inspire 决定。
    question_bank_enabled: bool = True
    # 是否给 AI 出题注入题库风格样例（多一次外网请求，默认关）
    question_bank_inspire: bool = False
    # 题库是英文题，用 LLM 批量翻成中文；关掉或翻译失败时保留英文原文
    question_bank_localize: bool = True
    question_bank_url: str = "https://opentdb.com/api.php"
    question_bank_token_url: str = "https://opentdb.com/api_token.php"
    question_bank_fallback_url: str = "https://the-trivia-api.com/v2/questions"
    question_bank_timeout: float = 6.0
    question_bank_use_token: bool = True
    question_bank_max_items: int = 10

    # Embedding：通义 text-embedding-v3
    embedding_base_url: str = "https://dashscope.aliyuncs.com/compatible-mode/v1"
    embedding_model: str = "text-embedding-v3"
    # 单次请求最多带几条文本。DashScope 的硬上限是 10，写大了服务端会直接
    # 400（batch size is invalid）—— 而一批失败就等于整份讲义灌库失败。
    embedding_batch_size: int = 10

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

    # ===== 课程资料清单（向 Go 要「这门课有哪些讲义/视频」）=====
    # 灌库和答疑都走这一个入口，避免 ai-service 再直连 MySQL 复制一份数据口径。
    # 清单里带预签名 URL，超时给短一点：拿不到就退回查库，别让灌库卡在网络上。
    manifest_timeout: float = 5.0
    manifest_cache_ttl: int = 60   # 秒；老师刚传完讲义就要能问到，别缓存太久

    # ===== MySQL：只有 ingest 脚本读课程资源元数据 =====
    # 本地默认 127.0.0.1:3306；docker 内网用 mysql:3306；生产建议给只读账号。
    mysql_host: str = "127.0.0.1"
    mysql_port: int = 3306
    mysql_user: str = "root"
    mysql_password: str = Field(default="", repr=False)
    mysql_database: str = "learningplatform"
    mysql_connect_timeout: int = 5

    @property
    def mysql_conn_kwargs(self) -> dict:
        """pymysql.connect 参数。密码只在这里传递，绝不拼进日志/异常文本。"""
        return {
            "host": self.mysql_host,
            "port": self.mysql_port,
            "user": self.mysql_user,
            "password": self.mysql_password,
            "database": self.mysql_database,
            "charset": "utf8mb4",
            "connect_timeout": self.mysql_connect_timeout,
        }

    # ===== MongoDB：RAG 知识库主存储（文档/切片/灌入状态/未答日志/原文件）=====
    mongodb_enabled: bool = True           # 关掉即强制走内存降级（测试 / 无 Mongo 环境）
    mongodb_strict: bool = False           # 上线设 true：连不上直接抛错，不静默降级
    mongodb_url: str = "mongodb://127.0.0.1:27017"
    mongodb_db: str = "ai_service"
    # 上线认证：也可直接把 user/pass 写进 mongodb_url；这里单独留字段便于密钥管理
    mongodb_username: str = ""
    mongodb_password: str = Field(default="", repr=False)
    mongodb_auth_source: str = "admin"
    mongodb_replica_set: str = ""
    mongodb_tls: bool = False
    mongodb_server_selection_timeout_ms: int = 2000
    mongodb_connect_timeout_ms: int = 2000
    mongodb_max_pool_size: int = 20
    mongodb_min_pool_size: int = 0

    # ===== MinIO：ingest 拉课程原文件（预留上线配置）=====
    # 本地公开桶可只靠资源 URL 直接下载；私有桶上线建议装 minio SDK 按 object_name 拉。
    minio_endpoint: str = "127.0.0.1:9020"     # 上线用内网地址 minio:9000
    minio_access_key: str = ""
    minio_secret_key: str = Field(default="", repr=False)
    minio_bucket: str = "learningplatform"
    minio_secure: bool = False
    ingest_local_file_root: str = ""            # 本地调试：把 object_name 映射到该目录下的文件
    ingest_fetch_timeout: int = 15

    # ===== 知识库 / 灌入 =====
    kb_collection_prefix: str = "kb"           # 集合前缀：kb_documents / kb_chunks / ...
    kb_gridfs_bucket: str = "kb_files"         # GridFS 存 PDF/DOCX 原文件，便于重解析
    ingest_chunk_size: int = 500               # token
    ingest_chunk_overlap: int = 50
    ingest_max_file_bytes: int = 50 * 1024 * 1024  # 单文件 50MB，防超大文件打爆内存
    ingest_parse_pdf: bool = True
    ingest_parse_docx: bool = True
    ingest_parse_subtitle: bool = True         # 配套 SRT/VTT 字幕（视频 STT 的务实替代）
    ingest_parse_video: bool = False           # 视频 STT 尚未实现（TODO），默认关闭
    ingest_dry_run: bool = False

    # ===== 混合检索 =====
    retrieval_hybrid_enabled: bool = True      # 关掉即退回纯向量（和改造前一致）
    # Top-K 取 5：讲义一份动辄上百个切片，只取 3 段时「这门课讲了什么」这类
    # 概括性问题经常只命中零散细节，模型据此判「没有相关内容」而拒答。
    retrieval_vector_top_k: int = 5
    retrieval_lexical_top_k: int = 5
    retrieval_rrf_k: int = 60                  # RRF 融合常数
    retrieval_lexical_min_tokens: int = 1     # 至少命中几个词法 token 才算候选
    retrieval_log_unanswered: bool = True     # 检索为空时落 kb_unanswered，做覆盖度闭环
    retrieval_max_query_chars: int = 500      # 落库前截断，防止把长对话写进 Mongo

    # gRPC 端口（给 Go 微服务调用；当前仍走 HTTP 转发，预留）
    grpc_port: int = 50051

    # 限流：每个用户每天 AI 调用上限
    daily_token_limit: int = 100

    # 内部 token 校验开关（测试环境关闭，避免 TestClient 全被 403）
    internal_auth_enabled: bool = True

    # 与 Go 主服务共享的密钥（请求头 X-Internal-Token）。
    # 必须和 Go 侧 AiService.InternalToken 同值，两边都由环境变量注入，
    # 源码里不留字面量 —— 之前硬编码在 main.py，轮换一次密钥要改两处并重新构建镜像。
    internal_token: str = Field(default="", repr=False)


settings = Settings()
