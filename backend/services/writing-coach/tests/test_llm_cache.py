"""LCO Phase2/3 — L2 Milvus 语义缓存 + L3 Neo4j 知识图谱 单元测试。

覆盖:
- L2 disabled 跳过 search(env L2_ENABLED=false)
- L2 enabled 正常 embed + milvus search 命中/未命中
- L2 任意异常降级 return None(CLC nil-safe)
- L3 driver nil/None 不崩(CLC:driver 初始化失败置 disabled,l3_record/l3_enhance 安全)
- LLMCache 三层串联:L1 hit 不查 L2;L1 miss + L2 hit 返回 L2;全 miss 调 call_fn 并 persist
"""
from __future__ import annotations

import json
import os
import sys
from unittest.mock import AsyncMock, MagicMock, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest

# ---------- L2 语义缓存测试 ----------

def _make_resp(status_code: int, payload: dict):
    """构造 httpx 假响应。"""
    r = MagicMock()
    r.status_code = status_code
    r.raise_for_status = MagicMock()
    r.json = MagicMock(return_value=payload)
    return r


@pytest.mark.asyncio
async def test_l2_disabled_skips_search(monkeypatch):
    """L2_ENABLED=false → l2_search 直接 return None,不调网络。"""
    from app.semantic import L2SemanticCache
    monkeypatch.setenv("L2_ENABLED", "false")
    cache = L2SemanticCache()
    assert cache.enabled is False
    # patch httpx 确保不被调用
    with patch("app.semantic.httpx.AsyncClient") as mock_client:
        result = await cache.l2_search("any prompt", model="m", scene="s")
        assert result is None
        assert mock_client.call_count == 0  # disabled 不发起任何请求


@pytest.mark.asyncio
async def test_l2_enabled_hit_returns_response(monkeypatch):
    """L2 enabled + embed ok + milvus 返回 distance≥radius 的行 → 返回 response。"""
    from app.semantic import L2SemanticCache
    monkeypatch.setenv("L2_ENABLED", "true")
    monkeypatch.setenv("MILVUS_ADDR", "milvus:19530")
    monkeypatch.setenv("EMBEDDING_URL", "http://embed/embed")
    cache = L2SemanticCache()
    assert cache.enabled is True

    embed_resp = _make_resp(200, {"vector": [0.1] * 384})
    search_resp = _make_resp(200, {"code": 0, "data": [{"response": "cached plan", "scene": "gen_plan", "model": "m"}]})

    call_count = 0
    async def fake_post(self, url, json=None, **kw):
        nonlocal call_count
        call_count += 1
        if "embed" in url:
            return embed_resp
        return search_resp

    with patch("app.semantic.httpx.AsyncClient") as mock_client:
        inst = MagicMock()
        inst.post = lambda url, json=None, **kw: fake_post(inst, url, json=json, **kw)
        # make async context manager
        am = MagicMock()
        am.__aenter__ = AsyncMock(return_value=inst)
        am.__aexit__ = AsyncMock(return_value=None)
        mock_client.return_value = am
        result = await cache.l2_search("hello world", model="m", scene="gen_plan")
        assert result == "cached plan"
        assert call_count == 2  # embed + search


@pytest.mark.asyncio
async def test_l2_embed_failure_degrades_to_none(monkeypatch):
    """embed 抛异常 → l2_search 降级 return None,不抛(CLC/降级安全)。"""
    from app.semantic import L2SemanticCache
    monkeypatch.setenv("L2_ENABLED", "true")
    cache = L2SemanticCache()

    with patch("app.semantic.httpx.AsyncClient") as mock_client:
        am = MagicMock()
        am.__aenter__ = AsyncMock(side_effect=RuntimeError("network down"))
        am.__aexit__ = AsyncMock(return_value=None)
        mock_client.return_value = am
        result = await cache.l2_search("hello", model="m", scene="s")
        assert result is None  # 降级不抛


