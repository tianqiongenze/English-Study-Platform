// 服务运行时健康 Tab(Phase4 · 2026-08-21)—— HPA + 熔断 + 限流 + API 错误率。
// 数据源:admin-go /api/admin/runtime/hpa(K8s API 直查)/resilience(admin-go 直抓各 Go 服务 /metrics)/apis(Java actuator + probeAll)。
// 仿 OpsPanel 资源监控 Tab 的 Table + Card + Badge 范式。5s 刷新。
import { useCallback, useEffect, useState } from "react";
import { Card, Table, Tag, Badge, Row, Col, Statistic, Typography, Alert, Space } from "antd";
import { ThunderboltOutlined, ApiOutlined, RiseOutlined } from "@ant-design/icons";
import { api } from "../../api";
import type { AdminHPAResponse, AdminResilienceResponse, AdminApisResponse, AdminHPAStatus, AdminResilienceStatus } from "@esp/api-client";

const { Text, Title } = Typography;
const REFRESH_MS = 5000;

export default function RuntimePanel() {
  const [hpa, setHpa] = useState<AdminHPAResponse | null>(null);
  const [resil, setResil] = useState<AdminResilienceResponse | null>(null);
  const [apis, setApis] = useState<AdminApisResponse | null>(null);
  const [err, setErr] = useState("");

  const load = useCallback(async () => {
    try {
      const [h, r, a] = await Promise.all([api.adminRuntimeHPA(), api.adminRuntimeResilience(), api.adminRuntimeApis()]);
      setHpa(h); setResil(r); setApis(a); setErr("");
    } catch (e) { setErr(`${e}`); }
  }, []);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const scalingCount = hpa?.items?.filter(i => i.scaling).length ?? 0;
  const cbOpen = resil?.items?.filter(i => i.cb_state === "open" || i.cb_state === "half-open").length ?? 0;
  const rlRejected = resil?.items?.reduce((a, i) => a + (parseInt(i.rl_rejected) || 0), 0) ?? 0;
  const upCount = apis?.items?.filter(i => i.status === "UP" || i.status === "ok").length ?? 0;
  const downCount = apis?.items?.filter(i => i.status === "DOWN" || i.status === "down").length ?? 0;

  const hpaColumns = [
    { title: "服务", dataIndex: "name", key: "name", render: (n: string) => <Text code>{n}</Text> },
    { title: "Min/Max", key: "mm", render: (_: unknown, r: AdminHPAStatus) => `${r.minReplicas}/${r.maxReplicas}` },
    { title: "当前", dataIndex: "currentReplicas", key: "cur" },
    { title: "期望", dataIndex: "desiredReplicas", key: "des", render: (v: number, r: AdminHPAStatus) => v !== r.currentReplicas ? <Tag color="warning">{v}</Tag> : v },
    { title: "CPU 阈值", dataIndex: "targetCpu", key: "tgt" },
    { title: "CPU 当前", dataIndex: "currentCpu", key: "curCpu", render: (v: string) => v || "-" },
    { title: "状态", key: "st", render: (_: unknown, r: AdminHPAStatus) => r.scaling ? <Tag color="processing" icon={<RiseOutlined />}>扩缩中</Tag> : <Tag color="success">稳定</Tag> },
  ];

  const cbColor = (s: string) => s === "open" ? "error" : s === "half-open" ? "warning" : s === "closed" ? "success" : "default";

  return (
    <div>
      {err && <Alert type="error" message={err} style={{ marginBottom: 12 }} closable />}

      <Row gutter={12} style={{ marginBottom: 12 }}>
        <Col flex="auto"><Card size="small"><Statistic title="HPA 扩缩中" value={scalingCount} valueStyle={{ color: scalingCount > 0 ? "#faad14" : undefined }} prefix={<RiseOutlined />} /></Card></Col>
        <Col flex="auto"><Card size="small"><Statistic title="熔断打开" value={cbOpen} valueStyle={{ color: cbOpen > 0 ? "#ff4d4f" : undefined }} prefix={<ThunderboltOutlined />} /></Card></Col>
        <Col flex="auto"><Card size="small"><Statistic title="限流拒绝(累计)" value={rlRejected} valueStyle={{ color: rlRejected > 0 ? "#ff4d4f" : undefined }} /></Card></Col>
        <Col flex="auto"><Card size="small"><Statistic title="服务 UP/DOWN" value={`${upCount}/${downCount}`} prefix={<ApiOutlined />} /></Card></Col>
      </Row>

      <Card title={`① HPA 状态(${hpa?.items?.length ?? 0} 个,K8s API 直查)`} size="small" style={{ marginBottom: 12 }}>
        <Table dataSource={hpa?.items || []} columns={hpaColumns} rowKey="name" size="small" pagination={false} />
      </Card>

      <Card title="② 熔断 + 限流(各 Go 服务 /metrics 直抓)" size="small" style={{ marginBottom: 12 }}>
        <Row gutter={[8, 8]}>
          {resil?.items?.map((r: AdminResilienceStatus) => (
            <Col key={r.service} flex="auto">
              <Card size="small" style={{ minWidth: 180 }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 4 }}>
                  <Text strong>{r.service}</Text>
                  <Badge status={cbColor(r.cb_state)} text={r.cb_state} />
                </div>
                <div style={{ fontSize: 12 }}>
                  <div>熔断触发: <Text type={parseInt(r.cb_trips) > 0 ? "danger" : "secondary"}>{r.cb_trips || "0"}</Text></div>
                  <div>限流放行/拒绝: <Text type="success">{r.rl_allowed || "0"}</Text> / <Text type={parseInt(r.rl_rejected) > 0 ? "danger" : "secondary"}>{r.rl_rejected || "0"}</Text></div>
                </div>
              </Card>
            </Col>
          ))}
        </Row>
      </Card>

      <Card title="③ API 运行状态(服务级健康 + Java 请求计数)" size="small">
        <Space wrap>
          {apis?.items?.map(a => (
            <Tag key={a.service} color={a.status === "UP" || a.status === "ok" ? "success" : "error"} style={{ marginBottom: 4 }}>
              {a.service}: {a.status}{a.req_count ? ` (${a.req_count} req)` : ""}
            </Tag>
          ))}
        </Space>
      </Card>
    </div>
  );
}
