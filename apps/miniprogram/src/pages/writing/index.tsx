import { useState } from "react";
import { View, Text, Textarea, Button } from "@tarojs/components";
import { api } from "../../api";
import { t } from "../../i18n";

type Tab = "grade" | "polish" | "translate" | "agent";

export default function Writing() {
  const [tab, setTab] = useState<Tab>("grade");
  const [essay, setEssay] = useState("");
  const [text, setText] = useState("");
  const [r, setR] = useState<{ score: number; feedback: string } | null>(null);
  const [polish, setPolish] = useState<{ polished: string; suggestions: { original: string; polished: string; reason: string }[] } | null>(null);
  const [translate, setTranslate] = useState<{ translation: string; vocab: { word: string; pos: string; meaning: string; example: string }[] } | null>(null);
  const [loading, setLoading] = useState(false);
  const [threadId, setThreadId] = useState<string | null>(null);
  const [agentStatus, setAgentStatus] = useState("");

  async function run() {
    setLoading(true);
    try {
      if (tab === "grade") setR(await api.grade("Describe your day", essay, "toefl"));
      else if (tab === "polish") setPolish(await api.polish(text));
      else if (tab === "translate") setTranslate(await api.translate(text));
    } finally { setLoading(false); }
  }
  async function agentRun() {
    setLoading(true);
    try {
      const res = await api.writingRun("Describe your day", essay, "toefl", false);
      setThreadId(res.thread_id); setAgentStatus(res.status);
    } catch (e) { setAgentStatus(`${t("miniprogram.failed")}:${e}`); }
    finally { setLoading(false); }
  }
  async function agentState() {
    if (!threadId) return;
    setLoading(true);
    try { const s = await api.writingState(threadId); setAgentStatus(s.status); } finally { setLoading(false); }
  }

  return (
    <View className="page">
      <Text style={{ fontSize: "32rpx", fontWeight: 700 }}>{t("writing.title")}</Text>
      <View style={{ display: "flex", gap: "12rpx", marginTop: "12rpx" }}>
        <Button size="mini" type={tab === "grade" ? "primary" : "default"} onClick={() => setTab("grade")}>{t("miniprogram.writing.tabGrade")}</Button>
        <Button size="mini" type={tab === "polish" ? "primary" : "default"} onClick={() => setTab("polish")}>{t("writing.polish")}</Button>
        <Button size="mini" type={tab === "translate" ? "primary" : "default"} onClick={() => setTab("translate")}>{t("writing.translate")}</Button>
        <Button size="mini" type={tab === "agent" ? "primary" : "default"} onClick={() => setTab("agent")}>{t("miniprogram.writing.tabAgent")}</Button>
      </View>
      {tab === "grade" && (
        <View className="card">
          <Textarea value={essay} onInput={(e) => setEssay(e.detail.value)} placeholder={t("writing.placeholder")} style={{ width: "100%", height: "200rpx" }} />
          <Button type="primary" loading={loading} onClick={run}>{t("miniprogram.writing.aiGrade")}</Button>
          {r && <View><Text>{t("result.score")}:{r.score}/30</Text><Text className="hint">{r.feedback}</Text></View>}
        </View>
      )}
      {tab === "polish" && (
        <View className="card">
          <Textarea value={text} onInput={(e) => setText(e.detail.value)} placeholder={t("miniprogram.writing.polishPlaceholder")} style={{ width: "100%", height: "200rpx" }} />
          <Button type="primary" loading={loading} onClick={run}>{t("writing.polish")}</Button>
          {polish && <View><Text>{polish.polished}</Text>{polish.suggestions.length > 0 && <View>{polish.suggestions.map((s, i) => <Text key={i}>{s.original}→{s.polished}({s.reason}){"\n"}</Text>)}</View>}</View>}
        </View>
      )}
      {tab === "translate" && (
        <View className="card">
          <Textarea value={text} onInput={(e) => setText(e.detail.value)} placeholder={t("miniprogram.writing.translatePlaceholder")} style={{ width: "100%", height: "200rpx" }} />
          <Button type="primary" loading={loading} onClick={run}>{t("writing.translate")}</Button>
          {translate && <View><Text>{translate.translation}</Text>{translate.vocab.length > 0 && <View>{translate.vocab.map((v, i) => <Text key={i}>{v.word}({v.pos}) {v.meaning}{"\n"}</Text>)}</View>}</View>}
        </View>
      )}
      {tab === "agent" && (
        <View className="card">
          <Textarea value={essay} onInput={(e) => setEssay(e.detail.value)} placeholder={t("writing.placeholder")} style={{ width: "100%", height: "200rpx" }} />
          <Button type="primary" loading={loading} onClick={agentRun} style={{ marginTop: "12rpx" }}>{t("miniprogram.writing.agentRun")}</Button>
          {threadId && <View style={{ marginTop: "12rpx" }}><Text className="hint">thread: {threadId} · {t("miniprogram.index.status")}: {agentStatus}</Text><Button onClick={agentState}>{t("miniprogram.writing.checkStatus")}</Button></View>}
        </View>
      )}
    </View>
  );
}