@pytest.mark.asyncio
async def test_l2_milvus_empty_data_returns_none(monkeypatch):
    """milvus 返回空 data → return None(未命中)。"""
    from app.semantic import L2SemanticCache
    monkeypatch.setenv("L2_ENABLED", "true")
    cache = L2SemanticCache()

    async def fake_post(self, url, json=None, **kw):
        if "embed" in url:
            return _make_resp(200, {"vector": [0.1] * 384})
        return _make_resp(200, {"code": 0, "data": []})

    with patch("app.semantic.httpx.AsyncClient") as mock_client:
        inst = MagicMock()
        inst.post = lambda url, json=None, **kw: fake_post(inst, url, json=json, **kw)
        am = MagicMock()
        am.__aenter__ = AsyncMock(return_value=inst)
        am.__aexit__ = AsyncMock(return_value=None)
        mock_client.return_value = am
        result = await cache.l2_search("hello", model="m", scene="s")
        assert result is None


@pytest.mark.asyncio
async def test_l2_scene_mismatch_returns_none(monkeypatch):
    """命中但 scene 不匹配 → return None(避免跨场景串味)。"""
    from app.semantic import L2SemanticCache
    monkeypatch.setenv("L2_ENABLED", "true")
    cache = L2SemanticCache()

    async def fake_post(self, url, json=None, **kw):
        if "embed" in url:
            return _make_resp(200, {"vector": [0.1] * 384})
        return _make_resp(200, {"code": 0, "data": [{"response": "r", "scene": "other_scene", "model": "m"}]})

    with patch("app.semantic.httpx.AsyncClient") as mock_client:
        inst = MagicMock()
        inst.post = lambda url, json=None, **kw: fake_post(inst, url, json=json, **kw)
        am = MagicMock()
        am.__aenter__ = AsyncMock(return_value=inst)
        am.__aexit__ = AsyncMock(return_value=None)
        mock_client.return_value = am
        result = await cache.l2_search("hello", model="m", scene="gen_plan")
        assert result is None


# ---------- L3 知识图谱测试(CLC nil-safe)----------

@pytest.mark.asyncio
async def test_l3_driver_none_does_not_crash(monkeypatch):
    """CLC:driver 初始化失败(置 disabled)→ l3_record/l3_enhance 安全不崩。"""
    from app.knowledge import L3KnowledgeGraph
    monkeypatch.setenv("L3_ENABLED", "true")
    monkeypatch.setenv("NEO4J_URL", "bolt://neo4j:7687")
    kg = L3KnowledgeGraph()
    # 强制 driver 初始化失败 → disabled + _driver=None(CLC nil-safe)
    with patch("neo4j.GraphDatabase.driver", side_effect=RuntimeError("cannot connect")):
        # l3_record 不应抛(降级),且 driver 仍 None
        await kg.l3_record("prompt", "response", model="m", scene="s")
        assert kg._driver is None
        assert kg.enabled is False  # driver init 失败后 disabled
    # disabled 后再次调用也安全
    r = await kg.l3_enhance("prompt")
    assert r is None


@pytest.mark.asyncio
async def test_l3_disabled_skips(monkeypatch):
    """L3_ENABLED=false → l3_record/l3_enhance 直接跳过,不建 driver。"""
    from app.knowledge import L3KnowledgeGraph
    monkeypatch.setenv("L3_ENABLED", "false")
    kg = L3KnowledgeGraph()
    assert kg.enabled is False
    with patch("neo4j.GraphDatabase.driver") as mock_drv:
        await kg.l3_record("p", "r", model="m", scene="s")
        assert mock_drv.call_count == 0
        r = await kg.l3_enhance("p")
        assert r is None


@pytest.mark.asyncio
async def test_l3_empty_prompt_skips(monkeypatch):
    """CLC:空 prompt → l3_record/l3_enhance 直接跳过(不建连不发查询)。"""
    from app.knowledge import L3KnowledgeGraph
    monkeypatch.setenv("L3_ENABLED", "true")
    kg = L3KnowledgeGraph()
    with patch("neo4j.GraphDatabase.driver") as mock_drv:
        await kg.l3_record("", "r", model="m", scene="s")
        await kg.l3_record("p", "", model="m", scene="s")
        r = await kg.l3_enhance("")
        assert r is None
        assert mock_drv.call_count == 0  # 空输入根本不建 driver


