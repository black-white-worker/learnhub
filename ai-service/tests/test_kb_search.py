"""RRF 融合 / hit 归一化 / 查询截断测试（对应 app/kb/search.py）。"""
import pytest
from app.kb.search import cap_query, chunk_key, make_hit, rrf_fuse


class TestChunkKeyAndHit:
    def test_key_is_stable(self):
        assert chunk_key("同一段文本") == chunk_key("同一段文本")

    def test_key_differs_by_text(self):
        assert chunk_key("文本A") != chunk_key("文本B")

    def test_make_hit_shape(self):
        h = make_hit("正文", {"title": "第1章"}, 0.9, "vector")
        assert h["key"] == chunk_key("正文")
        assert h["text"] == "正文" and h["metadata"]["title"] == "第1章"
        assert h["score"] == 0.9 and h["sources"] == ["vector"]

    def test_make_hit_copies_metadata(self):
        meta = {"title": "x"}
        h = make_hit("t", meta)
        meta["title"] = "mutated"
        assert h["metadata"]["title"] == "x"

    def test_cap_query(self):
        assert cap_query("  太长的问题  ", 4) == "太长的问"
        assert cap_query("", 10) == ""
        assert cap_query("短", -1) == "短"


class TestRRF:
    def test_single_list_keeps_order(self):
        a = [make_hit("A", score=0.9), make_hit("B", score=0.1)]
        out = rrf_fuse(a)
        assert [h["text"] for h in out] == ["A", "B"]
        assert out[0]["rrf"] > out[1]["rrf"]

    def test_two_lists_dedup_and_merge_sources(self):
        a = [make_hit("共享片段", {"title": "讲义"}, 0.8, "vector")]
        b = [make_hit("共享片段", {"page": 3}, 0.6, "lexical")]
        out = rrf_fuse(a, b)
        assert len(out) == 1
        assert set(out[0]["sources"]) == {"vector", "lexical"}
        assert out[0]["metadata"] == {"title": "讲义", "page": 3}
        assert out[0]["score"] == 0.8
        assert out[0]["source"] == "vector+lexical"

    def test_item_in_both_lists_ranks_higher(self):
        a = [make_hit("A", source="vector"), make_hit("B", source="vector")]
        b = [make_hit("A", source="lexical")]
        out = rrf_fuse(a, b)
        assert out[0]["text"] == "A"

    def test_top_k(self):
        a = [make_hit(f"t{i}", source="vector") for i in range(10)]
        assert len(rrf_fuse(a, top_k=3)) == 3

    def test_empty_lists(self):
        assert rrf_fuse([], []) == []
        assert rrf_fuse() == []

    def test_k_must_be_positive(self):
        with pytest.raises(ValueError):
            rrf_fuse([make_hit("A")], k=0)

    def test_higher_k_flattens_scores(self):
        a = [make_hit("A"), make_hit("B")]
        tight = rrf_fuse(a, k=1)
        loose = rrf_fuse(a, k=1000)
        assert tight[0]["rrf"] - tight[1]["rrf"] > loose[0]["rrf"] - loose[1]["rrf"]
