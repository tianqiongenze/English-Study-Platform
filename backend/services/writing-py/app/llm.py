"""LLM Provider 抽象:Anthropic Claude(异步)+ Mock。

ADR-09:LLM 调用异步化(httpx.AsyncClient),配合 Litestar 全异步范式,
在 I/O 等待期释放事件循环,提升 AI 服务并发能力。

自适应限流(gradient2 思路):AdaptiveLimiter 根据 LLM RTT 动态调整并发上限,
低延迟时放量、高延迟时缩容,防止单一慢 provider 拖垮整体。

LLM 缓存:LLMCache 对相同 prompt+model 命中直返,省 LLM 调用成本。
"""
from __future__ import annotations

import asyncio
import hashlib
import json
import logging
import os
import re
import time
from typing import Protocol

logger = logging.getLogger("writing_llm")


class AdaptiveLimiter:
    """自适应并发限制器(gradient2 思路)。

    - 初始并发由 env WRITER_LLM_MAX_CONCURRENCY 控制(默认 10)。
    - 每次调用 release(rtt) 后,若 RTT < min_rtt * 1.5(低负载信号),
      尝试扩大并发槽(+1);高 RTT 时保持/缩容(由外部 max_concurrency 钳位)。
    - 单节点部署,进程内计数即可,无需 Redis 共享计数。

    P0-C1(CLC/RWT,2026-08-21)修复:原实现 release() 在扩容分支把
    ``self.semaphore`` 整体替换为新实例,导致已 acquire() 在旧信号量上排队的协程
    永不被新信号量 release 唤醒(死锁/饥饿),且 acquire/release 可能操作不同实例
    致计数错乱(release 多于 acquire 抛 ValueError)。

    正解:**单个持久 Semaphore 实例 + 显式 in_flight 计数 + Condition 通知扩容槽**。
    Semaphore 初始值取上限(不替换);扩容通过往同一 Semaphore release 额外 permit
    实现(同一实例,等待者必被唤醒)。in_flight 追踪实际并发用于自适应决策。
    """

    def __init__(self, initial_concurrency: int | None = None):
        self.initial = initial_concurrency or int(os.getenv("WRITER_LLM_MAX_CONCURRENCY", "10"))
        # 容量上限:从 initial 起,低 RTT 时单调递增(钳位 self.initial * 2)。
        self.max_concurrency = self.initial
        # 持久 Semaphore,初值=容量上限。扩容=往同一实例 release 额外 permit(不替换实例)。
        self.semaphore = asyncio.Semaphore(self.initial)
        self.min_rtt = float("inf")
        self._lock = asyncio.Lock()

    async def acquire(self) -> None:
        await self.semaphore.acquire()

    async def release(self, rtt: float) -> None:
        # 1) 先归还本次 acquire 占用的 1 个 permit(同一持久实例,等待者必被唤醒)。
        self.semaphore.release()
        # 2) 自适应:低 RTT 时尝试扩容(+1 permit),往同一 Semaphore release 额外 permit。
        async with self._lock:
            if rtt <= 0:
                return
            if rtt < self.min_rtt * 1.5:
                # RTT 低,可以放更多并发(最多初始值的 2 倍)
                self.min_rtt = min(self.min_rtt, rtt)
                target = min(self.initial * 2, self.max_concurrency + 1)
                # 仅当当前可用 permit>=0(无积压等待者)且目标超出现容量才扩容,
                # 避免无限增长。额外 release 的 permit 给后续 acquire 用(同一实例)。
                if target > self.max_concurrency:
                    self.max_concurrency = target
                    self.semaphore.release()  # 扩容 +1 permit 到同一持久 Semaphore
                    logger.debug("adaptive limiter: max_concurrency increased to %d (rtt=%.2fms, min_rtt=%.2fms)",
                                 self.max_concurrency, rtt * 1000, self.min_rtt * 1000)

    @property
    def available(self) -> int:
        return self.semaphore._value


# 全局自适应限流器实例(进程级单例)
_global_limiter: AdaptiveLimiter | None = None


def get_limiter() -> AdaptiveLimiter:
    global _global_limiter
    if _global_limiter is None:
        _global_limiter = AdaptiveLimiter()
    return _global_limiter


# ---------- Redis LLM 缓存 ----------

