// 主题/主题色/语言 切换器(Web/H5 顶栏与设置页复用)。样式全部走 CSS 变量。

import * as React from "react";
import { useTheme, ACCENTS, THEMES, type AppTheme, type Accent } from "./theme";
import { useI18n } from "./i18n-react";
import { SunIcon, MoonIcon, MonitorIcon, GlobeIcon } from "./icons";
import type { Locale } from "./i18n";

const swStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 4,
  background: "var(--esp-color-bg-subtle)",
  border: "1px solid var(--esp-color-border)",
  borderRadius: "var(--esp-radius-round)",
  padding: 3,
};

const btnStyle = (active: boolean): React.CSSProperties => ({
  display: "flex",
  alignItems: "center",
  gap: 4,
  border: "none",
  background: active ? "var(--esp-brand)" : "transparent",
  color: active ? "var(--esp-brand-text-on)" : "var(--esp-color-text-secondary)",
  borderRadius: "var(--esp-radius-round)",
  padding: "4px 10px",
  fontSize: "var(--esp-font-size-sm)",
  cursor: "pointer",
  transition: "all var(--esp-motion-duration-fast) var(--esp-motion-ease)",
});

export function ThemeSwitcher({ compact = false }: { compact?: boolean }) {
  const { theme, setTheme } = useTheme();
  const i18n = useI18n();
  if (compact) {
    // 紧凑:图标循环亮->暗->系统
    const order: AppTheme[] = ["light", "dark", "system"];
    const next =
      order[(order.indexOf(theme) + 1 + order.length) % order.length] ?? "system";
    const icon = theme === "light" ? <MoonIcon size={16} /> : theme === "dark" ? <MonitorIcon size={16} /> : <SunIcon size={16} />;
    return (
      <button
        style={btnStyle(false) as React.CSSProperties}
        title={i18n.t("theme.switch")}
        aria-label={i18n.t("theme.switch")}
        onClick={() => setTheme(next)}
      >
        {icon}
      </button>
    );
  }
  return (
    <div style={swStyle} role="group" aria-label={i18n.t("theme.switch")}>
      {THEMES.map((t) => (
        <button
          key={t}
          style={btnStyle(t === theme)}
          onClick={() => setTheme(t)}
          title={i18n.t(`theme.${t}` as "theme.light")}
        >
          {t === "light" ? <SunIcon size={14} /> : t === "dark" ? <MoonIcon size={14} /> : <MonitorIcon size={14} />}
          {i18n.t(`theme.${t}` as "theme.light")}
        </button>
      ))}
    </div>
  );
}

export function AccentPicker({ compact = false }: { compact?: boolean }) {
  const { accent, setAccent } = useTheme();
  const i18n = useI18n();
  const colors: Record<Accent, string> = {
    blue: "var(--esp-color-primary)",
    coral: "#ff6b4a",
    forest: "#22a06b",
    grape: "#8b5cf6",
  };
  if (compact) {
    const order: Accent[] = ACCENTS;
    const next = order[(order.indexOf(accent) + 1) % order.length] ?? "blue";
    return (
      <button
        style={{ ...btnStyle(false), cursor: "pointer" }}
        title={i18n.t("theme.accent")}
        aria-label={i18n.t("theme.accent")}
        onClick={() => setAccent(next)}
      >
        <span
          style={{
            display: "inline-block",
            width: 14,
            height: 14,
            borderRadius: 99,
            background: colors[accent],
          }}
        />
      </button>
    );
  }
  return (
    <div style={swStyle} role="group" aria-label={i18n.t("theme.accent")}>
      {ACCENTS.map((a) => (
        <button
          key={a}
          style={btnStyle(a === accent)}
          onClick={() => setAccent(a)}
          title={i18n.t(`accent.${a}` as "accent.blue")}
          aria-label={i18n.t(`accent.${a}` as "accent.blue")}
        >
          <span
            style={{
              display: "inline-block",
              width: 14,
              height: 14,
              borderRadius: 99,
              background: colors[a],
            }}
          />
          {i18n.t(`accent.${a}` as "accent.blue")}
        </button>
      ))}
    </div>
  );
}

export function LangSwitcher({ compact = false }: { compact?: boolean }) {
  const { locale, setLocale } = useI18n();
  const langs: Locale[] = ["zh", "en"];
  if (compact) {
    const next: Locale = locale === "zh" ? "en" : "zh";
    return (
      <button
        style={btnStyle(false) as React.CSSProperties}
        title={i18nCompat().t("lang.switch")}
        aria-label="Language"
        onClick={() => setLocale(next)}
      >
        <GlobeIcon size={16} />
        <span style={{ fontSize: "var(--esp-font-size-xs)" }}>{next.toUpperCase()}</span>
      </button>
    );
  }
  return (
    <div style={swStyle} role="group" aria-label="Language">
      {langs.map((l) => (
        <button
          key={l}
          style={btnStyle(l === locale)}
          onClick={() => setLocale(l)}
        >
          {l === "zh" ? "中文" : "EN"}
        </button>
      ))}
    </div>
  );
}

// compact 形态不依赖 useI18n 覆写文案,取当前语言近似(t 在模块内不可用)
function i18nCompat() {
  return { t: (k: string) => (k === "lang.switch" ? "" : "") };
}