# ---------- LLMCache 三层串联测试 ----------

class _FakeRedis:
    """简易异步 Redis mock。"""
    def __init__(self):
        self.store = {}
    async def get(self, k):
        return self.store.get(k)
    async def setex(self, k, ttl, v):
        self.store[k] = v


@pytest.mark.asyncio
async def test_llmcache_l1_hit_skips_l2(monkeypatch):
    """L1 命中 → 不查 L2(call_fn 不调)。"""
    from app.llm import LLMCache
    redis = _FakeRedis()
    redis.store["k1"] = json.dumps("l1 cached")
    cache = LLMCache(redis, ttl=100)
    call_fn = AsyncMock(return_value="should not be called")

    # patch l2 search 确保不被调
    if cache.l2 is not None:
        cache.l2.l2_search = AsyncMock(return_value=None)

    result = await cache.get_or_call("k1", call_fn, prompt="p", model="m", scene="s")
    assert result == "l1 cached"
    assert call_fn.call_count == 0
    if cache.l2 is not None:
        assert cache.l2.l2_search.call_count == 0


@pytest.mark.asyncio
async def test_llmcache_l1_miss_l2_hit(monkeypatch):
    """L1 miss → L2 hit → 返回 L2 结果,不调 call_fn。"""
    from app.llm import LLMCache
    redis = _FakeRedis()
    cache = LLMCache(redis, ttl=100)
    call_fn = AsyncMock(return_value="should not be called")

    # L2 mock 命中
    if cache.l2 is not None:
        cache.l2.l2_search = AsyncMock(return_value="l2 semantic hit")
    cache.l3 = None  # 简化:l3 不测

    result = await cache.get_or_call("missing_key", call_fn, prompt="p", model="m", scene="s")
    assert result == "l2 semantic hit"
    assert call_fn.call_count == 0
    if cache.l2 is not None:
        assert cache.l2.l2_search.call_count == 1
    # L2 命中应回填 L1
    assert redis.store.get("missing_key") is not None


@pytest.mark.asyncio
async def test_llmcache_all_miss_calls_fn_and_persists(monkeypatch):
    """L1+L2 都 miss → 调 call_fn → 返回结果 + 持久化 L1/L2/L3。"""
    from app.llm import LLMCache
    redis = _FakeRedis()
    cache = LLMCache(redis, ttl=100)
    call_fn = AsyncMock(return_value="fresh llm result")

    if cache.l2 is not None:
        cache.l2.l2_search = AsyncMock(return_value=None)
        cache.l2.l2_store = AsyncMock()
    cache.l3 = MagicMock()
    cache.l3.l3_record = AsyncMock()

    result = await cache.get_or_call("miss_key", call_fn, prompt="p", model="m", scene="s")
    assert result == "fresh llm result"
    assert call_fn.call_count == 1
    # L1 持久化
    assert json.loads(redis.store["miss_key"]) == "fresh llm result"
    # L2 store 调用
    if cache.l2 is not None:
        assert cache.l2.l2_store.call_count == 1
    # L3 record 调用
    assert cache.l3.l3_record.call_count == 1


@pytest.mark.asyncio
async def test_llmcache_persist_degrades_on_l2_failure(monkeypatch):
    """L2 store 抛异常 → 不影响 L1 set / L3 record(降级安全)。"""
    from app.llm import LLMCache
    redis = _FakeRedis()
    cache = LLMCache(redis, ttl=100)

    if cache.l2 is not None:
        cache.l2.l2_search = AsyncMock(return_value=None)
        cache.l2.l2_store = AsyncMock(side_effect=RuntimeError("milvus down"))
    cache.l3 = MagicMock()
    cache.l3.l3_record = AsyncMock()

    call_fn = AsyncMock(return_value="result")
    # 不应抛(L2 失败被降级吞掉)
    result = await cache.get_or_call("k2", call_fn, prompt="p", model="m", scene="s")
    assert result == "result"
    # L1 仍写入
    assert json.loads(redis.store["k2"]) == "result"
    # L3 仍被调用(L2 失败不影响 L3)
    assert cache.l3.l3_record.call_count == 1
