"""中文/英文分词与词法打分测试（零依赖实现，对应 app/kb/tokens.py）。"""
from app.kb.tokens import lexical_score, token_weight, tokenize


class TestTokenize:
    def test_chinese_bigrams(self):
        t = tokenize("检索增强生成")
        assert "检索" in t and "增强" in t and "生成" in t
        assert "检索增强" not in t          # 只取相邻二元组，不生成更长 n-gram

    def test_english_and_numbers(self):
        t = tokenize("RAG 和 ChromaDB v3 是什么")
        assert "rag" in t and "chromadb" in t and "v3" in t
        assert "a" not in t                 # 单字母英文词丢弃

    def test_case_insensitive(self):
        assert tokenize("MongoDB") == tokenize("mongodb")

    def test_stopwords_not_everything(self):
        """「课程/资料」等实词不能被当停用词过滤掉。"""
        t = tokenize("课程资料里讲了什么")
        assert "课程" in t and "资料" in t
        assert "什么" not in t

    def test_content_word_survives_next_to_function_word(self):
        t = tokenize("资料中未找到")
        assert "未找" in t                # 含单字虚词「中」的组合不能被整词丢掉

    def test_empty_input(self):
        assert tokenize("") == []
        assert tokenize(None) == []

    def test_max_tokens_cap(self):
        t = tokenize("数据分析机器学习深度学习" * 50, max_tokens=8)
        assert len(t) == 8

    def test_stable_and_unique(self):
        t = tokenize("RAG RAG 检索 检索")
        assert t == sorted(set(t))


class TestLexicalScore:
    def test_no_overlap_is_zero(self):
        assert lexical_score(["rag"], ["检索", "增强"]) == 0.0

    def test_empty_inputs(self):
        assert lexical_score([], ["rag"]) == 0.0
        assert lexical_score(["rag"], []) == 0.0

    def test_full_match_is_one(self):
        assert lexical_score(["rag", "检索"], ["rag", "检索", "增强"]) == 1.0

    def test_chinese_bigram_weighted_higher_than_english_word(self):
        assert token_weight("检索") == 2.0
        assert token_weight("rag") == 1.0
        q = ["rag", "检索"]
        only_cn = lexical_score(q, ["检索"])
        only_en = lexical_score(q, ["rag"])
        assert only_cn > only_en

    def test_score_between_zero_and_one(self):
        s = lexical_score(["rag", "检索", "增强"], ["检索"])
        assert 0.0 < s < 1.0
