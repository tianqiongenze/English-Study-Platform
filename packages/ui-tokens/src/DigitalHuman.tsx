// 数字人渲染(FR-6.1 升级):同一个 SVG 骨架按 persona 参数化(发型/肤色/发色/服饰/饰品/品牌色)。
// emotion 四态 happy/neutral/encouraging/sad 由 conversation-go 经 WS ai_reply.emotion 驱动。
// 用法: <DigitalHuman persona={resolvePersona(...)} emotion={emotion} active={speaking} text={bubble} />
// 选择器: <DigitalHumanPicker locale={locale} value={id} onChange={setId} />

import * as React from "react";
import type { Locale } from "./i18n";
import type { Persona } from "./personas";
import { PERSONAS, resolvePersona } from "./personas";

export type HumanEmotion = "happy" | "neutral" | "encouraging" | "sad";
export type { Persona };

const MOUTH: Record<HumanEmotion, string> = {
  happy: "M30 52 Q45 64 60 52",
  neutral: "M30 56 L60 56",
  encouraging: "M30 54 Q45 60 60 54",
  sad: "M30 60 Q45 50 60 60",
};
const BROW: Record<HumanEmotion, [string, string]> = {
  happy: ["M28 34 L40 32", "M50 32 L62 34"],
  encouraging: ["M28 33 L40 31", "M50 31 L62 33"],
  neutral: ["M28 34 L40 34", "M50 34 L62 34"],
  sad: ["M28 35 L40 33", "M50 33 L62 35"],
};

/** 背发(脸后方) */
function BackHair({ p }: { p: Persona }) {
  switch (p.hairStyle) {
    case "bob":
      return <path d="M15 42 C15 12 75 12 75 42 L75 50 C75 62 63 67 56 62 C56 50 42 50 32 58 C24 62 15 60 15 50 Z" fill={p.hair} />;
    case "long":
      return <path d="M17 44 C15 16 75 16 73 44 L74 64 C70 62 64 60 62 58 C60 50 46 50 40 58 C36 64 28 66 20 64 Z" fill={p.hair} />;
    case "ponytail":
      return <path d="M15 42 C15 12 75 12 75 42 L75 48 C75 56 67 60 62 58 L62 60 C62 70 56 74 48 73 C52 66 50 58 44 54 C40 58 34 64 32 74 C26 72 22 66 22 58 Z" fill={p.hair} />;
    case "curls":
      return <><circle cx="20" cy="26" r="9" fill={p.hair} /><circle cx="45" cy="16" r="10" fill={p.hair} /><circle cx="70" cy="26" r="9" fill={p.hair} /><circle cx="32" cy="18" r="7" fill={p.hair} /><circle cx="58" cy="18" r="7" fill={p.hair} /></>;
    case "buns":
      return <><circle cx="32" cy="18" r="10" fill={p.hair} /><circle cx="58" cy="18" r="10" fill={p.hair} /><path d="M18 40 C18 18 45 16 45 16 C45 16 72 18 72 40 Z" fill={p.hair} /></>;
    case "spiky":
      return <path d="M16 40 L18 22 L28 30 L30 14 L42 24 L45 8 L58 22 L62 12 L74 26 L74 42 Z" fill={p.hair} />;
    case "cap":
      return <path d="M16 36 C16 16 74 16 74 36 Z" fill={p.hair} />;
    default:
      return null;
  }
}

/** 刘海(脸前方) */
function Bangs({ p }: { p: Persona }) {
  switch (p.hairStyle) {
    case "bob":
    case "long":
      return <path d="M20 34 C24 22 40 20 45 24 C50 20 66 22 70 34 C64 26 56 26 45 27 C34 26 26 26 20 34 Z" fill={p.hair} />;
    default:
      return null;
  }
}

function Accessories({ p }: { p: Persona }) {
  switch (p.accessory) {
    case "glasses":
      return (
        <>
          <rect x="29" y="37" width="11" height="9" rx="2" fill="none" stroke={p.outfit2} strokeWidth="2" />
          <rect x="50" y="37" width="11" height="9" rx="2" fill="none" stroke={p.outfit2} strokeWidth="2" />
          <path d="M40 41 L50 41" stroke={p.outfit2} strokeWidth="2" />
        </>
      );
    case "headphones":
      return (
        <>
          <path d="M20 38 A25 25 0 0 1 70 38" fill="none" stroke={p.outfit2} strokeWidth="4" />
          <rect x="15" y="34" width="9" height="14" rx="4" fill={p.outfit2} />
          <rect x="66" y="34" width="9" height="14" rx="4" fill={p.outfit2} />
        </>
      );
    case "flower":
      return (
        <g transform="translate(68 18)">
          {[0, 72, 144, 216, 288].map((a) => <circle key={a} cx="0" cy="-5" r="4" fill={p.outfit2} transform={`rotate(${a})`} />)}
          <circle r="3.4" fill="#ffd94a" />
        </g>
      );
    case "star":
      return <path d="M70 12 L72.5 18 L79 20 L72.5 22 L70 28 L67.5 22 L61 20 L67.5 18 Z" fill={p.outfit2} opacity="0.9" />;
    case "cap":
      return <path d="M16 34 L74 34 L70 30 L20 30 Z" fill={p.outfit2} rx="3" />;
    default:
      return null;
  }
}

