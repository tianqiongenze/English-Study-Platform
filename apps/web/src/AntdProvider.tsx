// antd v6 上下文 Provider —— 单点注入,全局生效。
// 职责:
//   1. ConfigProvider:主题(亮/暗 + accent)随 @esp/ui-tokens ThemeProvider 联动;locale 随 I18n 联动。
//   2. App(antd App 上下文):提供 message/notification/modal 静态方法(useApp hook),
//      使各页面用 antd 的全局提示替代手写 toast。
// 放置:main.tsx 中 ThemeProvider > I18nProvider > AntdProvider > ErrorBoundary > App。
// antd v6 原生支持 React 19,无需 v5-patch-react-19。

import { useMemo, type ReactNode } from "react";
import { App as AntdApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import enUS from "antd/locale/en_US";
import arEG from "antd/locale/ar_EG";
import frFR from "antd/locale/fr_FR";
import ruRU from "antd/locale/ru_RU";
import esES from "antd/locale/es_ES";
import deDE from "antd/locale/de_DE";
import heIL from "antd/locale/he_IL";
import elGR from "antd/locale/el_GR";
import { useTheme, useI18n, RTL_LOCALES, type Locale } from "@esp/ui-tokens";
import { buildAntdTheme } from "./antdTheme";

// i18n locale -> antd locale 包(ar=ar_EG, he=he_IL, el=el_GR 等映射)。
const ANTD_LOCALE: Record<Locale, typeof zhCN> = {
  zh: zhCN,
  en: enUS,
  ar: arEG,
  fr: frFR,
  ru: ruRU,
  es: esES,
  de: deDE,
  he: heIL,
  el: elGR,
};

export default function AntdProvider({ children }: { children: ReactNode }) {
  const { resolved, accent } = useTheme();
  const { locale } = useI18n();

  // 主题随 resolved/accent 变化重建(亮/暗切 + accent 切换 antd 同步)。
  const theme = useMemo(
    () => buildAntdTheme(resolved, accent),
    [resolved, accent]
  );

  // antd locale 跟随 i18n;RTL 语言(ar/he)设 direction=rtl。
  const antdLocale = ANTD_LOCALE[locale];
  const direction = RTL_LOCALES.has(locale) ? "rtl" : "ltr";

  return (
    <ConfigProvider theme={theme} locale={antdLocale} direction={direction}>
      <AntdApp>{children}</AntdApp>
    </ConfigProvider>
  );
}
