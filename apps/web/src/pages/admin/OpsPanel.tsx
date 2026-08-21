// 运维操作面板(Phase3 · 2026-08-21)—— 实时资源监控 + 实时日志 + 可管理操作。
// 对应用户需求:"管理员登录后的统一运维监测平台还需要更多实时(负载/内存/硬盘/线程)、
//   可以管理操作的信息(包括实时的日志信息等)"。
// 数据源:admin-go /api/admin/ops/resources(Prometheus 聚合)+ /logs/{svc}(K8s pod log)+ /scale|restart/{svc}(K8s API)。
// 鉴权:走 @esp/api-client(esp_token httpOnly cookie,role=admin)。
// 图表:recharts 时序折线(60s 窗口,5s 刷新);日志:antd Tabs + 虚拟滚动 + 级别过滤;操作:Popconfirm 二次确认(BRR 铁律)。

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Card,
  Row,
  Col,
  Statistic,
  Progress,
  Tabs,
  Select,
  Button,
  InputNumber,
  Space,
  Popconfirm,
  Tag,
  Alert,
  Typography,
  message,
} from "antd";
import {
  ReloadOutlined,
  PlayCircleOutlined,
  AppstoreOutlined,
  DatabaseOutlined,
  MessageOutlined,
  BarChartOutlined,
  MonitorOutlined,
} from "@ant-design/icons";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  ReferenceLine,
} from "recharts";
import { api } from "../../api";
import { useAuth } from "../../auth";
import type { AdminOpsResources } from "@esp/api-client";
import CachePanel from "./CachePanel";
import MqPanel from "./MqPanel";
import GrafanaPanel from "./GrafanaPanel";
import RuntimePanel from "./RuntimePanel";

const { Text, Title } = Typography;

const REFRESH_MS = 5000;
const HISTORY_LEN = 60; // 60 个采样点 × 5s = 5min 窗口

// 资源指标元数据:key → 标签/单位/是否百分比/危险阈值。
interface MetricMeta {
  label: string;
  unit: string;
  percent?: boolean; // 百分比则用 Progress + 0-100
  dangerAt?: number; // 超过此值标红
}
const METRIC_META: Record<string, MetricMeta> = {
  node_cpu_usage: { label: "节点 CPU", unit: "%", percent: true, dangerAt: 80 },
  node_memory_usage: { label: "节点内存", unit: "%", percent: true, dangerAt: 85 },
  node_disk_usage: { label: "节点磁盘", unit: "%", percent: true, dangerAt: 90 },
  node_load1: { label: "节点负载(1m)", unit: "", dangerAt: 8 },
  container_cpu_cores: { label: "容器 CPU(核)", unit: "core" },
  container_mem_bytes: { label: "容器内存", unit: "B", dangerAt: 4 * 1024 ** 3 },
  pod_total: { label: "Pod 总数", unit: "" },
  pod_ready: { label: "Pod 就绪", unit: "" },
};

function fmtBytes(b: number): string {
  if (!b || isNaN(b)) return "N/A";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = b;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}

// 服务清单(与 admin-go catalog 对齐,供日志/操作下拉选择)。
const SERVICE_OPTIONS = [
  "gateway-go", "bff-java", "learning-java", "teacher-java", "writing-py",
  "emotion-svc", "content-go", "esp-admin", "conversation-go", "orchestrator-rs",
  "frontend-portal", "frontend-react", "frontend-h5",
];

