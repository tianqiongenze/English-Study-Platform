import React from "react";
import ReactDOM from "react-dom/client";
import "@esp/ui-tokens/tokens.css";
import App from "./App";
import ErrorBoundary from "./ErrorBoundary";
import AntdProvider from "./AntdProvider";
import {
  ThemeProvider,
  I18nProvider,
  createI18n,
  HTML_LANG,
  RTL_LOCALES,
  type Locale,
} from "@esp/ui-tokens";
import "./styles.css";

// i18n 单源实例(词典来自 @esp/ui-tokens);locale 持久化到 localStorage 'esp_locale',
// 恢复时以存储值为准,首次访问默认 zh。<html lang>/<html dir> 同步方便浏览器词典/读屏 + RTL 渲染。
const ALL_LOCALES: Locale[] = ["zh", "en", "ar", "fr", "ru", "es", "de", "he", "el"];
function readStoredLocale(): Locale {
  try {
    const v = localStorage.getItem("esp_locale");
    return (ALL_LOCALES as string[]).includes(v ?? "") ? (v as Locale) : "zh";
  } catch {
    return "zh";
  }
}

// 同步 <html lang> 与 dir(RTL 语言设 rtl,其余 ltr)。
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
// 首帧同步 <html lang/dir>(不依赖切换回调)
applyHtmlLangDir(i18n.locale);

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider>
      <I18nProvider i18n={i18n}>
        <AntdProvider>
          <ErrorBoundary>
            <App />
          </ErrorBoundary>
        </AntdProvider>
      </I18nProvider>
    </ThemeProvider>
  </React.StrictMode>
);