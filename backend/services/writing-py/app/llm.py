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
    - 单节点部署,进程内 asyncio.Semaphore 即可,无需 Redis 共享计数。
    """

    def __init__(self, initial_concurrency: int | None = None):
        self.initial = initial_concurrency or int(os.getenv("WRITER_LLM_MAX_CONCURRENCY", "10"))
        self.max_concurrency = self.initial
        self.semaphore = asyncio.Semaphore(self.initial)
        self.min_rtt = float("inf")
        self._lock = asyncio.Lock()

    async def acquire(self) -> None:
        await self.semaphore.acquire()

    async def release(self, rtt: float) -> None:
        self.semaphore.release()
        async with self._lock:
            if rtt <= 0:
                return
            if rtt < self.min_rtt * 1.5:
                # RTT 低,可以放更多并发(最多初始值的 2 倍)
                self.min_rtt = min(self.min_rtt, rtt)
                target = min(self.initial * 2, self.max_concurrency + 1)
                if self.semaphore._value > 0 and target > self.max_concurrency:
                    self.max_concurrency = target
                    # 扩容:创建新信号量(原有等待者不受影响)
                    self.semaphore = asyncio.Semaphore(self.max_concurrency)
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

    def cache_key(self, prompt: str, model: str = "", **kwargs) -> str:
        raw = json.dumps({"prompt": prompt, "model": model, **kwargs}, sort_keys=True)
        return f"llm:cache:{hashlib.sha256(raw.encode()).hexdigest()}"

    async def get_or_call(self, key: str, call_fn):
        if self.redis is None:
            return await call_fn()
        try:
            cached = await self.redis.get(key)
            if cached:
                logger.debug("llm cache hit: %s", key[:32])
                return json.loads(cached)
        except Exception as e:
            logger.warning("llm cache read failed (non-fatal): %s", e)
        result = await call_fn()
        try:
            await self.redis.setex(key, self.ttl, json.dumps(result))
        except Exception as e:
            logger.warning("llm cache write failed (non-fatal): %s", e)
        return result


# 全局缓存实例(进程级单例)
_global_cache: LLMCache | None = None


def get_cache() -> LLMCache | None:
    """获取 LLM 缓存实例。CACHE_ENABLED=false 或 Redis 不可用时返回 None。"""
    global _global_cache
    if _global_cache is not None:
        return _global_cache
    cache_enabled = os.getenv("CACHE_ENABLED", "true").lower() != "false"
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

        # LLM 缓存:相同 prompt+model 命中直返
        cache = get_cache()
        if cache is not None:
            cache_key = cache.cache_key(user, model=self.model, system=system)

            async def _call():
                return await self._raw_call(body, headers)

            cached = await cache.get_or_call(cache_key, _call)
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
