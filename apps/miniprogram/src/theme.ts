// 小程序主题系统:esp_theme(storage)+ 系统深色跟随 + 导航/页面 chrome 联动。
// 与 Web/H5 的 localStorage['esp_theme'] 同 key 同语义(light | dark | system)。
// 主题变量值在 app.scss(page / .app-root.theme-dark 两段),此处只管偏好与系统 chrome。
import Taro from "@tarojs/taro";

export type MpTheme = "light" | "dark";

// 亮/暗 品牌导航色(与 app.config.ts window.navigationBarBackgroundColor 亮默认一致)
const NAV_BG: Record<MpTheme, string> = { light: "#1677ff", dark: "#1668dc" };
const PAGE_BG: Record<MpTheme, string> = { light: "#f5f5f5", dark: "#000000" };

type Listener = (t: MpTheme) => void;
const listeners = new Set<Listener>();

/** 解析当前主题:显式 light/dark 优先;其余(system/未设置)跟随系统深浅色。 */
export function resolveTheme(): MpTheme {
  let stored: unknown = "";
  try {
    stored = Taro.getStorageSync("esp_theme");
  } catch {
    /* 低基础库/私有模式忽略 */
  }
  if (stored === "light" || stored === "dark") return stored;
  try {
    const info = Taro.getSystemInfoSync();
    return info.theme === "dark" ? "dark" : "light";
  } catch {
    return "light";
  }
}

/** 联动导航栏/页面底色(品牌色导航:暗 #1668dc / 亮 #1677ff)。 */
export function applyChrome(t: MpTheme): void {
  try {
    Taro.setNavigationBarColor({ frontColor: "#ffffff", backgroundColor: NAV_BG[t] });
    Taro.setBackgroundColor({ backgroundColor: PAGE_BG[t] });
  } catch {
    /* 低基础库忽略 */
  }
}

/** 订阅主题变化(App 根据此刷新 data 标记)。返回退订函数。 */
export function subscribeTheme(fn: Listener): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

/** 切换主题:持久化 + 联动系统 chrome + 通知订阅者。 */
export function setMpTheme(next: MpTheme): void {
  try {
    Taro.setStorageSync("esp_theme", next);
  } catch {
    /* 忽略 */
  }
  applyChrome(next);
  listeners.forEach((fn) => fn(next));
}