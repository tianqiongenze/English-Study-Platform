import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { api } from "../api";

export default function Login() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState(""); const [password, setPassword] = useState("");
  const [name, setName] = useState(""); const [err, setErr] = useState("");
  const { setAuth } = useAuth(); const nav = useNavigate();

  async function submit(e: React.FormEvent) {
    e.preventDefault(); setErr("");
    try {
      const r = mode === "login" ? await api.login(email, password)
        : await api.register(email, password, name || email.split("@")[0], "learner");
      setAuth(r.token, r.user); nav("/");
    } catch (e) { setErr("登录失败:" + e); }
  }

  return (
    <div className="app">
      <h2 className="title" style={{ textAlign: "center", marginTop: 40 }}>ESP · Home-Tutor</h2>
      <div className="tabs" style={{ justifyContent: "center" }}>
        <button className={`tab ${mode === "login" ? "active" : ""}`} onClick={() => setMode("login")}>登录</button>
        <button className={`tab ${mode === "register" ? "active" : ""}`} onClick={() => setMode("register")}>注册</button>
      </div>
      <form onSubmit={submit}>
        {mode === "register" && <input value={name} onChange={(e) => setName(e.target.value)} placeholder="姓名" />}
        <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="邮箱" type="email" required />
        <input value={password} onChange={(e) => setPassword(e.target.value)} placeholder="密码" type="password" required />
        <button className="primary" type="submit" style={{ width: "100%" }}>{mode === "login" ? "登录" : "注册"}</button>
        {err && <p className="err">{err}</p>}
      </form>
    </div>
  );
}
