// 安全图片组件:Unsplash 直链 + 渐变 fallback。
// 网络不可靠/暗色主题下也不断链:onError 隐藏 <img>,展示 --esp-img-fallback-* 渐变底色。
import { useState, type CSSProperties, type ReactNode } from "react";

// Unsplash 免费商用直链(无需 key,热链合规)。w=800 控制体积 +=lazy。
export const UNSPLASH = {
  /** 学习/口语场景:人群共学 */
  study:
    "https://images.unsplash.com/photo-1522203851244-10e1ebd683d3?auto=format&fit=crop&w=800&q=60",
  /** 森林绿/阅读/内容库场景 */
  forest:
    "https://images.unsplash.com/photo-1523240795612-9a054b0db644?auto=format&fit=crop&w=800&q=60",
  /** 写作场景:书桌 */
  writing:
    "https://images.unsplash.com/photo-1517245386807-bb43f82c33c4?auto=format&fit=crop&w=800&q=60",
} as const;

export default function SafeImg({
  src,
  alt,
  tone = 1,
  className,
  style,
  children,
}: {
  src: string;
  alt: string;
  /** fallback 渐变 1-4(对应 --esp-img-fallback-1..4,暗色有专属变体) */
  tone?: 1 | 2 | 3 | 4;
  className?: string;
  style?: CSSProperties;
  children?: ReactNode;
}) {
  const [broken, setBroken] = useState(false);
  return (
    <div
      className={`esp-img ${broken ? "esp-img--broken" : ""} ${className ?? ""}`.trim()}
      style={{ ...style, ["--esp-fb" as string]: `var(--esp-img-fallback-${tone})` } as CSSProperties}
    >
      <img
        src={src}
        alt={broken ? "" : alt}
        loading="lazy"
        onError={() => setBroken(true)}
      />
      <div className="esp-img__fallback" aria-hidden="true" />
      {children}
    </div>
  );
}