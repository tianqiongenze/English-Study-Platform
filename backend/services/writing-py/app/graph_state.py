"""WritingState 与 LangGraph 节点定义(ADR-18)。

写作批改智能体多步链:批改(grade) → 润色(polish) → [人工干预] → 学习笔记(notes)。
人工干预模式在 polish 后 interrupt_before notes,挂起等待教师反馈。

节点工厂接收 llm,返回 (state) -> dict 的节点函数(通过闭包绑定 llm)。
"""
from __future__ import annotations

from typing import Callable

from .llm import LlmProvider, extract_json


class WritingState(dict):
    """可变 state(LangGraph 会合并节点返回的 dict)。"""

    prompt: str
    essay: str
    exam: str
    grade_result: dict
    polished: dict
    teacher_feedback: str
    notes: dict
    status: str


def make_grade_node(llm: LlmProvider) -> Callable:
    async def grade_node(state: WritingState) -> dict:
        """LLM 评分官:score/feedback/grammar/coherence。"""
        system = (
            "You are a strict but constructive TOEFL/IELTS writing examiner. "
            "Return ONLY valid JSON: {score(0-30), feedback(str), "
            "grammar:[{original,corrected,reason}], coherence:{score(0-10),transitions:[str],paragraph_flow:str}}."
        )
        user = f"Exam: {state.get('exam','toefl')}\nPrompt: {state.get('prompt','')}\nEssay:\n{state.get('essay','')}"
        raw = await llm.chat(system, user)
        grade_result = extract_json(raw)
        return {"grade_result": grade_result, "status": "graded"}
    return grade_node


def make_polish_node(llm: LlmProvider) -> Callable:
    async def polish_node(state: WritingState) -> dict:
        """LLM editor:基于批改结果润色。"""
        grade = state.get("grade_result", {})
        essay = state.get("essay", "")
        system = (
            "You are an English writing editor. Polish the essay for grammar and style, "
            "merging simple sentences into advanced ones. Return ONLY JSON: "
            "{polished:str, suggestions:[{original,polished,reason}]}."
        )
        user = f"Essay:\n{essay}\n\nExaminer feedback:\n{grade.get('feedback','')}"
        raw = await llm.chat(system, user)
        polished = extract_json(raw)
        return {"polished": polished, "status": "polished"}
    return polish_node


def make_notes_node(llm: LlmProvider) -> Callable:
    async def notes_node(state: WritingState) -> dict:
        """学习笔记节点:综合批改+润色+教师反馈,生成生词/语法点/改进清单。"""
        grade = state.get("grade_result", {})
        polished = state.get("polished", {})
        teacher_feedback = state.get("teacher_feedback", "") or ""
        system = (
            "Generate structured Chinese learning notes for the student based on the grading, "
            "polishing and teacher feedback. Return ONLY JSON: "
            "{vocab:[{word,pos,meaning,example}], grammar_points:[str], "
            "improvements:[str], summary:str}."
        )
        user = (
            f"Score: {grade.get('score','')}\nFeedback: {grade.get('feedback','')}\n"
            f"Polished: {polished.get('polished','')}\nTeacher feedback: {teacher_feedback}"
        )
        raw = await llm.chat(system, user)
        notes = extract_json(raw)
        return {"notes": notes, "status": "done"}
    return notes_node
