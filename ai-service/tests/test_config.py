"""配置层测试：本地可跑 + 上线可配 + 密钥不泄露。

对应功能：app/core/config.py 的环境变量化改造。
测试全部用 _env_file=None 隔离真实 .env，不依赖任何外部服务。
"""
from app.core.config import Settings


def _settings(**kwargs) -> Settings:
    """构造隔离的 Settings：不读 .env，显式入参优先级最高。"""
    return Settings(_env_file=None, **kwargs)


class TestMysqlConfig:
    """ingest 的 MySQL 连接不再硬编码 localhost/root/root。"""

    def test_conn_kwargs_come_from_settings(self):
        s = _settings(mysql_host="mysql", mysql_port=3307, mysql_user="ai_reader",
                      mysql_password="s3cret", mysql_database="lp", mysql_connect_timeout=3)
        kw = s.mysql_conn_kwargs
        assert kw == {
            "host": "mysql", "port": 3307, "user": "ai_reader",
            "password": "s3cret", "database": "lp", "charset": "utf8mb4",
            "connect_timeout": 3,
        }

    def test_no_hardcoded_root_credentials_as_default(self, monkeypatch):
        # 容器/CI 会注入 MYSQL_* 环境变量，会盖掉配置里的默认值，
        # 让这条「源码不留凭据字面量」的断言误报。先摘掉环境变量再验默认值。
        for key in ("MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER",
                    "MYSQL_PASSWORD", "MYSQL_DATABASE"):
            monkeypatch.delenv(key, raising=False)
        s = _settings()
        kw = s.mysql_conn_kwargs
        assert kw["host"] == "127.0.0.1"
        assert kw["password"] == ""          # 源码里没有 root/root 字面量
        assert kw["charset"] == "utf8mb4"    # 中文不乱码

    def test_env_var_override(self, monkeypatch):
        monkeypatch.setenv("MYSQL_HOST", "db.internal")
        monkeypatch.setenv("MYSQL_PORT", "3308")
        monkeypatch.setenv("MYSQL_PASSWORD", "from-env")
        s = Settings(_env_file=None)
        assert s.mysql_host == "db.internal"
        assert s.mysql_port == 3308
        assert s.mysql_conn_kwargs["password"] == "from-env"

    def test_password_never_in_repr(self):
        s = _settings(mysql_password="super-secret-pw")
        assert "super-secret-pw" not in repr(s)


class TestMongoAndProdProfile:
    """本地默认优雅降级；上线可通过环境变量切成严格模式 + 认证 + 连接池。"""

    def test_local_defaults_are_safe(self):
        # 显式给值，避免被 conftest 里 MONGODB_ENABLED=false 的测试隔离环境影响
        s = _settings(mongodb_enabled=True, mongodb_strict=False,
                      mongodb_tls=False, mongodb_server_selection_timeout_ms=2000)
        assert s.mongodb_enabled is True
        assert s.mongodb_strict is False        # 连不上自动降级，服务不崩
        assert s.mongodb_tls is False
        assert s.mongodb_server_selection_timeout_ms == 2000

    def test_prod_profile_all_knobs_injectable(self):
        s = _settings(
            mongodb_url="mongodb://mongo-0:27017,mongo-1:27017",
            mongodb_strict=True, mongodb_tls=True,
            mongodb_username="ai", mongodb_password="prod-pw",
            mongodb_auth_source="admin", mongodb_replica_set="rs0",
            mongodb_max_pool_size=50, mongodb_min_pool_size=5,
            mongodb_server_selection_timeout_ms=8000,
        )
        assert s.mongodb_strict is True
        assert s.mongodb_tls is True
        assert s.mongodb_replica_set == "rs0"
        assert s.mongodb_max_pool_size == 50
        assert s.mongodb_min_pool_size == 5
        assert s.mongodb_server_selection_timeout_ms == 8000
        assert "prod-pw" not in repr(s)

    def test_can_disable_mongo_entirely(self):
        s = _settings(mongodb_enabled=False)
        assert s.mongodb_enabled is False


class TestRetrievalAndIngestKnobs:
    def test_retrieval_defaults(self):
        s = _settings()
        assert s.retrieval_hybrid_enabled is True
        # Top-K 默认 5：只取 3 段时「这门课讲了什么」经常只命中零散细节，
        # 模型据此判「没有相关内容」而拒答（线上现象）。
        assert s.retrieval_vector_top_k == 5
        assert s.retrieval_lexical_top_k == 5
        assert s.retrieval_rrf_k == 60
        assert s.retrieval_log_unanswered is True

    def test_video_stt_is_explicitly_off(self):
        """视频转写仍是 TODO：配置默认关闭，避免误以为已经支持。"""
        s = _settings()
        assert s.ingest_parse_video is False

    def test_ingest_limits(self):
        s = _settings()
        assert s.ingest_chunk_size == 500
        assert s.ingest_chunk_overlap == 50
        assert 0 < s.ingest_chunk_overlap < s.ingest_chunk_size
        assert s.ingest_max_file_bytes == 50 * 1024 * 1024


class TestSecretHygiene:
    def test_all_secrets_hidden_from_repr(self):
        s = _settings(llm_api_key="llm-SECRET", minio_secret_key="minio-SECRET",
                      internal_token="token-SECRET", mongodb_password="mongo-SECRET")
        r = repr(s)
        for secret in ("llm-SECRET", "minio-SECRET", "token-SECRET", "mongo-SECRET"):
            assert secret not in r
