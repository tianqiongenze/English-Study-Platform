import { useState, useRef, useEffect } from "react";
import { api } from "../api";
import { useAuth } from "../auth";
import type { Scenario } from "../types";
import { EspWsClient } from "@esp/ws-client";
import { useI18n, resolvePersona, type Persona } from "@esp/ui-tokens";
import Avatar, { Emotion } from "../components/Avatar";
import SafeImg, { UNSPLASH } from "../components/SafeImg";
import Skeleton from "../Skeleton";

export default function SpeechPanel() {
  const { t } = useI18n();
  const { user } = useAuth();
  const [scenarios, setScenarios] = useState<Scenario[]>([{ code: "airport", title: t("scenarios.airport"), system: "", difficulty: 2 }]);
  // 场景列表加载态:复用通用骨架屏
  const [scenariosLoading, setScenariosLoading] = useState(true);
  const [scenario, setScenario] = useState("airport");
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [log, setLog] = useState<string[]>([]);
  const [input, setInput] = useState("");
  const wsRef = useRef<EspWsClient | null>(null);
  // 数字人状态(FR-6.1):WS ai_reply 携带 emotion 时切换表情
  const [emotion, setEmotion] = useState<Emotion>("neutral");
  const [speaking, setSpeaking] = useState(false);
  const [bubble, setBubble] = useState("");
  // persona:优先服务端持久化值,否则按 userId 确定性派生(同一用户跨端一致)
  const [persona, setPersona] = useState<Persona>(() => resolvePersona(undefined, user?.id));

  const addLog = (s: string) => setLog((l) => [...l, s]);

  useEffect(() => {
    api.scenarios().then(setScenarios).catch(() => {}).finally(() => setScenariosLoading(false));
    // 拉取用户持久化的 persona 偏好(跨设备一致);失败走默认派生
    api.getDigitalHuman()
      .then((r) => setPersona(resolvePersona(r.persona_id, user?.id)))
      .catch(() => {});
    return () => wsRef.current?.close();
  }, []);

  async function startSession() {
    try {
      const data = await api.createSession(scenario);
      setSessionId(data.session_id);
      addLog(t("speech.log.createdDetail", { id: data.session_id, scenario }));
      const ws = new EspWsClient({
        url: `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws/speech/${data.session_id}`,
        onFrame: (f) => {
        if (f.type === "ai_reply" && f.text) {
          addLog(`${t("speech.log.ai")}: ${f.text}${f.emotion ? ` [${f.emotion}]` : ""}`);
          setBubble(f.text);
          setSpeaking(true);
          // emotion 帧切换数字人表情
          if (f.emotion === "happy" || f.emotion === "neutral" || f.emotion === "encouraging" || f.emotion === "sad") setEmotion(f.emotion);
          window.setTimeout(() => setSpeaking(false), 2000);
        }
        else if (f.type === "correction") addLog(`${t("speech.log.correction")}: ${f.original} → ${f.corrected}`);
        else if (f.type === "hint") addLog(`${t("speech.log.hint")}: ${(f.options || []).join(" / ")}`);
        else if (f.type === "end") { addLog(`${t("speech.log.end")}: ${f.summary}`); setSpeaking(false); setEmotion("neutral"); }
        else if (f.type === "error") addLog(`${t("speech.log.error")}: ${f.message}`);
        },
      });
      ws.connect();
      wsRef.current = ws;
    } catch (e) { addLog(`${t("speech.log.createFail")}: ${e}`); }
  }

  function send() {
    if (!wsRef.current || !input) return;
    wsRef.current.send(JSON.stringify({ type: "text_input", text: input }));
    addLog(`${t("speech.log.me")}: ${input}`);
    setInput("");
  }
  const hint = () => wsRef.current?.send(JSON.stringify({ type: "text_input", text: "/hint" }));
  const end = () => wsRef.current?.send(JSON.stringify({ type: "end" }));

  return (
    <div>
      {/* 口语场景横幅(unsplash,断网走渐变 fallback) */}
      <SafeImg src={UNSPLASH.study} alt="" tone={1} className="scene-banner">
        <span className="scene-banner__label">{t("nav.speaking")}</span>
      </SafeImg>

      {scenariosLoading ? <Skeleton height={36} /> : (
        <select value={scenario} onChange={(e) => setScenario(e.target.value)}>
          {scenarios.map((s) => <option key={s.code} value={s.code}>{s.title}</option>)}
        </select>
      )}{" "}
      <button className="primary" onClick={startSession}>{t("scenarios.start")}</button>
      {/* 数字人舞台(FR-6.1) */}
      <div style={{ display: "flex", gap: 16, alignItems: "flex-start", margin: "12px 0" }}>
        <Avatar persona={persona} emotion={emotion} active={speaking} text={sessionId ? bubble : undefined} />
        <div className="log" style={{ flex: 1 }}>{log.map((l, i) => <div key={i}>{l}</div>)}</div>
      </div>
      <input value={input} onChange={(e) => setInput(e.target.value)} placeholder={t("scenarios.inputPlaceholder")} onKeyDown={(e) => e.key === "Enter" && send()} />
      <button className="primary" onClick={send}>{t("scenarios.send")}</button>{" "}
      <button onClick={hint}>{t("scenarios.aiHint")}</button>{" "}
      <button onClick={end}>{t("scenarios.end")}</button>
      <p className="hint">{t("speech.hint")}</p>
    </div>
  );
}