// 顶栏统一组件:Logo + 主题/主题色/语言切换(compact)+ 用户区。
// LearningLayout 与 TeacherLayout 共用;切换器来自 @esp/ui-tokens(样式走 tokens)。
import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import {
  LogoIcon,
  ThemeSwitcher,
  AccentPicker,
  LangSwitcher,
  useI18n,
} from "@esp/ui-tokens";

export default function AppHeader({
  title,
  nav,
  roleLabel,
  children,
}: {
  title: ReactNode;
  nav?: ReactNode;
  roleLabel?: ReactNode;
  children?: ReactNode;
}) {
  const { user, logout } = useAuth();
  const { t } = useI18n();
  const navTo = useNavigate();
  return (
    <>
      <div className="app-header__brand">
        <LogoIcon size={28} />
        <span className="brand-title">{title}</span>
      </div>
      {nav}
      {children}
      <div className="app-header__side">
        <div className="app-header__switchers">
          <ThemeSwitcher compact />
          <AccentPicker compact />
          <LangSwitcher compact />
        </div>
        {user && (
          <div className="user">
            <span>
              {user.name}
              <span className="hint"> · {roleLabel ?? user.role}</span>
            </span>
            <button onClick={() => { logout(); navTo("/auth/login"); }}>
              {t("auth.logout")}
            </button>
          </div>
        )}
      </div>
    </>
  );
}