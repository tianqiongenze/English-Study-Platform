import { useState, useEffect } from "react";
import { motion } from "framer-motion";
import { api } from "../api";
import { useAuth } from "../auth";
import type { Mastery } from "../types";
import { Card, Input, Button, Typography, Statistic, Row, Col, Space, Empty, Tag } from "antd";
import { RocketOutlined, BookOutlined, BulbOutlined, BulbFilled } from "@ant-design/icons";
import { fadeUp, stagger } from "../motion";

const { Title, Text, Paragraph } = Typography;

// 学习中心面板 —— antd Card + Statistic + 消除硬编码 userId(后端从 JWT 取)。
export default function LearningPanel({ mode = "event" }: { mode?: "event" | "review" | "difficulty" }) {
  const { user } = useAuth();
  // userId 不再硬编码 "1";后端 P0-2 从 JWT 取,前端 ?user_id= 被忽略。
  // 保留输入框仅为兼容/调试,默认用 auth user.id。
  const [userId, setUserId] = useState(String(user?.id ?? 1));
  const [event, setEvent] = useState("");
  const [result, setResult] = useState<string>("");
  const [placed, setPlaced] = useState(false);
  const [placeInfo, setPlaceInfo] = useState("");
  const [reviewCount, setReviewCount] = useState(0);
  const [weakCount, setWeakCount] = useState(0);

  useEffect(() => {
    if (placed) return;
    (async () => {
      try {
        const r = await api.placement(Number(userId), [1, 2, 3, 4, 5], 0.4);
        setPlaceInfo(`能力定级完成:初始化 ${r.initialized} 个知识点基线`);
      } catch (e) { setPlaceInfo(`定级跳过:${e}`); }
      finally { setPlaced(true); }
    })();
  }, [userId, placed]);

  async function reportEvent() {
    try {
      const d = await api.reportEvent(Number(userId), "answer", event || "{}");
      setResult(JSON.stringify(d, null, 2));
    } catch (e) { setResult(`错误: ${e}`); }
  }
  async function review() {
    try {
      const d: Mastery[] = await api.review(Number(userId));
      setReviewCount(d.length);
      setResult(d.length ? d.map((m) => `kp${m.kpId}: ${m.masteryScore} (下次 ${m.nextReviewAt})`).join("\n") : "今日无复习任务");
    } catch (e) { setResult(`错误: ${e}`); }
  }
  async function weak() {
    try {
      const d: Mastery[] = await api.weak(Number(userId));
      setWeakCount(d.length);
      setResult(d.length ? d.map((m) => `kp${m.kpId}: ${m.masteryScore}`).join("\n") : "无数据");
    } catch (e) { setResult(`错误: ${e}`); }
  }
  async function difficulty() {
    try {
      const d = await api.difficulty(3, true);
      setResult(`下一难度: ${d.next_difficulty}, 倍速: ${d.speed_multiplier}, 脚手架: ${d.scaffolding}`);
    } catch (e) { setResult(`错误: ${e}`); }
  }
  useEffect(() => {
    const t0 = Date.now();
    return () => {
      const duration = Date.now() - t0;
      api.reportBehavior(Number(userId), "page_stay", "LearningPanel", duration).catch(() => {});
    };
  }, [userId]);
  async function recommend() {
    try {
      const d = await api.recommend(Number(userId));
      const parts = [
        `复习清单:${(d.review as any[]).length || 0} 项`,
        `薄弱知识点:${(d.weak_points as any[]).length || 0} 项`,
        `相似场景推荐:${(d.similar_scenarios as any[]).length || 0} 项`,
      ];
      setResult(parts.join("\n"));
    } catch (e) { setResult(`错误: ${e}`); }
  }

  const cardStyle = { marginBottom: 16 };

  if (mode === "review") {
    return (
      <motion.div variants={fadeUp} initial="hidden" animate="visible">
        <Card title="今日复习" size="small" style={cardStyle}>
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            <Space>
              <Text type="secondary">用户 ID:</Text>
              <Input value={userId} onChange={(e) => setUserId(e.target.value)} style={{ width: 100 }} size="small" />
            </Space>
            <Button type="primary" icon={<BookOutlined />} onClick={review}>查询今日复习</Button>
            <Button onClick={weak}>薄弱知识点</Button>
            {reviewCount > 0 && <Tag color="blue">{reviewCount} 项复习</Tag>}
            <pre style={{ background: "var(--esp-color-bg-muted, #f5f5f5)", padding: 12, borderRadius: 6, fontSize: 12, overflow: "auto", maxHeight: 300, margin: 0 }}>{result}</pre>
          </Space>
        </Card>
      </motion.div>
    );
  }
  if (mode === "difficulty") {
    return (
      <motion.div variants={fadeUp} initial="hidden" animate="visible">
        <Card title="动态难度" size="small" style={cardStyle}>
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            <Paragraph type="secondary">模拟一次答题(当前难度 3,答对)以触发动态难度调整。</Paragraph>
            <Button type="primary" icon={<RocketOutlined />} onClick={difficulty}>计算下一难度</Button>
            <pre style={{ background: "var(--esp-color-bg-muted, #f5f5f5)", padding: 12, borderRadius: 6, fontSize: 12, overflow: "auto", maxHeight: 200, margin: 0 }}>{result}</pre>
          </Space>
        </Card>
      </motion.div>
    );
  }
  return (
    <motion.div variants={stagger} initial="hidden" animate="visible">
      <Card size="small" style={cardStyle}>
        <Row gutter={16}>
          <Col span={8}><Statistic title="用户 ID" value={userId} prefix={<BulbOutlined />} /></Col>
          <Col span={8}><Statistic title="复习项" value={reviewCount} prefix={<BookOutlined />} /></Col>
          <Col span={8}><Statistic title="薄弱点" value={weakCount} prefix={<BulbFilled />} valueStyle={{ color: weakCount > 0 ? "#faad14" : undefined }} /></Col>
        </Row>
      </Card>

      {placeInfo && (
        <motion.div variants={fadeUp}>
          <Card size="small" style={cardStyle}><Paragraph>💡 {placeInfo}</Paragraph></Card>
        </motion.div>
      )}

      <motion.div variants={fadeUp}>
        <Card title="学习操作" size="small" style={cardStyle}>
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            <Space wrap>
              <Button type="primary" onClick={reportEvent}>上报学情事件</Button>
              <Button onClick={review}>今日复习</Button>
              <Button onClick={weak}>薄弱知识点</Button>
              <Button onClick={recommend}>为你推荐</Button>
            </Space>
            <Input value={event} onChange={(e) => setEvent(e.target.value)} placeholder='事件 payload JSON,如 {"kp_id":1,"correct":true}' />
            {result ? (
              <pre style={{ background: "var(--esp-color-bg-muted, #f5f5f5)", padding: 12, borderRadius: 6, fontSize: 12, overflow: "auto", maxHeight: 300, margin: 0 }}>{result}</pre>
            ) : (
              <Empty description="点击按钮查询学习数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </Space>
        </Card>
      </motion.div>
    </motion.div>
  );
}
