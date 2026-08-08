"""Litestar app for emotion service (ADR-09: FastAPI → Litestar + Granian)."""
from __future__ import annotations

from litestar import Litestar, post, get
from litestar.openapi import OpenAPIConfig
from litestar.params import Body
from litestar.status_codes import HTTP_200_OK
from litestar.contrib.opentelemetry import OpenTelemetryPlugin

from .schemas import SenseRequest, VisualRequest
from .service import sense_emotion, visual_for_word, persist_emotion, recent_emotions
from .tracing import setup_tracing, instrument_app

# 初始化 OTel tracing(在 app 创建前)
setup_tracing()


@get("/healthz", sync_to_thread=False)
def healthz() -> dict:
    return {"status": "ok"}


@post("/api/emotion/sense", status_code=HTTP_200_OK)
async def sense(data: SenseRequest = Body()) -> dict:
    resp = sense_emotion(data.text or data.audio)
    # 异步持久化情绪事件到 TiDB(写主库,非阻断)
    user_id = getattr(data, "user_id", 0) or 0
    if user_id:
        await persist_emotion(user_id, data.text or data.audio, resp.emotion)
    return resp.model_dump()


@post("/api/emotion/visual", status_code=HTTP_200_OK)
async def visual(data: VisualRequest = Body()) -> dict:
    return (await visual_for_word(data.word, data.context)).model_dump()


@get("/api/emotion/recent/{user_id:int}", status_code=HTTP_200_OK)
async def recent(user_id: int) -> dict:
    """查询用户近期情绪事件(从库 tidb-follower 读)。"""
    return {"events": await recent_emotions(user_id)}


app = Litestar(
    route_handlers=[healthz, sense, visual, recent],
    openapi_config=OpenAPIConfig(title="ESP Emotion Service", version="1.0.0"),
    plugins=[OpenTelemetryPlugin()],
)
# 应用 Litestar 自动埋点
instrument_app(app)
