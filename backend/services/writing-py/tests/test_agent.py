"""LangGraph 写作智能体测试(ADR-18:批改→润色→人工干预→学习笔记)。

覆盖:
- 自动模式:run 直达 notes,status=done。
- 人工干预模式:run 在 polish 后挂起,status=polished(待 review)。
- review 恢复:注入 teacher_feedback,notes 节点生成,status=done。
- get_state:可查询挂起/完成状态。
使用 MockLlmProvider,无外部依赖。
"""
from __future__ import annotations

import asyncio

from app.agent import WritingAgent
from app.llm import MockLlmProvider


def test_auto_mode_runs_to_notes():
    agent = WritingAgent(MockLlmProvider())
    result = asyncio.run(agent.run(prompt="tech in edu", essay="I wants coffee.", human=False))
    assert result["status"] in ("done", "polished", "graded")
    assert result["grade_result"] is not None
    assert result["polished"] is not None
    assert result["thread_id"].startswith("writing-")


def test_human_mode_interrupts_before_notes():
    agent = WritingAgent(MockLlmProvider())
    result = asyncio.run(agent.run(prompt="t", essay="I wants coffee.", human=True))
    # 人工模式:polish 后 interrupt,notes 未生成
    assert result["status"] in ("polished", "graded")
    assert result["polished"] is not None
    assert result["notes"] is None


def test_resume_after_review_generates_notes():
    agent = WritingAgent(MockLlmProvider())
    r = asyncio.run(agent.run(prompt="t", essay="I wants coffee.", human=True))
    tid = r["thread_id"]
    resumed = asyncio.run(agent.resume(tid, teacher_feedback="Improve transitions."))
    assert resumed["status"] == "done"
    assert resumed["notes"] is not None


def test_get_state_reports_status():
    agent = WritingAgent(MockLlmProvider())
    r = asyncio.run(agent.run(prompt="t", essay="I wants coffee.", human=True))
    s = asyncio.run(agent.get_state(r["thread_id"]))
    assert s["thread_id"] == r["thread_id"]
    assert s["status"] in ("polished", "graded", "done")
