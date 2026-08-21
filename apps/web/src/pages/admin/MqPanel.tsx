// MQ 消息 Tab(Phase4 · 2026-08-21)—— Pulsar topic 列表 + 积压/订阅 + peek 消息。
// 数据源:admin-go /api/admin/pulsar/topics + /pulsar/messages。只读 peek(不 ack 不重发,BRR 铁律)。
// 仿 OpsPanel 日志 Tab 的 Table + 抽屉范式。
import { useCallback, useEffect, useState } from "react";
import { Card, Table, Tag, Button, Drawer, Typography, Space, Alert, Empty, Select } from "antd";
import { ReloadOutlined, EyeOutlined } from "@ant-design/icons";
import { api } from "../../api";
import type { AdminPulsarTopicsResponse, AdminPulsarTopic, AdminPulsarMessagesResponse } from "@esp/api-client";

const { Text, Paragraph } = Typography;
const REFRESH_MS = 5000;

export default function MqPanel() {
  const [topics, setTopics] = useState<AdminPulsarTopicsResponse | null>(null);
  const [ns, setNs] = useState("esp/business");
  const [err, setErr] = useState("");
  const [peekTopic, setPeekTopic] = useState<string | null>(null);
  const [peekSub, setPeekSub] = useState<string>("");
  const [peekLimit, setPeekLimit] = useState(10);
  const [messages, setMessages] = useState<AdminPulsarMessagesResponse | null>(null);
  const [peekLoading, setPeekLoading] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await api.adminPulsarTopics(ns);
      setTopics(r);
      setErr("");
    } catch (e) {
      setErr(`${e}`);
    }
  }, [ns]);

  useEffect(() => {
    load();
    if (!autoRefresh) return;
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load, autoRefresh]);

  const doPeek = useCallback(async () => {
    if (!peekTopic || !peekSub) return;
    setPeekLoading(true);
    try {
      const r = await api.adminPulsarMessages(peekTopic, peekSub, peekLimit);
      setMessages(r);
      setErr("");
    } catch (e) {
      setErr(`${e}`);
      setMessages(null);
    } finally {
      setPeekLoading(false);
    }
  }, [peekTopic, peekSub, peekLimit]);

  const columns = [
    {
      title: "Topic", dataIndex: "topic", key: "topic",
      render: (t: string) => <Text code>{t}</Text>,
    },
    {
      title: "状态", dataIndex: "status", key: "status",
      render: (s: string) => s === "ok" ? <Tag color="success">UP</Tag> : <Tag color="error">DOWN</Tag>,
    },
    {
      title: "订阅数", key: "subs",
      render: (_: unknown, r: AdminPulsarTopic) => r.subscriptions?.length ?? 0,
    },
    {
      title: "积压", key: "backlog",
      render: (_: unknown, r: AdminPulsarTopic) => {
        const total = r.subscriptions?.reduce((a, s) => a + s.msg_backlog, 0) ?? 0;
        return <Tag color={total > 0 ? "warning" : "success"}>{total}</Tag>;
      },
    },
    {
      title: "生产者", dataIndex: "producers", key: "producers",
    },
    {
      title: "入速率", dataIndex: "msg_rate_in", key: "in",
      render: (v: string) => v ? `${parseFloat(v).toFixed(2)} msg/s` : "-",
    },
    {
      title: "操作", key: "action",
      render: (_: unknown, r: AdminPulsarTopic) => (
        <Button size="small" icon={<EyeOutlined />} onClick={() => {
          setPeekTopic(r.topic);
          setPeekSub(r.subscriptions?.[0]?.name ?? "");
          setMessages(null);
        }}>查看消息</Button>
      ),
    },
  ];

  const expandedRowRender = (r: AdminPulsarTopic) => {
    if (!r.subscriptions?.length) return <Empty description="无订阅" image={Empty.PRESENTED_IMAGE_SIMPLE} />;
    return (
      <Table
        size="small"
        pagination={false}
        rowKey="name"
        dataSource={r.subscriptions}
        columns={[
          { title: "订阅", dataIndex: "name", key: "name", render: (n: string) => <Text code>{n}</Text> },
          { title: "类型", dataIndex: "type", key: "type" },
          { title: "积压", dataIndex: "msg_backlog", key: "backlog", render: (b: number) => <Tag color={b > 0 ? "warning" : "success"}>{b}</Tag> },
          { title: "出速率", dataIndex: "msg_rate_out", key: "out", render: (v: string) => v ? `${parseFloat(v).toFixed(2)} msg/s` : "-" },
        ]}
      />
    );
  };

  return (
    <div>
      <Space style={{ marginBottom: 12 }} wrap>
        <Text>Namespace:</Text>
        <Select value={ns} onChange={setNs} style={{ width: 160 }}
          options={[{ value: "esp/business", label: "esp/business" }, { value: "esp/cache", label: "esp/cache" }]} />
        <Button icon={<ReloadOutlined />} onClick={load}>刷新</Button>
        <Button type={autoRefresh ? "primary" : "default"} onClick={() => setAutoRefresh(v => !v)}>
          {autoRefresh ? "停止自动" : "自动刷新(5s)"}
        </Button>
      </Space>

      {err && <Alert type="error" message={err} style={{ marginBottom: 12 }} closable />}

      <Card size="small">
        <Table
          dataSource={topics?.topics || []}
          columns={columns}
          rowKey="fqn"
          size="small"
          pagination={false}
          expandable={{ expandedRowRender, rowExpandable: (r) => (r.subscriptions?.length ?? 0) > 0 }}
          locale={{ emptyText: topics?.note || "无 topic" }}
        />
      </Card>

      <Drawer
        title={`peek 消息 · ${peekTopic}`}
        open={!!peekTopic}
        onClose={() => setPeekTopic(null)}
        width={720}
        extra={<Space>
          <Select size="small" value={peekSub} onChange={setPeekSub} style={{ width: 200 }}
            options={(topics?.topics?.find(t => t.topic === peekTopic)?.subscriptions ?? []).map(s => ({ value: s.name, label: s.name }))} />
          <Select size="small" value={peekLimit} onChange={setPeekLimit} style={{ width: 80 }}
            options={[10, 20, 50].map(n => ({ value: n, label: `${n}条` }))} />
          <Button size="small" type="primary" icon={<ReloadOutlined />} loading={peekLoading} onClick={doPeek}>拉取</Button>
        </Space>}
      >
        <Alert type="warning" message="只读 peek(不 ack 不 commit cursor,BRR 铁律)" style={{ marginBottom: 12 }} />
        {messages?.messages?.length ? (
          messages.messages.map((m, i) => (
            <Card key={i} size="small" style={{ marginBottom: 8 }}>
              <div style={{ fontSize: 12, color: "#888" }}>
                <Text code>{m.message_id}</Text> · {m.publish_time} · producer: {m.producer || "-"}
              </div>
              <pre style={{ background: "#f5f5f5", padding: 8, borderRadius: 4, fontSize: 12, overflow: "auto", maxHeight: 200, margin: "4px 0 0" }}>
                {m.payload_raw || JSON.stringify(m.payload, null, 2)}
              </pre>
            </Card>
          ))
        ) : (
          <Empty description={messages?.note || "点拉取查看消息"} />
        )}
      </Drawer>
    </div>
  );
}
