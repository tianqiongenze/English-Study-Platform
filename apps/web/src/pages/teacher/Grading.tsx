import { useState } from "react";
import { motion } from "framer-motion";
import { api } from "../../api";
import { Card, Input, Button, Typography, Alert, Space, Statistic, Row, Col } from "antd";
import { CheckCircleOutlined, AuditOutlined, TrophyOutlined } from "@ant-design/icons";
import { fadeUp, stagger } from "../../motion";

const { Title, Text, Paragraph } = Typography;

// 教师作业评阅 —— antd Form + Statistic(自动批改/终审结果)。
export default function Grading() {
  const [assignmentId, setAssignmentId] = useState("1");
  const [classId, setClassId] = useState("1");
  const [graded, setGraded] = useState<number | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  const [subId, setSubId] = useState("");
  const [finalScore, setFinalScore] = useState("");
  const [finalized, setFinalized] = useState<number | null>(null);

  async function autoGrade() {
    setErr(""); setGraded(null); setLoading(true);
    try {
      const r = await api.autoGrade(Number(assignmentId), classId ? Number(classId) : undefined);
      setGraded(r.graded);
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }
  async function finalize() {
    setErr(""); setFinalized(null); setLoading(true);
    try {
      const r = await api.finalize(Number(subId), Number(finalScore), classId ? Number(classId) : undefined);
      setFinalized(r.finalized);
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }

  return (
    <motion.div variants={stagger} initial="hidden" animate="visible">
      <Card title={<span><AuditOutlined /> 作业评阅(写作智能体自动批改 + 教师终审)</span>} size="small" style={{ marginBottom: 16 }}>
        <Paragraph type="secondary">原型 3.6 / F14:先批量自动评阅(写作智能体),再对单个 submission 终审确认最终分。</Paragraph>
      </Card>

      <motion.div variants={fadeUp}>
        <Card title="1. 批量自动评阅" size="small" style={{ marginBottom: 16 }}>
          <Space wrap style={{ marginBottom: 12 }}>
            <Space><Text>作业 ID:</Text><Input value={assignmentId} onChange={(e) => setAssignmentId(e.target.value)} style={{ width: 100 }} /></Space>
            <Space><Text>班级 ID:</Text><Input value={classId} onChange={(e) => setClassId(e.target.value)} style={{ width: 100 }} /></Space>
          </Space>
          <div><Button type="primary" icon={<CheckCircleOutlined />} onClick={autoGrade} loading={loading}>批量自动评阅</Button></div>
          {graded !== null && (
            <Row gutter={16} style={{ marginTop: 16 }}>
              <Col span={8}><Statistic title="已自动批改" value={graded} suffix="份" prefix={<CheckCircleOutlined style={{ color: "#52c41a" }} />} valueStyle={{ color: "#52c41a" }} /></Col>
            </Row>
          )}
        </Card>
      </motion.div>

      <motion.div variants={fadeUp}>
        <Card title="2. 教师终审(可选,调整最终分)" size="small">
          <Space wrap style={{ marginBottom: 12 }}>
            <Space><Text>提交 ID:</Text><Input value={subId} onChange={(e) => setSubId(e.target.value)} style={{ width: 100 }} /></Space>
            <Space><Text>最终分:</Text><Input value={finalScore} onChange={(e) => setFinalScore(e.target.value)} style={{ width: 100 }} placeholder="0-30" /></Space>
          </Space>
          <div><Button type="primary" danger icon={<TrophyOutlined />} onClick={finalize} loading={loading} disabled={!subId || !finalScore}>确认终审</Button></div>
          {finalized !== null && (
            <Alert type="success" showIcon icon={<TrophyOutlined />} style={{ marginTop: 16 }}
              message={`已终审 ${finalized} 份提交,分数已落库。`} />
          )}
        </Card>
      </motion.div>

      {err && <Alert type="error" message={err} style={{ marginTop: 16 }} closable />}
    </motion.div>
  );
}
