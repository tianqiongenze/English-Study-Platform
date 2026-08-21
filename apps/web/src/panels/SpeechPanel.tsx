import { useState, useRef, useEffect } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { api } from "../api";
import { useAuth } from "../auth";
import type { Scenario } from "../types";
import { EspWsClient } from "@esp/ws-client";
import { useI18n, resolvePersona, type Persona } from "@esp/ui-tokens";
import Avatar, { Emotion } from "../components/Avatar";
import SafeImg, { UNSPLASH } from "../components/SafeImg";
import Skeleton from "../Skeleton";
import { Card, Row, Col, Select, Button, Input, Space, Tag, Typography, Badge, Empty } from "antd";
import { PlayCircleOutlined, SendOutlined, BulbOutlined, PoweroffOutlined, AudioOutlined } from "@ant-design/icons";
import { fadeUp, stagger, EASE, DUR_SLOW } from "../motion";

const { Title, Text, Paragraph } = Typography;
const { TextArea } = Input;

// 口语陪练面板 —— 大厂级对话体验(对标流利说/多邻国口语)。
// antd Card 布局 + framer-motion 气泡弹入 + 数字人 emotion 动画。
export default function SpeechPanel() {
  const { t } = useI18n();
  const { user } = useAuth();
  const [scenarios, setScenarios] = useState<Scenario[]>([{ code: "airport", title: t("scenarios.airport"), system: "", difficulty: 2 }]);
  const [scenariosLoading, setScenariosLoading] = useState(true);
  const [scenario, setScenario] = useState("airport");
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [log, setLog] = useState<{ text: string; role: "ai" | "me" | "system"; emotion?: string }[]>([]);
  const [input, setInput] = useState("");
  const wsRef = useRef<EspWsClient | null>(null);
  const logEndRef = useRef<HTMLDivElement>(null);
  const [emotion, setEmotion] = useState<Emotion>("neutral");
  const [speaking, setSpeaking] = useState(false);
  const [bubble, setBubble] = useState("");
  const [persona, setPersona] = useState<Persona>(() => resolvePersona(undefined, user?.id));

  const addLog = (text: string, role: "ai" | "me" | "system" = "system", emotion?: string) =>
    setLog((l) => [...l, { text, role, emotion }]);

  useEffect(() => {
    api.scenarios().then(setScenarios).catch(() => {}).finally(() => setScenariosLoading(false));
    api.getDigitalHuman().then((r) => setPersona(resolvePersona(r.persona_id, user?.id))).catch(() => {});
    return () => wsRef.current?.close();
  }, []);

  useEffect(() => { logEndRef.current?.scrollIntoView({ behavior: "smooth" }); }, [log]);

  async function startSession() {
    try {
      const data = await api.createSession(scenario);
      setSessionId(data.session_id);
      setLog([]);
      addLog(t("speech.log.createdDetail", { id: data.session_id, scenario }), "system");
      const ws = new EspWsClient({
        url: `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws/speech/${data.session_id}`,
        onFrame: (f) => {
          if (f.type === "ai_reply" && f.text) {
            addLog(f.text, "ai", f.emotion);
            setBubble(f.text);
            setSpeaking(true);
            if (f.emotion === "happy" || f.emotion === "neutral" || f.emotion === "encouraging" || f.emotion === "sad") setEmotion(f.emotion);
            window.setTimeout(() => setSpeaking(false), 2000);
          } else if (f.type === "correction") addLog(`${t("speech.log.correction")}: ${f.original} → ${f.corrected}`, "system");
          else if (f.type === "hint") addLog(`${t("speech.log.hint")}: ${(f.options || []).join(" / ")}`, "system");
          else if (f.type === "end") { addLog(`${t("speech.log.end")}: ${f.summary}`, "system"); setSpeaking(false); setEmotion("neutral"); }
          else if (f.type === "error") addLog(`${t("speech.log.error")}: ${f.message}`, "system");
        },
      });
      ws.connect();
      wsRef.current = ws;
    } catch (e) { addLog(`${t("speech.log.createFail")}: ${e}`, "system"); }
  }

  function send() {
    if (!wsRef.current || !input) return;
    wsRef.current.send(JSON.stringify({ type: "text_input", text: input }));
    addLog(input, "me");
    setInput("");
  }
  const hint = () => wsRef.current?.send(JSON.stringify({ type: "text_input", text: "/hint" }));
  const end = () => wsRef.current?.send(JSON.stringify({ type: "end" }));

  const sceneBanner = (
    <SafeImg src={UNSPLASH.study} alt="" tone={1} className="scene-banner">
      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: DUR_SLOW, ease: EASE }}
        style={{ position: "relative", zIndex: 1 }}
      >
        <Title level={3} style={{ color: "#fff", margin: 0, textShadow: "0 2px 12px rgba(0,0,0,0.4)" }}>{t("nav.speaking")}</Title>
        <Text style={{ color: "rgba(255,255,255,0.9)", textShadow: "0 1px 6px rgba(0,0,0,0.4)" }}>AI 口语陪练 · 实时对话 + 智能纠错</Text>
      </motion.div>
    </SafeImg>
  );

  return (
    <div>
      {sceneBanner}

      <Row gutter={16} style={{ marginTop: 16 }}>
        {/* 左:场景选择 + 控制 */}
        <Col xs={24} md={8}>
          <motion.div variants={fadeUp} initial="hidden" animate="visible">
            <Card title="场景选择" size="small" style={{ marginBottom: 16 }}>
              <Space direction="vertical" style={{ width: "100%" }} size="middle">
                {scenariosLoading ? <Skeleton height={36} /> : (
                  <Select value={scenario} onChange={setScenario} style={{ width: "100%" }} size="large"
                    options={scenarios.map((s) => ({ value: s.code, label: s.title }))} />
                )}
                <Button type="primary" size="large" icon={<PlayCircleOutlined />} onClick={startSession} block>
                  {t("scenarios.start")}
                </Button>
                {sessionId && (
                  <div style={{ textAlign: "center" }}>
                    <Badge status="success" text={<Text type="secondary" style={{ fontSize: 12 }}>会话 {sessionId.slice(0, 8)}...</Text>} />
                  </div>
                )}
              </Space>
            </Card>
            <Card title="操作" size="small">
              <Space direction="vertical" style={{ width: "100%" }}>
                <Button block icon={<BulbOutlined />} onClick={hint}>{t("scenarios.aiHint")}</Button>
                <Button block danger icon={<PoweroffOutlined />} onClick={end}>{t("scenarios.end")}</Button>
              </Space>
            </Card>
          </motion.div>
        </Col>

        {/* 右:数字人 + 对话区 */}
        <Col xs={24} md={16}>
          <motion.div variants={fadeUp} initial="hidden" animate="visible">
            <Card size="small" style={{ marginBottom: 16 }}>
              <div style={{ display: "flex", gap: 16, alignItems: "center", justifyContent: "center", padding: "12px 0" }}>
                <motion.div
                  animate={{ scale: speaking ? 1.05 : 1 }}
                  transition={{ duration: 0.3, ease: EASE }}
                >
                  <Avatar persona={persona} emotion={emotion} active={speaking} text={sessionId ? bubble : undefined} />
                </motion.div>
                <div style={{ textAlign: "left" }}>
                  <Space direction="vertical" size={4}>
                    <Tag color="blue" icon={<AudioOutlined />}>{speaking ? "正在说话..." : "待命"}</Tag>
                    <Text type="secondary" style={{ fontSize: 12 }}>emotion: {emotion}</Text>
                  </Space>
                </div>
              </div>
            </Card>

            {/* 对话气泡区 */}
            <Card title="对话" size="small" style={{ marginBottom: 16, height: 320, overflow: "auto" }}
              styles={{ body: { height: "100%", overflow: "auto", padding: 12 } }}>
              <motion.div variants={stagger} initial="hidden" animate="visible">
                <AnimatePresence>
                  {log.length === 0 ? (
                    <Empty description="开始会话后显示对话" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ marginTop: 60 }} />
                  ) : (
                    log.map((l, i) => (
                      <motion.div key={i} variants={fadeUp} layout
                        style={{ display: "flex", justifyContent: l.role === "me" ? "flex-end" : "flex-start", marginBottom: 8 }}>
                        <div style={{
                          maxWidth: "75%", padding: "8px 14px", borderRadius: 12,
                          background: l.role === "me" ? "var(--esp-brand, #1677ff)" : (l.role === "ai" ? "var(--esp-color-bg-muted, #f0f0f0)" : "var(--esp-color-bg-elevated, #fafafa)"),
                          color: l.role === "me" ? "#fff" : "var(--esp-color-text, #000)",
                          border: l.role === "system" ? "1px dashed var(--esp-color-border, #d9d9d9)" : "none",
                          fontSize: 14,
                        }}>
                          {l.role === "ai" && l.emotion && <Tag style={{ marginBottom: 4 }} color="gold">{l.emotion}</Tag>}
                          <div>{l.text}</div>
                        </div>
                      </motion.div>
                    ))
                  )}
                </AnimatePresence>
              </motion.div>
              <div ref={logEndRef} />
            </Card>

            {/* 输入区 */}
            <Card size="small">
              <Space.Compact style={{ width: "100%" }}>
                <TextArea value={input} onChange={(e) => setInput(e.target.value)} placeholder={t("scenarios.inputPlaceholder")}
                  onPressEnter={(e) => { e.preventDefault(); send(); }} autoSize={{ minRows: 1, maxRows: 3 }} />
                <Button type="primary" size="large" icon={<SendOutlined />} onClick={send}>{t("scenarios.send")}</Button>
              </Space.Compact>
            </Card>
          </motion.div>
        </Col>
      </Row>

      <Paragraph type="secondary" style={{ marginTop: 12, fontSize: 12 }}>{t("speech.hint")}</Paragraph>
    </div>
  );
}
