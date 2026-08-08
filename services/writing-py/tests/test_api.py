"""Litestar 路由层测试 — 覆盖 app/main.py 的健康检查与三个业务端点(ADR-09)。

使用 litestar.testing.TestClient + MockLlmProvider,确保压测/离线可用、无外部依赖。
"""
from __future__ import annotations

import asyncio

from litestar.testing import TestClient

from app import main as main_module
from app.llm import MockLlmProvider
from app.schemas import GradeRequest, PolishRequest, TranslateRequest
from app.service import WritingService

client = TestClient(main_module.app)


def setup_function() -> None:
    # 确保使用确定性 mock,避免压测环境因无 API Key 报错
    main_module._service.llm = MockLlmProvider()


def test_healthz_ok():
    r = client.get("/healthz")
    assert r.status_code == 200
    assert r.json()["status"] == "ok"


def test_grade_endpoint_returns_valid_json():
    r = client.post("/api/writing/grade", json={
        "prompt": "Do you agree tech improves education?",
        "essay": "I wants coffee.",
        "exam": "toefl",
    })
    assert r.status_code == 200
    body = r.json()
    assert 0 <= body["score"] <= 30
    assert "feedback" in body
    assert "coherence" in body
    assert isinstance(body["grammar"], list)


def test_grade_endpoint_rejects_missing_field():
    r = client.post("/api/writing/grade", json={"prompt": "x"})  # 缺 essay
    assert r.status_code in (400, 422)


def test_polish_endpoint():
    r = client.post("/api/writing/polish", json={"text": "I wants to improving"})
    assert r.status_code == 200
    body = r.json()
    assert body["polished"]
    assert isinstance(body["suggestions"], list)


def test_translate_endpoint_returns_vocab():
    r = client.post("/api/writing/translate", json={"text": "Translate this example for vocab."})
    assert r.status_code == 200
    body = r.json()
    assert body["translation"]
    assert len(body["vocab"]) >= 3


def test_service_grade_async():
    svc = WritingService(MockLlmProvider())
    resp = asyncio.run(svc.grade(GradeRequest(prompt="t", essay="I wants coffee.")))
    assert 0 <= resp.score <= 30
    assert any(g.original == "I wants" for g in resp.grammar)


def test_service_polish_async():
    svc = WritingService(MockLlmProvider())
    resp = asyncio.run(svc.polish(PolishRequest(text="I wants to improving")))
    assert resp.polished


def test_service_translate_async():
    svc = WritingService(MockLlmProvider())
    resp = asyncio.run(svc.translate(TranslateRequest(text="example to translate")))
    assert len(resp.vocab) >= 3
