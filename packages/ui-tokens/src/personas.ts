// 数字人形象(persona)定义:8 预设 + 按 userId 确定性派生(零配置每人不同)。
// Web/H5 用 DigitalHuman.tsx SVG 渲染;小程序用 persona 的 `emoji/colors` 映射到自身组件。

export interface PersonaStyle {
  /** 发型(决定背发 SVG 形状) */
  hairStyle: "bob" | "spiky" | "curls" | "buns" | "ponytail" | "long" | "cap" | "none";
  /** 肤色 */
  skin: string;
  /** 发色 */
  hair: string;
  /** 服饰/边框主色(也作品牌色) */
  outfit: string;
  /** 服饰次色 */
  outfit2: string;
  /** 装饰 */
  accessory: "glasses" | "headphones" | "flower" | "star" | "cap" | "none";
}

export interface Persona extends PersonaStyle {
  id: string;
  name: { en: string; zh: string };
  /** 小程序降级展示的 emoji 人物 */
  emoji: string;
}

export const PERSONAS: Persona[] = [
  { id: "aurora",  name: { en: "Aurora", zh: "蓝宝石学姐" }, emoji: "👩🏻🦱",
    hairStyle: "bob", skin: "#ffe8d6", hair: "#2456c8", outfit: "#2456c8", outfit2: "#7fb0ff", accessory: "glasses" },
  { id: "sunny",   name: { en: "Sunny", zh: "阳光学长" }, emoji: "🧑🏽🦱",
    hairStyle: "spiky", skin: "#f0c8a0", hair: "#f0a020", outfit: "#f07a20", outfit2: "#ffce8a", accessory: "none" },
  { id: "mint",    name: { en: "Minty", zh: "薄荷萌友" }, emoji: "👧🏻",
    hairStyle: "curls", skin: "#ffe3d2", hair: "#2fae7a", outfit: "#2fae7a", outfit2: "#9fe3c0", accessory: "headphones" },
  { id: "violet",  name: { en: "Nova", zh: "紫罗兰AI向导" }, emoji: "🤖",
    hairStyle: "buns", skin: "#f1e6ff", hair: "#7a46d8", outfit: "#7a46d8", outfit2: "#c3a8f5", accessory: "star" },
  { id: "rose",    name: { en: "Rosie", zh: "玫红闺蜜" }, emoji: "👩🏼🦰",
    hairStyle: "ponytail", skin: "#ffe6e0", hair: "#d8486a", outfit: "#d8486a", outfit2: "#ffa8c0", accessory: "flower" },
  { id: "mentor",  name: { en: "Marble", zh: "沉稳导师" }, emoji: "🧑‍🏫",
    hairStyle: "long", skin: "#e8d5c0", hair: "#3a3a4a", outfit: "#3a5a8a", outfit2: "#9ab0d0", accessory: "none" },
  { id: "coral",   name: { en: "Blaze", zh: "火辣教练" }, emoji: "🧑🏿‍🦰",
    hairStyle: "cap", skin: "#d9a878", hair: "#b03030", outfit: "#e0552f", outfit2: "#ffb090", accessory: "cap" },
  { id: "sage",    name: { en: "Sage", zh: "森语精灵" }, emoji: "🧝🏻‍♀️",
    hairStyle: "long", skin: "#fff3e2", hair: "#3a8a5a", outfit: "#3a8a5a", outfit2: "#9ad0ae", accessory: "star" },
];

const PERSONA_IDS = PERSONAS.map((p) => p.id);

function hashSeed(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return h;
}

/** 用户最终形象:优先显式选择,否则按 userId 确定性派生(同一 userId 跨端一致)。 */
export function resolvePersona(storedId: string | undefined, userId?: string | number): Persona {
  if (storedId && PERSONA_IDS.includes(storedId)) {
    return PERSONAS.find((p) => p.id === storedId)!;
  }
  const idx = hashSeed(String(userId ?? "default")) % PERSONAS.length;
  return PERSONAS[idx];
}