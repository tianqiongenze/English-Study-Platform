import { useCallback, useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import { api } from "../../api";
import { useI18n, type MessageKey } from "@esp/ui-tokens";
import SafeImg, { UNSPLASH } from "../../components/SafeImg";
import type { ContentCourse, ContentItem, ContentLesson } from "@esp/types";
import { Card, Segmented, List, Typography, Tag, Radio, Button, Empty, Alert, Row, Col, Space, Statistic } from "antd";
import { CheckCircleOutlined, CloseCircleOutlined, BookOutlined } from "@ant-design/icons";
import { fadeUp, stagger, EASE, DUR_SLOW } from "../../motion";

const { Title, Text, Paragraph } = Typography;

const EXAMS = [
  { tag: "", label: "全部" }, { tag: "CET4", label: "四级 CET-4" }, { tag: "CET6", label: "六级 CET-6" },
  { tag: "TOEFL", label: "托福 TOEFL" }, { tag: "IELTS", label: "雅思 IELTS" },
];
const SKILL_LABEL: Record<string, string> = { vocab: "词汇", reading: "阅读", listening: "听力", grammar: "语法" };

function ItemCard({ item, onDone, t }: { item: ContentItem; onDone: () => void; t: (k: MessageKey, vars?: Record<string, string | number>) => string }) {
  const [pick, setPick] = useState<number | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<{ correct: boolean; score: number; explanation: string } | null>(null);
  const p = item.prompt || {};
  const isMC = item.item_type !== "vocab";

  async function answer(optionIndex?: number) {
    if (submitting || result) return;
    setSubmitting(true);
    try {
      const body = isMC ? { option_index: optionIndex } : {};
      const r = await api.submitAnswer(item.id, body, 0);
      setResult(r);
      setPick(optionIndex ?? null);
      if (r.correct) onDone();
    } catch (e) { setResult({ correct: false, score: 0, explanation: `提交失败: ${e}` }); }
    finally { setSubmitting(false); }
  }

  return (
    <motion.div layout variants={fadeUp}>
      <Card size="small" style={{ marginBottom: 8 }}>
        {isMC ? (
          <>
            <Paragraph>{p.question || p.title}</Paragraph>
            {p.text && <Paragraph type="secondary" style={{ fontSize: 12, whiteSpace: "pre-wrap" }}><details><summary>阅读全文</summary>{p.text}</details></Paragraph>}
            {p.script && <Paragraph type="secondary" style={{ fontSize: 12, whiteSpace: "pre-wrap" }}><details><summary>听力原文</summary>{p.script}</details></Paragraph>}
            <Radio.Group onChange={(e) => answer(e.target.value)} value={pick} disabled={!!result || submitting} style={{ width: "100%" }}>
              <Space direction="vertical" style={{ width: "100%" }}>
                {(p.options || []).map((opt, i) => (
                  <Radio key={i} value={i} style={{
                    padding: "8px 10px", borderRadius: 6, width: "100%",
                    background: pick === i ? "var(--esp-color-bg-subtle, #fafafa)" : undefined,
                  }}>
                    {String.fromCharCode(65 + i)}. {opt}
                  </Radio>
                ))}
              </Space>
            </Radio.Group>
          </>
        ) : (
          <>
            <Space>
              <Text strong style={{ fontSize: 20 }}>{p.word}</Text>
              <Text type="secondary">{p.phonetic ? `/${p.phonetic}/` : ""} {p.pos ? `<${p.pos}>` : ""}</Text>
              {item.cefr && <Tag color="green">{item.cefr}</Tag>}
            </Space>
            <Paragraph style={{ marginTop: 6 }}>
              <Text strong>{p.meaning_en}</Text><br />
              <Text type="secondary">{p.meaning_zh}</Text>
            </Paragraph>
            {p.example && <Paragraph type="secondary" italic style={{ fontSize: 13 }}>例: {p.example}</Paragraph>}
            <Button type="primary" disabled={!!result || submitting} onClick={() => answer()} loading={submitting}>
              {result ? "已完成" : "标记已掌握"}
            </Button>
          </>
        )}
        {result && (
          <Alert
            style={{ marginTop: 8 }}
            type={result.correct ? "success" : "error"}
            showIcon
            icon={result.correct ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
            message={result.correct ? "✓ 正确" : "✗ 未答对"}
            description={<>{result.score ? `${t("result.score")} ${result.score} · ` : ""}{result.explanation}</>}
          />
        )}
      </Card>
    </motion.div>
  );
}

export default function ContentLibrary() {
  const { t } = useI18n();
  const [examTag, setExamTag] = useState("");
  const [courses, setCourses] = useState<ContentCourse[]>([]);
  const [lessons, setLessons] = useState<ContentLesson[]>([]);
  const [items, setItems] = useState<ContentItem[]>([]);
  const [selCourse, setSelCourse] = useState<ContentCourse | null>(null);
  const [selLesson, setSelLesson] = useState<ContentLesson | null>(null);
  const [err, setErr] = useState("");

  const loadCourses = useCallback(async () => {
    try { setCourses(await api.listCourses(examTag)); setErr(""); } catch (e) { setErr(`加载课程失败: ${e}`); }
  }, [examTag]);
  useEffect(() => { loadCourses(); }, [loadCourses]);

  async function openCourse(c: ContentCourse) {
    setSelCourse(c); setSelLesson(null); setItems([]);
    try { setLessons(await api.listLessons(c.id)); } catch (e) { setErr(`加载课时失败: ${e}`); }
  }
  async function openLesson(l: ContentLesson) {
    setSelLesson(l); setItems([]);
    try { setItems(await api.listItems(l.id)); } catch (e) { setErr(`加载题目失败: ${e}`); }
  }
  const totalItems = useMemo(() => items.length, [items]);

  return (
    <motion.div variants={stagger} initial="hidden" animate="visible" className="motion-fade">
      <SafeImg src={UNSPLASH.forest} alt="" tone={3} className="scene-banner">
        <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: DUR_SLOW, ease: EASE }} style={{ position: "relative", zIndex: 1 }}>
          <Title level={3} style={{ color: "#fff", margin: 0, textShadow: "0 2px 12px rgba(0,0,0,0.4)" }}>{t("content.title")}</Title>
          <Text style={{ color: "rgba(255,255,255,0.9)" }}>共享学习内容 · 四六级/托福/雅思 · 逐题作答</Text>
        </motion.div>
      </SafeImg>

      {err && <Alert type="error" message={err} style={{ marginTop: 12, marginBottom: 12 }} closable />}

      <Card size="small" style={{ marginTop: 12, marginBottom: 12 }}>
        <Segmented value={examTag} onChange={(v) => setExamTag(v as string)} options={EXAMS.map((e) => ({ label: e.label, value: e.tag }))} />
      </Card>

      <Row gutter={12}>
        {/* 课程 */}
        <Col xs={24} md={6}>
          <motion.div variants={fadeUp}>
            <Card title={<span><BookOutlined /> 课程</span>} size="small">
              <List size="small" dataSource={courses} locale={{ emptyText: "无课程" }} renderItem={(c) => (
                <List.Item onClick={() => openCourse(c)} style={{
                  cursor: "pointer", padding: "8px",
                  background: selCourse?.id === c.id ? "var(--esp-brand-soft, rgba(22,119,255,0.12))" : undefined,
                  borderRadius: 6,
                }}>
                  <List.Item.Meta
                    title={<Text strong style={{ fontSize: 13 }}>{c.title}</Text>}
                    description={<Text type="secondary" style={{ fontSize: 11 }}>{c.exam_tag} · CEFR {c.cefr_range}</Text>}
                  />
                </List.Item>
              )} />
            </Card>
          </motion.div>
        </Col>
        {/* 课时 */}
        <Col xs={24} md={6}>
          <motion.div variants={fadeUp}>
            <Card title="课时" size="small">
              {!selCourse ? (
                <Empty description="先选课程" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              ) : lessons.length ? (
                <List size="small" dataSource={lessons} renderItem={(l) => (
                  <List.Item onClick={() => openLesson(l)} style={{
                    cursor: "pointer", padding: "8px",
                    background: selLesson?.id === l.id ? "var(--esp-brand-soft, rgba(22,119,255,0.12))" : undefined,
                    borderRadius: 6,
                  }}>
                    <List.Item.Meta
                      title={<Text strong style={{ fontSize: 13 }}>{l.title}</Text>}
                      description={<Text type="secondary" style={{ fontSize: 11 }}>{SKILL_LABEL[l.skill] || l.skill} · {l.cefr || "-"}</Text>}
                    />
                  </List.Item>
                )} />
              ) : (
                <Empty description="该课程课时建设中" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              )}
            </Card>
          </motion.div>
        </Col>
        {/* 题目 */}
        <Col xs={24} md={12}>
          <motion.div variants={fadeUp}>
            {selLesson ? (
              <Card title={selLesson.title} size="small" extra={<Statistic title="题数" value={totalItems} valueStyle={{ fontSize: 14 }} />}>
                {items.length ? (
                  <motion.div variants={stagger} initial="hidden" animate="visible">
                    {items.map((it) => <ItemCard key={it.id} item={it} onDone={() => {}} t={t} />)}
                  </motion.div>
                ) : (
                  <Empty description="该课时暂无题目" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>
            ) : (
              <Card size="small" style={{ borderStyle: "dashed" }}>
                <Empty description="选择左侧课程与课时开始学习" style={{ margin: "40px 0" }} />
              </Card>
            )}
          </motion.div>
        </Col>
      </Row>
    </motion.div>
  );
}
