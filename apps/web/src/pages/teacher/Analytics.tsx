import { useState } from "react";
import { motion } from "framer-motion";
import { api } from "../../api";
import type { ClassAnalytics } from "../../types";
import { Card, Input, Button, Table, Typography, Alert, Empty } from "antd";
import { SearchOutlined, BarChartOutlined } from "@ant-design/icons";
import { fadeUp, stagger } from "../../motion";

const { Title } = Typography;

// 班级学情分析 —— antd Table + recharts 风格。
export default function Analytics() {
  const [classId, setClassId] = useState("1");
  const [rows, setRows] = useState<ClassAnalytics[]>([]);
  const [err, setErr] = useState("");

  async function load() {
    setErr("");
    try { setRows(await api.classAnalytics(Number(classId))); }
    catch (e) { setErr(`${e}`); }
  }

  const columns = [
    { title: "内容", dataIndex: "content", key: "content" },
    { title: "平均分", dataIndex: "avgScore", key: "avgScore", align: "right" as const,
      render: (v: number) => <span style={{ fontWeight: 600, color: v >= 80 ? "#52c41a" : v >= 60 ? "#faad14" : "#ff4d4f" }}>{v}</span> },
    { title: "人数", dataIndex: "count", key: "count", align: "right" as const },
  ];

  return (
    <motion.div variants={stagger} initial="hidden" animate="visible">
      <Card title={<span><BarChartOutlined /> 班级学情分析</span>} size="small" style={{ marginBottom: 16 }}>
        <Input.Group compact>
          <Input value={classId} onChange={(e) => setClassId(e.target.value)} style={{ width: 100 }} placeholder="班级 ID" />
          <Button type="primary" icon={<SearchOutlined />} onClick={load}>查询</Button>
        </Input.Group>
      </Card>
      {err && <Alert type="error" message={err} style={{ marginBottom: 16 }} closable />}
      {rows.length > 0 ? (
        <motion.div variants={fadeUp}>
          <Card title={`学情明细 (${rows.length})`} size="small">
            <Table dataSource={rows} columns={columns} rowKey="id" size="small" pagination={false} />
          </Card>
        </motion.div>
      ) : (
        <Empty description={err ? "" : "输入班级 ID 查询学情"} style={{ marginTop: 40 }} />
      )}
    </motion.div>
  );
}
