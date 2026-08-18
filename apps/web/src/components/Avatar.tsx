// 数字人 Avatar 组件(FR-6.1 升级):统一走 @esp/ui-tokens DigitalHuman,
// 按 persona(发型/肤色/服饰/饰品)+ emotion 驱动表情,active 时嘴部说话动画。
// persona 由调用方经 resolvePersona(storedId, userId) 解析;无 persona 传 undefined 走默认。
import { DigitalHuman, resolvePersona, type Persona, type HumanEmotion } from "@esp/ui-tokens";

export type Emotion = HumanEmotion;

export interface AvatarProps {
  persona?: Persona;
  emotion?: Emotion;
  active?: boolean;
  text?: string;
  size?: number;
}

export default function Avatar({ persona, emotion = "neutral", active = false, text, size = 120 }: AvatarProps) {
  const p = persona ?? resolvePersona(undefined);
  return <DigitalHuman persona={p} emotion={emotion} active={active} text={text} size={size} />;
}
