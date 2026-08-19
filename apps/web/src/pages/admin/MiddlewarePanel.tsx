// 中间件统一访问面板(2026-08-19):替换 GovernanceDashboard 硬编码 VM IP 链接。
// 方案:BFF 代理——浏览器只访问 BFF(/api/admin/middleware/...),admin-go 在集群内
//   经 ExternalName svc 透明转发到宿主中间件 UI,前端不持宿主 IP(工业级,不暴露 VM IP)。
// 走 @esp/api-client(对齐 plan Phase4 admin 三面板走 api-client + 设计 token),消除直 fetch。
import { useEffect, useState } from "react";
import { api } from "../../api";
import type { AdminMiddlewareHealth, AdminMiddlewareHealthResponse } from "@esp/api-client";

const REFRESH_MS = 10000;

// 控制台链接前缀(经 BFF 代理,admin-go 剥 /api/admin/middleware/{name} 前缀后转发)。
// grafana:默认大盘 esp-overview;其它:根路径(各 UI 自身首页)。
const DEFAULT_PATH: Record<string, string> = {
  grafana: "/d/esp-overview",
  prometheus: "/",
  loki: "/",
  alertmanager: "/",
  tempo: "/",
  nacos: "/nacos/",
  sentinel: "/",
  "otel-collector": "/metrics",
};

const KIND_LABEL: Record<string, string> = {
  observability: "可观测",
  governance: "治理",
};

function StatusDot({ up }: { up: boolean }) {
  return (
    <span
      style={{
        width: 10,
        height: 10,
        borderRadius: 5,
        background: up ? "var(--esp-success, #16a34a)" : "var(--esp-danger, #dc2626)",
        display: "inline-block",
      }}
    />
  );
}

function MiddlewareCard({ m }: { m: AdminMiddlewareHealth }) {
  const up = m.status === "UP";
  const path = DEFAULT_PATH[m.name] ?? "/";
  // 经 BFF 代理:浏览器同源请求 /api/admin/middleware/<name>/<path>,admin-go 转发到 UI。
  const consoleUrl = `/api/admin/middleware/${m.name}${path}`;
  return (
    <div
      style={{
        border: `1px solid ${up ? "var(--esp-color-border, #e5e7eb)" : "var(--esp-danger, #ef4444)"}`,
        borderRadius: 8,
        padding: 12,
        background: up ? "var(--esp-success-bg, #f0fdf4)" : "var(--esp-danger-bg, #fef2f2)",
        minWidth: 240,
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 8, justifyContent: "space-between" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <StatusDot up={up} />
          <b style={{ textTransform: "capitalize" }}>{m.name}</b>
          <span
            style={{
              fontSize: 11,
              color: "var(--esp-color-text-on, #fff)",
              background: "var(--esp-color-text-muted, #6b7280)",
              padding: "1px 6px",
              borderRadius: 8,
            }}
          >
            {KIND_LABEL[m.kind] ?? m.kind}
          </span>
        </div>
      </div>
      <div style={{ fontSize: 12, color: "var(--esp-color-text-secondary, #6b7280)", marginTop: 4, fontFamily: "monospace" }}>
        {m.addr}
      </div>
      <div style={{ fontSize: 12, marginTop: 4, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <span style={{ color: up ? "var(--esp-success, #16a34a)" : "var(--esp-danger, #dc2626)", fontWeight: 600 }}>
          {m.status}
        </span>
        <span style={{ color: "var(--esp-color-text-secondary, #6b7280)" }}>{m.latency_ms}ms</span>
      </div>
      {!up && m.msg && (
        <div style={{ fontSize: 11, color: "var(--esp-danger, #b91c1c)", marginTop: 2, wordBreak: "break-all" }}>{m.msg}</div>
      )}
      <a
        href={consoleUrl}
        target="_blank"
        rel="noopener noreferrer"
        style={{
          display: "inline-block",
          marginTop: 8,
          padding: "4px 12px",
          background: up ? "var(--esp-primary, #3b82f6)" : "var(--esp-color-text-muted, #9ca3af)",
          color: "var(--esp-color-text-on, #fff)",
          borderRadius: 6,
          textDecoration: "none",
          fontSize: 13,
        }}
      >
        打开控制台
      </a>
    </div>
  );
}

export default function MiddlewarePanel() {
  const [data, setData] = useState<AdminMiddlewareHealthResponse | null>(null);
  const [err, setErr] = useState("");
  const [checkedAt, setCheckedAt] = useState("");

  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const d = await api.adminMiddlewareHealth();
        if (!alive) return;
        setData(d);
        setCheckedAt(new Date().toLocaleTimeString());
        setErr("");
      } catch (e) {
        if (alive) setErr(`${e}`);
      }
    };
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, []);

  const items = data?.items ?? [];
  const upCount = items.filter((m) => m.status === "UP").length;

  return (
    <div style={{ padding: 24 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
        <h2>中间件统一访问面板</h2>
        <div style={{ fontSize: 12, color: "var(--esp-color-text-secondary, #777)" }}>
          {checkedAt && `轮询: ${checkedAt} · `}
          在线 <span style={{ color: "var(--esp-success, #16a34a)", fontWeight: 600 }}>{upCount}</span> / 共 {items.length}
          {" "}· 经 BFF 代理(不暴露宿主 IP)
        </div>
      </div>
      {err && <p className="err">{err}</p>}

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))", gap: 12, marginTop: 16 }}>
        {items.map((m) => (
          <MiddlewareCard key={m.name} m={m} />
        ))}
        {!data && <span style={{ color: "var(--esp-color-text-muted, #999)" }}>加载中…</span>}
      </div>

      <p style={{ fontSize: 12, color: "var(--esp-color-text-muted, #9ca3af)", marginTop: 16 }}>
        控制台链接经 BFF 反向代理(admin-go → ExternalName svc → 宿主中间件 UI),前端不持有 VM IP:端口,
        浏览器同源访问 /api/admin/middleware/&lt;name&gt;/... 即可,鉴权靠 esp_token httpOnly cookie(role=admin)。
      </p>
    </div>
  );
}
