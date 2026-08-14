"""Writing Coach: service layer — assess (call writing-py + rubric), plan (call learning + LLM)."""
from __future__ import annotations
import os
import httpx
from .schemas import AssessRequest, AssessResponse, DimensionScore, PlanRequest, WritingPlan
from .db import save_plan
from .llm import new_provider

_provider = new_provider()
_WRITING_PY = os.getenv("WRITING_PY_URL", "http://writing-py:80")
_LEARNING = os.getenv("LEARNING_URL", "http://learning-java:80")


async def _fetch_base_score(req: AssessRequest) -> tuple[float, str]:
    """调 writing-py grade 拿基础分。失败返回 (0.0, "")。"""
    try:
        async with httpx.AsyncClient(timeout=10) as c:
            r = await c.post(f"{_WRITING_PY}/api/writing/grade",
                             json={"prompt": req.prompt, "essay": req.essay, "exam": req.exam})
            if r.status_code == 200:
                d = r.json()
                return float(d.get("score", 0)), d.get("feedback", "")
    except Exception:
        pass
    return 0.0, ""


async def _fetch_weak_points(user_id: int) -> list[str]:
    """调 learning 拿薄弱点。失败返回 []。"""
    try:
        async with httpx.AsyncClient(timeout=8) as c:
            r = await c.get(f"{_LEARNING}/api/learning/mastery/weak",
                            params={"user_id": user_id})
            if r.status_code == 200:
                items = r.json() or []
                return [f"kp-{it.get('kpId')}" for it in items
                        if it.get("kpId") is not None][:5]
    except Exception:
        pass
    return []


async def assess(req: AssessRequest) -> AssessResponse:
    """调 writing-py grade 拿基础分,叠加 rubric 多维评分。"""
    try:
        base_score, base_feedback = await _fetch_base_score(req)
    except Exception:
        base_score, base_feedback = 0.0, ""
    # rubric:基于 essay 文本特征估算各维度(简化版,真实可调 LLM)
    words = len(req.essay.split())
    dims = DimensionScore(
        grammar=min(30, base_score * 0.3 + (5 if words > 50 else 2)),
        coherence=min(25, base_score * 0.25 + (5 if words > 100 else 2)),
        vocabulary=min(25, base_score * 0.25 + (3 if any(len(w) > 7 for w in req.essay.split()) else 1)),
        task_completion=min(20, base_score * 0.2 + (5 if words > 80 else 2)),
    )
    total = round(dims.grammar + dims.coherence + dims.vocabulary + dims.task_completion, 1)
    return AssessResponse(base_score=base_score, dimensions=dims, total=total,
                          feedback=base_feedback or "评估完成")


async def gen_plan(req: PlanRequest) -> WritingPlan:
    """调 learning 拿薄弱点,LLM 生成提升方案,落库。

    learning-java GET /api/learning/mastery/weak 返回 List<UserMastery>,
    序列化为 [{userId, kpId, masteryScore, reviewCount, ...}],无 point 字段。
    用 kpId 作为薄弱知识点标识。
    """
    weak = await _fetch_weak_points(req.user_id)
    if not weak:
        weak = ["grammar-tense", "coherence-transition", "vocabulary-variety"]
    prompt = (
        f"你是英语写作教练。用户目标:{req.goal}。用户薄弱点:{weak}。"
        f"生成一份分 4 阶段、每阶段 2 周的写作能力提升方案,含具体练习任务。"
    )
    plan_text = _provider(prompt)
    plan_id = await save_plan(req.user_id, weak, plan_text)
    return WritingPlan(id=plan_id, user_id=req.user_id, weak_points=weak, plan=plan_text, status="active")
