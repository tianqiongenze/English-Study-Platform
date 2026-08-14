import { View, Text, Button } from "@tarojs/components";
import Taro, { useDidShow } from "@tarojs/taro";
import { useState } from "react";
import { getUser, logout, api } from "../../api";

interface Sess { id: number; user_id: number; scenario: string; status: string; started_at: string; ended_at: string | null }

// 小程序"我的"页:用户信息 + 会话历史(会话22-3 C 批次三端贯通)。
export default function Index() {
  const user = getUser();
  const [history, setHistory] = useState<Sess[]>([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");

  useDidShow(() => {
    if (!user) return;
    setLoading(true);
    api.sessionHistory()
      .then((r) => { setHistory(r); setLoading(false); })
      .catch((e) => { setErr(String(e.message || e)); setLoading(false); });
  });

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

      <Text style={{ marginTop: 16, fontWeight: "bold" }}>会话历史</Text>
      {loading && <Text>加载中…</Text>}
      {err && <Text style={{ color: "crimson" }}>加载失败:{err}</Text>}
      {!loading && !err && history.length === 0 && <Text style={{ color: "#888" }}>暂无会话记录。</Text>}
      {history.map((s) => (
        <View key={s.id} style={{ padding: "8px 0", borderBottom: "1px solid #eee" }}>
          <Text>{s.scenario}</Text>
          <Text style={{ fontSize: "12px", color: "#999", display: "block" }}>状态:{s.status} · 开始:{s.started_at}</Text>
          <Text style={{ fontSize: "12px", color: "#999" }}>结束:{s.ended_at ?? "进行中"}</Text>
        </View>
      ))}

      <Button onClick={() => { logout(); Taro.reLaunch({ url: "/pages/index/index" }); }}>退出登录</Button>
    </View>
  );
}
