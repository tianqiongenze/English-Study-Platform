"""LLMCache 单元测试(Phase1 统一 key 规范 + 双写迁移)。

验证:
1. cache_key 产出新统一规范 esp:cache:llm:exact:{model}:{sha256前16}
2. legacy_cache_key 产出旧 llm:cache:{full sha256}
3. get_or_call:新 key MISS → 调 call_fn → 双写新+旧 key
4. get_or_call:新 key HIT → 直接返回,不调 call_fn
5. 双写迁移:旧 key 有值 → 回填新 key 并返回旧值
"""
import asyncio
import json
import hashlib

from app.llm import LLMCache


class FakeRedis:
    """最小异步 fake redis(仅 get/setex/exists),构造函数注入 LLMCache。"""

    def __init__(self, store: dict[str, str] | None = None):
        self.store = store or {}
        self.get_calls = 0
        self.setex_calls = 0

    async def get(self, key: str):
        self.get_calls += 1
        return self.store.get(key)

    async def setex(self, key: str, ttl: int, value: str):
        self.setex_calls += 1
        self.store[key] = value
        return True


def test_cache_key_uses_unified_spec():
    """新 key 符合统一规范:esp:cache:llm:exact:{model}:{16位hex}。"""
    cache = LLMCache(redis_client=None)
    key = cache.cache_key(prompt="hello", model="claude-sonnet-4-6", system="sys")
    assert key.startswith("esp:cache:llm:exact:claude-sonnet-4-6:")
    digest_part = key.split(":")[-1]
    assert len(digest_part) == 16
    # 确定性:相同输入相同 key
    assert cache.cache_key(prompt="hello", model="claude-sonnet-4-6", system="sys") == key


def test_cache_key_model_isolated():
    """不同 model 产出不同 key(模型隔离)。"""
    cache = LLMCache(redis_client=None)
    k1 = cache.cache_key(prompt="hello", model="m1")
    k2 = cache.cache_key(prompt="hello", model="m2")
    assert k1 != k2


def test_legacy_cache_key_uses_old_spec():
    """旧 key:llm:cache:{full sha256}。"""
    cache = LLMCache(redis_client=None)
    key = cache.legacy_cache_key(prompt="hello", model="claude-sonnet-4-6", system="sys")
    assert key.startswith("llm:cache:")
    digest = key[len("llm:cache:"):]
    assert len(digest) == 64  # 完整 sha256


def test_get_or_call_miss_writes_both_keys():
    """新 key MISS → 调 call_fn → 双写新+旧 key。"""
    fake = FakeRedis()
    cache = LLMCache(redis_client=fake)

    async def call_fn():
        return {"score": 22}

    legacy = cache.legacy_cache_key(prompt="hello", model="m1")
    new = cache.cache_key(prompt="hello", model="m1")
    result = asyncio.run(cache.get_or_call(new, call_fn, legacy_key=legacy))
    assert result == {"score": 22}
    assert fake.store.get(new) == json.dumps({"score": 22})
    assert fake.store.get(legacy) == json.dumps({"score": 22})
    assert fake.setex_calls == 2  # 新+旧


def test_get_or_call_hit_new_key_skips_call():
    """新 key HIT → 直接返回,不调 call_fn。"""
    fake = FakeRedis(store={"esp:cache:llm:exact:m1:abcd1234": json.dumps({"x": 1})})
    cache = LLMCache(redis_client=fake)
    called = False

    async def call_fn():
        nonlocal called
        called = True
        return {"should": "not happen"}

    key = "esp:cache:llm:exact:m1:abcd1234"
    result = asyncio.run(cache.get_or_call(key, call_fn, legacy_key="llm:cache:deadbeef"))
    assert result == {"x": 1}
    assert called is False


def test_get_or_call_legacy_hit_backfills_new():
    """双写迁移:旧 key 命中 → 回填新 key 并返回旧值。"""
    legacy = "llm:cache:legacyfullsha256hashplaceholder0000000000000000"
    new = "esp:cache:llm:exact:m1:newdigest16chars"
    fake = FakeRedis(store={legacy: json.dumps({"migrated": True})})
    cache = LLMCache(redis_client=fake)

    async def call_fn():
        raise AssertionError("不应调用 call_fn,旧 key 应命中")

    result = asyncio.run(cache.get_or_call(new, call_fn, legacy_key=legacy))
    assert result == {"migrated": True}
    # 新 key 被回填
    assert fake.store.get(new) == json.dumps({"migrated": True})


def test_get_or_call_no_legacy_key():
    """legacy_key=None → 仅新 key 读写(不双写)。"""
    fake = FakeRedis()
    cache = LLMCache(redis_client=fake)

    async def call_fn():
        return "reply"

    new = cache.cache_key(prompt="hello", model="m1")
    result = asyncio.run(cache.get_or_call(new, call_fn, legacy_key=None))
    assert result == "reply"
    assert fake.store.get(new) == json.dumps("reply")
    assert fake.setex_calls == 1  # 仅新 key


def test_get_or_call_redis_none_passthrough():
    """redis_client=None → 直穿 call_fn,不读写。"""
    cache = LLMCache(redis_client=None)

    async def call_fn():
        return {"ok": True}

    result = asyncio.run(cache.get_or_call("anykey", call_fn, legacy_key="any"))
    assert result == {"ok": True}
