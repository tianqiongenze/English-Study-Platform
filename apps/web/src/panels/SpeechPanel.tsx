import { useState, useRef, useEffect } from "react";
import { api } from "../api";
import type { Scenario } from "../types";
import { EspWsClient } from "@esp/ws-client";
import Avatar, { Emotion } from "../components/Avatar";

export default function SpeechPanel() {
  const [scenarios, setScenarios] = useState<Scenario[]>([{ code: "airport", title: "机场登机", system: "", difficulty: 2 }]);
  const [scenario, setScenario] = useState("airport");
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [log, setLog] = useState<string[]>([]);
  const [input, setInput] = useState("");
  const wsRef = useRef<EspWsClient | null>(null);
  // 数字人状态(FR-6.1):WS ai_reply 携带 emotion 时切换表情
  const [emotion, setEmotion] = useState<Emotion>("neutral");
  const [speaking, setSpeaking] = useState(false);
  const [bubble, setBubble] = useState("");

  const addLog = (s: string) => setLog((l) => [...l, s]);

  useEffect(() => {
    api.scenarios().then(setScenarios).catch(() => {});
    return () => wsRef.current?.close();
  }, []);

  async function startSession() {
    try {
      const data = await api.createSession(scenario);
      setSessionId(data.session_id);
      addLog(`会话已创建: ${data.session_id} (场景: ${scenario})`);
      const ws = new EspWsClient({
        url: `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws/speech/${data.session_id}`,
        onFrame: (f) => {
        if (f.type === "ai_reply" && f.text) {
          addLog(`AI: ${f.text}${f.emotion ? ` [${f.emotion}]` : ""}`);
          setBubble(f.text);
          setSpeaking(true);
          // emotion 帧切换数字人表情
          if (f.emotion === "happy" || f.emotion === "neutral" || f.emotion === "encouraging" || f.emotion === "sad") setEmotion(f.emotion);
          window.setTimeout(() => setSpeaking(false), 2000);
        }
        else if (f.type === "correction") addLog(`纠错: ${f.original} → ${f.corrected}`);
        else if (f.type === "hint") addLog(`引导: ${(f.options || []).join(" / ")}`);
        else if (f.type === "end") { addLog(`结束: ${f.summary}`); setSpeaking(false); setEmotion("neutral"); }
        else if (f.type === "error") addLog(`错误: ${f.message}`);
        },
      });
      ws.connect();
      wsRef.current = ws;
    } catch (e) { addLog(`创建失败: ${e}`); }
  }

  function send() {
    if (!wsRef.current || !input) return;
    wsRef.current.send(JSON.stringify({ type: "text_input", text: input }));
    addLog(`我: ${input}`);
    setInput("");
  }
  const hint = () => wsRef.current?.send(JSON.stringify({ type: "text_input", text: "/hint" }));
  const end = () => wsRef.current?.send(JSON.stringify({ type: "end" }));

  return (
    <div>
      <select value={scenario} onChange={(e) => setScenario(e.target.value)}>
        {scenarios.map((s) => <option key={s.code} value={s.code}>{s.title}</option>)}
      </select>{" "}
      <button className="primary" onClick={startSession}>开始陪练</button>
      {/* 数字人舞台(FR-6.1) */}
      <div style={{ display: "flex", gap: 16, alignItems: "flex-start", margin: "12px 0" }}>
        <Avatar emotion={emotion} active={speaking} text={sessionId ? bubble : undefined} />
        <div className="log" style={{ flex: 1 }}>{log.map((l, i) => <div key={i}>{l}</div>)}</div>
      </div>
      <input value={input} onChange={(e) => setInput(e.target.value)} placeholder="说点什么(英文)..." onKeyDown={(e) => e.key === "Enter" && send()} />
      <button className="primary" onClick={send}>发送</button>{" "}
      <button onClick={hint}>提示</button>{" "}
      <button onClick={end}>结束</button>
      <p className="hint">提示:点击「提示」获取启发式引导;结束会生成学习笔记。数字人由 TTS 音频帧驱动唇形。</p>
    </div>
  );
}
