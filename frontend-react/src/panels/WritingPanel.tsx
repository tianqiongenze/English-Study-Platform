import { useState } from "react";
import { api } from "../api";
import type { GradeResult, PolishResult, TranslateResult } from "../types";

type Tab = "grade" | "polish" | "translate" | "agent";

export default function WritingPanel() {
  const [tab, setTab] = useState<Tab>("grade");
  const [prompt, setPrompt] = useState("Do you agree technology improves education?");
  const [essay, setEssay] = useState("");
  const [text, setText] = useState("");
  const [grade, setGrade] = useState<GradeResult | null>(null);
  const [polish, setPolish] = useState<PolishResult | null>(null);
  const [translate, setTranslate] = useState<TranslateResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");
  // LangGraph human-in-loop 状态(FR-6.2)
  const [threadId, setThreadId] = useState<string | null>(null);
  const [agentStatus, setAgentStatus] = useState("");
  const [agentNotes, setAgentNotes] = useState("");
  const [teacherFb, setTeacherFb] = useState("");

  async function run() {
    setLoading(true); setErr("");
    try {
      if (tab === "grade") setGrade(await api.grade(prompt, essay, "toefl"));
      else if (tab === "polish") setPolish(await api.polish(text));
      else if (tab === "translate") setTranslate(await api.translate(text));
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }

  // FR-6.2 发起 LangGraph 写作批改(human=true → status=awaiting_review)
  async function agentRun() {
    setLoading(true); setErr(""); setAgentNotes("");
    try {
      const r = await api.writingRun(prompt, essay, "toefl", true);
      setThreadId(r.thread_id); setAgentStatus(r.status);
      if (r.notes) setAgentNotes(r.notes);
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }
  // 教师人工审阅 → 恢复 LangGraph
  async function agentReview() {
    if (!threadId) return;
    setLoading(true); setErr("");
    try {
      const r = await api.writingReview(threadId, teacherFb);
      setAgentStatus(r.status); setAgentNotes(r.notes);
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }
  async function agentState() {
    if (!threadId) return;
    setLoading(true); setErr("");
    try {
      const r = await api.writingState(threadId);
      setAgentStatus(r.status); if (r.notes) setAgentNotes(r.notes);
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }

  return (
    <div>
      <div className="tabs">
        <button className={tab === "grade" ? "active" : ""} onClick={() => setTab("grade")}>AI 批改</button>
        <button className={tab === "polish" ? "active" : ""} onClick={() => setTab("polish")}>润色</button>
        <button className={tab === "translate" ? "active" : ""} onClick={() => setTab("translate")}>翻译</button>
        <button className={tab === "agent" ? "active" : ""} onClick={() => setTab("agent")}>智能体批改</button>
      </div>
      {tab === "grade" && (
        <>
          <input value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="作文题目" />
          <textarea rows={6} value={essay} onChange={(e) => setEssay(e.target.value)} placeholder="在此输入你的英文作文..." />
          <button className="primary" onClick={run} disabled={loading}>{loading ? "批改中..." : "AI 批改"}</button>
          {grade && (
            <div>
              <h3>预测分值: {grade.score}/30</h3>
              <p>{grade.feedback}</p>
              <h4>连贯性: {grade.coherence.score}/10</h4>
              <p className="hint">{grade.coherence.paragraph_flow}</p>
              {grade.grammar.length > 0 && (
                <>
                  <h4>语法问题</h4>
                  <ul>{grade.grammar.map((g, i) => <li key={i}>{g.original} → <b>{g.corrected}</b>({g.reason})</li>)}</ul>
                </>
              )}
            </div>
          )}
        </>
      )}
      {tab === "polish" && (
        <>
          <textarea rows={6} value={text} onChange={(e) => setText(e.target.value)} placeholder="输入要润色的英文..." />
          <button className="primary" onClick={run} disabled={loading}>{loading ? "润色中..." : "润色"}</button>
          {polish && (
            <div>
              <h4>润色结果</h4>
              <p>{polish.polished}</p>
              {polish.suggestions.length > 0 && (
                <ul>{polish.suggestions.map((s, i) => <li key={i}>{s.original} → <b>{s.polished}</b>({s.reason})</li>)}</ul>
              )}
            </div>
          )}
        </>
      )}
      {tab === "translate" && (
        <>
          <textarea rows={6} value={text} onChange={(e) => setText(e.target.value)} placeholder="输入要翻译的英文..." />
          <button className="primary" onClick={run} disabled={loading}>{loading ? "翻译中..." : "翻译"}</button>
          {translate && (
            <div>
              <h4>翻译</h4>
              <p>{translate.translation}</p>
              {translate.vocab.length > 0 && (
                <>
                  <h5>词汇</h5>
                  <ul>{translate.vocab.map((v, i) => <li key={i}><b>{v.word}</b>({v.pos}) {v.meaning} — {v.example}</li>)}</ul>
                </>
              )}
            </div>
          )}
        </>
      )}
      {err && <p className="err">{err}</p>}
      {tab === "agent" && (
        <>
          <input value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="作文题目" />
          <textarea rows={6} value={essay} onChange={(e) => setEssay(e.target.value)} placeholder="在此输入你的英文作文..." />
          <button className="primary" onClick={agentRun} disabled={loading}>{loading ? "运行中..." : "发起 Human-in-Loop 批改"}</button>
          {threadId && (
            <div>
              <p>thread_id: {threadId}</p>
              <p>状态: {agentStatus}</p>
              {agentNotes && <p className="hint">笔记: {agentNotes}</p>}
              {agentStatus === "awaiting_review" && (
                <>
                  <textarea rows={3} value={teacherFb} onChange={(e) => setTeacherFb(e.target.value)} placeholder="教师人工审阅意见..." />
                  <button className="primary" onClick={agentReview} disabled={loading}>提交人工审阅(恢复 LangGraph)</button>
                </>
              )}
              <button onClick={agentState} disabled={loading}>查询状态</button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
