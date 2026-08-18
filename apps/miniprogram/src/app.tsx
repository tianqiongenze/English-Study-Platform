import { Component } from "react";
import { View } from "@tarojs/components";
import "./app.scss";
import { resolveTheme, applyChrome, subscribeTheme, type MpTheme } from "./theme";
import { syncNavTitle } from "./i18n";

// 全局:从缓存恢复 token;登录态存 wx.storage(小程序无 localStorage)。
// 主题:esp_theme 偏好 → app-root 挂 theme-dark 类 + 系统 chrome 联动(主题切换联动项)。
export interface AuthUser { id: number; email: string; name: string; role: string }

class App extends Component<{}, { theme: MpTheme }> {
  state = { theme: resolveTheme() };
  private unsubscribeTheme: (() => void) | null = null;

  componentDidMount() {
    const token = wx.getStorageSync("esp_token");
    if (token) (this as any).token = token;

    // 主题:启动同步(首屏防闪烁 state 已在构造期取好)+ 导航/页面 chrome 联动
    const theme = resolveTheme();
    this.setState({ theme });
    applyChrome(theme);
    this.unsubscribeTheme = subscribeTheme((next) => this.setState({ theme: next }));

    // 语言:导航标题按当前 locale
    syncNavTitle();
  }

  componentWillUnmount() {
    this.unsubscribeTheme?.();
  }

  render() {
    const { theme } = this.state;
    // theme-dark 类驱动 app.scss 暗色变量套;data-theme 属性双写出,兼容 page[data-theme] 选择器写法
    return (
      <View
        className={theme === "dark" ? "app-root theme-dark" : "app-root"}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        {...({ "data-theme": theme } as any)}
      >
        {(this.props as any).children}
      </View>
    );
  }
}

export default App;