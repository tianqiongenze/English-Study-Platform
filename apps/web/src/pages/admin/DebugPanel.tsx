// 全语言统一深度诊断面板(Step4 · actuator 风格全 6 endpoint)。
// 方案:全语言统一 actuator 风格 —— Java 原生 actuator 全 6(env/loggers/threaddump/heapdump/beans/mappings);
//   Go pprof 等价 threaddump+heapdump + /debug/* 全等价;Python/Rust /debug/* 等价(threaddump/heapdump 标 NA)。
// 数据源:admin-go(/api/admin/debug/{service}/{endpoint} 代理到各服务 actuator/debug 端点)。
// 鉴权:走 esp_token httpOnly cookie(role=admin),admin-go requireAdmin 守门 + 下游二次校验。
// 走 @esp/api-client(对齐 plan Phase4 admin 面板走 api-client + 设计 token)。
import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../../api";
import { useAuth } from "../../auth";
import type { AdminDebugCapabilities, AdminCatalog, AdminDebugLoggers } from "@esp/api-client";

// actuator 全 6 endpoint + 语义标签(前端按同语义展示,全语言一致)。
const ENDPOINTS = ["env", "loggers", "threaddump", "heapdump", "beans", "mappings"] as const;
type Endpoint = (typeof ENDPOINTS)[number];

const ENDPOINT_LABEL: Record<Endpoint, string> = {
  env: "env 环境变量",
  loggers: "loggers 日志级别",
  threaddump: "threaddump 线程 dump",
  heapdump: "heapdump 堆 dump",
  beans: "beans Bean 清单",
  mappings: "mappings 路由表",
};

const LANG_BADGE: Record<string, string> = {
  Go: "var(--esp-color-info, #00ADD8)",
  Java: "var(--esp-warning, #ED8B00)",
  Python: "var(--esp-color-info, #3776AB)",
  Rust: "var(--esp-color-text-secondary, #000000)",
};

type Lang = string;

function LangBadge({ lang }: { lang: Lang }) {
  return (
    <span
      style={{
        fontSize: 11,
        color: "var(--esp-color-text-on, #fff)",
        background: LANG_BADGE[lang] || "var(--esp-text-muted, #6b7280)",
        padding: "1px 6px",
        borderRadius: 8,
      }}
    >
      {lang}
    </span>
  );
}

