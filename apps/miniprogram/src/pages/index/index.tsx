import { View, Text, Button } from "@tarojs/components";
import Taro, { useDidShow } from "@tarojs/taro";
import { useState } from "react";
import { getUser, logout, api } from "../../api";
import { PERSONAS, resolvePersona, type Persona } from "@esp/ui-tokens";
import { t } from "../../i18n";

interface Sess { id: number; user_id: number; scenario: string; status: string; started_at: string; ended_at: string | null }

// 小程序"我的"页:用户信息 + 数字人形象选择 + 会话历史(会话22-3 C 批次三端贯通 + 数字人 persona)。
export default function Index() {
  const user = getUser();
  const [history, setHistory] = useState<Sess[]>([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");
  // 数字人 persona(FR-6.1 per-user 形象,跨设备持久化)
  const [personaId, setPersonaId] = useState<string>("");
  const [persona, setPersona] = useState<Persona>(() => resolvePersona(undefined, user?.id));
  const [saving, setSaving] = useState(false);
  const [savedMsg, setSavedMsg] = useState(false);

  useDidShow(() => {
    if (!user) return;
    setLoading(true);
    api.sessionHistory()
      .then((r) => { setHistory(r); setLoading(false); })
      .catch((e) => { setErr(String(e.message || e)); setLoading(false); });
    api.getDigitalHuman()
      .then((r) => { setPersonaId(r.persona_id); setPersona(resolvePersona(r.persona_id, user?.id)); })
      .catch(() => { const def = resolvePersona(undefined, user?.id); setPersonaId(def.id); });
  });

  function pick(id: string) {
    setPersonaId(id);
    setPersona(resolvePersona(id, user?.id));
  }
  async function savePersona() {
    setSaving(true);
    try {
      await api.setDigitalHuman(personaId);
      setSavedMsg(true);
      window.setTimeout?.(() => setSavedMsg(false), 2000);
    } catch { /* 沿用本地值 */ }
    setSaving(false);
  }
  function resetPersona() {
    const def = resolvePersona(undefined, user?.id);
    setPersonaId(def.id);
    setPersona(def);
  }

  if (!user) {
    return (
      <View className="page">
        <View className="card">
          <Text className="login-hero__title">{t("miniprogram.index.welcome", { app: t("app.title") })}</Text>
          <Button type="primary" onClick={() => Taro.navigateTo({ url: "/pages/login/index" })}>
            {t("auth.login")}
          </Button>
        </View>
      </View>
    );
  }
  return (
    <View className="page">
      <View className="card">
        <Text>{t("common.hi")},{user.name}</Text>
        <Button type="primary" onClick={() => Taro.navigateTo({ url: "/pages/speech/index" })}>{t("nav.speaking")}</Button>
        <Button onClick={() => Taro.navigateTo({ url: "/pages/writing/index" })}>{t("nav.writing")}</Button>
        <Button onClick={() => Taro.navigateTo({ url: "/pages/learning/index" })}>{t("nav.learning")}</Button>
      </View>

      {/* 数字人 persona 选择(FR-6.1):emoji 降级卡片网格 + 保存/重置 */}
      <View className="card" style={{ marginTop: "16rpx" }}>
        <Text style={{ fontWeight: "bold", display: "block", marginBottom: "8rpx" }}>{t("persona.title")}</Text>
        <Text className="hint" style={{ display: "block", marginBottom: "12rpx" }}>{t("persona.hint")}</Text>
        <View style={{ display: "flex", alignItems: "center", gap: "16rpx", marginBottom: "12rpx" }}>
          <View style={{
            display: "inline-flex", alignItems: "center", justifyContent: "center",
            width: "96rpx", height: "96rpx", borderRadius: "48rpx",
            background: persona.outfit2, border: `4rpx solid ${persona.outfit}`,
          }}>
            <Text style={{ fontSize: "56rpx" }}>{persona.emoji}</Text>
          </View>
          <View>
            <Text style={{ fontWeight: "bold" }}>{persona.name.zh}</Text>
            <Text className="hint" style={{ display: "block" }}>{t("persona.current")}</Text>
          </View>
        </View>
        <View style={{ display: "flex", flexWrap: "wrap", gap: "12rpx" }}>
          {PERSONAS.map((p) => (
            <View
              key={p.id}
              onClick={() => pick(p.id)}
              style={{
                display: "inline-flex", alignItems: "center", justifyContent: "center",
                width: "88rpx", height: "88rpx", borderRadius: "44rpx",
                background: p.outfit2, border: `4rpx solid ${personaId === p.id ? p.outfit : "transparent"}`,
              }}
            >
              <Text style={{ fontSize: "48rpx" }}>{p.emoji}</Text>
            </View>
          ))}
        </View>
        <View style={{ display: "flex", gap: "12rpx", marginTop: "16rpx", alignItems: "center" }}>
          <Button type="primary" size="mini" loading={saving} onClick={savePersona}>{t("persona.save")}</Button>
          <Button size="mini" onClick={resetPersona}>{t("persona.reset")}</Button>
          {savedMsg && <Text style={{ color: "seagreen" }}>{t("persona.saved")}</Text>}
        </View>
      </View>

      <Text style={{ marginTop: 16, fontWeight: "bold" }}>{t("miniprogram.index.sessionHistory")}</Text>
      {loading && <Text>{t("action.loading")}</Text>}
      {err && <Text style={{ color: "crimson" }}>{t("miniprogram.index.loadFailed", { err })}</Text>}
      {!loading && !err && history.length === 0 && <Text className="hint">{t("empty.records")}</Text>}
      {history.map((s) => (
        <View key={s.id} className="card" style={{ padding: "16rpx" }}>
          <Text>{s.scenario}</Text>
          <Text style={{ fontSize: "12px", color: "#999", display: "block" }}>
            {t("miniprogram.index.status")}:{s.status} · {t("miniprogram.index.startedAt")}:{s.started_at}
          </Text>
          <Text style={{ fontSize: "12px", color: "#999" }}>
            {t("miniprogram.index.endedAt")}:{s.ended_at ?? t("miniprogram.index.ongoing")}
          </Text>
        </View>
      ))}

      <Button onClick={() => { logout(); Taro.reLaunch({ url: "/pages/index/index" }); }}>{t("auth.logout")}</Button>
    </View>
  );
}