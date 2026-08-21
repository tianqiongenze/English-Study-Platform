// API 在线测试面板 —— iframe 嵌入 bff Swagger UI(springdoc-openapi 聚合门户)。
// portal nginx /docs、/swagger-ui、/v3/api-docs 已反代到 bff-java;iframe 同源,
// esp_token httpOnly cookie 自动携带,无需额外鉴权。admin 角色可直接 "Try it out" 调试各 API。
import { Card, Typography, Alert, Space, Tag } from "antd";
import { ApiOutlined, CheckCircleOutlined } from "@ant-design/icons";

const { Title, Text, Link } = Typography;

export default function ApiTestPanel() {
  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 16 }}>
          <ApiOutlined style={{ fontSize: 28, color: "var(--esp-brand, #1677ff)" }} />
          <div>
            <Title level={4} style={{ margin: 0 }}>API 在线测试</Title>
            <Text type="secondary">在线调试全部 REST API(Swagger UI · Try it out)</Text>
          </div>
        </div>

        <Alert
          type="success"
          showIcon
          icon={<CheckCircleOutlined />}
          message="已接入 bff 聚合 Swagger UI"
          description={
            <Space size={4} wrap>
              <Text>bff-java 集成 springdoc-openapi,聚合 BFF + Learning + Teacher(Java)+ Writing + Emotion 清单。</Text>
              <Tag color="blue">/docs</Tag>
              <Tag color="blue">/swagger-ui</Tag>
              <Tag color="blue">/v3/api-docs</Tag>
            </Space>
          }
          style={{ marginBottom: 16 }}
        />

        <Text type="secondary" style={{ display: "block", marginBottom: 8 }}>
          说明:iframe 内 Swagger UI 与本页同源,esp_token httpOnly cookie 自动携带;
          点击任意接口 → "Try it out" → "Execute" 即可在线调试(需对应角色权限,admin 可访问全部)。
        </Text>
      </Card>

      <Card
        title="Swagger UI"
        styles={{ body: { padding: 0, height: "70vh" } }}
      >
        <iframe
          src="/docs"
          title="ESP Swagger UI"
          style={{ width: "100%", height: "70vh", border: "none", display: "block" }}
        />
      </Card>

      <Card type="inner" title="直链" style={{ marginTop: 16 }}>
        <Space direction="vertical">
          <Link href="/docs" target="_blank">/docs — Swagger UI 聚合门户(新窗口)</Link>
          <Link href="/v3/api-docs" target="_blank">/v3/api-docs — OpenAPI 3 JSON 原文</Link>
          <Link href="/swagger-ui/index.html" target="_blank">/swagger-ui/index.html — Swagger UI 标准路径</Link>
        </Space>
      </Card>
    </div>
  );
}
