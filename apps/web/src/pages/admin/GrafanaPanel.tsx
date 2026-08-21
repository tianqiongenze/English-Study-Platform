// Grafana 仪表盘 Tab(Phase4 · 2026-08-21)—— iframe 嵌 BFF 代理的 Grafana dashboard。
// 零后端代码:admin-go middleware.go 的 ReverseProxy 白名单已含 grafana,同源带 esp_token cookie。
// uid=esp-business-phase2(探查已核实);kiosk=tv 无 chrome 全屏;from/to 时间窗口。
import { Card, Typography, Space, Button } from "antd";
import { LinkOutlined } from "@ant-design/icons";

const { Text, Link } = Typography;

export default function GrafanaPanel() {
  const src = "/api/admin/middleware/grafana/d/esp-business-phase2?kiosk=tv&from=now-1h&to=now&theme=light";
  return (
    <div>
      <Card
        size="small"
        style={{ marginBottom: 8 }}
        title="Grafana · ESP Business Dashboard"
        extra={
          <Space>
            <Text type="secondary" style={{ fontSize: 12 }}>uid: esp-business-phase2</Text>
            <Link href={src} target="_blank"><LinkOutlined /> 新窗口</Link>
          </Space>
        }
      >
        <Text type="secondary" style={{ fontSize: 12 }}>
          经 admin-go middleware 代理(同源 esp_token cookie 自动携带);时间窗口 now-1h;可在新窗口用 Grafana 原生交互(下钻/时间选择)。
        </Text>
      </Card>
      <Card styles={{ body: { padding: 0, height: "68vh" } }}>
        <iframe
          src={src}
          title="ESP Grafana Dashboard"
          style={{ width: "100%", height: "68vh", border: "none", display: "block" }}
          sandbox="allow-same-origin allow-scripts allow-popups allow-forms"
        />
      </Card>
    </div>
  );
}
