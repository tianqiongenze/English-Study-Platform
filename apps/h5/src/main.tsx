import React, { Suspense, lazy } from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { ConfigProvider } from "antd-mobile";
import zhCN from "antd-mobile/es/locales/zh-CN";
import enUS from "antd-mobile/es/locales/en-US";
import arSA from "antd-mobile/es/locales/ar-SA";
import frFR from "antd-mobile/es/locales/fr-FR";
import ruRU from "antd-mobile/es/locales/ru-RU";
import esES from "antd-mobile/es/locales/es-ES";
import deDE from "antd-mobile/es/locales/de-DE";
import {
  createI18n,
  I18nProvider,
  ThemeProvider,
  useI18n,
  HTML_LANG,
  RTL_LOCALES,
  type Locale,
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

// 语言持久化键(与设计稿 /h5 端约定一致):esp_locale。支持 9 语言,首次默认 zh。
const ALL_LOCALES: Locale[] = ["zh", "en", "ar", "fr", "ru", "es", "de", "he", "el"];
function readStoredLocale(): Locale {
  try {
    const v = localStorage.getItem("esp_locale");
    return (ALL_LOCALES as string[]).includes(v ?? "") ? (v as Locale) : "zh";
  } catch {
    return "zh";
  }
}

// 同步 <html lang> 与 dir(RTL 语言 ar/he 设 rtl)。
function applyHtmlLangDir(l: Locale): void {
  document.documentElement.lang = HTML_LANG[l];
  document.documentElement.dir = RTL_LOCALES.has(l) ? "rtl" : "ltr";
}

const i18n = createI18n(readStoredLocale(), (l) => {
  try {
    localStorage.setItem("esp_locale", l);
  } catch {
    /* private mode */
  }
  applyHtmlLangDir(l);
});
applyHtmlLangDir(i18n.locale);

// antd-mobile locale 映射(注意:antd-mobile 无 he/el 包,回落 en-US)。
const MOBILE_LOCALE: Record<Locale, typeof zhCN> = {
  zh: zhCN,
  en: enUS,
  ar: arSA,
  fr: frFR,
  ru: ruRU,
  es: esES,
  de: deDE,
  he: enUS,
  el: enUS,
};

// config-provider 双语:locale 随 <I18nProvider> 联动切换(antd-mobile 内置包已随库提供)。
function ThemedApp() {
  const { locale } = useI18n();
  return (
    <ConfigProvider locale={MOBILE_LOCALE[locale]}>
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