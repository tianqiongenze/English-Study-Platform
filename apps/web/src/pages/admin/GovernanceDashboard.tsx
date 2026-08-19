// 服务治理仪表盘: Nacos注册状态 + 限流规则 + 基础设施监控链接
import { useEffect, useState } from "react";

interface NacosService {
  name: string;
  group: string;
  instances: number;
  healthy: number;
}
interface SentinelRule {
  resource: string;
  grade: number;
  count: number;
  strategy: number;
  controlBehavior?: number;
}

export default function GovernanceDashboard() {
  const [nacos, setNacos] = useState<NacosService[]>([]);
  const [rules, setRules] = useState<Record<string, SentinelRule[]>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      fetch("/api/admin/governance/nacos", { credentials: "same-origin" }).then(r => r.json()),
      fetch("/api/admin/governance/sentinel", { credentials: "same-origin" }).then(r => r.json()),
    ]).then(([nacosData, sentinelData]) => {
      setNacos(nacosData.services || []);
      setRules(sentinelData.rules || {});
      setLoading(false);
    }).catch(() => setLoading(false));
  }, []);

  if (loading) {
    return <div style={{ padding: 24, color: "var(--esp-color-text-secondary, #6b7280)" }}>加载中...</div>;
  }

  const totalInstances = nacos.reduce((s, svc) => s + svc.instances, 0);
  const totalHealthy = nacos.reduce((s, svc) => s + svc.healthy, 0);
  const totalRules = Object.values(rules).reduce((s, rls) => s + rls.length, 0);

  return (
    <div style={{ padding: 24 }}>
      <h2>服务治理</h2>

      {/* 概览卡片 */}
      <div style={{ display: "flex", gap: 16, marginBottom: 24 }}>
        {[
          { label: "注册服务", value: nacos.length, color: "var(--esp-primary, #3b82f6)" },
          { label: "健康实例", value: `${totalHealthy}/${totalInstances}`, color: totalHealthy === totalInstances ? "var(--esp-success, #16a34a)" : "var(--esp-danger, #dc2626)" },
          { label: "限流规则", value: totalRules, color: "var(--esp-warning, #f97316)" },
        ].map(card => (
          <div key={card.label} style={{ flex: 1, border: "1px solid var(--esp-color-border, #e5e7eb)", borderRadius: 8, padding: 16, textAlign: "center" }}>
            <div style={{ fontSize: 24, fontWeight: 700, color: card.color }}>{card.value}</div>
            <div style={{ fontSize: 13, color: "var(--esp-color-text-secondary, #6b7280)", marginTop: 4 }}>{card.label}</div>
          </div>
        ))}
      </div>

      {/* Nacos 服务注册状态 */}
      <section>
        <h3>Nacos 服务注册 ({nacos.length} 个服务)</h3>
        {nacos.length === 0 ? (
          <div style={{ color: "var(--esp-color-text-muted, #9ca3af)", fontSize: 14 }}>无已注册服务或 Nacos 不可达</div>
        ) : (
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(250px, 1fr))", gap: 12 }}>
            {nacos.map(svc => (
              <div key={svc.name} style={{ border: "1px solid var(--esp-color-border, #e5e7eb)", borderRadius: 8, padding: 12 }}>
                <b>{svc.name}</b> <span style={{ fontSize: 12, color: "var(--esp-color-text-secondary, #6b7280)" }}>({svc.group})</span>
                <div style={{ fontSize: 13, marginTop: 4 }}>
                  实例: {svc.instances} · 健康:{" "}
                  <span style={{ color: svc.healthy === svc.instances ? "var(--esp-success, #16a34a)" : "var(--esp-danger, #dc2626)" }}>{svc.healthy}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Sentinel 限流规则 */}
      <section style={{ marginTop: 24 }}>
        <h3>限流熔断规则</h3>
        {Object.keys(rules).length === 0 ? (
          <div style={{ color: "var(--esp-color-text-muted, #9ca3af)", fontSize: 14 }}>无已配置的限流规则</div>
        ) : (
          Object.entries(rules).map(([svc, rls]) => (
            <div key={svc} style={{ marginBottom: 16 }}>
              <h4>{svc}</h4>
              <table style={{ width: "100%", borderCollapse: "collapse" }}>
                <thead>
                  <tr style={{ borderBottom: "2px solid var(--esp-color-border, #e5e7eb)" }}>
                    <th style={{ textAlign: "left", padding: 6 }}>资源</th>
                    <th style={{ textAlign: "left", padding: 6 }}>类型</th>
                    <th style={{ textAlign: "left", padding: 6 }}>阈值</th>
                    <th style={{ textAlign: "left", padding: 6 }}>策略</th>
                  </tr>
                </thead>
                <tbody>
                  {rls.map((rule, i) => (
                    <tr key={i} style={{ borderBottom: "1px solid var(--esp-color-border, #f3f4f6)" }}>
                      <td style={{ padding: 6 }}>{rule.resource}</td>
                      <td style={{ padding: 6 }}>{rule.grade === 1 ? "QPS限流" : "并发限流"}</td>
                      <td style={{ padding: 6 }}>{rule.count}</td>
                      <td style={{ padding: 6 }}>
                        {rule.strategy === 0 ? "直接" : rule.strategy === 1 ? "关联" : "链路"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))
        )}
      </section>

      {/* Grafana + Sentinel 控制台链接(经 BFF 代理,2026-08-19 修复硬编码 VM IP) */}
      <section style={{ marginTop: 24 }}>
        <h3>基础设施监控</h3>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          <a
            href="/api/admin/middleware/grafana/d/esp-overview"
            target="_blank"
            rel="noopener noreferrer"
            style={{
              display: "inline-block", padding: "8px 16px", background: "var(--esp-warning, #f97316)",
              color: "var(--esp-color-text-on, #fff)", borderRadius: 6, textDecoration: "none",
            }}
          >
            Grafana 业务大盘
          </a>
          <a
            href="/api/admin/middleware/sentinel/"
            target="_blank"
            rel="noopener noreferrer"
            style={{
              display: "inline-block", padding: "8px 16px", background: "var(--esp-primary, #3b82f6)",
              color: "var(--esp-color-text-on, #fff)", borderRadius: 6, textDecoration: "none",
            }}
          >
            Sentinel 控制台
          </a>
          <a
            href="/web/admin/middleware"
            style={{
              display: "inline-block", padding: "8px 16px", background: "var(--esp-color-text-muted, #6b7280)",
              color: "var(--esp-color-text-on, #fff)", borderRadius: 6, textDecoration: "none",
            }}
          >
            中间件统一访问面板 →
          </a>
        </div>
      </section>
    </div>
  );
}
