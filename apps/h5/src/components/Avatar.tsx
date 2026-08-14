// 数字人 Avatar(H5,SVG,原型 6.6 / FR-6.1)
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
    <div style={{ textAlign: "center", margin: "8px 0" }}>
      <svg viewBox="0 0 90 80" width="90" height="80" role="img" aria-label={`数字人(${f.label})`}>
        <circle cx="45" cy="40" r="34" fill="#eef2ff" stroke={f.color} strokeWidth="3" />
        <circle cx="35" cy="40" r="3" fill={f.color} />
        <circle cx="55" cy="40" r="3" fill={f.color} />
        <path d={f.brow} stroke={f.color} strokeWidth="2" fill="none" strokeLinecap="round" />
        <path d={f.mouth} stroke={f.color} strokeWidth="2.5" fill="none" strokeLinecap="round"
          style={{ transformOrigin: "45px 56px", animation: active ? "avatar-talk 0.35s infinite alternate" : "none" }} />
      </svg>
      {text && <div style={{ background: "#1e293b", border: "1px solid #334155", borderRadius: 8, padding: "6px 10px", fontSize: "0.8rem", marginTop: 4 }}>{text}</div>}
    </div>
  );
}
