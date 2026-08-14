import { useEffect, useState } from "react";
import { useAuth } from "../auth";
import { useNavigate } from "react-router-dom";
import { List, Button, Card, DotLoading } from "antd-mobile";
import { api } from "../api";
import type { SessionSummary } from "../types";

// H5 个人中心:用户信息 + 会话历史(antd-mobile 列表,会话22-3 C 批次三端贯通)。
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
    <div className="app" style={{ padding: 12 }}>
      <Card title="个人中心" extra={<span style={{ fontSize: "0.8rem" }}>{user?.name}</span>}>
        <pre style={{ fontSize: "0.75rem", margin: 0 }}>{JSON.stringify(user, null, 2)}</pre>
      </Card>

      <h3 style={{ marginTop: 16, marginBottom: 8 }}>会话历史</h3>
      {loading && <span><DotLoading /> 加载中…</span>}
      {err && <div style={{ color: "crimson" }}>加载失败:{err}</div>}
      {!loading && !err && history.length === 0 && <div style={{ color: "#888" }}>暂无会话记录。</div>}
      {!loading && !err && history.length > 0 && (
        <List>
          {history.map((s) => (
            <List.Item key={s.id} description={`状态:${s.status} · 结束:${s.ended_at ?? "进行中"}`}>
              {s.scenario}
              <span style={{ fontSize: "0.7rem", color: "#999", display: "block" }}>开始:{s.started_at}</span>
            </List.Item>
          ))}
        </List>
      )}

      <div style={{ marginTop: 16 }}>
        <Button block color="danger" onClick={() => { logout(); nav("/login"); }}>退出登录</Button>
      </div>
    </div>
  );
}
