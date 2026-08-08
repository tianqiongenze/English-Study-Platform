"""ESP 多级缓存公共包(Python 侧,与 Java 侧 esp-cache 语义对齐)。

Cache-Aside 模式:
  读:L1 cachetools(LRU 进程内) → L2 redis.asyncio(Redis Cluster) → loader(DB/LLM)
      L2/L1 命中直接返回;loader 回填 L2(set TTL)+ L1。
  空结果缓存短 TTL 防穿透(空 sentinel)。

feature flag:env CACHE_ENABLED=false 时全部 no-op,直调 loader(回退安全)。
L2 连接/异常降级为纯 L1(只 warn 不阻断)。

跨实例 L1 一致性:本进程 L1 不做主动广播失效(Python 服务多为无状态计算,
副本间靠 L2 + TTL 兜底;写侧 evict 删 L2 即可,各副本 L1 在 TTL 内自然过期)。
如需强一致可扩展 publish/subscribe,本期按 TTL 兜底。
"""
from __future__ import annotations

import functools
import json
import logging
import os
from typing import Any, Awaitable, Callable

try:
    from cachetools import LRUCache
except ImportError:  # 降级:用 dict(不淘汰,仅小规模/测试)
    LRUCache = None  # type: ignore

logger = logging.getLogger("esp_cache")

_ENABLED = os.getenv("CACHE_ENABLED", "true").lower() != "false"
_L1_MAX = int(os.getenv("CACHE_L1_MAX", "4096"))
_L2_TTL = int(os.getenv("CACHE_L2_TTL", "300"))
_EMPTY_TTL = int(os.getenv("CACHE_EMPTY_TTL", "30"))
_EMPTY_SENTINEL = "__ESP_CACHE_EMPTY__"

_l1: "LRUCache | dict" = LRUCache(maxsize=_L1_MAX) if LRUCache else {}
_redis = None  # 延迟初始化


async def _get_redis():
    global _redis
    if not _ENABLED:
        return None
    if _redis is None:
        try:
            import redis.asyncio as aioredis  # noqa: WPS433
            host = os.getenv("REDIS_HOST", "redis-cluster")
            port = int(os.getenv("REDIS_PORT", "6379"))
            # Redis Cluster 模式(经 ExternalName redis-cluster → redis-cluster-new,3主3从)
            _redis = aioredis.RedisCluster(host=host, port=port, decode_responses=True,
                                           socket_connect_timeout=1, socket_timeout=1)
            await _redis.ping()
        except Exception as exc:
            logger.warning("L2 redis init failed, degrade to L1-only: %s", exc)
            _redis = None
    return _redis


def _key(cache_name: str, *parts: Any) -> str:
    """明文拼接,对齐 Java esp-cache(CacheService.java 调用方拼 'learning:due:42' 等明文 key)。

    P1-6:去掉 sha256(json(parts))[:24],改为 cache_name:part1:part2...
    三端(Python/Go/Java)对同一实体产出相同 Redis key,跨语言缓存命中。
    """
    return f"{cache_name}:" + ":".join(str(p) for p in parts)


async def get_or_load(cache_name: str, parts: tuple,
                      loader: Callable[[], Awaitable[Any]],
                      cache_empty: bool = True,
                      ttl: int | None = None) -> Any:
    """读:L1 → L2 → loader。parts 构造 cache key 的输入元组。ttl 覆盖默认 L2 TTL。"""
    if not _ENABLED:
        return await loader()

    k = _key(cache_name, *parts)

    if k in _l1:
        return _l1[k]

    r = await _get_redis()
    if r is not None:
        try:
            raw = await r.get(k)
            if raw is not None:
                if raw == _EMPTY_SENTINEL:
                    _l1[k] = None
                    return None
                val = json.loads(raw)
                _l1[k] = val
                return val
        except Exception as exc:
            logger.warning("L2 read failed %s: %s", k, exc)

    val = await loader()
    if val is None and not cache_empty:
        return None

    if r is not None:
        try:
            payload = _EMPTY_SENTINEL if val is None else json.dumps(val, default=str, ensure_ascii=False)
            await r.setex(k, (ttl or (_EMPTY_TTL if val is None else _L2_TTL)), payload)
        except Exception as exc:
            logger.warning("L2 write failed %s: %s", k, exc)
    _l1[k] = val
    return val


async def evict(cache_name: str, *parts: Any) -> None:
    """失效:删 L2 + 本地 L1。多副本 L1 靠 TTL 兜底。"""
    if not _ENABLED:
        return
    k = _key(cache_name, *parts)
    _l1.pop(k, None)
    r = await _get_redis()
    if r is not None:
        try:
            await r.delete(k)
        except Exception as exc:
            logger.warning("L2 evict failed %s: %s", k, exc)


def cached(cache_name: str, extractor: Callable[..., tuple] | None = None):
    """装饰器:对 async 方法做 L1+L2 缓存。
    extractor(*args, **kwargs) -> tuple 返回构造 key 的输入元组;
    默认 (args, sorted(kwargs))。"""
    def deco(fn: Callable[..., Awaitable[Any]]):
        @functools.wraps(fn)
        async def wrapper(*args, **kwargs):
            if not _ENABLED:
                return await fn(*args, **kwargs)
            parts = extractor(*args, **kwargs) if extractor else (args, tuple(sorted(kwargs.items())))
            return await get_or_load(cache_name, parts, lambda: fn(*args, **kwargs))
        return wrapper
    return deco
