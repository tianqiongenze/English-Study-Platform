"""Writing Coach: schemas (Pydantic v2)."""
from __future__ import annotations
from typing import Optional
from pydantic import BaseModel, Field


class Template(BaseModel):
    id: Optional[int] = None
    category: str           # argumentative / letter / chart / narrative
    title: str
    structure: str          # 段落结构骨架
    difficulty: int = 2     # 1-3


class Essay(BaseModel):
    id: Optional[int] = None
    category: str
    title: str
    content: str
    difficulty: int = 2
    word_count: int = 0


class AssessRequest(BaseModel):
    user_id: int
    prompt: str
    essay: str
    exam: str = "toefl"


class DimensionScore(BaseModel):
    grammar: float
    coherence: float
    vocabulary: float
    task_completion: float


class AssessResponse(BaseModel):
    base_score: float                       # 来自 writing-py grade
    dimensions: DimensionScore              # rubric 多维
    total: float                            # 加权总分
    feedback: str


class PlanRequest(BaseModel):
    user_id: int
    goal: str = "general"                   # toefl / ielts / general


class WritingPlan(BaseModel):
    id: Optional[int] = None
    user_id: int
    weak_points: list[str]
    plan: str                               # LLM 生成的提升方案(分阶段)
    status: str = "active"


class ProgressRequest(BaseModel):
    user_id: int
    plan_id: int
    task_id: str
    completed: bool = True
    score: float = 0.0
