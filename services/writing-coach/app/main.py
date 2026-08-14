"""Writing Coach: Litestar app (ADR-09 pattern)."""
from __future__ import annotations
from typing import Annotated
from litestar import Litestar, get, post, Request
from litestar.params import Body
from litestar.status_codes import HTTP_200_OK

from .schemas import (
    AssessRequest, AssessResponse, PlanRequest, WritingPlan, ProgressRequest,
)
from .service import assess, gen_plan
from .db import list_templates, list_essays, get_plan, save_progress


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


app = Litestar(route_handlers=[
    healthz, templates, essays, assess_route, plan_route, plan_get, progress_route,
])
