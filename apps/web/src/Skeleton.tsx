import React from "react";

/**
 * 通用骨架屏组件(纯 CSS shimmer 动画)。
 * 颜色走 @esp/ui-tokens 的 CSS 变量(--esp-color-bg-container / --esp-color-border),
 * 随 data-theme 亮暗自动切换,与全局 styles.css 一致。
 */

let styleInjected = false;
function injectStyle() {
  if (styleInjected || typeof document === "undefined") return;
  const style = document.createElement("style");
  style.textContent = `
@keyframes esp-shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
.esp-skeleton {
  background: linear-gradient(90deg, var(--esp-color-bg-container) 25%, var(--esp-color-bg-subtle) 50%, var(--esp-color-bg-container) 75%);
  background-size: 200% 100%;
  animation: esp-shimmer 1.4s linear infinite;
  border-radius: var(--esp-radius-base);
}
`;
  document.head.appendChild(style);
  styleInjected = true;
}

injectStyle();

export type SkeletonProps = {
  /** 宽度,默认 100% */
  width?: number | string;
  /** 高度,默认 16px */
  height?: number | string;
  /** 圆角,默认 6px */
  radius?: number | string;
  /** 重复行数,默认 1 */
  count?: number;
  /** 多行时行间距,默认 8px */
  gap?: number;
  className?: string;
  style?: React.CSSProperties;
};

/** 单个/多行骨架块 */
export default function Skeleton({
  width = "100%",
  height = 16,
  radius = 6,
  count = 1,
  gap = 8,
  className,
  style,
}: SkeletonProps) {
  const rows = Array.from({ length: Math.max(1, count) });
  return (
    <div className={className} style={{ width, display: "flex", flexDirection: "column", gap, ...style }}>
      {rows.map((_, i) => (
        <div
          key={i}
          className="esp-skeleton"
          style={{ height, borderRadius: radius, width: i === rows.length - 1 && count > 1 ? "60%" : "100%" }}
        />
      ))}
    </div>
  );
}

/** 页面级骨架屏:用于 React.lazy 路由的 Suspense fallback */
export function SkeletonPage() {
  return (
    <div className="panel" aria-busy="true" aria-label="页面加载中">
      <Skeleton height={24} width="40%" style={{ marginBottom: 16 }} />
      <Skeleton count={4} height={14} gap={10} />
      <div style={{ height: 12 }} />
      <Skeleton height={120} radius={8} />
    </div>
  );
}
