// 轻量 i18n React 封装(零依赖,react client)。Web/H5 共用。
// 用法:
//   <I18nProvider locale={locale} onLocaleChange={setLocale} tr={i18n}>{app}</I18nProvider>
//   const { t } = useI18n();

import * as React from "react";
import type { I18n, Locale, MessageKey } from "./i18n";

const ctx = React.createContext<I18n | null>(null);

export function I18nProvider({
  i18n,
  children,
}: {
  i18n: I18n;
  children: React.ReactNode;
}) {
  const [, force] = React.useReducer((x: number) => x + 1, 0);
  React.useEffect(() => {
    const un = i18n.subscribe(() => force());
    return un;
  }, [i18n]);
  return React.createElement(ctx.Provider, { value: i18n }, children);
}

export function useI18n(): {
  t: (key: MessageKey, vars?: Record<string, string | number>) => string;
  locale: Locale;
  setLocale: (l: Locale) => void;
  toggle: () => void;
} {
  const i18n = React.useContext(ctx);
  if (!i18n) throw new Error("useI18n must be used within <I18nProvider>");
  return {
    t: (key, vars) => i18n.t(key, vars),
    locale: i18n.locale,
    setLocale: (l) => i18n.setLocale(l),
    toggle: () => i18n.toggle(),
  };
}
