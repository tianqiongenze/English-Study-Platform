"""Litestar app for emotion service (ADR-09: FastAPI → Litestar + Granian)."""
from __future__ import annotations

import os
import time

import redis.asyncio as aioredis
from litestar import Litestar, post, get
from litestar import Request
from litestar.middleware import ASGIMiddleware
from litestar.types import Scope, Receive, Send, ASGIApp
from litestar.openapi import OpenAPIConfig
from litestar.params import Body
from litestar.status_codes import HTTP_200_OK
from litestar.contrib.opentelemetry import OpenTelemetryPlugin

from .schemas import SenseRequest, VisualRequest
from .service import sense_emotion, visual_for_word, persist_emotion, recent_emotions
from .tracing import setup_tracing, instrument_app

# 初始化 OTel tracing(在 app 创建前)
setup_tracing()


# ---------- Redis 滑动窗口限流(分布式多副本安全) ----------
class RedisRateLimiter:
    """Redis ZSET 滑动窗口限流,多副本共享计数。
    env: REDIS_HOST/REDIS_PORT(与 esp-cache 对齐);降级:Redis 不可用时全量放行。"""
    def __init__(self):
        host = os.getenv("REDIS_HOST", "redis-cluster")
        port = int(os.getenv("REDIS_PORT", "6379"))
        try:
            self._redis = aioredis.RedisCluster(
                host=host, port=port, decode_responses=True,
                socket_connect_timeout=2, socket_timeout=2,
            )
        except Exception:
            self._redis = None

    async def allow(self, client_id: str = "global") -> bool:
        if self._redis is None:
            return True
        try:
            key = f"esp:ratelimit:emotion-svc:{client_id}"
            pipe = self._redis.pipeline()
            now = time.time()
            window = 60  # 60s 滑动窗口
            pipe.zremrangebyscore(key, 0, now - window)
            pipe.zadd(key, {str(now): now})
            pipe.zcard(key)
            pipe.expire(key, window)
            results = await pipe.execute()
            request_count = results[2]
            return request_count <= 200  # max_requests
        except Exception:
            return True  # Redis 异常降级:全量放行

_rate_limiter = RedisRateLimiter()

class RateLimitMiddleware(ASGIMiddleware):
    async def handle(self, scope: Scope, receive: Receive, send: Send, next_app: ASGIApp) -> None:
        if not await _rate_limiter.allow():
            import json as _json
            body = _json.dumps({"error": "rate limit exceeded", "retry_after": 1}).encode()
            await send({
                "type": "http.response.start",
                "status": 429,
                "headers": [
                    [b"content-type", b"application/json"],
                    [b"content-length", str(len(body)).encode()],
                ],
            })
            await send({"type": "http.response.body", "body": body})
            return
        await next_app(scope, receive, send)


def _trusted_user_id(request: Request) -> int:
    """P0-3 IDOR 修复:只认 BFF 注入的 X-User-Id(经 JWT 校验的可信值),
    忽略请求体/path 里的 user_id(防伪造写他人情绪事件 / 查他人历史)。"""
    raw = request.headers.get("X-User-Id") or "0"
    try:
        return int(raw)
    except (TypeError, ValueError):
        return 0


@get("/healthz", sync_to_thread=False)
def healthz() -> dict:
    return {"status": "ok"}


@post("/api/emotion/sense", status_code=HTTP_200_OK)
async def sense(data: SenseRequest = Body(), request: Request = None) -> dict:
    resp = sense_emotion(data.text or data.audio)
    # P0-3:持久化的 user_id 取自 BFF 注入的可信 header,不信任请求体 data.user_id
    user_id = _trusted_user_id(request) if request else 0
    if user_id:
        await persist_emotion(user_id, data.text or data.audio, resp.emotion)
    return resp.model_dump()


@post("/api/emotion/visual", status_code=HTTP_200_OK)
async def visual(data: VisualRequest = Body()) -> dict:
    return (await visual_for_word(data.word, data.context)).model_dump()


@get("/api/emotion/recent/{user_id:int}", status_code=HTTP_200_OK)
async def recent(user_id: int, request: Request = None) -> dict:
    """查询用户近期情绪事件(读池读,esp-db-py)。
    P0-3:路径 user_id 必须等于 BFF 注入的可信 X-User-Id(防查他人历史)。"""
    trusted = _trusted_user_id(request) if request else 0
    if trusted == 0 or trusted != user_id:
        return {"error": "forbidden"}
    return {"events": await recent_emotions(user_id)}


app = Litestar(
    route_handlers=[healthz, sense, visual, recent],
    openapi_config=OpenAPIConfig(title="ESP Emotion Service", version="1.0.0"),
    plugins=[OpenTelemetryPlugin()],
    middleware=[RateLimitMiddleware()],
)
# 应用 Litestar 自动埋点
instrument_app(app)

# --- Nacos 服务注册 ---
import os as _os
import logging as _logging
import atexit
from nacos import NacosClient as _NacosClient

_nacos_client = None
try:
    _nacos_addr = _os.getenv("NACOS_ADDR", "nacos:8848")
    _pod_ip = _os.getenv("POD_IP", "127.0.0.1")
    _svc_port = int(_os.getenv("PORT", "8084"))
    _nacos_client = _NacosClient(server_addr=_nacos_addr)
    _nacos_client.add_naming_instance(
        "emotion-svc", ip=_pod_ip, port=_svc_port,
        health_check=True, health_check_type="http",
        health_check_url="/healthz", metadata={"version": "1.0.0"},
    )
    _logging.getLogger("emotion_svc").info("nacos registered: emotion-svc %s:%s", _pod_ip, _svc_port)
except Exception as exc:
    _logging.getLogger("emotion_svc").warning("nacos registration failed (non-fatal): %s", exc)
    _nacos_client = None

if _nacos_client is not None:
    _c = _nacos_client
    atexit.register(lambda: _c.remove_naming_instance("emotion-svc", ip=_pod_ip, port=_svc_port))
