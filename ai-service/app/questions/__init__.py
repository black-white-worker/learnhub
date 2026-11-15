"""出题域：随机命题蓝图、中文专业度校验、中文知识素材源、通识题库连接器。

拆出来的原因：`app/api/generate.py` 只负责编排（鉴权 → 取题 → 调 LLM → 后处理），
随机策略与校验都是纯函数，必须能脱离 FastAPI / 网络独立单测。
"""

from app.questions.variety import (  # noqa: F401
    RecentStemCache,
    apply_chinese_quality,
    assess_chinese_quality,
    chinese_ratio,
    build_blueprint,
    blueprint_text,
    dedupe_questions,
    is_similar,
    normalize_question,
    normalize_stem,
    parse_llm_json,
    random_sampling,
    shuffle_options,
    stem_fingerprint,
    validate_question,
)

__all__ = [
    "RecentStemCache",
    "apply_chinese_quality",
    "assess_chinese_quality",
    "chinese_ratio",
    "build_blueprint",
    "blueprint_text",
    "dedupe_questions",
    "is_similar",
    "normalize_question",
    "normalize_stem",
    "parse_llm_json",
    "random_sampling",
    "shuffle_options",
    "stem_fingerprint",
    "validate_question",
]
