import { View, Text, Button } from "@tarojs/components";
import Taro from "@tarojs/taro";
import { getUser, logout } from "../../api";

export default function Index() {
  const user = getUser();
  if (!user) {
    return (
      <View className="page">
        <Text>欢迎来到 Home-Tutor</Text>
        <Button type="primary" onClick={() => Taro.navigateTo({ url: "/pages/login/index" })}>微信登录</Button>
      </View>
    );
  }
  return (
    <View className="page">
      <Text>你好,{user.name}</Text>
      <Button type="primary" onClick={() => Taro.navigateTo({ url: "/pages/speech/index" })}>口语陪练</Button>
      <Button onClick={() => Taro.navigateTo({ url: "/pages/writing/index" })}>写作批改</Button>
      <Button onClick={() => Taro.navigateTo({ url: "/pages/learning/index" })}>自适应学习</Button>
      <Button onClick={() => { logout(); Taro.reLaunch({ url: "/pages/index/index" }); }}>退出登录</Button>
    </View>
  );
}
