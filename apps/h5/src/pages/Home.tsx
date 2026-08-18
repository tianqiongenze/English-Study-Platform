import { useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { ThemeSwitcher, AccentPicker, LangSwitcher, useI18n } from "@esp/ui-tokens";
import { useAuth } from "../auth";

// Home 卡图:unsplash 免费商用直链(w=600 竖图),本地梯度兜底(--esp-img-fallback-*)
const HERO_IMG =
  "https://images.unsplash.com/photo-1522203851244-10e1ebd683d3?auto=format&fit=crop&w=600&q=60";
const WRITING_IMG =
  "https://images.unsplash.com/photo-1517245386807-bb43f82c33c4?auto=format&fit=crop&w=600&q=60";
const LEARNING_IMG =
  "https://images.unsplash.com/photo-1434030216411-0b793f4b4173?auto=format&fit=crop&w=600&q=60";

/** 竖图卡片:loading=lazy;加载失败隐藏图片露出容器渐变兜底 */
function FeatureCard({
  to,
  img,
  fallback,
  label,
}: {
  to: string;
  img: string;
  fallback: string;
  label: string;
}) {
  const [broken, setBroken] = useState(false);
  return (
    <NavLink to={to} className="feature-card" style={{ background: fallback }} aria-label={label}>
      {!broken && <img src={img} alt={label} loading="lazy" onError={() => setBroken(true)} />}
      <span className="feature-label">{label}</span>
    </NavLink>
  );
}

export default function Home() {
  const { user, logout } = useAuth(); const nav = useNavigate(); const { t } = useI18n();
  return (
    <div className="app">
      <header>
        <span className="title">ESP</span>
        <div className="header-right">
          <ThemeSwitcher compact />
          <AccentPicker compact />
          <LangSwitcher compact />
          <span className="header-user">{user?.name} <button onClick={() => { logout(); nav("/login"); }}>{t("auth.logout")}</button></span>
        </div>
      </header>
      <section className="feature-cards" aria-label={t("nav.home")}>
        <FeatureCard to="/speech" img={HERO_IMG} fallback="var(--esp-img-fallback-1)" label={t("nav.speaking")} />
        <FeatureCard to="/writing" img={WRITING_IMG} fallback="var(--esp-img-fallback-2)" label={t("nav.writing")} />
        <FeatureCard to="/learning" img={LEARNING_IMG} fallback="var(--esp-img-fallback-3)" label={t("nav.learning")} />
      </section>
      <nav className="tabs">
        <NavLink to="/speech" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.speaking")}</NavLink>
        <NavLink to="/writing" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.writing")}</NavLink>
        <NavLink to="/learning" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.learning")}</NavLink>
        <NavLink to="/profile" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.profile")}</NavLink>
      </nav>
      <Outlet />
    </div>
  );
}