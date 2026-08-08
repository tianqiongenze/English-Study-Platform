import asyncio

from app.llm import MockLlmProvider, extract_json
from app.schemas import GradeRequest, PolishRequest, TranslateRequest
from app.service import WritingService


def test_grade_returns_valid_structure():
    svc = WritingService(MockLlmProvider())
    resp = asyncio.run(svc.grade(GradeRequest(prompt="topic", essay="I wants coffee.")))
    assert 0 <= resp.score <= 30
    assert resp.feedback
    assert 0 <= resp.coherence.score <= 10
    assert any(g.original == "I wants" for g in resp.grammar)


def test_polish_returns_polished_and_suggestions():
    svc = WritingService(MockLlmProvider())
    resp = asyncio.run(svc.polish(PolishRequest(text="I wants to improving")))
    assert resp.polished
    assert len(resp.suggestions) >= 1


def test_translate_returns_translation_and_vocab():
    svc = WritingService(MockLlmProvider())
    resp = asyncio.run(svc.translate(TranslateRequest(text="This is an example to translate for vocab.")))
    assert resp.translation
    assert len(resp.vocab) >= 3


def test_extract_json_handles_codefence():
    raw = "```json\n{\"score\": 22, \"feedback\": \"ok\"}\n```"
    data = extract_json(raw)
    assert data["score"] == 22


def test_extract_json_handles_leading_prose():
    raw = "Here is the result: {\"score\": 18, \"feedback\": \"x\"} done."
    data = extract_json(raw)
    assert data["score"] == 18


def test_grade_cached_returns_same_result():
    """相同输入两次调用,第二次应命中 esp-cache L1(本地进程内)返回相同结果。"""
    import os
    os.environ["CACHE_ENABLED"] = "true"
    svc = WritingService(MockLlmProvider())
    req = GradeRequest(prompt="cache topic", essay="She go school every day.")
    r1 = asyncio.run(svc.grade(req))
    r2 = asyncio.run(svc.grade(req))
    assert r1.score == r2.score
    assert r1.feedback == r2.feedback
