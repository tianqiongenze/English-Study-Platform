import { NavLink, Outlet } from "react-router-dom";
import { useI18n } from "@esp/ui-tokens";
import { useAuth } from "../auth";
import AppHeader from "../components/AppHeader";

export default function LearningLayout() {
  const { t } = useI18n();
  const { user } = useAuth();
  return (
    <div className="app">
      <header>
        <AppHeader title={`${t("app.title")} · ${t("learning.title")}`}>
          <nav className="tabs">
            <NavLink to="/learning/speech" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.speaking")}</NavLink>
            <NavLink to="/learning/writing" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.writing")}</NavLink>
            <NavLink to="/learning/knowledge" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.learning")}</NavLink>
            <NavLink to="/learning/review" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>复习清单</NavLink>
            <NavLink to="/learning/difficulty" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>动态难度</NavLink>
            <NavLink to="/learning/content" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.content")}</NavLink>
            <NavLink to="/learning/community" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.community")}</NavLink>
            <NavLink to="/learning/ranking" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("nav.ranking")}</NavLink>
            {user?.role === "admin" && (
              <NavLink to="/admin" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>管理员监控</NavLink>
            )}
          </nav>
        </AppHeader>
      </header>
      <div className="panel"><Outlet /></div>
    </div>
  );
}