"""Writing Coach tests: API contract smoke (offline, mocks writing-py/learning)."""
import sys, os, json
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import httpx
from httpx import AsyncClient, ASGITransport
import pytest

# 注入 mock env before import
os.environ.setdefault("LLM_MODE", "mock")
os.environ.setdefault("WRITING_PY_URL", "http://writing-py:80")
os.environ.setdefault("LEARNING_URL", "http://learning-java:80")


@pytest.mark.asyncio
async def test_healthz():
    from app.main import app
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://t") as c:
        r = await c.get("/healthz")
        assert r.status_code == 200
        assert r.json()["status"] == "ok"


@pytest.mark.asyncio
async def test_assess_offline(monkeypatch):
    """assess 在 writing-py 不可达时应降级返回(不抛异常)。"""
    # 让 service 内部对 writing-py 的 httpx 调用抛异常,触发降级路径。
    # 不 patch AsyncClient.post(会误伤测试客户端 ASGI transport),改 patch service._fetch_base_score。
    async def _fail(*a, **kw):
        raise httpx.ConnectError("offline")
    import app.service as svc
    monkeypatch.setattr(svc, "_fetch_base_score", _fail)
    from app.main import app
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://t") as c:
        r = await c.post("/api/writing-coach/assess", json={
            "user_id": 1, "prompt": "p", "essay": "This is a short essay.", "exam": "toefl"})
        assert r.status_code == 200
        body = r.json()
        assert "total" in body and "dimensions" in body


@pytest.mark.asyncio
async def test_plan_uses_kpId_and_returns_lastrowid(monkeypatch):
    """gen_plan: learning 返回 [{kpId,...}],weak_points 取 kpId;save_plan 返回 lastrowid 回填 id。"""
    import app.service as svc

    async def _weak(uid):
        return ["kp-101", "kp-202"]
    monkeypatch.setattr(svc, "_fetch_weak_points", _weak)
    monkeypatch.setattr(svc, "_provider", lambda p: "MOCK PLAN TEXT")

    async def _save_plan(uid, weak, plan):
        return 7777
    # service.py 在模块顶层 `from .db import save_plan`,故 patch service 命名空间的引用
    import app.service as svc
    monkeypatch.setattr(svc, "save_plan", _save_plan)

    from app.main import app
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://t") as c:
        r = await c.post("/api/writing-coach/plan", json={"user_id": 42, "goal": "toefl"})
        assert r.status_code == 200
        body = r.json()
        assert body["id"] == 7777          # lastrowid 回填
        assert body["user_id"] == 42
        assert body["weak_points"] == ["kp-101", "kp-202"]   # 来自 learning kpId
        assert body["plan"] == "MOCK PLAN TEXT"
        assert body["status"] == "active"