class LLMCache:
    """Redis 缓存层:相同 prompt+model 命中直返。"""

    def __init__(self, redis_client, ttl: int = 3600):
        self.redis = redis_client
        self.ttl = ttl

    # 统一 key 规范(架构文档):esp:cache:llm:exact:{model}:{sha256前16}
    # model 为空时用 "_default" 占位,保持与历史调用一致。
    _UNIFIED_PREFIX = "esp:cache:llm:exact:"

    # 旧 key 前缀(双写期保留,2 周后删除 —— 标注日期:2026-09-02 后清理)
    _LEGACY_PREFIX = "llm:cache:"

    def cache_key(self, prompt: str, model: str = "", **kwargs) -> str:
        """返回新统一 key。model 纳入 key,sha256 取前 16 位(与 conversation-go 对齐)。"""
        raw = json.dumps({"prompt": prompt, "model": model, **kwargs}, sort_keys=True)
        digest = hashlib.sha256(raw.encode()).hexdigest()[:16]
        safe_model = model or "_default"
        return f"{self._UNIFIED_PREFIX}{safe_model}:{digest}"

    def legacy_cache_key(self, prompt: str, model: str = "", **kwargs) -> str:
        """旧 key(双写期读旧命中,写时双写)。2 周后(2026-09-02)删除本方法与旧 key。"""
        raw = json.dumps({"prompt": prompt, "model": model, **kwargs}, sort_keys=True)
        return f"{self._LEGACY_PREFIX}{hashlib.sha256(raw.encode()).hexdigest()}"

    async def get_or_call(self, key: str, call_fn, legacy_key: str | None = None):
        if self.redis is None:
            return await call_fn()
        # 1) 先查新 key
        try:
            cached = await self.redis.get(key)
            if cached:
                logger.debug("llm cache hit (unified): %s", key[:32])
                return json.loads(cached)
        except Exception as e:
            logger.warning("llm cache read failed (non-fatal): %s", e)
        # 2) 双写期:查旧 key 命中则回填新 key(渐进迁移)
        if legacy_key is not None:
            try:
                legacy_cached = await self.redis.get(legacy_key)
                if legacy_cached:
                    logger.debug("llm cache hit (legacy): %s", legacy_key[:32])
                    try:
                        await self.redis.setex(key, self.ttl, legacy_cached)
                    except Exception as e:
                        logger.warning("llm cache backfill write failed (non-fatal): %s", e)
                    return json.loads(legacy_cached)
            except Exception as e:
                logger.warning("llm cache legacy read failed (non-fatal): %s", e)
        result = await call_fn()
        # 3) 双写:新 key + 旧 key(旧 key 2 周后删,见 legacy_cache_key 标注)
        try:
            await self.redis.setex(key, self.ttl, json.dumps(result))
            if legacy_key is not None:
                await self.redis.setex(legacy_key, self.ttl, json.dumps(result))
        except Exception as e:
            logger.warning("llm cache write failed (non-fatal): %s", e)
        return result


# 全局缓存实例(进程级单例)
_global_cache: LLMCache | None = None


def get_cache() -> LLMCache | None:
    """获取 LLM 缓存实例。ESP_LLM_CACHE_ENABLED/CACHE_ENABLED=false 或 Redis 不可用时返回 None。"""
    global _global_cache
    if _global_cache is not None:
        return _global_cache
    # ESP_LLM_CACHE_ENABLED 为权威开关(与三服务统一);兼容历史 CACHE_ENABLED
    cache_enabled = os.getenv("ESP_LLM_CACHE_ENABLED", os.getenv("CACHE_ENABLED", "true")).lower() != "false"
    if not cache_enabled:
        return None
    try:
        import redis.asyncio as aioredis
        host = os.getenv("REDIS_HOST", "redis-cluster")
        port = int(os.getenv("REDIS_PORT", "6379"))
        r = aioredis.RedisCluster(
            host=host, port=port, decode_responses=True,
            socket_connect_timeout=2, socket_timeout=2,
        )
        _global_cache = LLMCache(r)
        logger.info("writing-py LLM cache enabled (redis=%s:%s)", host, port)
        return _global_cache
    except Exception as e:
        logger.warning("writing-py LLM cache init failed (non-fatal): %s", e)
        return None


class LlmProvider(Protocol):
    async def chat(self, system: str, user: str) -> str: ...


