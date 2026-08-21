import { useState } from "react";
import { motion } from "framer-motion";
import { api } from "../../api";
import type { LessonPlan as LessonPlanType } from "../../types";
import { Card, Input, Select, Button, Typography, List, Tag, Space, Alert, Empty } from "antd";
import { ThunderboltOutlined, BookOutlined, TrophyOutlined, MessageOutlined } from "@ant-design/icons";
import { fadeUp, stagger } from "../../motion";

const { Title, Text, Paragraph } = Typography;
const { TextArea } = Input;

// 教师一键教案生成 —— antd Form + 结果 Card(课文 + 练习/游戏/讨论 List)。
export default function LessonPlan() {
  const [topic, setTopic] = useState("Travel");
  const [level, setLevel] = useState("B1");
  const [plan, setPlan] = useState<LessonPlanType | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");

  async function generate() {
    setLoading(true); setErr("");
    try { setPlan(await api.lessonPlan(topic, level)); }
    catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }

  return (
    <motion.div variants={stagger} initial="hidden" animate="visible">
      <Card title="一键教案生成" size="small" style={{ marginBottom: 16 }}>
        <Space wrap>
          <Input value={topic} onChange={(e) => setTopic(e.target.value)} placeholder="主题(如 Travel)" style={{ width: 200 }} />
          <Select value={level} onChange={setLevel} style={{ width: 100 }}
            options={[{ value: "A2", label: "A2" }, { value: "B1", label: "B1" }, { value: "B2", label: "B2" }, { value: "C1", label: "C1" }]} />
          <Button type="primary" icon={<ThunderboltOutlined />} onClick={generate} loading={loading}>生成教案</Button>
        </Space>
      </Card>
      {err && <Alert type="error" message={err} style={{ marginBottom: 16 }} closable />}
      {plan && (
        <motion.div variants={fadeUp}>
          <Card title={<span><BookOutlined /> 课文</span>} size="small" style={{ marginBottom: 12 }}>
            <Paragraph style={{ whiteSpace: "pre-wrap", margin: 0 }}>{plan.passage}</Paragraph>
          </Card>
          <Card title={`练习 (${plan.exercises.length})`} size="small" style={{ marginBottom: 12 }}>
            <List size="small" dataSource={plan.exercises} locale={{ emptyText: "无" }} renderItem={(e: any, i) => (
              <List.Item><Text code>{i + 1}</Text> {typeof e === "string" ? e : JSON.stringify(e)}</List.Item>
            )} />
          </Card>
          <Card title={<span><TrophyOutlined /> 游戏 ({plan.games.length})</span>} size="small" style={{ marginBottom: 12 }}>
            <List size="small" dataSource={plan.games} locale={{ emptyText: "无" }} renderItem={(g: any, i) => (
              <List.Item><Text code>{i + 1}</Text> {typeof g === "string" ? g : JSON.stringify(g)}</List.Item>
            )} />
          </Card>
          <Card title={<span><MessageOutlined /> 讨论 ({plan.discussion.length})</span>} size="small">
            <List size="small" dataSource={plan.discussion} locale={{ emptyText: "无" }} renderItem={(d: any, i) => (
              <List.Item><Text code>{i + 1}</Text> {typeof d === "string" ? d : JSON.stringify(d)}</List.Item>
            )} />
          </Card>
        </motion.div>
      )}
      {!plan && !err && <Empty description="输入主题生成教案" style={{ marginTop: 40 }} />}
    </motion.div>
  );
}
