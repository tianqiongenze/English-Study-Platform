import React from "react";
import ReactDOM from "react-dom/client";
import "@esp/ui-tokens/tokens.css";
import App from "./App";
import ErrorBoundary from "./ErrorBoundary";
import { ThemeProvider, I18nProvider, createI18n } from "@esp/ui-tokens";
import "./styles.css";

// i18n 单源实例(词典来自 @esp/ui-tokens);locale 持久化到 localStorage 'esp_locale',
// 恢复时以存储值为准,首次访问默认 zh。<html lang> 同步方便浏览器词典/读屏。
function readStoredLocale(): "en" | "zh" {
  try {
    const v = localStorage.getItem("esp_locale");
    return v === "en" || v === "zh" ? v : "zh";
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
  document.documentElement.lang = l === "zh" ? "zh-CN" : "en";
});
// 首帧同步 <html lang>(不依赖切换回调)
document.documentElement.lang = i18n.locale === "zh" ? "zh-CN" : "en";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider>
      <I18nProvider i18n={i18n}>
        <ErrorBoundary>
          <App />
        </ErrorBoundary>
      </I18nProvider>
    </ThemeProvider>
  </React.StrictMode>
);