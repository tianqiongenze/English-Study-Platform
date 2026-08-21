// 缓存监控 Tab(Phase4 · 2026-08-21)—— L1 Redis + L2 Milvus + L3 Neo4j 三层缓存实时情况。
// 数据源:admin-go /api/admin/cache/stats(LCO 命中率 + Milvus collection + Neo4j 节点)+ /cache/milvus + /cache/neo4j。
// 仿 OpsPanel 资源监控 Tab 的 Statistic + recharts 折线 + Card 范式。5s 刷新,降级显示 DOWN。
import { useCallback, useEffect, useState } from "react";
import { Card, Row, Col, Statistic, Progress, Tag, Empty, Typography, Alert } from "antd";
import { DatabaseOutlined, ThunderboltOutlined, ShareAltOutlined } from "@ant-design/icons";
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from "recharts";
import { api } from "../../api";
import type { AdminCacheStatsResponse } from "@esp/api-client";

const { Text } = Typography;
const REFRESH_MS = 5000;
const HISTORY_LEN = 60;

export default function CachePanel() {
  const [stats, setStats] = useState<AdminCacheStatsResponse | null>(null);
  const [history, setHistory] = useState<number[]>([]); // L1 命中率历史
  const [err, setErr] = useState("");

  const load = useCallback(async () => {
    try {
      const r = await api.adminCacheStats();
      setStats(r);
      setErr("");
      // L1 命中率推入历史
      const l1 = r.layers?.find((l) => l.layer === "L1+L2+L3");
      const rate = parseFloat(l1?.hit_rate ?? "");
      setHistory((prev) => {
        const arr = [...prev, isNaN(rate) ? 0 : rate * 100];
        if (arr.length > HISTORY_LEN) arr.shift();
        return arr;
      });
    } catch (e) {
      setErr(`${e}`);
    }
  }, []);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const l1 = stats?.layers?.find((l) => l.layer === "L1+L2+L3");
  const milvus = stats?.layers?.find((l) => l.name === "Milvus");
  const neo4j = stats?.layers?.find((l) => l.name === "Neo4j");
  const chartData = history.map((v, i) => ({ t: i, hitRate: v }));

  const statusTag = (st?: string) =>
    !st ? null : st === "ok" ? <Tag color="success">UP</Tag> : st === "na" ? <Tag color="default">N/A</Tag> : <Tag color="error">DOWN</Tag>;

  return (
    <div>
      {err && <Alert type="error" message={err} style={{ marginBottom: 12 }} closable />}

      <Row gutter={[12, 12]}>
        <Col flex="auto">
          <Card size="small" title={<span><ThunderboltOutlined /> L1+L2+L3 命中率 {statusTag(l1?.status)}</span>}>
            <Statistic
              title="综合命中率"
              value={l1?.hit_rate ? (parseFloat(l1.hit_rate) * 100).toFixed(2) : "N/A"}
              suffix={l1?.hit_rate ? "%" : ""}
              valueStyle={{ color: "#1677ff" }}
            />
            <div style={{ fontSize: 12, marginTop: 8 }}>
              <Text type="secondary">命中 {l1?.hits || "-"}</Text> · <Text type="secondary">未命中 {l1?.misses || "-"}</Text>
            </div>
            {l1?.extra?.l2_hits !== undefined && (
              <div style={{ fontSize: 12 }}>
                <Text type="secondary">L2 命中 {l1.extra.l2_hits} / 未命中 {l1.extra.l2_misses}</Text>
              </div>
            )}
          </Card>
        </Col>
        <Col flex="auto">
          <Card size="small" title={<span><DatabaseOutlined /> L2 Milvus 向量库 {statusTag(milvus?.status)}</span>}>
            <Statistic
              title={milvus?.extra?.collection || "collection"}
              value={milvus?.extra?.num_entities ?? "N/A"}
              suffix="条"
            />
          </Card>
        </Col>
        <Col flex="auto">
          <Card size="small" title={<span><ShareAltOutlined /> L3 Neo4j 知识图谱 {statusTag(neo4j?.status)}</span>}>
            {neo4j?.extra ? (
              <div style={{ fontSize: 13, lineHeight: 2 }}>
                {Object.entries(neo4j.extra).map(([k, v]) => (
                  <div key={k}><Tag color="blue">{k}</Tag> {v}</div>
                ))}
              </div>
            ) : (
              <Empty description="无数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </Card>
        </Col>
      </Row>

      <Card title="L1 缓存命中率(5min)" size="small" style={{ marginTop: 12 }}>
        <ResponsiveContainer width="100%" height={180}>
          <LineChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="t" hide />
            <YAxis domain={[0, 100]} unit="%" />
            <Tooltip />
            <Legend />
            <Line type="monotone" dataKey="hitRate" name="L1 命中率%" stroke="#52c41a" strokeWidth={2} dot={false} isAnimationActive={false} />
          </LineChart>
        </ResponsiveContainer>
      </Card>

      <Alert
        type="info"
        style={{ marginTop: 12 }}
        message="三层缓存说明"
        description="L1 Redis(精确哈希)+ L2 Milvus(语义向量 COSINE)+ L3 Neo4j(知识图谱);命中率由 LCO llm-cache-proxy /metrics 导出,5s 刷新;某层 DOWN 时独立降级不阻塞其它层(RWT)。"
      />
    </div>
  );
}
