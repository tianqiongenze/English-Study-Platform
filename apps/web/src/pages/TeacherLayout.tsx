import { NavLink, Outlet } from "react-router-dom";
import { useI18n } from "@esp/ui-tokens";
import AppHeader from "../components/AppHeader";

export default function TeacherLayout() {
  const { t } = useI18n();
  return (
    <div className="app">
      <header>
        <AppHeader title={`${t("app.title")} · ${t("nav.teacher")}`} roleLabel={t("nav.teacher")}>
          <nav className="tabs">
            <NavLink to="/teacher/lesson-plan" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>教案生成</NavLink>
            <NavLink to="/teacher/analytics" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>{t("teacher.reports")}</NavLink>
            <NavLink to="/teacher/grading" className={({ isActive }) => `tab ${isActive ? "active" : ""}`}>作业评阅</NavLink>
          </nav>
        </AppHeader>
      </header>
      <div className="panel"><Outlet /></div>
    </div>
  );
}