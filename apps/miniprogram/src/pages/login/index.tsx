import { useState } from "react";
import { View, Text, Button, Image } from "@tarojs/components";
import Taro from "@tarojs/taro";
import { api, setToken, setUser } from "../../api";
import { t, toggleLocale, i18n, syncNavTitle } from "../../i18n";
import { resolveTheme, setMpTheme } from "../../theme";

// 登录 hero 图(Unsplash 直链,可离线 fallback 到渐变底)
const HERO_IMG =
  "https://images.unsplash.com/photo-1434030216411-0b793f4b4173?auto=format&fit=crop&w=750&q=60";

export default function Login() {
  const [loading, setLoading] = useState(false);
  const [imgFailed, setImgFailed] = useState(false);
  const [locale, setLoc] = useState(i18n.locale);
  const [theme, setThemeUI] = useState(resolveTheme());

  async function wxLogin() {
    setLoading(true);
    try {
      const { code } = await Taro.login();
      const res = await api.loginByCode(code);
      setToken(res.token); setUser(res.user);
      Taro.reLaunch({ url: "/pages/index/index" });
    } catch (e) {
      Taro.showToast({ title: t("miniprogram.login.failed"), icon: "error" });
    } finally { setLoading(false); }
  }

  // 中/英切换:写 esp_locale + 联动导航标题
  function switchLocale() {
    const next = toggleLocale();
    setLoc(next);
    syncNavTitle();
  }

  // 亮/暗切换:写 esp_theme + 系统 chrome 联动(app-root 订阅刷新暗色变量)
  function switchTheme() {
    const next = theme === "light" ? "dark" : "light";
    setMpTheme(next);
    setThemeUI(next);
  }

  return (
    <View className="page">
      <View className="card login-hero">
        {imgFailed ? (
          <View className="login-hero__fallback">
            <Text className="login-hero__fallbackText">{t("app.tagline")}</Text>
          </View>
        ) : (
          <Image
            className="login-hero__img"
            src={HERO_IMG}
            mode="aspectFill"
            onError={() => setImgFailed(true)}
          />
        )}
        <View className="login-hero__body">
          <Text className="login-hero__title">{t("app.title")}</Text>
          <Text className="login-hero__sub">{t("miniprogram.login.wechatHint")}</Text>
          <Button type="primary" loading={loading} onClick={wxLogin}>
            {t("miniprogram.login.wechat")}
          </Button>
        </View>
      </View>

      <View className="login-tools">
        <Text className="tbtn" onClick={switchLocale}>
          {locale === "zh" ? t("lang.en") : t("lang.zh")}
        </Text>
        <Text className="tbtn" onClick={switchTheme}>
          {theme === "light" ? t("theme.dark") : t("theme.light")}
        </Text>
      </View>
    </View>
  );
}