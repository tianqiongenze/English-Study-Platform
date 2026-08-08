"""Litestar app for writing service (ADR-09: FastAPI → Litestar + Granian; ADR-18: LangGraph).

契约(路径/请求/响应模型)与 FastAPI 版本完全一致,前后端零影响。
新增 LangGraph 写作智能体端点:/api/writing/{run,review,state}。
"""
from __future__ import annotations

import json
import logging
import os
import uuid
from typing import Annotated

from litestar import Litestar, Request, get, post
from litestar.openapi import OpenAPIConfig
from litestar.params import Body
from litestar.status_codes import HTTP_200_OK
from litestar.contrib.opentelemetry import OpenTelemetryPlugin

from .agent import WritingAgent
from .llm import new_provider
from .pulsar_producer import publish_writing_graded
from .schemas import (
    GradeRequest, GradeResponse, PolishRequest, PolishResponse,
    ReviewRequest, RunRequest, TranslateRequest, TranslateResponse,
)
from .service import WritingService
from .tracing import setup_tracing, instrument_app

logger = logging.getLogger("writing_service")

# 复用 esp-cache-py 的 Redis Cluster 连接做幂等(SETNX + TTL)
try:
    from esp_cache import _get_redis as _get_idem_redis  # noqa: WPS433
except Exception:  # 降级:无 redis 时幂等失效,业务照常跑
    _get_idem_redis = None  # type: ignore

_IDEM_TTL = 300  # 写作批改幂等窗口 5min

_service = WritingService(new_provider())
_agent = WritingAgent(new_provider())

# 初始化 OTel tracing(在 app 创建前)
setup_tracing()


@get("/healthz", sync_to_thread=False)
def healthz() -> dict:
    return {"status": "ok"}


@post("/api/writing/grade", status_code=HTTP_200_OK)
async def grade(data: Annotated[GradeRequest, Body()], request: Request) -> GradeResponse:
    # 幂等:X-Request-Id(缺失生成 uuid),SETNX esp:idem:writing:grade:<rid> TTL 300s
    rid = request.headers.get("x-request-id") or str(uuid.uuid4())
    idem_key = f"esp:idem:writing:grade:{rid}"
    redis = await _get_idem_redis() if _get_idem_redis else None
    if redis is not None:
        try:
            cached = await redis.get(idem_key)
            if cached:
                payload = json.loads(cached)
                logger.info("grade idempotent replay: rid=%s", rid)
                return GradeResponse(**payload["resp"])
        except Exception as exc:
            logger.warning("grade idem read failed (non-fatal): %s", exc)

    resp = await _service.grade(data)
    # EDA: 批改完成发 writing.graded 给 teacher-java(默认 no-op;PULSAR_ENABLED=true 时异步发,失败只 log)
    # P1-6 断点1:注入 user_id+score 透传,打通 writing→mastery 链路
    publish_writing_graded({"status": "graded", "user_id": data.user_id, "score": resp.score})

    # 回写幂等缓存(存响应体,便于 replay 时返回上次结果)
    if redis is not None:
        try:
            # nx=True:仅当 key 不存在时写(并发场景下首个赢家持有);ex=秒
            await redis.set(idem_key, json.dumps({"resp": resp.model_dump()}, default=str),
                            nx=True, ex=_IDEM_TTL)
        except Exception as exc:
            logger.warning("grade idem write failed (non-fatal): %s", exc)
    return resp


@post("/api/writing/polish", status_code=HTTP_200_OK)
async def polish(data: Annotated[PolishRequest, Body()]) -> PolishResponse:
    return await _service.polish(data)


@post("/api/writing/translate", status_code=HTTP_200_OK)
async def translate(data: Annotated[TranslateRequest, Body()]) -> TranslateResponse:
    return await _service.translate(data)


@post("/api/writing/run", status_code=HTTP_200_OK)
async def run(data: Annotated[RunRequest, Body()]) -> dict:
    """启动写作智能体多步链(批改→润色→[人工干预]→学习笔记)。"""
    return await _agent.run(data.prompt, data.essay, data.exam, human=data.human)


@post("/api/writing/review", status_code=HTTP_200_OK)
async def review(data: Annotated[ReviewRequest, Body()]) -> dict:
    """教师人工干预:提交 teacher_feedback,恢复 LangGraph → notes。"""
    return await _agent.resume(data.thread_id, data.teacher_feedback)


@get("/api/writing/state/{thread_id:str}", status_code=HTTP_200_OK)
async def state(thread_id: str) -> dict:
    return await _agent.get_state(thread_id)


app = Litestar(
    route_handlers=[healthz, grade, polish, translate, run, review, state],
    openapi_config=OpenAPIConfig(title="ESP Writing Service", version="1.0.0"),
    plugins=[OpenTelemetryPlugin()],
)
# 应用 Litestar 自动埋点
instrument_app(app)