export interface DigitalHumanProps {
  persona: Persona;
  emotion?: HumanEmotion;
  active?: boolean;
  text?: string;
  size?: number;
}

export default function DigitalHuman({ persona, emotion = "neutral", active = false, text, size = 120 }: DigitalHumanProps) {
  const brow = BROW[emotion] || BROW.neutral;
  return (
    <div className={`esp-dhuman ${active ? "esp-dhuman--active" : ""}`} aria-label={persona.name.zh} style={{ position: "relative", display: "inline-block" }}>
      <svg viewBox="0 0 90 80" width={size} height={Math.round(size * 80 / 90)} role="img" aria-label={persona.name.zh}>
        {/* 服饰光环(frame) */}
        <circle cx="45" cy="40" r="38" fill="none" stroke={persona.outfit} strokeWidth="2" opacity="0.35" />
        <BackHair p={persona} />
        {/* 脸 */}
        <circle cx="45" cy="40" r="28" fill={persona.skin} stroke={persona.outfit2} strokeWidth="2" />
        <Bangs p={persona} />
        {/* 眼睛 */}
        <circle cx="35" cy="39" r="3.4" fill={persona.outfit} />
        <circle cx="55" cy="39" r="3.4" fill={persona.outfit} />
        <circle cx="36.4" cy="37.6" r="1.1" fill="#fff" />
        <circle cx="56.4" cy="37.6" r="1.1" fill="#fff" />
        {/* 眉毛 + 嘴 */}
        <path d={brow[0]} stroke={persona.outfit} strokeWidth="2.2" fill="none" strokeLinecap="round" />
        <path d={brow[1]} stroke={persona.outfit} strokeWidth="2.2" fill="none" strokeLinecap="round" />
        <path d={MOUTH[emotion] || MOUTH.neutral} stroke={persona.outfit} strokeWidth="2.6" fill="none" strokeLinecap="round"
          style={{ transformOrigin: "45px 56px", animation: active ? "esp-talk 0.35s infinite alternate" : "none" }} />
        <Accessories p={persona} />
      </svg>
      {text && <div className="esp-dhuman__bubble" style={{ position: "absolute", top: -6, right: -8, maxWidth: 180, background: "var(--esp-color-bg-elevated)", border: "1px solid var(--esp-color-border)", color: "var(--esp-color-text)", borderRadius: "var(--esp-radius-lg)", padding: "6px 10px", fontSize: "var(--esp-font-size-sm)", boxShadow: "var(--esp-shadow-1)" }}>{text}</div>}
      <style>{`@keyframes esp-talk { from { transform: scaleY(1);} to { transform: scaleY(0.55);} }`}</style>
    </div>
  );
}

/** 形象选择器(卡片网格) */
export function DigitalHumanPicker({
  locale,
  value,
  onChange,
}: {
  locale: Locale;
  value: string;
  onChange: (id: string) => void;
}) {
  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(88px, 1fr))", gap: "var(--esp-space-2)" }}>
      {PERSONAS.map((p) => {
        const active = p.id === value;
        return (
          <button
            key={p.id}
            type="button"
            onClick={() => onChange(p.id)}
            aria-pressed={active}
            style={{
              display: "flex", flexDirection: "column", alignItems: "center", gap: 4, cursor: "pointer",
              border: active ? "2px solid var(--esp-brand)" : "1px solid var(--esp-color-border)",
              background: active ? "var(--esp-brand-soft)" : "var(--esp-color-bg-subtle)",
              borderRadius: "var(--esp-radius-card)", padding: "var(--esp-space-2) var(--esp-space-1)",
              color: "var(--esp-color-text)", fontSize: "var(--esp-font-size-sm)",
            }}
          >
            <span style={{ fontSize: 22 }} aria-hidden>{p.emoji}</span>
            <span>{locale === "zh" ? p.name.zh : p.name.en}</span>
          </button>
        );
      })}
    </div>
  );
}

export { PERSONAS, resolvePersona };