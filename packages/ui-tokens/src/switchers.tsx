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

// 9 语言短码与原生名称(用于 LangSwitcher 展示,不随 i18n 切换而变)。
const LOCALE_LABELS: Record<Locale, string> = {
  zh: "中文",
  en: "EN",
  ar: "العربية",
  fr: "FR",
  ru: "RU",
  es: "ES",
  de: "DE",
  he: "עברית",
  el: "ΕΛ",
};

// compact 循环顺序(覆盖全部 9 语言)。
const LOCALE_ORDER: Locale[] = ["zh", "en", "ar", "fr", "ru", "es", "de", "he", "el"];

export function LangSwitcher({ compact = false }: { compact?: boolean }) {
  const { locale, setLocale } = useI18n();
  const i18n = useI18n();
  if (compact) {
    const idx = LOCALE_ORDER.indexOf(locale);
    const next: Locale = LOCALE_ORDER[(idx + 1) % LOCALE_ORDER.length] ?? "en";
    return (
      <button
        style={btnStyle(false) as React.CSSProperties}
        title={i18n.t("lang.switch")}
        aria-label={i18n.t("lang.switch")}
        onClick={() => setLocale(next)}
      >
        <GlobeIcon size={16} />
        <span style={{ fontSize: "var(--esp-font-size-xs)" }}>
          {LOCALE_LABELS[locale]}
        </span>
      </button>
    );
  }
  // 非紧凑:原生 <select> 下拉(9 语言用 chip 行过宽),零依赖、可访问性好。
  return (
    <label
      style={{
        display: "flex",
        alignItems: "center",
        gap: 4,
        background: "var(--esp-color-bg-subtle)",
        border: "1px solid var(--esp-color-border)",
        borderRadius: "var(--esp-radius-round)",
        padding: "4px 10px",
        fontSize: "var(--esp-font-size-sm)",
        color: "var(--esp-color-text-secondary)",
        cursor: "pointer",
      }}
    >
      <GlobeIcon size={14} />
      <span style={{ whiteSpace: "nowrap" }}>{i18n.t("lang.switch")}</span>
      <select
        value={locale}
        onChange={(e) => setLocale(e.target.value as Locale)}
        style={{
          border: "none",
          background: "transparent",
          color: "var(--esp-color-text)",
          fontSize: "var(--esp-font-size-sm)",
          cursor: "pointer",
          outline: "none",
        }}
        aria-label={i18n.t("lang.switch")}
      >
        {LOCALE_ORDER.map((l) => (
          <option key={l} value={l}>
            {LOCALE_LABELS[l]}
          </option>
        ))}
      </select>
    </label>
  );
}