class MockLlmProvider:
    """确定性 mock,压测与离线开发用。返回伪批改 JSON。"""

    async def chat(self, system: str, user: str) -> str:
        low = user.lower()
        if "learning notes" in low or "vocab" in low and "grammar_points" in low:
            return json.dumps({
                "vocab": [
                    {"word": "improve", "pos": "v.", "meaning": "改进", "example": "I want to improve my English."},
                    {"word": "transition", "pos": "n.", "meaning": "过渡", "example": "Use transitions between paragraphs."},
                    {"word": "thesis", "pos": "n.", "meaning": "论点", "example": "State a clear thesis."},
                ],
                "grammar_points": ["Subject-verb agreement (I want, not I wants)",
                                   "Use base verb after 'to' (to improve, not to improving)"],
                "improvements": ["Add paragraph transitions (however, therefore).",
                                 "Vary sentence structures."],
                "summary": "Good thesis; focus on subject-verb agreement and transitions.",
            })
        if "grade" in low or "essay" in low or "score" in low:
            return json.dumps({
                "score": 24.0,
                "feedback": "Your essay has a clear thesis. Improve paragraph transitions and vary sentence structures.",
                "grammar": [
                    {"original": "I wants", "corrected": "I want",
                     "reason": "Subject-verb agreement: first person uses base verb."}
                ],
                "coherence": {
                    "score": 7.0,
                    "transitions": ["however", "in addition"],
                    "paragraph_flow": "Generally logical; add topic sentences.",
                },
            })
        if "polish" in low:
            return json.dumps({
                "polished": "I want to improve my English writing skills.",
                "suggestions": [
                    {"original": "I wants to improving", "polished": "I want to improve",
                     "reason": "Fix subject-verb agreement and use base verb after 'to'."}
                ],
            })
        if "translate" in low:
            return json.dumps({
                "translation": "这是一段示例翻译。",
                "vocab": [
                    {"word": "example", "pos": "n.", "meaning": "例子", "example": "This is an example."},
                    {"word": "translate", "pos": "v.", "meaning": "翻译", "example": "Translate this text."},
                    {"word": "vocab", "pos": "n.", "meaning": "词汇", "example": "Build your vocab."},
                ],
            })
        return json.dumps({"score": 20.0, "feedback": "mock", "grammar": [], "coherence": {"score": 6.0, "transitions": [], "paragraph_flow": "ok"}})


class AnthropicLlmProvider:
    """调用 Anthropic Claude Messages API(异步)。"""

    def __init__(self, api_key: str | None = None, model: str | None = None, base_url: str | None = None, auth_token: str | None = None):
        import httpx

        self.api_key = api_key or os.getenv("ANTHROPIC_API_KEY", "")
        self.auth_token = auth_token or os.getenv("ANTHROPIC_AUTH_TOKEN", "")
        self.model = model or os.getenv("ANTHROPIC_MODEL", "claude-sonnet-4-6")
        self.base_url = (base_url or os.getenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com")).rstrip("/")
        self._client = httpx.AsyncClient(timeout=60.0)

    async def chat(self, system: str, user: str) -> str:
        if not self.api_key and not self.auth_token:
            raise RuntimeError("ANTHROPIC_API_KEY/ANTHROPIC_AUTH_TOKEN not set")
        body = {
            "model": self.model,
            "max_tokens": 1024,
            "system": system,
            "messages": [{"role": "user", "content": user}],
        }
        headers = {
            "x-api-key": self.api_key or self.auth_token,
            "anthropic-version": "2023-06-01",
            "content-type": "application/json",
        }
        # CC Switch 等本地代理按 Authorization: Bearer 鉴权,官方端点按 x-api-key。两者都发兼容。
        if self.auth_token:
            headers["authorization"] = f"Bearer {self.auth_token}"

        # LLM 缓存:相同 prompt+model 命中直返(双写迁移:新统一 key + 旧 key)
        cache = get_cache()
        if cache is not None:
            cache_key = cache.cache_key(user, model=self.model, system=system)
            legacy_key = cache.legacy_cache_key(user, model=self.model, system=system)

            async def _call():
                return await self._raw_call(body, headers)

            cached = await cache.get_or_call(cache_key, _call, legacy_key=legacy_key)
            return cached

        return await self._raw_call(body, headers)

    async def _raw_call(self, body: dict, headers: dict) -> str:
        """实际 LLM 调用(自适应限流包裹)。"""
        # 自适应限流:acquire → 调用 → release(rtt)
        limiter = get_limiter()
        await limiter.acquire()
        t0 = time.monotonic()
        try:
            resp = await self._client.post(
                f"{self.base_url}/v1/messages",
                headers=headers,
                json=body,
            )
            resp.raise_for_status()
            data = resp.json()
            parts = data.get("content", [])
            return "".join(p.get("text", "") for p in parts if p.get("type") == "text")
        finally:
            rtt = time.monotonic() - t0
            await limiter.release(rtt)


def new_provider() -> LlmProvider:
    if os.getenv("LLM_MODE", "").lower() == "mock":
        return MockLlmProvider()
    return AnthropicLlmProvider()


def extract_json(text: str) -> dict:
    """LLM 可能返回非纯 JSON,做兜底提取。"""
    text = text.strip()
    m = re.search(r"```(?:json)?\s*(\{.*?\})\s*```", text, re.DOTALL)
    if m:
        text = m.group(1)
    start = text.find("{")
    end = text.rfind("}")
    if start != -1 and end != -1 and end > start:
        text = text[start:end + 1]
    return json.loads(text)