function Section({ title, right, children }: { title: string; right?: React.ReactNode; children: React.ReactNode }) {
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

export default function DebugPanel() {
  const user = useAuth((s) => s.user);
  const [catalog, setCatalog] = useState<AdminCatalog | null>(null);
  const [caps, setCaps] = useState<AdminDebugCapabilities | null>(null);
  const [svcName, setSvcName] = useState<string>("");
  const [endpoint, setEndpoint] = useState<Endpoint>("env");
  const [data, setData] = useState<string>("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [logLevelInput, setLogLevelInput] = useState("DEBUG");

  const langOf = useCallback(
    (name: string): Lang => {
      const s = (catalog?.services || []).find((x) => x.name === name);
      return s?.lang || "";
    },
    [catalog]
  );

  // 加载 catalog(服务清单)+ 能力矩阵。
  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const [c, cap] = await Promise.all([api.adminCatalog(), api.adminDebugCapabilities()]);
        if (!alive) return;
        setCatalog(c);
        setCaps(cap);
        if (!svcName && c.services.length) setSvcName(c.services[0].name);
        setErr("");
      } catch (e) {
        if (alive) setErr(`${e}`);
      }
    })();
    return () => {
      alive = false;
    };
  }, [svcName]);

  const supportedForSvc = useMemo<Endpoint[]>(() => {
    if (!caps || !svcName) return [];
    const lang = langOf(svcName);
    const allowed = caps.capabilities[lang] || [];
    return ENDPOINTS.filter((e) => allowed.includes(e));
  }, [caps, svcName, langOf]);

  // 查询某 endpoint(JSON 展示)。
  const query = useCallback(
    async (ep: Endpoint) => {
      setBusy(true);
      setErr("");
      setData("");
      try {
        const r = await api.adminDebugEndpoint(svcName, ep);
        setData(JSON.stringify(r, null, 2).slice(0, 4000));
      } catch (e: any) {
        // heapdump 等非 JSON 走专门下载;此处仅 JSON endpoint。
        setErr(`${e?.status || ""} ${e?.message || e}`);
      } finally {
        setBusy(false);
      }
    },
    [svcName]
  );

  // heapdump 下载(Java only):触发浏览器下载二进制。
  const downloadHeapdump = useCallback(async () => {
    setBusy(true);
    setErr("");
    try {
      const blob = await api.adminDebugHeapdump(svcName);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${svcName}-heapdump.hprof`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
      setData(`heapdump 已触发下载(${svcName}, ${(blob.size / 1024).toFixed(1)} KB)`);
    } catch (e: any) {
      setErr(`heapdump 下载失败: ${e?.message || e}`);
    } finally {
      setBusy(false);
    }
  }, [svcName]);

  // loggers 动态调级。
  const setLogLevel = useCallback(async () => {
    setBusy(true);
    setErr("");
    try {
      const r: AdminDebugLoggers = await api.adminDebugSetLogLevel(svcName, logLevelInput);
      setData(`日志级别已设置: ${r.configuredLevel} (effective: ${r.effectiveLevel || r.configuredLevel})`);
    } catch (e: any) {
      setErr(`调级失败: ${e?.message || e}`);
    } finally {
      setBusy(false);
    }
  }, [svcName, logLevelInput]);

  // endpoint 切换时自动查询(除 heapdump 需点按钮下载)。
  useEffect(() => {
    if (!svcName || endpoint === "heapdump") return;
    query(endpoint);
  }, [svcName, endpoint, query]);

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
        <h2>全语言统一深度诊断面板</h2>
        <span style={{ fontSize: 12, color: "var(--esp-color-text-secondary, #777)" }}>
          actuator 风格 · 全 6 endpoint · Java 原生 / Go pprof 等价 / Python·Rust 等价
        </span>
      </div>
      {user?.role !== "admin" && <p className="err">仅管理员可查看(role=admin)</p>}
      {err && <p className="err">{err}</p>}

      <Section
        title="① 选服务 + 诊断 endpoint"
        right={
          <div style={{ display: "flex", gap: 8 }}>
            <select
              style={{ padding: "6px 8px" }}
              value={svcName}
              onChange={(e) => setSvcName(e.target.value)}
            >
              {(catalog?.services || []).map((s) => (
                <option key={s.name} value={s.name}>
                  {s.name} ({s.lang})
                </option>
              ))}
            </select>
          </div>
        }
      >
        <div style={{ fontSize: 12, color: "var(--esp-color-text-secondary, #555)", marginBottom: 8 }}>
          当前服务:
          <b style={{ marginLeft: 6 }}>
            {svcName} <LangBadge lang={langOf(svcName)} />
          </b>
          。支持:
          {supportedForSvc.length ? supportedForSvc.join(" / ") : "—"}
        </div>
        <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
          {ENDPOINTS.map((ep) => {
            const avail = supportedForSvc.includes(ep);
            const active = endpoint === ep;
            return (
              <button
                key={ep}
                disabled={!avail}
                onClick={() => setEndpoint(ep)}
                style={{
                  padding: "5px 10px",
                  cursor: avail ? "pointer" : "not-allowed",
                  border: `1px solid ${active ? "var(--esp-primary, #2563eb)" : "var(--esp-color-border, #e5e7eb)"}`,
                  background: active
                    ? "var(--esp-primary-bg, #dbeafe)"
                    : avail
                      ? "var(--esp-color-bg-subtle, #f9fafb)"
                      : "var(--esp-color-bg-muted, #f3f4f6)",
                  color: avail
                    ? "var(--esp-color-text, #111)"
                    : "var(--esp-color-text-muted, #aaa)",
                  opacity: avail ? 1 : 0.5,
                  borderRadius: 6,
                }}
                title={avail ? ENDPOINT_LABEL[ep] : `${ENDPOINT_LABEL[ep]} — 该语言无原生等价(NA)`}
              >
                {ep}
                {!avail && <span style={{ fontSize: 10, marginLeft: 2 }}>NA</span>}
              </button>
            );
          })}
        </div>
      </Section>

      <Section title={`② ${ENDPOINT_LABEL[endpoint]}`}>
        {endpoint === "heapdump" ? (
          <div>
            <p style={{ fontSize: 13, color: "var(--esp-color-text-secondary, #555)" }}>
              heapdump(堆快照)为二进制文件,点击下方按钮经 admin-go 代理下载 Java actuator/heapdump。
              {langOf(svcName) !== "Java" && (
                <span style={{ color: "var(--esp-danger, #dc2626)" }}> 当前服务非 Java,无原生 heapdump(NA)。</span>
              )}
            </p>
            <button
              className="primary"
              disabled={langOf(svcName) !== "Java" || busy}
              onClick={downloadHeapdump}
              style={{ opacity: langOf(svcName) === "Java" ? 1 : 0.5 }}
            >
              下载 {svcName} heapdump (.hprof)
            </button>
          </div>
        ) : endpoint === "loggers" ? (
          <div>
            <p style={{ fontSize: 13, color: "var(--esp-color-text-secondary, #555)" }}>
              日志级别动态调(等价 actuator loggers POST)。Java 调 logback级别;Go/Python/Rust 调运行期级别变量。
            </p>
            <div style={{ display: "flex", gap: 8, marginBottom: 8 }}>
              <select style={{ padding: "6px 8px" }} value={logLevelInput} onChange={(e) => setLogLevelInput(e.target.value)}>
                {["TRACE", "DEBUG", "INFO", "WARN", "ERROR"].map((l) => (
                  <option key={l} value={l}>{l}</option>
                ))}
              </select>
              <button className="primary" disabled={busy} onClick={setLogLevel}>
                应用日志级别
              </button>
              <button disabled={busy} onClick={() => query("loggers")}>
                刷新当前级别
              </button>
            </div>
          </div>
        ) : endpoint === "threaddump" ? (
          <div>
            <p style={{ fontSize: 13, color: "var(--esp-color-text-secondary, #555)" }}>
              {langOf(svcName) === "Java"
                ? "Java threaddump(JVM 线程快照,actuator 原生)。"
                : langOf(svcName) === "Go"
                  ? "Go 用 pprof goroutine profile 等价 threaddump(goroutine = Go 的轻量线程)。"
                  : "该语言无原生 threaddump 等价(NA)。"}
            </p>
            <button disabled={busy} onClick={() => query("threaddump")}>刷新</button>
          </div>
        ) : (
          <div style={{ display: "flex", gap: 8, marginBottom: 8 }}>
            <button disabled={busy} onClick={() => query(endpoint)}>刷新</button>
          </div>
        )}
        {busy && <span style={{ color: "var(--esp-color-text-muted, #999)" }}>加载中…</span>}
        {data && (
          <pre
            style={{
              background: "var(--esp-color-bg-muted, #f3f4f6)",
              padding: 8,
              borderRadius: 6,
              fontSize: 12,
              overflow: "auto",
              maxHeight: 420,
              whiteSpace: "pre-wrap",
              wordBreak: "break-all",
            }}
          >
            {data}
          </pre>
        )}
      </Section>

      <Section title="③ 全语言能力矩阵(actuator 全 6 endpoint 覆盖)">
        <table className="analytics" style={{ width: "100%" }}>
          <thead>
            <tr>
              <th>语言</th>
              {ENDPOINTS.map((e) => (
                <th key={e}>{e}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {caps &&
              Object.entries(caps.capabilities).map(([lang, eps]) => (
                <tr key={lang}>
                  <td>
                    <LangBadge lang={lang} />
                  </td>
                  {ENDPOINTS.map((e) => {
                    const has = eps.includes(e);
                    return (
                      <td key={e} style={{ textAlign: "center" }}>
                        {has ? (
                          <span style={{ color: "var(--esp-success, #16a34a)", fontWeight: 600 }}>原生</span>
                        ) : lang === "Go" && (e === "threaddump" || e === "heapdump") ? (
                          <span style={{ color: "var(--esp-warning, #ED8B00)" }}>pprof 等价</span>
                        ) : (
                          <span style={{ color: "var(--esp-color-text-muted, #aaa)" }}>NA</span>
                        )}
                      </td>
                    );
                  })}
                </tr>
              ))}
          </tbody>
        </table>
        {caps && (
          <p style={{ fontSize: 12, color: "var(--esp-color-text-secondary, #555)", marginTop: 8 }}>{caps.note}</p>
        )}
      </Section>
    </div>
  );
}
