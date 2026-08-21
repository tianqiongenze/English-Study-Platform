import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { api } from "../api";
import type { GradeResult, PolishResult, TranslateResult } from "../types";
import { useI18n } from "@esp/ui-tokens";
import SafeImg, { UNSPLASH } from "../components/SafeImg";
import { Card, Tabs, Input, Button, Typography, Statistic, Descriptions, Tag, List, Space, Alert, Steps } from "antd";
import { LoadingOutlined, ThunderboltOutlined, CheckCircleOutlined, EditOutlined } from "@ant-design/icons";
import { fadeUp, stagger, EASE, DUR_SLOW } from "../motion";

const { Title, Text, Paragraph } = Typography;
const { TextArea } = Input;

type Tab = "grade" | "polish" | "translate" | "agent";

// 写作批改面板 —— 大厂级批改体验(antd Steps + Statistic 分数 + Descriptions 逐项)。
export default function WritingPanel() {
  const { t } = useI18n();
  const [tab, setTab] = useState<Tab>("grade");
  const [prompt, setPrompt] = useState("Do you agree technology improves education?");
  const [essay, setEssay] = useState("");
  const [text, setText] = useState("");
  const [grade, setGrade] = useState<GradeResult | null>(null);
  const [polish, setPolish] = useState<PolishResult | null>(null);
  const [translate, setTranslate] = useState<TranslateResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");
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

  async function agentRun() {
    setLoading(true); setErr(""); setAgentNotes("");
    try {
      const r = await api.writingRun(prompt, essay, "toefl", true);
      setThreadId(r.thread_id); setAgentStatus(r.status);
      if (r.notes) setAgentNotes(r.notes ?? "");
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }
  async function agentReview() {
    if (!threadId) return;
    setLoading(true); setErr("");
    try {
      const r = await api.writingReview(threadId, teacherFb);
      setAgentStatus(r.status); setAgentNotes(r.notes ?? "");
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }
  async function agentState() {
    if (!threadId) return;
    setLoading(true); setErr("");
    try {
      const r = await api.writingState(threadId);
      setAgentStatus(r.status); if (r.notes) setAgentNotes(r.notes ?? "");
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }

  return (
    <div>
      <SafeImg src={UNSPLASH.writing} alt="" tone={3} className="scene-banner">
        <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: DUR_SLOW, ease: EASE }} style={{ position: "relative", zIndex: 1 }}>
          <Title level={3} style={{ color: "#fff", margin: 0, textShadow: "0 2px 12px rgba(0,0,0,0.4)" }}>{t("nav.writing")}</Title>
          <Text style={{ color: "rgba(255,255,255,0.9)", textShadow: "0 1px 6px rgba(0,0,0,0.4)" }}>AI 写作批改 · 智能体 Human-in-Loop</Text>
        </motion.div>
      </SafeImg>

      {err && <Alert type="error" message={err} style={{ margin: "12px 0" }} closable onClose={() => setErr("")} />}

      <Card style={{ marginTop: 16 }}>
        <Tabs
          activeKey={tab}
          onChange={(k) => setTab(k as Tab)}
          items={[
            {
              key: "grade", label: <span><CheckCircleOutlined /> {t("writing.grade")}</span>,
              children: (
                <Space direction="vertical" style={{ width: "100%" }} size="middle">
                  <Input value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder={t("writing.topic")} size="large" />
                  <TextArea rows={6} value={essay} onChange={(e) => setEssay(e.target.value)} placeholder={t("writing.placeholder")} />
                  <Button type="primary" size="large" icon={loading ? <LoadingOutlined /> : <CheckCircleOutlined />} onClick={run} loading={loading}>{t("writing.grade")}</Button>
                  <AnimatePresence>
                    {grade && (
                      <motion.div variants={stagger} initial="hidden" animate="visible">
                        <motion.div variants={fadeUp}>
                          <Card type="inner" style={{ background: "var(--esp-color-bg-subtle, #fafafa)" }}>
                            <Space size="large">
                              <Statistic title="预测分值" value={grade.score} suffix="/30" valueStyle={{ color: "#1677ff", fontSize: 36 }} />
                              <div style={{ flex: 1 }}><Text strong>总评</Text><Paragraph>{grade.feedback}</Paragraph></div>
                            </Space>
                          </Card>
                        </motion.div>
                        <motion.div variants={fadeUp} style={{ marginTop: 12 }}>
                          <Card type="inner" title={`连贯性 ${grade.coherence.score}/10`} size="small">
                            <Paragraph type="secondary">{grade.coherence.paragraph_flow}</Paragraph>
                          </Card>
                        </motion.div>
                        {grade.grammar.length > 0 && (
                          <motion.div variants={fadeUp} style={{ marginTop: 12 }}>
                            <Card type="inner" title="语法问题" size="small">
                              <List size="small" dataSource={grade.grammar} renderItem={(g) => (
                                <List.Item><Text><Text delete>{g.original}</Text> → <Text strong>{g.corrected}</Text> <Tag>{g.reason}</Tag></Text></List.Item>
                              )} />
                            </Card>
                          </motion.div>
                        )}
                      </motion.div>
                    )}
                  </AnimatePresence>
                </Space>
              ),
            },
            {
              key: "polish", label: <span><EditOutlined /> {t("writing.polish")}</span>,
              children: (
                <Space direction="vertical" style={{ width: "100%" }} size="middle">
                  <TextArea rows={6} value={text} onChange={(e) => setText(e.target.value)} placeholder="输入要润色的英文..." />
                  <Button type="primary" icon={<EditOutlined />} onClick={run} loading={loading}>{t("writing.polish")}</Button>
                  {polish && (
                    <Card type="inner" title="润色结果" size="small">
                      <Paragraph>{polish.polished}</Paragraph>
                      {polish.suggestions.length > 0 && (
                        <List size="small" header={<Text strong>建议</Text>} dataSource={polish.suggestions} renderItem={(s) => (
                          <List.Item><Text><Text delete>{s.original}</Text> → <Text strong>{s.polished}</Text> <Tag>{s.reason}</Tag></Text></List.Item>
                        )} />
                      )}
                    </Card>
                  )}
                </Space>
              ),
            },
            {
              key: "translate", label: <span>🌐 {t("writing.translate")}</span>,
              children: (
                <Space direction="vertical" style={{ width: "100%" }} size="middle">
                  <TextArea rows={6} value={text} onChange={(e) => setText(e.target.value)} placeholder="输入要翻译的英文..." />
                  <Button type="primary" onClick={run} loading={loading}>{t("writing.translate")}</Button>
                  {translate && (
                    <Card type="inner" title={t("writing.translate")} size="small">
                      <Paragraph>{translate.translation}</Paragraph>
                      {translate.vocab.length > 0 && (
                        <List size="small" header={<Text strong>词汇</Text>} dataSource={translate.vocab} renderItem={(v) => (
                          <List.Item><Text><Text strong>{v.word}</Text> <Tag>{v.pos}</Tag> {v.meaning} — {v.example}</Text></List.Item>
                        )} />
                      )}
                    </Card>
                  )}
                </Space>
              ),
            },
            {
              key: "agent", label: <span><ThunderboltOutlined /> 智能体批改</span>,
              children: (
                <Space direction="vertical" style={{ width: "100%" }} size="middle">
                  <Input value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder={t("writing.topic")} size="large" />
                  <TextArea rows={6} value={essay} onChange={(e) => setEssay(e.target.value)} placeholder={t("writing.placeholder")} />
                  <Button type="primary" size="large" icon={<ThunderboltOutlined />} onClick={agentRun} loading={loading}>发起 Human-in-Loop 批改</Button>
                  {threadId && (
                    <Card type="inner" size="small">
                      <Descriptions size="small" column={1} bordered>
                        <Descriptions.Item label="thread_id"><Text code>{threadId}</Text></Descriptions.Item>
                        <Descriptions.Item label="状态"><Tag color={agentStatus === "awaiting_review" ? "warning" : "processing"}>{agentStatus}</Tag></Descriptions.Item>
                        {agentNotes && <Descriptions.Item label="笔记"><Text type="secondary">{agentNotes}</Text></Descriptions.Item>}
                      </Descriptions>
                      {agentStatus === "awaiting_review" && (
                        <Space direction="vertical" style={{ width: "100%", marginTop: 12 }} size="middle">
                          <TextArea rows={3} value={teacherFb} onChange={(e) => setTeacherFb(e.target.value)} placeholder="教师人工审阅意见..." />
                          <Button type="primary" onClick={agentReview} loading={loading}>提交人工审阅(恢复 LangGraph)</Button>
                        </Space>
                      )}
                      <Button style={{ marginTop: 8 }} onClick={agentState} loading={loading}>查询状态</Button>
                    </Card>
                  )}
                </Space>
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
