// 数字人 Avatar(小程序,Taro,FR-6.1 升级):persona emoji 降级渲染。
// 小程序无 DOM,ui-tokens 的 SVG DigitalHuman 用 web CSS vars 不可直接复用,
// 故按 persona.emoji + persona 主题色展示;emotion 切换文案,active 切说话态。
import { View } from "@tarojs/components";
import { PERSONAS, resolvePersona, type Persona } from "@esp/ui-tokens";
import { t } from "../i18n";

export type Emotion = "happy" | "neutral" | "encouraging" | "sad";
const EMOTION_KEY: Record<Emotion, "miniprogram.avatar.happy" | "miniprogram.avatar.neutral" | "miniprogram.avatar.encouraging" | "miniprogram.avatar.sad"> = {
  happy: "miniprogram.avatar.happy",
  neutral: "miniprogram.avatar.neutral",
  encouraging: "miniprogram.avatar.encouraging",
  sad: "miniprogram.avatar.sad",
};

export interface AvatarProps {
  persona?: Persona;
  emotion?: Emotion;
  active?: boolean;
  text?: string;
}

export default function Avatar({ persona, emotion = "neutral", active = false, text }: AvatarProps) {
  const p = persona ?? resolvePersona(undefined);
  return (
    <View style={{ textAlign: "center", margin: "12rpx 0" }}>
      <View style={{
        display: "inline-flex", alignItems: "center", justifyContent: "center",
        width: "120rpx", height: "120rpx", borderRadius: "60rpx",
        background: p.outfit2, border: `4rpx solid ${p.outfit}`,
      }}>
        <View style={{ fontSize: "72rpx" }}>{active ? "🗣️" : p.emoji}</View>
      </View>
      <View style={{ color: p.outfit, fontSize: "24rpx", marginTop: "6rpx" }}>
        {t(EMOTION_KEY[emotion] || EMOTION_KEY.neutral)}{active ? t("miniprogram.avatar.speaking") : ""}
      </View>
      {text ? (
        <View style={{ background: "#1e293b", border: "1rpx solid #334155", borderRadius: "8rpx", padding: "8rpx", fontSize: "26rpx", marginTop: "6rpx", color: "#e2e8f0" }}>{text}</View>
      ) : null}
    </View>
  );
}

export { PERSONAS, resolvePersona };
