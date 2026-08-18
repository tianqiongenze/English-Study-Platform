import React, { Suspense, lazy } from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { ConfigProvider } from "antd-mobile";
import zhCN from "antd-mobile/es/locales/zh-CN";
import enUS from "antd-mobile/es/locales/en-US";
import {
  createI18n,
  I18nProvider,
  ThemeProvider,
  useI18n,
} from "@esp/ui-tokens";
import { useAuth } from "./auth";
import Login from "./pages/Login";
import ErrorBoundary from "./ErrorBoundary";
import { SkeletonPage } from "./Skeleton";
import "@esp/ui-tokens/tokens.css";
import "@esp/ui-tokens/tokens-mobile.css";
import "./styles.css";

// 路由懒加载(React.lazy):非首屏页面级组件按需分包。
// chunk 命名由 Vite 自动按模块名处理,无需 /* webpackChunkName */ 注释。
const Home = lazy(() => import("./pages/Home"));
const Speech = lazy(() => import("./pages/Speech"));
const Writing = lazy(() => import("./pages/Writing"));
const Learning = lazy(() => import("./pages/Learning"));
const Profile = lazy(() => import("./pages/Profile"));

/** 懒加载路由包裹器:加载期间展示页面级骨架屏 */
function Lazy({ children }: { children: React.JSX.Element }) {
  return <Suspense fallback={<SkeletonPage />}>{children}</Suspense>;
}

function Require({ children }: { children: React.JSX.Element }) {
  const user = useAuth((s) => s.user);
  return user ? children : <Navigate to="/login" replace />;
}

// 语言持久化键(与设计稿 /h5 端约定一致):esp_locale
function readStoredLocale(): "zh" | "en" {
  try {
    return localStorage.getItem("esp_locale") === "en" ? "en" : "zh";
  } catch {
    return "zh";
  }
}
const i18n = createI18n(readStoredLocale(), (l) => {
  try {
    localStorage.setItem("esp_locale", l);
  } catch {
    /* private mode */
  }
});

// config-provider 双语:locale 随 <I18nProvider> 联动切换(antd-mobile 内置包已随库提供)。
function ThemedApp() {
  const { locale } = useI18n();
  return (
    <ConfigProvider locale={locale === "zh" ? zhCN : enUS}>
      <BrowserRouter basename="/h5">
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<Require><Lazy><Home /></Lazy></Require>}>
            <Route index element={<Navigate to="/speech" replace />} />
            <Route path="speech" element={<Lazy><Speech /></Lazy>} />
            <Route path="writing" element={<Lazy><Writing /></Lazy>} />
            <Route path="learning" element={<Lazy><Learning /></Lazy>} />
            <Route path="profile" element={<Lazy><Profile /></Lazy>} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </ConfigProvider>
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider>
      <I18nProvider i18n={i18n}>
        <ErrorBoundary>
          <ThemedApp />
        </ErrorBoundary>
      </I18nProvider>
    </ThemeProvider>
  </React.StrictMode>
);