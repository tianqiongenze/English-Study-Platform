"""Litestar app for emotion service (ADR-09: FastAPI → Litestar + Granian)."""
from __future__ import annotations

from litestar import Litestar, post, get
from litestar import Request
from litestar.openapi import OpenAPIConfig
from litestar.params import Body
from litestar.status_codes import HTTP_200_OK
from litestar.contrib.opentelemetry import OpenTelemetryPlugin

from .schemas import SenseRequest, VisualRequest
from .service import sense_emotion, visual_for_word, persist_emotion, recent_emotions
from .tracing import setup_tracing, instrument_app

# 初始化 OTel tracing(在 app 创建前)
setup_tracing()


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
)
# 应用 Litestar 自动埋点
instrument_app(app)
