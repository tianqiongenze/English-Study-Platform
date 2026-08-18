import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { api } from "../api";
import { useI18n, LogoIcon } from "@esp/ui-tokens";
import SafeImg, { UNSPLASH } from "../components/SafeImg";

export default function Login() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState("learner");
  const [err, setErr] = useState("");
  const { setAuth } = useAuth();
  const { t } = useI18n();
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
    <div className="auth-page">
      {/* hero:Unsplash 学习场景图 + 品牌氛围;网络不可靠时渐变 fallback */}
      <section className="auth-hero" aria-hidden="true">
        <SafeImg src={UNSPLASH.study} alt="" tone={1} />
        <div className="auth-hero__overlay">
          <p className="auth-hero__tag">{t("app.tagline")}</p>
        </div>
      </section>

      <main className="auth-main">
        <div className="auth-card">
          <div className="auth-card__brand">
            <LogoIcon size={40} />
            <div>
              <div className="auth-card__name">{t("app.title")}</div>
              <div className="auth-card__tag">{t("app.tagline")}</div>
            </div>
          </div>

          {/* 品牌渐变标题 */}
          <h2 className="auth-title">{mode === "login" ? t("auth.welcome") : t("auth.register")}</h2>
          <p className="auth-subtitle">{t("auth.enter")}</p>

          <div className="tabs">
            <button className={`tab ${mode === "login" ? "active" : ""}`} onClick={() => setMode("login")}>{t("auth.login")}</button>
            <button className={`tab ${mode === "register" ? "active" : ""}`} onClick={() => setMode("register")}>{t("auth.register")}</button>
          </div>

          <form onSubmit={submit}>
            {mode === "register" && (
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("auth.name")} />
            )}
            <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder={t("auth.email")} type="email" required />
            <input value={password} onChange={(e) => setPassword(e.target.value)} placeholder={t("auth.password")} type="password" required />
            {mode === "register" && (
              <select value={role} onChange={(e) => setRole(e.target.value)}>
                <option value="learner">学习者</option>
                <option value="teacher">{t("nav.teacher")}</option>
              </select>
            )}
            <button className="primary" type="submit">{mode === "login" ? t("auth.login") : "注册并登录"}</button>
            {err && <p className="err">{err}</p>}
          </form>

          {mode === "register" && (
            <p className="auth-footer">
              {t("auth.haveAccount")}{" "}
              <button type="button" onClick={() => setMode("login")}>{t("auth.login")}</button>
            </p>
          )}
          {mode === "login" && (
            <p className="auth-footer">
              {t("auth.noAccount")}{" "}
              <button type="button" onClick={() => setMode("register")}>{t("auth.register")}</button>
            </p>
          )}
        </div>
      </main>
    </div>
  );
}