import { useEffect, useMemo, useState } from "react";
import { useAuth } from "../../auth";
import type { AdminProbe, AdminProbeResponse, AdminCatalog } from "@esp/api-client";
import {
  Card,
  Row,
  Col,
  Statistic,
  Tag,
  Badge,
  Table,
  Input,
  Button,
  Alert,
  Typography,
  Space,
  Empty,
} from "antd";
import { ReloadOutlined, CheckCircleOutlined, CloseCircleOutlined } from "@ant-design/icons";

const { Title, Text, Link: AntLink } = Typography;

// 统一管理员实时监测门户 —— 聚合:全服务健康探活 + 基础设施探活 + Prometheus 指标/告警 + API 目录。
// 数据源:esp-admin(经 portal/web nginx /api/admin/ 代理);鉴权靠 esp_token httpOnly cookie(role=admin)。
// Phase3e 美化(2026-08-21):inline style 全换 antd 组件(Card/Statistic/Tag/Badge/Table/Input.Search),
// 对齐互联网大厂级视觉(与 AdminLayout/OpsPanel 同构)。

type Probe = AdminProbe;
type ProbeResp = AdminProbeResponse;
type Catalog = AdminCatalog;

const REFRESH_MS = 5000;

/** 语言标签(Tag color) */
const LANG_TAG: Record<string, string> = {
  Go: "cyan",
  Java: "gold",
  Python: "blue",
  Rust: "default",
};

/** 封装 fetch-json,统一错误处理 */
async function getJSON<T>(url: string): Promise<T> {
  const r = await fetch(url, { credentials: "same-origin" });
  if (!r.ok) throw new Error(`HTTP ${r.status}`);
  return r.json();
}

function ProbeCard({ p }: { p: Probe }) {
  const up = p.status === "UP";
  return (
    <Card
      size="small"
      style={{ minWidth: 220, borderColor: up ? undefined : "#ff4d4f" }}
      styles={{ body: { padding: 12 } }}
    >
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 4 }}>
        <Space>
          <Badge status={up ? "success" : "error"} />
          <Text strong>{p.name}</Text>
          {p.lang && <Tag color={LANG_TAG[p.lang] || "default"}>{p.lang}</Tag>}
        </Space>
        <Tag color={up ? "success" : "error"}>{p.status}</Tag>
      </div>
      <Text type="secondary" style={{ fontSize: 12 }}>{p.svc}:{p.port}{p.path}</Text>
      <div style={{ fontSize: 12, marginTop: 4 }}>
        {p.code ? `HTTP ${p.code} · ` : ""}{p.latency_ms}ms
      </div>
      {!up && p.msg && <Text type="danger" style={{ fontSize: 11, wordBreak: "break-all", display: "block", marginTop: 4 }}>{p.msg}</Text>}
    </Card>
  );
}

