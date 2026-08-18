import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useAuth } from "../../auth";
import { api } from "../../api";
import type { AdminProbe, AdminProbeResponse, AdminCatalog } from "@esp/api-client";

// 统一管理员实时监测门户 -- 聚合:全服务健康探活 + 基础设施探活 + Prometheus 指标/告警 + API 目录。
// 数据源:esp-admin(经 portal/web nginx /api/admin/ 代理);鉴权靠 esp_token httpOnly cookie(role=admin)。
// P2-22:直 fetch 改 api-client;硬编码颜色改 CSS 变量。

type Probe = AdminProbe;
type ProbeResp = AdminProbeResponse;
type Catalog = AdminCatalog;

const REFRESH_MS = 5000;

/** 语言标签背景色(CSS 变量优先,hex 作 fallback) */
const LANG_COLOR: Record<string, string> = {
  Go: "var(--esp-color-info, #00ADD8)",
  Java: "var(--esp-warning, #ED8B00)",
  Python: "var(--esp-color-info, #3776AB)",
  Rust: "var(--esp-color-text-secondary, #000000)",
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
    <div style={{ border: `1px solid ${up ? "var(--esp-success, #22c55e)" : "var(--esp-danger, #ef4444)"}`, borderRadius: 8, padding: "8px 12px", background: up ? "var(--esp-success-bg, #f0fdf4)" : "var(--esp-danger-bg, #fef2f2)", minWidth: 220 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <span style={{ width: 10, height: 10, borderRadius: 5, background: up ? "var(--esp-success, #16a34a)" : "var(--esp-danger, #dc2626)", display: "inline-block" }} />
        <b>{p.name}</b>
        {p.lang && <span style={{ fontSize: 11, color: "var(--esp-color-text-on, #fff)", background: LANG_COLOR[p.lang] || "var(--esp-text-muted, #6b7280)", padding: "1px 6px", borderRadius: 8 }}>{p.lang}</span>}
      </div>
      <div style={{ fontSize: 12, color: "var(--esp-color-text-secondary, #555)", marginTop: 2 }}>{p.svc}:{p.port}{p.path}</div>
      <div style={{ fontSize: 12, marginTop: 4 }}>
        <span style={{ color: up ? "var(--esp-success, #16a34a)" : "var(--esp-danger, #dc2626)", fontWeight: 600 }}>{p.status}</span>
        {p.code ? ` (${p.code})` : ""} · {p.latency_ms}ms
      </div>
      {!up && p.msg && <div style={{ fontSize: 11, color: "var(--esp-danger, #b91c1c)", marginTop: 2, wordBreak: "break-all" }}>{p.msg}</div>}
    </div>
  );
}

function Section({ title, right, children }: { title: string; right?: ReactNode; children: ReactNode }) {
  return (
    <section style={{ margin: "16px 0" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 8 }}>
        <h3 style={{ margin: 0 }}>{title}</h3>
        {right}
      </div>
      {children}
    </section>
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

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
        <h2>统一运维监测平台</h2>
        <div style={{ fontSize: 12, color: "var(--esp-color-text-secondary, #777)" }}>
          {checkedAt && `轮询: ${checkedAt} · `}
          Prometheus: {promAlive === null ? "检测中..." : promAlive ? <b style={{ color: "var(--esp-success, #16a34a)" }}>在线</b> : <b style={{ color: "var(--esp-danger, #dc2626)" }}>离线</b>}
          {firing.length > 0 && <span style={{ color: "var(--esp-danger, #dc2626)", fontWeight: 600 }}> · 告警 {firing.length} 条</span>}
        </div>
      </div>
      {user?.role !== "admin" && <p className="err">仅管理员可查看(role=admin)</p>}
      {err && <p className="err">{err}</p>}

      {/* 汇总条 */}
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap", margin: "8px 0" }}>
        {[["服务", svc], ["基础设施", infra]].map(([k, v]: any[]) => (
          <div key={k} style={{ border: "1px solid var(--esp-color-border, #e5e7eb)", borderRadius: 8, padding: "6px 14px", background: "var(--esp-color-bg-subtle, #f9fafb)" }}>
            <b>{k}</b>: UP <span style={{ color: "var(--esp-success, #16a34a)" }}>{v?.summary?.up ?? "-"}</span> / DOWN{" "}
            <span style={{ color: "var(--esp-danger, #dc2626)" }}>{v?.summary?.down ?? "-"}</span> / 共 {v?.summary?.total ?? "-"}
          </div>
        ))}
      </div>

      <Section title="① 全服务健康状态(实时探活)">
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
          {(svc?.items || []).map((p) => <ProbeCard key={p.name} p={p} />)}
          {!svc && <span style={{ color: "var(--esp-color-text-muted, #999)" }}>加载中…</span>}
        </div>
      </Section>

      <Section title="② 基础设施实时监测">
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
          {(infra?.items || []).map((p) => <ProbeCard key={p.name} p={p} />)}
          {!infra && <span style={{ color: "var(--esp-color-text-muted, #999)" }}>加载中…</span>}
        </div>
      </Section>

      <Section title={`③ Prometheus 指标查询${alerts ? " · 活动告警 " + firing.length + " 条" : ""}`}>
        <div style={{ display: "flex", gap: 8, marginBottom: 8 }}>
          <input style={{ flex: 1, padding: "6px 8px" }} value={promQL} onChange={(e) => setPromQL(e.target.value)} placeholder="PromQL 查询(如 sum(up))" />
          <button className="primary" onClick={runQuery}>查询</button>
        </div>
        {firing.length > 0 && (
          <ul style={{ fontSize: 13 }}>
            {firing.map((f: any, i: number) => (
              <li key={i} style={{ color: "var(--esp-danger, #dc2626)" }}>
                <b>{f.labels?.alertname || f.labels?.alert}</b> @ {f.labels?.job || f.labels?.instance} · {f.labels?.severity}
              </li>
            ))}
          </ul>
        )}
        {promResult && <pre style={{ background: "var(--esp-color-bg-muted, #f3f4f6)", padding: 8, borderRadius: 6, fontSize: 12, overflow: "auto", maxHeight: 200 }}>{promResult}</pre>}
      </Section>

      <Section title="④ 统一 API 目录(全部服务)">
        <table className="analytics" style={{ width: "100%" }}>
          <thead><tr><th>服务</th><th>语言</th><th>ClusterIP</th><th>健康端点</th></tr></thead>
          <tbody>
            {(catalog?.services || []).map((s) => (
              <tr key={s.name}><td>{s.name}</td><td>{s.lang}</td><td>{s.svc}:{s.port}</td><td>{s.path}</td></tr>
            ))}
          </tbody>
        </table>
      </Section>
    </div>
  );
}
