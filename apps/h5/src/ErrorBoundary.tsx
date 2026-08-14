import React from "react";

/**
 * 全局错误边界:捕获子树渲染期未处理异常,渲染降级 UI。
 * 降级风格与现有深色主题一致(底 #0f172a / 文字 #e2e8f0)。
 */
type Props = { children: React.ReactNode };
type State = { error: Error | null };

export default class ErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // 上报/记录:控制台留痕(componentStack 便于定位出错组件)
    console.error("[ErrorBoundary] 捕获到未处理异常:", error, info.componentStack);
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
      background: "#0f172a",
      color: "#e2e8f0",
      fontFamily: "system-ui, sans-serif",
      textAlign: "center",
    };
    const btn: React.CSSProperties = {
      padding: "8px 16px",
      background: "#3b82f6",
      color: "#fff",
      border: "none",
      borderRadius: 6,
      cursor: "pointer",
      fontSize: "0.9rem",
    };

    return (
      <div style={box} role="alert">
        <h2 style={{ margin: 0, fontSize: "1.2rem" }}>页面出现异常</h2>
        <p style={{ margin: 0, color: "#94a3b8" }}>
          抱歉,页面渲染时发生了错误,请重试;若持续失败请刷新页面。
        </p>
        {this.state.error.message ? (
          <code
            style={{
              maxWidth: 480,
              background: "#1e293b",
              color: "#f87171",
              padding: "6px 10px",
              borderRadius: 6,
              fontSize: "0.8rem",
              wordBreak: "break-all",
            }}
          >
            {this.state.error.message}
          </code>
        ) : null}
        <div style={{ display: "flex", gap: 12, marginTop: 8 }}>
          <button style={btn} onClick={this.handleRetry}>重试</button>
          <button style={{ ...btn, background: "#1e293b", border: "1px solid #334155", color: "#e2e8f0" }} onClick={this.handleReload}>
            刷新页面
          </button>
        </div>
      </div>
    );
  }
}
