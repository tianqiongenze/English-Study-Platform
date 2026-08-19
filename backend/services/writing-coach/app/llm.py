"""Writing Coach: async LLM provider for improvement plan generation.

- 自适应限流(asyncio.Semaphore, gradient2 思路):低 RTT 放量、高 RTT 缩容。
- Redis 缓存:相同 prompt+model+scene 命中直返,省 LLM 调用成本。
"""
from __future__ import annotations

import asyncio
import hashlib
import json
import logging
import os
import time

logger = logging.getLogger("writing_coach_llm")

# ---------- 自适应限流(gradient2) ----------

class AdaptiveLimiter:
    """自适应并发限制器(gradient2 思路)。
    初始并发由 env WRITER_COACH_MAX_CONCURRENCY 控制(默认 5)。
    低 RTT 放量(最多 2x),高 RTT 保持。
    """

    def __init__(self, initial: int | None = None):
        self.initial = initial or int(os.getenv("WRITER_COACH_MAX_CONCURRENCY", "5"))
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
                self.min_rtt = min(self.min_rtt, rtt)
                target = min(self.initial * 2, self.max_concurrency + 1)
                if self.semaphore._value > 0 and target > self.max_concurrency:
                    self.max_concurrency = target
                    self.semaphore = asyncio.Semaphore(self.max_concurrency)
                    logger.debug("adaptive limiter: max=%d (rtt=%.2fms)", self.max_concurrency, rtt * 1000)

_limiter: AdaptiveLimiter | None = None

def _get_limiter() -> AdaptiveLimiter:
    global _limiter
    if _limiter is None:
        _limiter = AdaptiveLimiter()
    return _limiter

# ---------- Redis LLM 缓存 ----------

class LLMCache:
    """三层 LLM 缓存(LCO 铁律):
    L1 Redis 精确缓存(sha256 prompt+model)→ L2 Milvus 语义缓存(cosine≥0.92)→ L3 Neo4j 知识图谱。
    读:L1 miss → L2 search → 若仍 miss 则调 LLM;写:L1 set + L2 store + L3 record(fire-and-forget,降级安全)。
    """

    def __init__(self, redis_client, ttl: int = 3600):
        self.redis = redis_client
        self.ttl = ttl
        # L2 Milvus 语义缓存 + L3 Neo4j 知识图谱(LCO Phase2/3)。延迟初始化,失败降级不影响主链路。
        try:
            from .semantic import L2SemanticCache
            self.l2 = L2SemanticCache()
        except Exception as e:
            logger.warning("L2 semantic cache init failed (non-fatal, disabled): %s", e)
            self.l2 = None
        try:
            from .knowledge import L3KnowledgeGraph
            self.l3 = L3KnowledgeGraph()
        except Exception as e:
            logger.warning("L3 knowledge graph init failed (non-fatal, disabled): %s", e)
            self.l3 = None

    def cache_key(self, prompt: str, model: str = "", **kwargs) -> str:
        raw = json.dumps({"prompt": prompt, "model": model, **kwargs}, sort_keys=True)
        return f"llm:cache:{hashlib.sha256(raw.encode()).hexdigest()}"

    async def get_or_call(self, key: str, call_fn, prompt: str = "", model: str = "", scene: str = ""):
        """读路径:L1 → L2 → call_fn。prompt/model/scene 用于 L2/L3(可选,向后兼容)。"""
        # L1 Redis 精确缓存
        if self.redis is not None:
            try:
                cached = await self.redis.get(key)
                if cached:
                    logger.info("L1 hit: %s", key[:32])
                    return json.loads(cached)
            except Exception as e:
                logger.warning("llm cache read failed (non-fatal): %s", e)
        # L2 Milvus 语义缓存(CLC:l2 为 None 时跳过)
        if self.l2 is not None and prompt:
            try:
                l2_hit = await self.l2.l2_search(prompt, model=model, scene=scene)
                if l2_hit:
                    # 命中 L2 时回填 L1(热数据加速下次 L1 直中)
                    if self.redis is not None:
                        try:
                            await self.redis.setex(key, self.ttl, json.dumps(l2_hit))
                        except Exception:
                            pass
                    return l2_hit
            except Exception as e:
                logger.warning("L2 search failed (non-fatal, degrade): %s", e)
        # 未命中,调真实 LLM
        result = await call_fn()
        # 写路径:L1 set + L2 store + L3 record(fire-and-forget,降级安全)
        await self._persist(key, result, prompt=prompt, model=model, scene=scene)
        return result

    async def _persist(self, key: str, result, prompt: str = "", model: str = "", scene: str = "") -> None:
        """L1 set + L2 store + L3 record。任一失败不影响其他(降级安全)。"""
        # L1 Redis
        if self.redis is not None:
            try:
                await self.redis.setex(key, self.ttl, json.dumps(result))
            except Exception as e:
                logger.warning("llm cache write failed (non-fatal): %s", e)
        # L2 Milvus store(CLC:l2 None 守卫)
        if self.l2 is not None and prompt and result:
            try:
                await self.l2.l2_store(prompt, str(result), model=model, scene=scene)
            except Exception as e:
                logger.warning("L2 store failed (non-fatal): %s", e)
        # L3 Neo4j record(CLC:l3 None 守卫)
        if self.l3 is not None and prompt and result:
            try:
                await self.l3.l3_record(prompt, str(result), model=model, scene=scene)
            except Exception as e:
                logger.warning("L3 record failed (non-fatal): %s", e)


