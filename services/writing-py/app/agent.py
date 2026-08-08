"""LangGraph 写作智能体:批改 → 润色 → [人工干预] → 学习笔记(ADR-18)。

- 自动模式(human=False):grade → polish → notes 直达。
- 人工干预模式(human=True):grade → polish 后 interrupt_before notes 挂起,
  教师经 review 提交 teacher_feedback 后恢复 → notes。
- checkpointer:测试 MemorySaver;生产 RedisSaver(此处按可注入设计)。
"""
from __future__ import annotations

import uuid
from typing import Any

from langgraph.checkpoint.memory import MemorySaver
from langgraph.graph import END, START, StateGraph

from .graph_state import WritingState, make_grade_node, make_notes_node, make_polish_node
from .llm import LlmProvider


class WritingAgent:
    """封装 LangGraph 写作智能体,提供 run / resume / get_state。"""

    def __init__(self, llm: LlmProvider, checkpointer: Any | None = None):
        self.llm = llm
        self.checkpointer = checkpointer or MemorySaver()
        self._graph_auto = self._build(human=False)
        self._graph_human = self._build(human=True)

    def _build(self, human: bool):
        g = StateGraph(WritingState)
        g.add_node("grade", make_grade_node(self.llm))
        g.add_node("polish", make_polish_node(self.llm))
        g.add_node("notes", make_notes_node(self.llm))
        g.add_edge(START, "grade")
        g.add_edge("grade", "polish")
        g.add_edge("polish", "notes")
        g.add_edge("notes", END)
        interrupt = ["notes"] if human else None
        return g.compile(checkpointer=self.checkpointer, interrupt_before=interrupt)

    @staticmethod
    def new_thread_id() -> str:
        return f"writing-{uuid.uuid4().hex[:16]}"

    async def run(self, prompt: str, essay: str, exam: str = "toefl",
                  human: bool = False, thread_id: str | None = None) -> dict:
        """启动链。返回 {thread_id, status, ...state}。"""
        tid = thread_id or self.new_thread_id()
        graph = self._graph_human if human else self._graph_auto
        config = {"configurable": {"thread_id": tid}}
        init: WritingState = {
            "prompt": prompt, "essay": essay, "exam": exam,
            "teacher_feedback": "", "status": "running",
        }
        # 用 invoke 直跑到底(自动模式)或到 interrupt(人工模式)
        result = await graph.ainvoke(init, config=config)
        return {"thread_id": tid, **_slim_state(result)}

    async def resume(self, thread_id: str, teacher_feedback: str) -> dict:
        """人工干预恢复:注入 teacher_feedback,继续 notes 节点。"""
        config = {"configurable": {"thread_id": thread_id}}
        # 更新 state 中 teacher_feedback 并恢复执行
        from langgraph.types import Command
        result = await self._graph_human.ainvoke(
            Command(resume={"teacher_feedback": teacher_feedback, "status": "resumed"}),
            config=config,
        )
        return {"thread_id": thread_id, **_slim_state(result)}

    async def get_state(self, thread_id: str) -> dict:
        config = {"configurable": {"thread_id": thread_id}}
        snap = await self._graph_human.aget_state(config)
        vals = snap.values if snap else {}
        return {"thread_id": thread_id, **_slim_state(vals)}


def _slim_state(state: dict) -> dict:
    """提取对外可见字段。"""
    return {
        "status": state.get("status", "running"),
        "grade_result": state.get("grade_result"),
        "polished": state.get("polished"),
        "teacher_feedback": state.get("teacher_feedback", ""),
        "notes": state.get("notes"),
    }