export default function AdminDashboard() {
  const user = useAuth((s) => s.user);
  const [svc, setSvc] = useState<ProbeResp | null>(null);
  const [infra, setInfra] = useState<ProbeResp | null>(null);
  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const [alerts, setAlerts] = useState<{ status: string; data: { alerts: any[] } } | null>(null);
  const [promAlive, setPromAlive] = useState<boolean | null>(null);
  const [promQL, setPromQL] = useState("sum(up{job=\"kubernetes-pods\"})");
  const [promResult, setPromResult] = useState<string>("");
  const [err, setErr] = useState("");
  const [checkedAt, setCheckedAt] = useState("");

  // 核心:服务 + 基础设施每 5s 轮询
  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const [s, i, c] = await Promise.all([
          getJSON<ProbeResp>("/api/admin/services"),
          getJSON<ProbeResp>("/api/admin/infra"),
          getJSON<Catalog>("/api/admin/catalog"),
        ]);
        if (!alive) return;
        setSvc(s); setInfra(i); setCatalog(c); setCheckedAt(new Date().toLocaleTimeString()); setErr("");
      } catch (e) { if (alive) setErr(`${e}`); }
    };
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => { alive = false; clearInterval(id); };
  }, []);

  // 告警 + Prometheus 可达性
  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const a = await getJSON<{ status: string; data: { alerts: any[] } }>("/api/admin/metrics/alerts");
        if (!alive) return;
        setAlerts(a); setPromAlive(true);
      } catch { if (alive) setPromAlive(false); }
    })();
    return () => { alive = false; };
  }, []);

  const firing = useMemo(() => (alerts?.data?.alerts || []).filter((x: any) => x.state === "firing"), [alerts]);

  async function runQuery() {
    setPromResult("查询中…");
    try {
      const r = await getJSON<any>(`/api/admin/metrics/query?query=${encodeURIComponent(promQL)}`);
      const result = r?.data?.result || [];
      setPromResult(result.length ? JSON.stringify(result, null, 1).slice(0, 600) : "(空结果)");
    } catch (e) { setPromResult(`查询失败: ${e}`); }
  }

  const catalogCols = [
    { title: "服务", dataIndex: "name", key: "name" },
    { title: "语言", dataIndex: "lang", key: "lang", render: (l: string) => <Tag color={LANG_TAG[l] || "default"}>{l}</Tag> },
    { title: "ClusterIP", key: "addr", render: (_: any, r: any) => `${r.svc}:${r.port}` },
    { title: "健康端点", dataIndex: "path", key: "path", render: (p: string) => <Text code>{p}</Text> },
  ];

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16, flexWrap: "wrap", gap: 8 }}>
        <Title level={4} style={{ margin: 0 }}>统一运维监测平台</Title>
        <Space size={12}>
          <AntLink href="/web/admin/ops">运维操作面板 →</AntLink>
          {checkedAt && <Text type="secondary" style={{ fontSize: 12 }}>轮询 {checkedAt} · </Text>}
          <Text type="secondary" style={{ fontSize: 12 }}>Prometheus:</Text>
          {promAlive === null
            ? <Text type="secondary" style={{ fontSize: 12 }}>检测中...</Text>
            : promAlive
              ? <Tag icon={<CheckCircleOutlined />} color="success">在线</Tag>
              : <Tag icon={<CloseCircleOutlined />} color="error">离线</Tag>}
          {firing.length > 0 && <Tag color="error">告警 {firing.length} 条</Tag>}
        </Space>
      </div>

      {user?.role !== "admin" && <Alert type="warning" message="仅管理员可查看(role=admin)" style={{ marginBottom: 16 }} />}
      {err && <Alert type="error" message={err} style={{ marginBottom: 16 }} closable />}

      {/* 汇总条:服务/基础设施 UP/DOWN/Total */}
      <Row gutter={12} style={{ marginBottom: 16 }}>
        {[["服务", svc], ["基础设施", infra]].map(([k, v]: any[]) => (
          <Col key={k} flex="auto">
            <Card size="small">
              <Row gutter={16}>
                <Col span={6}><Statistic title={k} value={v?.summary?.up ?? "-"} prefix={<CheckCircleOutlined style={{ color: "#52c41a" }} />} valueStyle={{ color: "#52c41a" }} /></Col>
                <Col span={6}><Statistic title="DOWN" value={v?.summary?.down ?? "-"} valueStyle={{ color: "#ff4d4f" }} /></Col>
                <Col span={6}><Statistic title="总计" value={v?.summary?.total ?? "-"} /></Col>
                <Col span={6}>
                  <Statistic
                    title="可用率"
                    value={v?.summary?.total ? Math.round((v.summary.up / v.summary.total) * 100) : 0}
                    suffix="%"
                  />
                </Col>
              </Row>
            </Card>
          </Col>
        ))}
      </Row>

      <Card title="① 全服务健康状态(实时探活)" size="small" style={{ marginBottom: 16 }}>
        {svc?.items?.length ? (
          <Row gutter={[8, 8]}>
            {svc.items.map((p) => <Col key={p.name}><ProbeCard p={p} /></Col>)}
          </Row>
        ) : svc ? <Empty description="无服务" /> : <Text type="secondary">加载中…</Text>}
      </Card>

      <Card title="② 基础设施实时监测" size="small" style={{ marginBottom: 16 }}>
        {infra?.items?.length ? (
          <Row gutter={[8, 8]}>
            {infra.items.map((p) => <Col key={p.name}><ProbeCard p={p} /></Col>)}
          </Row>
        ) : infra ? <Empty description="无基础设施" /> : <Text type="secondary">加载中…</Text>}
      </Card>

      <Card
        title={`③ Prometheus 指标查询${alerts ? ` · 活动告警 ${firing.length} 条` : ""}`}
        size="small"
        style={{ marginBottom: 16 }}
      >
        <Space.Compact style={{ width: "100%", marginBottom: 8 }}>
          <Input
            value={promQL}
            onChange={(e) => setPromQL(e.target.value)}
            placeholder="PromQL 查询(如 sum(up))"
            onPressEnter={runQuery}
          />
          <Button type="primary" icon={<ReloadOutlined />} onClick={runQuery}>查询</Button>
        </Space.Compact>
        {firing.length > 0 && (
          <div style={{ marginBottom: 8 }}>
            {firing.map((f: any, i: number) => (
              <Tag key={i} color="error" style={{ marginBottom: 4 }}>
                {f.labels?.alertname || f.labels?.alert} @ {f.labels?.job || f.labels?.instance} · {f.labels?.severity}
              </Tag>
            ))}
          </div>
        )}
        {promResult && (
          <pre style={{ background: "var(--esp-color-bg-muted, #f3f4f6)", padding: 8, borderRadius: 6, fontSize: 12, overflow: "auto", maxHeight: 200, margin: 0 }}>
            {promResult}
          </pre>
        )}
      </Card>

      <Card title="④ 统一 API 目录(全部服务)" size="small">
        <Table
          dataSource={catalog?.services || []}
          columns={catalogCols}
          rowKey="name"
          pagination={false}
          size="small"
        />
      </Card>
    </div>
  );
}
