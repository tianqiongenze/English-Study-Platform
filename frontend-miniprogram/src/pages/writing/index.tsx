import { useState } from "react";
import { View, Text, Textarea, Button } from "@tarojs/components";
import { api } from "../../api";

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
      if (tab === "grade") setR(await api.grade(essay));
      else if (tab === "polish") setPolish(await api.polish(text));
      else if (tab === "translate") setTranslate(await api.translate(text));
    } finally { setLoading(false); }
  }
  async function agentRun() {
    setLoading(true);
    try {
      const res = await api.writingRun(essay, false);
      setThreadId(res.thread_id); setAgentStatus(res.status);
    } catch (e) { setAgentStatus(`失败:${e}`); }
    finally { setLoading(false); }
  }
  async function agentState() {
    if (!threadId) return;
    setLoading(true);
    try { const s = await api.writingState(threadId); setAgentStatus(s.status); } finally { setLoading(false); }
  }

  return (
    <View className="page">
      <Text>写作</Text>
      <View style={{ display: "flex", gap: "12rpx", marginTop: "12rpx" }}>
        <Button size="mini" type={tab === "grade" ? "primary" : "default"} onClick={() => setTab("grade")}>批改</Button>
        <Button size="mini" type={tab === "polish" ? "primary" : "default"} onClick={() => setTab("polish")}>润色</Button>
        <Button size="mini" type={tab === "translate" ? "primary" : "default"} onClick={() => setTab("translate")}>翻译</Button>
        <Button size="mini" type={tab === "agent" ? "primary" : "default"} onClick={() => setTab("agent")}>智能体</Button>
      </View>
      {tab === "grade" && (
        <View>
          <Textarea value={essay} onInput={(e) => setEssay(e.detail.value)} placeholder="输入英文作文..." style={{ width: "100%", height: "200rpx" }} />
          <Button type="primary" loading={loading} onClick={run}>AI 批改</Button>
          {r && <View><Text>分数:{r.score}/30</Text><Text className="hint">{r.feedback}</Text></View>}
        </View>
      )}
      {tab === "polish" && (
        <View>
          <Textarea value={text} onInput={(e) => setText(e.detail.value)} placeholder="输入要润色的英文..." style={{ width: "100%", height: "200rpx" }} />
          <Button type="primary" loading={loading} onClick={run}>润色</Button>
          {polish && <View><Text>{polish.polished}</Text>{polish.suggestions.length > 0 && <View>{polish.suggestions.map((s, i) => <Text key={i}>{s.original}→{s.polished}({s.reason}){"\n"}</Text>)}</View>}</View>}
        </View>
      )}
      {tab === "translate" && (
        <View>
          <Textarea value={text} onInput={(e) => setText(e.detail.value)} placeholder="输入要翻译的英文..." style={{ width: "100%", height: "200rpx" }} />
          <Button type="primary" loading={loading} onClick={run}>翻译</Button>
          {translate && <View><Text>{translate.translation}</Text>{translate.vocab.length > 0 && <View>{translate.vocab.map((v, i) => <Text key={i}>{v.word}({v.pos}) {v.meaning}{"\n"}</Text>)}</View>}</View>}
        </View>
      )}
      {tab === "agent" && (
        <View>
          <Textarea value={essay} onInput={(e) => setEssay(e.detail.value)} placeholder="输入英文作文..." style={{ width: "100%", height: "200rpx" }} />
          <Button type="primary" loading={loading} onClick={agentRun} style={{ marginTop: "12rpx" }}>智能体批改(LangGraph)</Button>
          {threadId && <View style={{ marginTop: "12rpx" }}><Text className="hint">thread: {threadId} · 状态: {agentStatus}</Text><Button onClick={agentState}>查询状态</Button></View>}
        </View>
      )}
    </View>
  );
}
