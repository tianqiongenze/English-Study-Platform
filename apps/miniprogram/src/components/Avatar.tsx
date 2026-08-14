// 数字人 Avatar(小程序,Taro,SVG,原型 6.6 / FR-6.1)
import { View } from "@tarojs/components";
export type Emotion = "happy" | "neutral" | "encouraging" | "sad";
const FACE: Record<Emotion, { mouth: string; brow: string; color: string; label: string }> = {
  happy:       { mouth: "M30 52 Q45 64 60 52", brow: "M28 34 L40 32 M50 32 L62 34", color: "#22c55e", label: "开心" },
  neutral:     { mouth: "M30 56 L60 56",       brow: "M28 34 L40 34 M50 34 L62 34", color: "#64748b", label: "平静" },
  encouraging: { mouth: "M30 54 Q45 60 60 54", brow: "M28 33 L40 31 M50 31 L62 33", color: "#2D6CDF", label: "鼓励" },
  sad:         { mouth: "M30 60 Q45 50 60 60", brow: "M28 35 L40 33 M50 33 L62 35", color: "#f97316", label: "关切" },
};
export default function Avatar({ emotion = "neutral", active = false, text }: { emotion?: Emotion; active?: boolean; text?: string }) {
  const f = FACE[emotion] || FACE.neutral;
  return (
    <View style={{ textAlign: "center", margin: "12rpx 0" }}>
      <View style={{ fontSize: "60rpx" }}>{active ? "🗣️" : "🧑‍🏫"}</View>
      <View style={{ color: f.color, fontSize: "24rpx" }}>{f.label}{active ? "(说话中)" : ""}</View>
      {text ? <View style={{ background: "#1e293b", border: "1rpx solid #334155", borderRadius: "8rpx", padding: "8rpx", fontSize: "26rpx", marginTop: "6rpx", color: "#e2e8f0" }}>{text}</View> : null}
    </View>
  );
}
