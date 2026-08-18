"""Writing Coach: Litestar app (ADR-09 pattern)."""
from __future__ import annotations
from typing import Annotated
from litestar import Litestar, get, post, Request
from litestar.middleware import ASGIMiddleware
from litestar.types import Scope, Receive, Send, ASGIApp
from litestar.params import Body
from litestar.status_codes import HTTP_200_OK

import os
import time

import redis.asyncio as aioredis

from .schemas import (
    AssessRequest, AssessResponse, PlanRequest, WritingPlan, ProgressRequest,
)
from .service import assess, gen_plan
from .db import list_templates, list_essays, get_plan, save_progress


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
            key = f"esp:ratelimit:writing-coach:{client_id}"
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
    """P0-3 IDOR 修复:只认 BFF 注入的 X-User-Id(经 JWT 校验),忽略请求体/path 的 user_id。"""
    raw = request.headers.get("X-User-Id") or "0"
    try:
        return int(raw)
    except (TypeError, ValueError):
        return 0


@get("/healthz", sync_to_thread=False)
def healthz() -> dict:
    return {"status": "ok"}


@get("/api/writing-coach/templates")
async def templates(category: str | None = None) -> list[dict]:
    return await list_templates(category)


@get("/api/writing-coach/essays")
async def essays(category: str | None = None) -> list[dict]:
    return await list_essays(category)


@post("/api/writing-coach/assess", status_code=HTTP_200_OK)
async def assess_route(data: Annotated[AssessRequest, Body()], request: Request) -> AssessResponse:
    # P0-3:用可信 header 覆盖请求体 user_id,防替他人提交作文评分
    trusted = _trusted_user_id(request)
    if trusted:
        data.user_id = trusted
    return await assess(data)


@post("/api/writing-coach/plan", status_code=HTTP_200_OK)
async def plan_route(data: Annotated[PlanRequest, Body()], request: Request) -> WritingPlan:
    trusted = _trusted_user_id(request)
    if trusted:
        data.user_id = trusted
    return await gen_plan(data)


@get("/api/writing-coach/plan/{user_id:int}")
async def plan_get(user_id: int, request: Request) -> dict:
    """P0-3:路径 user_id 必须等于可信 X-User-Id(防查他人写作计划)。"""
    trusted = _trusted_user_id(request)
    if trusted == 0 or trusted != user_id:
        return {"error": "forbidden"}
    p = await get_plan(user_id)
    return p or {"error": "no plan"}


@post("/api/writing-coach/progress", status_code=HTTP_200_OK)
async def progress_route(data: Annotated[ProgressRequest, Body()], request: Request) -> dict:
    # P0-3:进度记录归属取可信 header,防替他人记进度
    trusted = _trusted_user_id(request)
    if trusted:
        data.user_id = trusted
    pid = await save_progress(data.user_id, data.plan_id, data.task_id, data.completed, data.score)
    return {"id": pid}


app = Litestar(
    route_handlers=[healthz, templates, essays, assess_route, plan_route, plan_get, progress_route],
    middleware=[RateLimitMiddleware()],
)

# --- Nacos 服务注册 ---
import os as _os
import logging as _logging
import atexit
from nacos import NacosClient as _NacosClient

_nacos_client = None
try:
    _nacos_addr = _os.getenv("NACOS_ADDR", "nacos:8848")
    _pod_ip = _os.getenv("POD_IP", "127.0.0.1")
    _svc_port = int(_os.getenv("PORT", "8085"))
    _nacos_client = _NacosClient(server_addr=_nacos_addr)
    _nacos_client.add_naming_instance(
        "writing-coach", ip=_pod_ip, port=_svc_port,
        health_check=True, health_check_type="http",
        health_check_url="/healthz", metadata={"version": "1.0.0"},
    )
    _logging.getLogger("writing_coach").info("nacos registered: writing-coach %s:%s", _pod_ip, _svc_port)
except Exception as exc:
    _logging.getLogger("writing_coach").warning("nacos registration failed (non-fatal): %s", exc)
    _nacos_client = None

if _nacos_client is not None:
    _c = _nacos_client
    atexit.register(lambda: _c.remove_naming_instance("writing-coach", ip=_pod_ip, port=_svc_port))
