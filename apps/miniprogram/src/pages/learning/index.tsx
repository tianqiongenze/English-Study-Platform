import { useState, useEffect } from "react";
import { View, Text, Input, Button } from "@tarojs/components";
import { api } from "../../api";
import { t } from "../../i18n";

export default function Learning() {
  const [uid, setUid] = useState("1");
  const [out, setOut] = useState("");
  const [placeInfo, setPlaceInfo] = useState("");
  const [placed, setPlaced] = useState(false);
  // FR-2.1 首次进入触发 placement 定级
  useEffect(() => {
    if (placed) return;
    (async () => {
      try {
        const r = await api.placement(Number(uid), [1, 2, 3, 4, 5], 0.4);
        setPlaceInfo(t("miniprogram.learning.placementDone", { n: String(r.initialized) }));
      } catch (e) {
        setPlaceInfo(t("miniprogram.learning.placementSkip", { e: String(e) }));
      } finally { setPlaced(true); }
    })();
  }, [uid, placed]);

  const ev = async () => { try { setOut(JSON.stringify(await api.reportEvent(Number(uid), "answer", "{}"))); } catch (e) { setOut(`${t("miniprogram.speech.error")}:${e}`); } };
  const rv = async () => { try { setOut(JSON.stringify(await api.review(Number(uid)))); } catch (e) { setOut(`${t("miniprogram.speech.error")}:${e}`); } };
  const wk = async () => { try { setOut(JSON.stringify(await api.weak(Number(uid)))); } catch (e) { setOut(`${t("miniprogram.speech.error")}:${e}`); } };
  // FR-2.2 动态难度调整
  const df = async () => {
    try {
      const d = await api.difficulty(3, true);
      setOut(t("miniprogram.learning.nextDifficulty", { d: String(d.next_difficulty), v: String(d.speed_multiplier), s: String(d.scaffolding) }));
    } catch (e) { setOut(`${t("miniprogram.speech.error")}:${e}`); }
  };

  return (
    <View className="page">
      <View className="card">
        <Text style={{ fontSize: "32rpx", fontWeight: 700 }}>{t("miniprogram.learning.adaptive")}</Text>
        <Input value={uid} onInput={(e) => { setUid(e.detail.value); setPlaced(false); }} placeholder={t("miniprogram.learning.userId")} style={{ border: "1px solid var(--esp-color-border, #334155)", padding: "8rpx" }} />
        {placeInfo ? <Text className="hint">💡 {placeInfo}</Text> : null}
        <Button type="primary" onClick={ev}>{t("miniprogram.learning.reportEvent")}</Button>
        <Button onClick={rv}>{t("miniprogram.learning.reviewToday")}</Button>
        <Button onClick={wk}>{t("miniprogram.learning.weakPoints")}</Button>
        <Button onClick={df}>{t("miniprogram.learning.difficulty")}</Button>
        <Text className="log">{out}</Text>
      </View>
    </View>
  );
}