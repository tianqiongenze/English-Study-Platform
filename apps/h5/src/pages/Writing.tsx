import { useState } from "react";
import { api } from "../api";
import type { GradeResult, PolishResult, TranslateResult } from "../types";
import { useI18n } from "@esp/ui-tokens";

type Tab = "grade" | "polish" | "translate" | "agent";

export default function Writing() {
  const { t } = useI18n();
  const [tab, setTab] = useState<Tab>("grade");
  const [essay, setEssay] = useState("");
  const [text, setText] = useState("");
  const [r, setR] = useState<GradeResult | null>(null);
  const [polish, setPolish] = useState<PolishResult | null>(null);
  const [translate, setTranslate] = useState<TranslateResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [threadId, setThreadId] = useState<string | null>(null);
  const [agentStatus, setAgentStatus] = useState("");

  async function run() {
    setLoading(true); try {
      if (tab === "grade") setR(await api.grade("Describe your favorite book", essay, "toefl"));
      else if (tab === "polish") setPolish(await api.polish(text));
      else if (tab === "translate") setTranslate(await api.translate(text));
    } finally { setLoading(false); }
  }
  async function agentRun() {
    setLoading(true);
    try { const res = await api.writingRun("Describe your favorite book", essay, "toefl", false); setThreadId(res.thread_id); setAgentStatus(res.status); }
    catch (e) { setAgentStatus(`失败:${e}`); } finally { setLoading(false); }
  }
  async function agentState() { if (!threadId) return; setLoading(true); try { const s = await api.writingState(threadId); setAgentStatus(s.status); } finally { setLoading(false); } }

  return (
    <div className="panel immersive">
      <h3>{t("writing.title")}</h3>
      <div style={{ display: "flex", gap: 6, marginBottom: 8, flexWrap: "wrap" }}>
        <button className={tab === "grade" ? "primary" : ""} onClick={() => setTab("grade")}>{t("writing.grade")}</button>
        <button className={tab === "polish" ? "primary" : ""} onClick={() => setTab("polish")}>{t("writing.polish")}</button>
        <button className={tab === "translate" ? "primary" : ""} onClick={() => setTab("translate")}>{t("writing.translate")}</button>
        <button className={tab === "agent" ? "primary" : ""} onClick={() => setTab("agent")}>智能体</button>
      </div>
      {tab === "grade" && (
        <>
          <textarea rows={5} value={essay} onChange={(e) => setEssay(e.target.value)} placeholder="输入英文作文..." />
          <button className="primary" onClick={run} disabled={loading} style={{ width: "100%" }}>{loading ? "批改中..." : "AI 批改"}</button>
          {r && <div style={{ marginTop: 8 }}><p>{t("result.score")}: {r.score}/30</p><p className="hint">{r.feedback}</p></div>}
        </>
      )}
      {tab === "polish" && (
        <>
          <textarea rows={5} value={text} onChange={(e) => setText(e.target.value)} placeholder="输入要润色的英文..." />
          <button className="primary" onClick={run} disabled={loading} style={{ width: "100%" }}>{loading ? "润色中..." : "润色"}</button>
          {polish && <div style={{ marginTop: 8 }}><p>{polish.polished}</p>{polish.suggestions.length > 0 && <ul>{polish.suggestions.map((s, i) => <li key={i}>{s.original}→<b>{s.polished}</b>({s.reason})</li>)}</ul>}</div>}
        </>
      )}
      {tab === "translate" && (
        <>
          <textarea rows={5} value={text} onChange={(e) => setText(e.target.value)} placeholder="输入要翻译的英文..." />
          <button className="primary" onClick={run} disabled={loading} style={{ width: "100%" }}>{loading ? "翻译中..." : "翻译"}</button>
          {translate && <div style={{ marginTop: 8 }}><p>{translate.translation}</p>{translate.vocab.length > 0 && <ul>{translate.vocab.map((v, i) => <li key={i}><b>{v.word}</b>({v.pos}) {v.meaning}</li>)}</ul>}</div>}
        </>
      )}
      {tab === "agent" && (
        <>
          <textarea rows={5} value={essay} onChange={(e) => setEssay(e.target.value)} placeholder="输入英文作文..." />
          <button className="primary" onClick={agentRun} disabled={loading} style={{ width: "100%" }}>{loading ? "运行中..." : "智能体批改(LangGraph)"}</button>
          {threadId && <div style={{ marginTop: 8 }}><p className="hint">thread: {threadId} · 状态: {agentStatus}</p><button onClick={agentState}>查询状态</button></div>}
        </>
      )}
    </div>
  );
}
