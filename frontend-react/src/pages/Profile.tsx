import { useEffect, useState } from "react";
import { useAuth } from "../auth";
import { useNavigate } from "react-router-dom";
import { api } from "../api";
import type { SessionSummary } from "../types";

// 个人中心:用户信息卡 + 会话历史入口(会话22-3 C 批次)。
export default function Profile() {
  const { user, logout } = useAuth();
  const nav = useNavigate();
  const [history, setHistory] = useState<SessionSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");

  useEffect(() => {
    let alive = true;
    api.sessionHistory()
      .then((r) => { if (alive) { setHistory(r); setLoading(false); } })
      .catch((e) => { if (alive) { setErr(String(e.message || e)); setLoading(false); } });
    return () => { alive = false; };
  }, []);

  return (
    <div className="app">
      <h1>个人中心</h1>
      <section style={{ marginBottom: 16 }}>
        <pre>{JSON.stringify(user, null, 2)}</pre>
      </section>

      <h2>会话历史</h2>
      {loading && <p>加载中…</p>}
      {err && <p style={{ color: "crimson" }}>加载失败:{err}</p>}
      {!loading && !err && history.length === 0 && <p>暂无会话记录。</p>}
      {!loading && !err && history.length > 0 && (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th align="left">场景</th>
              <th align="left">状态</th>
              <th align="left">开始</th>
              <th align="left">结束</th>
            </tr>
          </thead>
          <tbody>
            {history.map((s) => (
              <tr key={s.id} style={{ borderBottom: "1px solid #333" }}>
                <td>{s.scenario}</td>
                <td>{s.status}</td>
                <td>{s.started_at}</td>
                <td>{s.ended_at ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div style={{ marginTop: 16 }}>
        <button onClick={() => { logout(); nav("/auth/login"); }}>退出登录</button>
      </div>
    </div>
  );
}
