import { useState } from "react";
import { View, Text, Button } from "@tarojs/components";
import Taro from "@tarojs/taro";
import { api, setToken, setUser } from "../../api";

export default function Login() {
  const [loading, setLoading] = useState(false);
  async function wxLogin() {
    setLoading(true);
    try {
      const { code } = await Taro.login();
      const res = await api.loginByCode(code);
      setToken(res.token); setUser(res.user);
      Taro.reLaunch({ url: "/pages/index/index" });
    } catch (e) {
      Taro.showToast({ title: "登录失败", icon: "error" });
    } finally { setLoading(false); }
  }
  return (
    <View className="page">
      <Text>ESP · Home-Tutor</Text>
      <Text className="hint">使用微信账号一键登录</Text>
      <Button type="primary" loading={loading} onClick={wxLogin}>微信一键登录</Button>
    </View>
  );
}
