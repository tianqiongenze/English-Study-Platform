"""LLM Provider 抽象:Anthropic Claude(异步)+ Mock。

ADR-09:LLM 调用异步化(httpx.AsyncClient),配合 Litestar 全异步范式,
在 I/O 等待期释放事件循环,提升 AI 服务并发能力。
"""
from __future__ import annotations

import json
import os
import re
from typing import Protocol


class LlmProvider(Protocol):
    async def chat(self, system: str, user: str) -> str: ...


class MockLlmProvider:
    """确定性 mock,压测与离线开发用。返回伪批改 JSON。"""

    async def chat(self, system: str, user: str) -> str:
        low = user.lower()
        if "learning notes" in low or "vocab" in low and "grammar_points" in low:
            return json.dumps({
                "vocab": [
                    {"word": "improve", "pos": "v.", "meaning": "改进", "example": "I want to improve my English."},
                    {"word": "transition", "pos": "n.", "meaning": "过渡", "example": "Use transitions between paragraphs."},
                    {"word": "thesis", "pos": "n.", "meaning": "论点", "example": "State a clear thesis."},
                ],
                "grammar_points": ["Subject-verb agreement (I want, not I wants)",
                                   "Use base verb after 'to' (to improve, not to improving)"],
                "improvements": ["Add paragraph transitions (however, therefore).",
                                 "Vary sentence structures."],
                "summary": "Good thesis; focus on subject-verb agreement and transitions.",
            })
        if "grade" in low or "essay" in low or "score" in low:
            return json.dumps({
                "score": 24.0,
                "feedback": "Your essay has a clear thesis. Improve paragraph transitions and vary sentence structures.",
                "grammar": [
                    {"original": "I wants", "corrected": "I want",
                     "reason": "Subject-verb agreement: first person uses base verb."}
                ],
                "coherence": {
                    "score": 7.0,
                    "transitions": ["however", "in addition"],
                    "paragraph_flow": "Generally logical; add topic sentences.",
                },
            })
        if "polish" in low:
            return json.dumps({
                "polished": "I want to improve my English writing skills.",
                "suggestions": [
                    {"original": "I wants to improving", "polished": "I want to improve",
                     "reason": "Fix subject-verb agreement and use base verb after 'to'."}
                ],
            })
        if "translate" in low:
            return json.dumps({
                "translation": "这是一段示例翻译。",
                "vocab": [
                    {"word": "example", "pos": "n.", "meaning": "例子", "example": "This is an example."},
                    {"word": "translate", "pos": "v.", "meaning": "翻译", "example": "Translate this text."},
                    {"word": "vocab", "pos": "n.", "meaning": "词汇", "example": "Build your vocab."},
                ],
            })
        return json.dumps({"score": 20.0, "feedback": "mock", "grammar": [], "coherence": {"score": 6.0, "transitions": [], "paragraph_flow": "ok"}})


class AnthropicLlmProvider:
    """调用 Anthropic Claude Messages API(异步)。"""

    def __init__(self, api_key: str | None = None, model: str | None = None, base_url: str | None = None):
        import httpx

        self.api_key = api_key or os.getenv("ANTHROPIC_API_KEY", "")
        self.model = model or os.getenv("ANTHROPIC_MODEL", "claude-sonnet-4-6")
        self.base_url = (base_url or os.getenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com")).rstrip("/")
        self._client = httpx.AsyncClient(timeout=30.0)

    async def chat(self, system: str, user: str) -> str:
        if not self.api_key:
            raise RuntimeError("ANTHROPIC_API_KEY not set")
        body = {
            "model": self.model,
            "max_tokens": 1024,
            "system": system,
            "messages": [{"role": "user", "content": user}],
        }
        resp = await self._client.post(
            f"{self.base_url}/v1/messages",
            headers={
                "x-api-key": self.api_key,
                "anthropic-version": "2023-06-01",
                "content-type": "application/json",
            },
            json=body,
        )
        resp.raise_for_status()
        data = resp.json()
        parts = data.get("content", [])
        return "".join(p.get("text", "") for p in parts if p.get("type") == "text")


def new_provider() -> LlmProvider:
    if os.getenv("LLM_MODE", "").lower() == "mock":
        return MockLlmProvider()
    return AnthropicLlmProvider()


def extract_json(text: str) -> dict:
    """LLM 可能返回非纯 JSON,做兜底提取。"""
    text = text.strip()
    m = re.search(r"```(?:json)?\s*(\{.*?\})\s*```", text, re.DOTALL)
    if m:
        text = m.group(1)
    start = text.find("{")
    end = text.rfind("}")
    if start != -1 and end != -1 and end > start:
        text = text[start:end + 1]
    return json.loads(text)
