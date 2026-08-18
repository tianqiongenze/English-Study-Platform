// 主题系统:html[data-theme] + html[data-accent] + localStorage 持久化。
// Web/H5 共用;小程序(Taro)不用本文件,自行写 storage 注入样式。

import * as React from "react";

export type AppTheme = "light" | "dark" | "system";
export type Accent = "blue" | "coral" | "forest" | "grape";

export const ACCENTS: Accent[] = ["blue", "coral", "forest", "grape"];
export const THEMES: AppTheme[] = ["light", "dark", "system"];

export const THEME_KEY = "esp_theme";
export const ACCENT_KEY = "esp_accent";

const mql = () =>
  typeof window !== "undefined"
    ? window.matchMedia("(prefers-color-scheme: dark)")
    : null;

export function resolveTheme(theme: AppTheme): "light" | "dark" {
  if (theme === "system") return mql()?.matches ? "dark" : "light";
  return theme;
}

export function applyTheme(theme: AppTheme, accent: Accent) {
  if (typeof document === "undefined") return;
  document.documentElement.dataset.theme = resolveTheme(theme);
  document.documentElement.dataset.accent = accent;
  try {
    localStorage.setItem(THEME_KEY, theme);
    localStorage.setItem(ACCENT_KEY, accent);
  } catch {
    /* private mode */
  }
}

export function readStoredTheme(): { theme: AppTheme; accent: Accent } {
  let theme: AppTheme = "system";
  let accent: Accent = "blue";
  try {
    const t = localStorage.getItem(THEME_KEY) as AppTheme | null;
    if (t && THEMES.includes(t)) theme = t;
    const a = localStorage.getItem(ACCENT_KEY) as Accent | null;
    if (a && ACCENTS.includes(a)) accent = a;
  } catch {
    /* private mode */
  }
  return { theme, accent };
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [, force] = React.useReducer((x: number) => x + 1, 0);
  const { theme, accent } = readStoredTheme();
  const [cur, setCur] = React.useState<{ theme: AppTheme; accent: Accent }>({
    theme,
    accent,
  });

  const setTheme = React.useCallback(
    (t: AppTheme) => {
      setCur((s) => {
        applyTheme(t, s.accent);
        return { ...s, theme: t };
      });
      force();
    },
    []
  );
  const setAccent = React.useCallback(
    (a: Accent) => {
      setCur((s) => {
        applyTheme(s.theme, a);
        return { ...s, accent: a };
      });
      force();
    },
    []
  );

  React.useEffect(() => {
    const m = mql();
    applyTheme(cur.theme, cur.accent);
    if (!m) return;
    const onChange = () => {
      if (cur.theme === "system") applyTheme("system", cur.accent);
    };
    m.addEventListener("change", onChange);
    return () => m.removeEventListener("change", onChange);
  }, [cur]);

  return (
    <ThemeCtx.Provider
      value={{
        theme: cur.theme,
        accent: cur.accent,
        resolved: resolveTheme(cur.theme),
        setTheme,
        setAccent,
      }}
    >
      {children}
    </ThemeCtx.Provider>
  );
}

export const ThemeCtx = React.createContext<{
  theme: AppTheme;
  accent: Accent;
  resolved: "light" | "dark";
  setTheme: (t: AppTheme) => void;
  setAccent: (a: Accent) => void;
} | null>(null);

export function useTheme() {
  const v = React.useContext(ThemeCtx);
  if (!v) throw new Error("useTheme must be used within <ThemeProvider>");
  return v;
}