import React from "react";
import { useI18n } from "@esp/ui-tokens";

/**
 * 全局错误边界:捕获子树渲染期未处理异常,渲染降级 UI。
 * 降级背景/文字走 @esp/ui-tokens 变量(亮暗随主题)。
 */
type Props = { children: React.ReactNode };
type State = { error: Error | null };

function CrashActions({ onRetry, onReload }: { onRetry: () => void; onReload: () => void }) {
  const { t } = useI18n();
  const btn: React.CSSProperties = {
    padding: "8px 16px",
    background: "var(--esp-brand)",
    color: "var(--esp-brand-text-on)",
    border: "none",
    borderRadius: 6,
    cursor: "pointer",
    fontSize: "0.9rem",
  };
  return (
    <div style={{ display: "flex", gap: 12, marginTop: 8 }}>
      <button style={btn} onClick={onRetry}>{t("action.retry")}</button>
      <button style={{ ...btn, background: "var(--esp-color-bg-subtle)", border: "1px solid var(--esp-color-border)", color: "var(--esp-color-text)" }} onClick={onReload}>
        {t("action.refresh")}
      </button>
    </div>
  );
}

export default class ErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // 上报/记录:控制台留痕(componentStack 便于定位出错组件)
    console.error("[ErrorBoundary] 捕获到未处理异常:", error, info.componentStack);
    // OTel span 上报(自定义事件供 OpenTelemetry SDK 或全局 listener 捕获)
    if (typeof window !== "undefined") {
      window.dispatchEvent(new CustomEvent("esp-error", {
        detail: { error: error.message, stack: error.stack, component: info.componentStack },
      }));
    }
  }

  handleRetry = () => {
    this.setState({ error: null });
  };

  handleReload = () => {
    window.location.reload();
  };

  render() {
    if (!this.state.error) return this.props.children;

    const box: React.CSSProperties = {
      minHeight: "100vh",
      boxSizing: "border-box",
      display: "flex",
      flexDirection: "column",
      alignItems: "center",
      justifyContent: "center",
      gap: 12,
      padding: 24,
      background: "var(--esp-color-bg-layout)",
      color: "var(--esp-color-text)",
      fontFamily: "var(--esp-font-family)",
      textAlign: "center",
    };

    return (
      <div style={box} role="alert">
        <h2 style={{ margin: 0, fontSize: "1.2rem" }}>页面出现异常</h2>
        <p style={{ margin: 0, color: "var(--esp-color-text-tertiary)" }}>
          抱歉,页面渲染时发生了错误,请重试;若持续失败请刷新页面。
        </p>
        {this.state.error.message ? (
          <code
            style={{
              maxWidth: 480,
              background: "var(--esp-color-bg-subtle)",
              color: "var(--esp-color-error)",
              padding: "6px 10px",
              borderRadius: 6,
              fontSize: "0.8rem",
              wordBreak: "break-all",
            }}
          >
            {this.state.error.message}
          </code>
        ) : null}
        <CrashActions onRetry={this.handleRetry} onReload={this.handleReload} />
      </div>
    );
  }
}