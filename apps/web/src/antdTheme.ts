// antd v6 主题单真源 —— 与 tokens.css 同源,ConfigProvider 主题注入零摩擦。
// 设计 token 来源:packages/ui-tokens/tokens.css 的 CSS variables;
// 此处抽为 JS 常量,供 antd ConfigProvider theme 消费,消除"antd 主题 vs tokens.css 主题"双真源漂移。
// 亮/暗:antd algorithm 切换(default/dark);accent:colorPrimary 跟随 --esp-brand 切换。
//
// 注意(antd v6):原生支持 React 19,无需 @ant-design/v5-patch-react-19(该包已从 npm 下架,
// v5 时代的 React 19 兼容补丁,antd v6 不再需要)。
// 位置:放 apps/web(antd 的真实消费方),非 packages/ui-tokens(ui-tokens 不依赖 antd,保持纯 CSS/无 UI 库)。

import type { ThemeConfig } from "antd";
import { theme as antdTheme } from "antd";
import type { Accent, AppTheme } from "@esp/ui-tokens";

// 与 tokens.css :root 同源的主色/中性色/字体/圆角/间距。
// accent → --esp-brand 映射(与 tokens.css html[data-accent="*"] 块一一对应)。
const ACCENT_PRIMARY: Record<Accent, string> = {
  blue: "#1677ff",
  coral: "#ff6b4a",
  forest: "#22a06b",
  grape: "#8b5cf6",
};

const ACCENT_HOVER: Record<Accent, string> = {
  blue: "#4096ff",
  coral: "#ff8a6e",
  forest: "#3fb67f",
  grape: "#a47bf8",
};

// 与 tokens.css --esp-font-family 同源。
const FONT_FAMILY =
  '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif';

/**
 * 构建 antd ThemeConfig:亮/暗 + accent 跟随。
 * @param resolved 已解析的实际主题("light" | "dark",system 已在外层 resolve)
 * @param accent 当前 accent(blue/coral/forest/grape)
 */
export function buildAntdTheme(
  resolved: "light" | "dark",
  accent: Accent
): ThemeConfig {
  const algorithm =
    resolved === "dark"
      ? antdTheme.darkAlgorithm
      : antdTheme.defaultAlgorithm;

  return {
    algorithm,
    token: {
      // brand —— 跟随 accent(与 --esp-brand / --esp-color-primary 同源)
      colorPrimary: ACCENT_PRIMARY[accent],
      colorPrimaryHover: ACCENT_HOVER[accent],
      colorInfo: ACCENT_PRIMARY[accent],
      // functional —— 与 tokens.css --esp-color-* 同源
      colorSuccess: "#52c41a",
      colorWarning: "#faad14",
      colorError: "#ff4d4f",
      // typography —— 与 --esp-font-family / --esp-font-size-base 同源
      fontFamily: FONT_FAMILY,
      fontSize: 14,
      // radius —— 与 --esp-radius-base(6px)同源
      borderRadius: 6,
      borderRadiusLG: 8,
      borderRadiusSM: 4,
      // motion —— 与 --esp-motion-duration-base(0.2s)同源
      motionDurationMid: "0.2s",
    },
    components: {
      Layout: {
        // 与 --esp-color-bg-layout 同源
        bodyBg: resolved === "dark" ? "#0a0f1a" : "#f5f5f5",
        headerBg: resolved === "dark" ? "#111827" : "#ffffff",
        siderBg: resolved === "dark" ? "#0f1623" : "#ffffff",
      },
      Card: {
        // 与 --esp-radius-card(12px)/--esp-shadow-card 同源
        borderRadiusLG: 12,
      },
      Menu: {
        // 暗色侧边栏菜单用 accent 高亮
        itemSelectedColor: ACCENT_PRIMARY[accent],
      },
    },
  };
}

/** 默认主题(light + blue),供非主题上下文兜底。 */
export const DEFAULT_ANTD_THEME: ThemeConfig = buildAntdTheme("light", "blue");

export type { AppTheme };
