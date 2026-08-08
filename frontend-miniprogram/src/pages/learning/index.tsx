import { useState, useEffect } from "react";
import { View, Text, Input, Button } from "@tarojs/components";
import { api } from "../../api";

export default function Learning() {
  const [uid, setUid] = useState("1");
  const [out, setOut] = useState("");
  const [placeInfo, setPlaceInfo] = useState("");
  const [placed, setPlaced] = useState(false);
  // FR-2.1 首次进入触发 placement 定级
  useEffect(() => {
    if (placed) return;
    (async () => {
      try { const r = await api.placement(Number(uid), [1, 2, 3, 4, 5], 0.4); setPlaceInfo(`能力定级完成:初始化 ${r.initialized} 个知识点基线`); }
      catch (e) { setPlaceInfo(`定级跳过:${e}`); } finally { setPlaced(true); }
    })();
  }, [uid, placed]);

  const ev = async () => { try { setOut(JSON.stringify(await api.reportEvent(Number(uid)))); } catch (e) { setOut(`错误:${e}`); } };
  const rv = async () => { try { setOut(JSON.stringify(await api.review(Number(uid)))); } catch (e) { setOut(`错误:${e}`); } };
  const wk = async () => { try { setOut(JSON.stringify(await api.weak(Number(uid)))); } catch (e) { setOut(`错误:${e}`); } };
  // FR-2.2 动态难度调整
  const df = async () => { try { const d = await api.difficulty(3, true); setOut(`下一难度:${d.next_difficulty} 倍速:${d.speed_multiplier} 脚手架:${d.scaffolding}`); } catch (e) { setOut(`错误:${e}`); } };

  return (
    <View className="page">
      <Text>自适应学习</Text>
      <Input value={uid} onInput={(e) => { setUid(e.detail.value); setPlaced(false); }} placeholder="用户 ID" style={{ border: "1px solid #334", padding: "8rpx" }} />
      {placeInfo ? <Text className="hint">💡 {placeInfo}</Text> : null}
      <Button type="primary" onClick={ev}>上报学情</Button>
      <Button onClick={rv}>今日复习</Button>
      <Button onClick={wk}>薄弱点</Button>
      <Button onClick={df}>难度调整</Button>
      <Text className="log">{out}</Text>
    </View>
  );
}
