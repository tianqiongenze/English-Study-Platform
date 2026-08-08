"""Litestar 路由层测试 — 覆盖 app/main.py 健康检查与 emotion/visual 端点 (ADR-09)。"""
from __future__ import annotations

from litestar.testing import TestClient

from app import main as main_module

client = TestClient(main_module.app)


def test_healthz_ok():
    r = client.get("/healthz")
    assert r.status_code == 200
    assert r.json()["status"] == "ok"


def test_sense_endpoint_negative():
    r = client.post("/api/emotion/sense", json={"text": "I feel sad and tired"})
    assert r.status_code == 200
    body = r.json()
    assert body["emotion"] == "negative"
    assert body["encouragement"]


def test_sense_endpoint_positive():
    r = client.post("/api/emotion/sense", json={"text": "I am excited and confident"})
    assert r.status_code == 200
    assert r.json()["emotion"] == "positive"


def test_sense_endpoint_uses_audio_when_no_text():
    # text 为空时回退使用 audio 字段
    r = client.post("/api/emotion/sense", json={"text": "", "audio": "I am worried"})
    assert r.status_code == 200
    assert r.json()["emotion"] == "negative"


def test_sense_endpoint_handles_empty():
    # 字段均缺省时仍可处理(默认空串 -> neutral),不应 4xx
    r = client.post("/api/emotion/sense", json={})
    assert r.status_code == 200
    assert r.json()["emotion"] == "neutral"


def test_visual_endpoint():
    r = client.post("/api/emotion/visual", json={"word": "Breathtaking", "context": "a mountain"})
    assert r.status_code == 200
    body = r.json()
    assert body["image_url"].startswith("https://")
    assert body["alt"]


def test_visual_endpoint_requires_word():
    r = client.post("/api/emotion/visual", json={"context": "x"})
    assert r.status_code in (400, 422)
