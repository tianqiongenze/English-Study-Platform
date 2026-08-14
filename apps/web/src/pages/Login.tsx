import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { api } from "../api";

export default function Login() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState("learner");
  const [err, setErr] = useState("");
  const { setAuth } = useAuth();
  const nav = useNavigate();

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    try {
      const res = mode === "login"
        ? await api.login(email, password)
        : await api.register(email, password, name || email.split("@")[0], role);
      setAuth(res.token, res.user);
      nav(res.user.role === "teacher" ? "/teacher/lesson-plan" : "/learning/speech");
    } catch (e) {
      setErr(`${e}`);
    }
  }

  return (
    <div className="auth">
      <h1>English Study Platform</h1>
      <div className="tabs">
        <button className={`tab ${mode === "login" ? "active" : ""}`} onClick={() => setMode("login")}>登录</button>
        <button className={`tab ${mode === "register" ? "active" : ""}`} onClick={() => setMode("register")}>注册</button>
      </div>
      <form onSubmit={submit}>
        {mode === "register" && (
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="姓名" />
        )}
        <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="邮箱" type="email" required />
        <input value={password} onChange={(e) => setPassword(e.target.value)} placeholder="密码" type="password" required />
        {mode === "register" && (
          <select value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="learner">学习者</option>
            <option value="teacher">教师</option>
          </select>
        )}
        <button className="primary" type="submit">{mode === "login" ? "登录" : "注册并登录"}</button>
        {err && <p className="err">{err}</p>}
      </form>
      <p className="hint">JWT 由 BFF 签发;本地开发 BFF 未启时可直连各服务(见 vite.config 代理)。</p>
    </div>
  );
}
