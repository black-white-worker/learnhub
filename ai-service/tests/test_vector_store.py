"""向量库抽象层测试（对应 app/rag/vector_store.py）。

重点：增量重灌依赖的 ID 稳定性与唯一性、显式传 embeddings 避免维度不匹配、
delete_by_resource 按 resource_id 过滤、query 的三元组返回结构。
测试用 fake chromadb，不装 chromadb、不调 embedding API。
"""
import sys
import types
from unittest.mock import MagicMock, patch

import pytest

from app.rag import vector_store as vs


@pytest.fixture
def fake_chroma():
    coll = MagicMock()
    client = MagicMock()
    client.get_or_create_collection.return_value = coll
    mod = types.ModuleType("chromadb")
    mod.PersistentClient = MagicMock(return_value=client)
    with patch.dict(sys.modules, {"chromadb": mod}), \
         patch.object(vs.os, "makedirs"), \
         patch.object(vs, "embed_batch", return_value=[[0.1, 0.2]]):
        store = vs.ChromaStore()
    return store, coll


class TestUpsertIds:
    def test_same_text_in_different_resources_has_distinct_ids(self, fake_chroma):
        store, coll = fake_chroma
        with patch.object(vs, "embed_batch", return_value=[[0.1], [0.2]]):
            store.upsert(1, ["相同正文", "相同正文"],
                         [{"resource_id": 7}, {"resource_id": 8}])
        ids = coll.upsert.call_args.kwargs["ids"]
        assert len(ids) == len(set(ids))

    def test_duplicate_slices_in_same_resource_are_not_collapsed(self, fake_chroma):
        store, coll = fake_chroma
        with patch.object(vs, "embed_batch", return_value=[[0.1], [0.2]]):
            store.upsert(1, ["重复段落", "重复段落"],
                         [{"resource_id": 7}, {"resource_id": 7}])
        ids = coll.upsert.call_args.kwargs["ids"]
        assert len(ids) == 2 and ids[0] != ids[1]

    def test_ids_are_stable_for_identical_input(self, fake_chroma):
        store, coll = fake_chroma
        with patch.object(vs, "embed_batch", return_value=[[0.1]]):
            store.upsert(1, ["正文"], [{"resource_id": 7}])
            first = coll.upsert.call_args.kwargs["ids"]
            store.upsert(1, ["正文"], [{"resource_id": 7}])
            second = coll.upsert.call_args.kwargs["ids"]
        assert first == second

    def test_missing_metadata_does_not_crash(self, fake_chroma):
        store, coll = fake_chroma
        with patch.object(vs, "embed_batch", return_value=[[0.1], [0.2]]):
            store.upsert(1, ["A", "B"], [])
        ids = coll.upsert.call_args.kwargs["ids"]
        assert len(ids) == 2


class TestUpsertContract:
    def test_embeddings_are_passed_explicitly(self, fake_chroma):
        store, coll = fake_chroma
        with patch.object(vs, "embed_batch", return_value=[[0.3, 0.4]]) as emb:
            store.upsert(1, ["正文"], [{"resource_id": 7}])
        emb.assert_called_once_with(["正文"])
        kw = coll.upsert.call_args.kwargs
        assert kw["embeddings"] == [[0.3, 0.4]]
        assert kw["documents"] == ["正文"]
        assert kw["metadatas"] == [{"resource_id": 7}]

    def test_empty_chunks_skips_everything(self, fake_chroma):
        store, coll = fake_chroma
        with patch.object(vs, "embed_batch") as emb:
            store.upsert(1, [], [])
        emb.assert_not_called()
        coll.upsert.assert_not_called()


class TestDeleteAndQuery:
    def test_delete_by_resource_filters_on_metadata(self, fake_chroma):
        store, coll = fake_chroma
        store.delete_by_resource(1, 7)
        assert coll.delete.call_args.kwargs["where"] == {"resource_id": 7}

    def test_query_returns_triplets(self, fake_chroma):
        store, coll = fake_chroma
        coll.query.return_value = {
            "documents": [["片段A", "片段B"]],
            "metadatas": [[{"title": "讲义"}, {"title": "课件"}]],
            "distances": [[0.1, 0.3]],
        }
        out = store.query(1, [0.1, 0.2], top_k=2)
        assert out == [("片段A", {"title": "讲义"}, 0.1),
                       ("片段B", {"title": "课件"}, 0.3)]
        assert coll.query.call_args.kwargs["n_results"] == 2

    def test_query_no_hits_returns_empty(self, fake_chroma):
        store, coll = fake_chroma
        coll.query.return_value = {"documents": [[]], "metadatas": [[]], "distances": [[]]}
        assert store.query(1, [0.1], top_k=3) == []


class TestFactory:
    def test_get_vector_store_returns_chroma(self):
        mod = types.ModuleType("chromadb")
        mod.PersistentClient = MagicMock(return_value=MagicMock())
        with patch.dict(sys.modules, {"chromadb": mod}), \
             patch.object(vs.os, "makedirs"):
            assert isinstance(vs.get_vector_store(), vs.ChromaStore)
