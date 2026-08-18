import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { api } from "../api";
import { useI18n } from "@esp/ui-tokens";

export default function Login() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState(""); const [password, setPassword] = useState("");
  const [name, setName] = useState(""); const [err, setErr] = useState("");
  const { setAuth } = useAuth(); const nav = useNavigate(); const { t } = useI18n();

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
      <h2 className="title" style={{ textAlign: "center", marginTop: 40 }}>{t("app.title")}</h2>
      <div className="tabs" style={{ justifyContent: "center" }}>
        <button className={`tab ${mode === "login" ? "active" : ""}`} onClick={() => setMode("login")}>{t("auth.login")}</button>
        <button className={`tab ${mode === "register" ? "active" : ""}`} onClick={() => setMode("register")}>{t("auth.register")}</button>
      </div>
      <form onSubmit={submit}>
        {mode === "register" && <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("auth.name")} />}
        <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder={t("auth.email")} type="email" required />
        <input value={password} onChange={(e) => setPassword(e.target.value)} placeholder={t("auth.password")} type="password" required />
        <button className="primary" type="submit" style={{ width: "100%" }}>{mode === "login" ? t("auth.login") : t("auth.register")}</button>
        {err && <p className="err">{err}</p>}
      </form>
    </div>
  );
}