export default function OpsPanel() {
  const { user } = useAuth();
  const [resources, setResources] = useState<AdminOpsResources | null>(null);
  const [history, setHistory] = useState<Record<string, number[]>>({});
  const [err, setErr] = useState("");
  const [checkedAt, setCheckedAt] = useState("");

  // 日志
  const [logSvc, setLogSvc] = useState("gateway-go");
  const [logTail, setLogTail] = useState(200);
  const [logs, setLogs] = useState("");
  const [logLoading, setLogLoading] = useState(false);
  const [logFilter, setLogFilter] = useState("");
  const [autoRefresh, setAutoRefresh] = useState(false);
  const logBoxRef = useRef<HTMLPreElement>(null);

  // 操作
  const [scaleSvc, setScaleSvc] = useState("gateway-go");
  const [scaleReplicas, setScaleReplicas] = useState(1);

  const loadResources = useCallback(async () => {
    try {
      const r = await api.adminOpsResources();
      setResources(r);
      setCheckedAt(new Date().toLocaleTimeString());
      setErr("");
      // 推入历史(每指标保留 HISTORY_LEN 个点)。
      setHistory((prev) => {
        const next: Record<string, number[]> = {};
        for (const [k, m] of Object.entries(r.metrics) as [string, { value?: string }][]) {
          const v = parseFloat(m.value ?? "");
          const arr = prev[k] ? [...prev[k], v] : [v];
          if (arr.length > HISTORY_LEN) arr.shift();
          next[k] = arr;
        }
        return next;
      });
    } catch (e) {
      setErr(`${e}`);
    }
  }, []);

  useEffect(() => {
    loadResources();
    const id = setInterval(loadResources, REFRESH_MS);
    return () => clearInterval(id);
  }, [loadResources]);

  const loadLogs = useCallback(async () => {
    setLogLoading(true);
    try {
      const txt = await api.adminLogs(logSvc, logTail);
      setLogs(typeof txt === "string" ? txt : JSON.stringify(txt));
      setErr("");
    } catch (e) {
      setLogs("");
      setErr(`${e}`);
    } finally {
      setLogLoading(false);
    }
  }, [logSvc, logTail]);

  useEffect(() => {
    loadLogs();
    if (!autoRefresh) return;
    const id = setInterval(loadLogs, REFRESH_MS);
    return () => clearInterval(id);
  }, [loadLogs, autoRefresh]);

  // 日志过滤(按级别关键字 ERROR/WARN/INFO 或自由文本)。
  const filteredLogs = logFilter
    ? logs.split("\n").filter((l) => l.toLowerCase().includes(logFilter.toLowerCase())).join("\n")
    : logs;

  const doScale = async () => {
    try {
      const r = await api.adminScale(scaleSvc, scaleReplicas);
      message.success(`${scaleSvc} → ${scaleReplicas} 副本(${r.status})`);
    } catch (e) {
      message.error(`扩缩容失败: ${e}`);
    }
  };

  const doRestart = async (svc: string) => {
    try {
      const r = await api.adminRestart(svc);
      message.success(`${svc} 重启已触发(${r.status})`);
    } catch (e) {
      message.error(`重启失败: ${e}`);
    }
  };

  // 资源指标卡片渲染。
  const renderMetric = (key: string) => {
    const meta = METRIC_META[key];
    const raw = resources?.metrics?.[key]?.value;
    const val = parseFloat(raw ?? "");
    if (!meta) return null;
    return (
      <Card size="small" key={key}>
        {meta.percent && !isNaN(val) ? (
          <Statistic
            title={meta.label}
            value={val}
            precision={1}
            suffix="%"
            valueStyle={{ color: val >= (meta.dangerAt ?? 100) ? "#ff4d4f" : undefined }}
          />
        ) : key === "container_mem_bytes" ? (
          <Statistic title={meta.label} value={fmtBytes(val)} />
        ) : key.startsWith("pod_") ? (
          <Statistic title={meta.label} value={isNaN(val) ? "N/A" : val} />
        ) : (
          <Statistic
            title={meta.label}
            value={isNaN(val) ? "N/A" : val}
            precision={key === "container_cpu_cores" || key === "node_load1" ? 2 : 0}
            valueStyle={{ color: meta.dangerAt && val >= meta.dangerAt ? "#ff4d4f" : undefined }}
          />
        )}
        {meta.percent && !isNaN(val) && (
          <Progress percent={val} size="small" status={val >= (meta.dangerAt ?? 100) ? "exception" : "normal"} style={{ marginTop: 8 }} />
        )}
      </Card>
    );
  };

  // CPU 时序图数据(转 recharts 格式)。
  const cpuHistory = (history.node_cpu_usage || []).map((v, i) => ({ t: i, cpu: v }));
  const memHistory = (history.node_memory_usage || []).map((v, i) => ({ t: i, mem: v }));

  if (user?.role !== "admin") {
    return <Alert type="warning" message="仅管理员可查看(role=admin)" />;
  }

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}>运维操作平台</Title>
        <Text type="secondary">实时资源监控 · 日志 · 可管理操作 · 轮询 {checkedAt}</Text>
      </div>

      {err && <Alert type="error" message={err} style={{ marginBottom: 16}} closable />}

      <Tabs
        defaultActiveKey="runtime"
        items={[
          {
            key: "runtime",
            label: <span><MonitorOutlined /> 服务运行</span>,
            children: <RuntimePanel />,
          },
          {
            key: "resources",
            label: <span><AppstoreOutlined /> 资源监控</span>,
            children: (
              <div>
                <Row gutter={[12, 12]}>
                  {["node_cpu_usage", "node_memory_usage", "node_disk_usage", "node_load1"].map(renderMetric)}
                </Row>
                <Row gutter={[12, 12]} style={{ marginTop: 12 }}>
                  {["container_cpu_cores", "container_mem_bytes", "pod_total", "pod_ready"].map(renderMetric)}
                </Row>

                <Row gutter={16} style={{ marginTop: 16 }}>
                  <Col span={12}>
                    <Card title="CPU 使用率(5min)" size="small">
                      <ResponsiveContainer width="100%" height={180}>
                        <LineChart data={cpuHistory}>
                          <CartesianGrid strokeDasharray="3 3" />
                          <XAxis dataKey="t" hide />
                          <YAxis domain={[0, 100]} unit="%" />
                          <Tooltip />
                          <ReferenceLine y={80} stroke="#ff4d4f" strokeDasharray="4 4" />
                          <Line type="monotone" dataKey="cpu" stroke="#1677ff" strokeWidth={2} dot={false} isAnimationActive={false} />
                        </LineChart>
                      </ResponsiveContainer>
                    </Card>
                  </Col>
                  <Col span={12}>
                    <Card title="内存使用率(5min)" size="small">
                      <ResponsiveContainer width="100%" height={180}>
                        <LineChart data={memHistory}>
                          <CartesianGrid strokeDasharray="3 3" />
                          <XAxis dataKey="t" hide />
                          <YAxis domain={[0, 100]} unit="%" />
                          <Tooltip />
                          <ReferenceLine y={85} stroke="#ff4d4f" strokeDasharray="4 4" />
                          <Line type="monotone" dataKey="mem" stroke="#52c41a" strokeWidth={2} dot={false} isAnimationActive={false} />
                        </LineChart>
                      </ResponsiveContainer>
                    </Card>
                  </Col>
                </Row>
              </div>
            ),
          },
          {
            key: "cache",
            label: <span><DatabaseOutlined /> 缓存监控</span>,
            children: <CachePanel />,
          },
          {
            key: "mq",
            label: <span><MessageOutlined /> MQ 消息</span>,
            children: <MqPanel />,
          },
          {
            key: "grafana",
            label: <span><BarChartOutlined /> Grafana</span>,
            children: <GrafanaPanel />,
          },
          {
            key: "logs",
            label: <span><ReloadOutlined /> 实时日志</span>,
            children: (
              <div>
                <Space style={{ marginBottom: 12 }} wrap>
                  <Select
                    value={logSvc}
                    onChange={setLogSvc}
                    style={{ width: 180 }}
                    options={SERVICE_OPTIONS.map((s) => ({ value: s, label: s }))}
                  />
                  <Text>行数:</Text>
                  <InputNumber min={50} max={2000} step={50} value={logTail} onChange={(v) => setLogTail(v ?? 200)} />
                  <Text>过滤:</Text>
                  <Select
                    value={logFilter}
                    onChange={setLogFilter}
                    style={{ width: 140 }}
                    allowClear
                    placeholder="ERROR/WARN/关键字"
                    options={[
                      { value: "", label: "全部" },
                      { value: "ERROR", label: "ERROR" },
                      { value: "WARN", label: "WARN" },
                      { value: "INFO", label: "INFO" },
                    ]}
                  />
                  <Button icon={<ReloadOutlined />} loading={logLoading} onClick={loadLogs}>刷新</Button>
                  <Popconfirm
                    title="自动刷新(每5s拉取)"
                    onConfirm={() => setAutoRefresh((v) => !v)}
                  >
                    <Button type={autoRefresh ? "primary" : "default"} icon={<PlayCircleOutlined />}>
                      {autoRefresh ? "停止自动" : "自动刷新"}
                    </Button>
                  </Popconfirm>
                </Space>
                <pre
                  ref={logBoxRef}
                  style={{
                    background: "var(--esp-color-bg-muted, #1e1e1e)",
                    color: "var(--esp-color-text, #d4d4d4)",
                    padding: 12,
                    borderRadius: 6,
                    fontSize: 12,
                    lineHeight: 1.5,
                    height: "55vh",
                    overflow: "auto",
                    margin: 0,
                    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
                    whiteSpace: "pre-wrap",
                    wordBreak: "break-all",
                  }}
                >
                  {filteredLogs || "(空 —— 选择服务并刷新)"}
                </pre>
              </div>
            ),
          },
          {
            key: "ops",
            label: <span><PlayCircleOutlined /> 可管理操作</span>,
            children: (
              <div>
                <Alert
                  type="warning"
                  showIcon
                  message="L2 不可逆操作(BRR 铁律)"
                  description="扩缩容/重启会改变集群运行态,执行前请确认。操作需管理员权限,已二次确认。"
                  style={{ marginBottom: 16 }}
                />
                <Row gutter={16}>
                  <Col span={12}>
                    <Card title="扩缩容(Deployment)" size="small">
                      <Space direction="vertical" style={{ width: "100%" }}>
                        <Select
                          value={scaleSvc}
                          onChange={setScaleSvc}
                          style={{ width: "100%" }}
                          options={SERVICE_OPTIONS.map((s) => ({ value: s, label: s }))}
                        />
                        <Space>
                          <Text>目标副本数:</Text>
                          <InputNumber min={0} max={20} value={scaleReplicas} onChange={(v) => setScaleReplicas(v ?? 1)} />
                        </Space>
                        <Popconfirm
                          title={`确认将 ${scaleSvc} 扩缩到 ${scaleReplicas} 副本?`}
                          okText="确认执行"
                          cancelText="取消"
                          okButtonProps={{ danger: true }}
                          onConfirm={doScale}
                        >
                          <Button type="primary" danger>执行扩缩容</Button>
                        </Popconfirm>
                      </Space>
                    </Card>
                  </Col>
                  <Col span={12}>
                    <Card title="滚动重启(Rollout Restart)" size="small">
                      <Space direction="vertical" style={{ width: "100%" }}>
                        <Select
                          defaultValue="gateway-go"
                          style={{ width: "100%" }}
                          options={SERVICE_OPTIONS.map((s) => ({ value: s, label: s }))}
                          onChange={(v) => setScaleSvc(v)}
                        />
                        <Alert
                          type="info"
                          message="重启 = 给 Deployment template 加 restartedAt 注解,触发滚动更新。"
                          style={{ marginBottom: 8 }}
                        />
                        <Popconfirm
                          title={`确认滚动重启 ${scaleSvc}?`}
                          description="将逐个重建 Pod,期间服务短暂降级。"
                          okText="确认重启"
                          cancelText="取消"
                          okButtonProps={{ danger: true }}
                          onConfirm={() => doRestart(scaleSvc)}
                        >
                          <Button danger>触发重启</Button>
                        </Popconfirm>
                      </Space>
                    </Card>
                  </Col>
                </Row>
                <Card title="操作说明" size="small" style={{ marginTop: 16 }}>
                  <ul style={{ margin: 0, paddingLeft: 20, lineHeight: 2 }}>
                    <li><Tag color="blue">资源监控</Tag> 只读,Prometheus 查询,5s 自动刷新</li>
                    <li><Tag color="green">实时日志</Tag> kubectl logs 等价,支持级别过滤 + 自动刷新</li>
                    <li><Tag color="orange">扩缩容/重启</Tag> L2 不可逆,需 RBAC(esp-admin-sa)+ Popconfirm 二次确认</li>
                    <li>若日志/操作报 503 "RBAC not configured":需 apply 33-admin-go.yaml 的 ServiceAccount/Role/RoleBinding</li>
                  </ul>
                </Card>
              </div>
            ),
          },
        ]}
      />
    </div>
  );
}
