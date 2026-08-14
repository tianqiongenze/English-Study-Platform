"""English-Study-Platform writing & grading AI service (FastAPI)."""
from __future__ import annotations

import os
import re
from typing import Literal

from pydantic import BaseModel, Field


class GradeRequest(BaseModel):
    prompt: str
    essay: str
    exam: Literal["toefl", "ielts"] = "toefl"
    user_id: int = 0  # 批改结果持久化到 writing_tasks(0=不持久化)


class GrammarIssue(BaseModel):
    original: str
    corrected: str
    reason: str


class Coherence(BaseModel):
    score: float = Field(..., ge=0, le=10)
    transitions: list[str]
    paragraph_flow: str


class GradeResponse(BaseModel):
    score: float = Field(..., ge=0, le=30)
    feedback: str
    grammar: list[GrammarIssue]
    coherence: Coherence


class PolishRequest(BaseModel):
    text: str


class Suggestion(BaseModel):
    original: str
    polished: str
    reason: str


class PolishResponse(BaseModel):
    polished: str
    suggestions: list[Suggestion]


class VocabItem(BaseModel):
    word: str
    pos: str
    meaning: str
    example: str


class TranslateRequest(BaseModel):
    text: str


class TranslateResponse(BaseModel):
    translation: str
    vocab: list[VocabItem]


# === LangGraph 写作智能体(ADR-18) ===
class RunRequest(BaseModel):
    prompt: str
    essay: str
    exam: Literal["toefl", "ielts"] = "toefl"
    human: bool = False  # True=人工干预模式,polish 后挂起


class ReviewRequest(BaseModel):
    thread_id: str
    teacher_feedback: str
