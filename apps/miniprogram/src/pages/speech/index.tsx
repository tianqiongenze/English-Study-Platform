import { useState, useRef } from "react";
import { View, Text, Button, Picker, Input } from "@tarojs/components";
import { api, openSpeechWs } from "../../api";
import Avatar, { Emotion } from "../../components/Avatar";

export default function Speech() {
  const [scenarios, setScenarios] = useState<{ code: string; title: string }[]>([{ code: "airport", title: "机场登机" }]);
  const [idx, setIdx] = useState(0);
  const [log, setLog] = useState<string[]>([]);
  const [input, setInput] = useState("");
  const wsRef = useRef<any>(null);
  // 数字人(FR-6.1)
  const [emotion, setEmotion] = useState<Emotion>("neutral");
  const [speaking, setSpeaking] = useState(false);
  const [bubble, setBubble] = useState("");
  const add = (s: string) => setLog((l) => [...l, s]);

  async function start() {
    const s = scenarios[idx] || scenarios[0];
    try {
      const d = await api.createSession(s.code);
      add(`会话:${d.session_id}`);
      wsRef.current = openSpeechWs(d.session_id, (f) => {
        if (f.type === "ai_reply" && f.text) {
          add(`AI: ${f.text}`);
          setBubble(f.text); setSpeaking(true);
          if (f.emotion === "happy" || f.emotion === "neutral" || f.emotion === "encouraging" || f.emotion === "sad") setEmotion(f.emotion);
          setTimeout(() => setSpeaking(false), 2000);
        }
        else if (f.type === "correction") add(`纠错:${f.original}→${f.corrected}`);
        else if (f.type === "hint") add(`引导:${(f.options || []).join("/")}`);
        else if (f.type === "end") { add(`结束:${f.summary}`); setSpeaking(false); setEmotion("neutral"); }
        else if (f.type === "error") add(`错误:${f.message}`);
      });
    } catch (e) { add(`失败:${e}`); }
  }
  const send = () => {
    if (!wsRef.current || !input) return;
    wsRef.current.send(JSON.stringify({ type: "text_input", text: input }));
    add(`我: ${input}`); setInput("");
  };
  const hint = () => wsRef.current?.send(JSON.stringify({ type: "text_input", text: "/hint" }));
  const end = () => wsRef.current?.send(JSON.stringify({ type: "end" }));

  return (
    <View className="page">
      <Picker mode="selector" range={scenarios.map((s) => s.title)} value={idx} onChange={(e) => setIdx(Number(e.detail.value))}>
        <View className="picker">场景:{scenarios[idx]?.title}</View>
      </Picker>
      <Button type="primary" onClick={start}>开始陪练</Button>
      <Avatar emotion={emotion} active={speaking} text={bubble || undefined} />
      <View className="log">{log.map((l, i) => <View key={i}>{l}</View>)}</View>
      <Input value={input} onInput={(e) => setInput(e.detail.value)} placeholder="说点什么(英文)..." />
      <View style={{ display: "flex", gap: "12rpx", marginTop: "12rpx" }}>
        <Button type="primary" onClick={send} style={{ flex: 1 }}>发送</Button>
        <Button onClick={hint}>提示</Button>
        <Button onClick={end}>结束</Button>
      </View>
    </View>
  );
}