# ---------- Provider ----------

def new_provider():
    """Return an async callable(prompt, max_tokens) -> str. Real if ANTHROPIC_API_KEY set; mock only if LLM_MODE=mock."""
    key = os.getenv("ANTHROPIC_API_KEY", "")
    auth_token = os.getenv("ANTHROPIC_AUTH_TOKEN", "")
    mode = os.getenv("LLM_MODE", "").lower()

    # Redis 缓存(CACHE_ENABLED=false 时 no-op)
    cache_enabled = os.getenv("CACHE_ENABLED", "true").lower() != "false"
    cache: LLMCache | None = None
    if cache_enabled:
        try:
            import redis.asyncio as aioredis
            host = os.getenv("REDIS_HOST", "redis-cluster")
            port = int(os.getenv("REDIS_PORT", "6379"))
            r = aioredis.RedisCluster(
                host=host, port=port, decode_responses=True,
                socket_connect_timeout=2, socket_timeout=2,
            )
            cache = LLMCache(r)
            logger.info("writing-coach LLM cache enabled (redis=%s:%s)", host, port)
        except Exception as e:
            logger.warning("writing-coach LLM cache init failed (non-fatal): %s", e)

    if mode != "mock" and (key or auth_token):
        try:
            import httpx

            model = os.getenv("ANTHROPIC_MODEL", "claude-sonnet-4-6")
            base_url = os.getenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com").rstrip("/")

            async def _real(prompt: str, max_tokens: int = 1024) -> str:
                async def _call():
                    limiter = _get_limiter()
                    await limiter.acquire()
                    t0 = time.monotonic()
                    try:
                        headers = {
                            "x-api-key": key or auth_token,
                            "anthropic-version": "2023-06-01",
                            "content-type": "application/json",
                        }
                        if auth_token:
                            headers["authorization"] = f"Bearer {auth_token}"
                        async with httpx.AsyncClient(timeout=60.0) as client:
                            resp = await client.post(
                                f"{base_url}/v1/messages",
                                headers=headers,
                                json={
                                    "model": model,
                                    "max_tokens": max_tokens,
                                    "messages": [{"role": "user", "content": prompt}],
                                },
                            )
                            resp.raise_for_status()
                            data = resp.json()
                            return data["content"][0]["text"]
                    finally:
                        rtt = time.monotonic() - t0
                        await limiter.release(rtt)

                if cache is not None:
                    key_str = cache.cache_key(prompt, model=model)
                    return await cache.get_or_call(key_str, _call, prompt=prompt, model=model, scene="gen_plan")
                return await _call()

            return _real
        except Exception as e:
            logger.warning("writing-coach httpx provider init failed (non-fatal, falling back to mock): %s", e)

    # Mock fallback(同步包装为异步)
    def _mock(prompt: str, max_tokens: int = 1024) -> str:
        return (
            "【写作能力提升方案(Mock)】\n"
            "阶段1(1-2周):夯实语法——每日 1 篇改错练习,聚焦主谓一致/时态。\n"
            "阶段2(3-4周):段落连贯——学习 topic sentence + 过渡词,每两天写 1 段。\n"
            "阶段3(5-6周):词汇升级——替换 20 个高频词为学术词;每周完成 1 篇限时作文。\n"
            "阶段4(7-8周):全真模拟——按考试时间完成整套写作,结合批改迭代。\n"
        )

    async def _mock_async(prompt: str, max_tokens: int = 1024) -> str:
        return _mock(prompt, max_tokens)

    return _mock_async
