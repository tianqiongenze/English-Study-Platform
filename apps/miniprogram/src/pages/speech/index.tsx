import { useState, useRef, useEffect } from "react";
import { View, Text, Button, Picker, Input } from "@tarojs/components";
import { api, openSpeechWs, getUser } from "../../api";
import Avatar, { Emotion } from "../../components/Avatar";
import { resolvePersona, type Persona } from "@esp/ui-tokens";
import { t } from "../../i18n";

export default function Speech() {
  const user = getUser();
  const [scenarios, setScenarios] = useState<{ code: string; title: string }[]>([
    { code: "airport", title: t("miniprogram.speech.scenarioAirport") },
  ]);
  const [idx, setIdx] = useState(0);
  const [log, setLog] = useState<string[]>([]);
  const [input, setInput] = useState("");
  const wsRef = useRef<any>(null);
  // 数字人(FR-6.1)
  const [emotion, setEmotion] = useState<Emotion>("neutral");
  const [speaking, setSpeaking] = useState(false);
  const [bubble, setBubble] = useState("");
  const [persona, setPersona] = useState<Persona>(() => resolvePersona(undefined, user?.id));
  const add = (s: string) => setLog((l) => [...l, s]);

  useEffect(() => {
    api.getDigitalHuman().then((r) => setPersona(resolvePersona(r.persona_id, user?.id))).catch(() => {});
  }, []);

  async function start() {
    const s = scenarios[idx] || scenarios[0];
    try {
      const d = await api.createSession(s.code);
      add(`${t("miniprogram.speech.session")}:${d.session_id}`);
      wsRef.current = openSpeechWs(d.session_id, (f) => {
        if (f.type === "ai_reply" && f.text) {
          add(`AI: ${f.text}`);
          setBubble(f.text); setSpeaking(true);
          if (f.emotion === "happy" || f.emotion === "neutral" || f.emotion === "encouraging" || f.emotion === "sad") setEmotion(f.emotion);
          setTimeout(() => setSpeaking(false), 2000);
        }
        else if (f.type === "correction") add(`${t("miniprogram.speech.correction")}:${f.original}→${f.corrected}`);
        else if (f.type === "hint") add(`${t("miniprogram.speech.guidance")}:${(f.options || []).join("/")}`);
        else if (f.type === "end") { add(`${t("miniprogram.speech.end")}:${f.summary}`); setSpeaking(false); setEmotion("neutral"); }
        else if (f.type === "error") add(`${t("miniprogram.speech.error")}:${f.message}`);
      });
    } catch (e) { add(`${t("miniprogram.failed")}:${e}`); }
  }
  const send = () => {
    if (!wsRef.current || !input) return;
    wsRef.current.send(JSON.stringify({ type: "text_input", text: input }));
    add(`${t("miniprogram.speech.me")}: ${input}`); setInput("");
  };
  const hint = () => wsRef.current?.send(JSON.stringify({ type: "text_input", text: "/hint" }));
  const end = () => wsRef.current?.send(JSON.stringify({ type: "end" }));

  return (
    <View className="page">
      <Picker mode="selector" range={scenarios.map((s) => s.title)} value={idx} onChange={(e) => setIdx(Number(e.detail.value))}>
        <View className="picker">{t("miniprogram.scenario")}:{scenarios[idx]?.title}</View>
      </Picker>
      <Button type="primary" onClick={start}>{t("scenarios.start")}</Button>
      <Avatar persona={persona} emotion={emotion} active={speaking} text={bubble || undefined} />
      <View className="log">{log.map((l, i) => <View key={i}>{l}</View>)}</View>
      <Input value={input} onInput={(e) => setInput(e.detail.value)} placeholder={t("miniprogram.speech.inputPlaceholder")} />
      <View style={{ display: "flex", gap: "12rpx", marginTop: "12rpx" }}>
        <Button type="primary" onClick={send} style={{ flex: 1 }}>{t("miniprogram.speech.send")}</Button>
        <Button onClick={hint}>{t("miniprogram.speech.hintButton")}</Button>
        <Button onClick={end}>{t("miniprogram.speech.endSession")}</Button>
      </View>
    </View>
  );
}