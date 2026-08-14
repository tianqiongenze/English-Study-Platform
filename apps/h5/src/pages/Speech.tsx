import { useState, useRef, useEffect } from "react";
import { api } from "../api";
import type { Scenario } from "../types";
import { EspWsClient } from "@esp/ws-client";
import Avatar, { Emotion } from "../components/Avatar";
import Skeleton from "../Skeleton";

export default function Speech() {
  const [scenarios, setScenarios] = useState<Scenario[]>([{ code: "airport", title: "机场登机", system: "", difficulty: 2 }]);
  // 场景列表加载态:复用通用骨架屏
  const [scenariosLoading, setScenariosLoading] = useState(true);
  const [scenario, setScenario] = useState("airport");
  const [log, setLog] = useState<string[]>([]);
  const [input, setInput] = useState("");
  const wsRef = useRef<EspWsClient | null>(null);
  // 数字人(FR-6.1)
  const [emotion, setEmotion] = useState<Emotion>("neutral");
  const [speaking, setSpeaking] = useState(false);
  const [bubble, setBubble] = useState("");
  const add = (s: string) => setLog((l) => [...l, s]);

  useEffect(() => { api.scenarios().then(setScenarios).catch(() => {}).finally(() => setScenariosLoading(false)); return () => wsRef.current?.close(); }, []);

  async function start() {
    try {
      const d = await api.createSession(scenario);
      add(`会话: ${d.session_id} (${scenario})`);
      const ws = new EspWsClient({
        url: `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws/speech/${d.session_id}`,
        onFrame: (f) => {
        if (f.type === "ai_reply" && f.text) {
          add(`AI: ${f.text}`);
          setBubble(f.text); setSpeaking(true);
          if (f.emotion === "happy" || f.emotion === "neutral" || f.emotion === "encouraging" || f.emotion === "sad") setEmotion(f.emotion);
          window.setTimeout(() => setSpeaking(false), 2000);
        }
        else if (f.type === "correction") add(`纠错: ${f.original}→${f.corrected}`);
        else if (f.type === "hint") add(`引导: ${(f.options || []).join("/")}`);
        else if (f.type === "end") { add(`结束: ${f.summary}`); setSpeaking(false); setEmotion("neutral"); }
        else if (f.type === "error") add(`错误: ${f.message}`);
        },
      });
      ws.connect();
      wsRef.current = ws;
    } catch (e) { add(`失败: ${e}`); }
  }
  const send = () => { if (wsRef.current && input) { wsRef.current.send(JSON.stringify({ type: "text_input", text: input })); add(`我: ${input}`); setInput(""); } };
  const hint = () => wsRef.current?.send(JSON.stringify({ type: "text_input", text: "/hint" }));
  const end = () => wsRef.current?.send(JSON.stringify({ type: "end" }));

  return (
    <div>
      {scenariosLoading ? <Skeleton height={40} /> : (
        <select value={scenario} onChange={(e) => setScenario(e.target.value)}>
          {scenarios.map((s) => <option key={s.code} value={s.code}>{s.title}</option>)}
        </select>
      )}
      <button className="primary" onClick={start} style={{ marginTop: 6 }}>开始陪练</button>
      <Avatar emotion={emotion} active={speaking} text={bubble || undefined} />
      <div className="log">{log.map((l, i) => <div key={i}>{l}</div>)}</div>
      <input value={input} onChange={(e) => setInput(e.target.value)} placeholder="说点什么(英文)..." onKeyDown={(e) => e.key === "Enter" && send()} />
      <div style={{ display: "flex", gap: 6, marginTop: 6 }}>
        <button className="primary" onClick={send} style={{ flex: 1 }}>发送</button>
        <button onClick={hint}>提示</button>
        <button onClick={end}>结束</button>
      </div>
    </div>
  );
}
