"""Writing Coach: Litestar app (ADR-09 pattern)."""
from __future__ import annotations
from typing import Annotated
from litestar import Litestar, get, post
from litestar.params import Body
from litestar.status_codes import HTTP_200_OK

from .schemas import (
    AssessRequest, AssessResponse, PlanRequest, WritingPlan, ProgressRequest,
)
from .service import assess, gen_plan
from .db import list_templates, list_essays, get_plan, save_progress


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
async def assess_route(data: Annotated[AssessRequest, Body()]) -> AssessResponse:
    return await assess(data)


@post("/api/writing-coach/plan", status_code=HTTP_200_OK)
async def plan_route(data: Annotated[PlanRequest, Body()]) -> WritingPlan:
    return await gen_plan(data)


@get("/api/writing-coach/plan/{user_id:int}")
async def plan_get(user_id: int) -> dict:
    p = await get_plan(user_id)
    return p or {"error": "no plan"}


@post("/api/writing-coach/progress", status_code=HTTP_200_OK)
async def progress_route(data: Annotated[ProgressRequest, Body()]) -> dict:
    pid = await save_progress(data.user_id, data.plan_id, data.task_id, data.completed, data.score)
    return {"id": pid}


app = Litestar(route_handlers=[
    healthz, templates, essays, assess_route, plan_route, plan_get, progress_route,
])
