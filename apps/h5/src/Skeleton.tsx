import React from "react";

/**
 * 通用骨架屏组件(纯 CSS shimmer 动画)。
 *
 * 说明:@esp/ui-tokens 的 tokens.css 在 :root 下是亮色主题值
 * (如 --esp-color-bg-layout: #f5f5f5),而本应用为深色主题(#0f172a 底 /
 * #1e293b 容器),直接引用 token 会出现亮色骨架与深色背景冲突。
 * 故此处使用与 styles.css 完全一致的深色常量(#1e293b / #334155),
 * 保证视觉风格统一;样式由组件自注入,零外部依赖。
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
  background: linear-gradient(90deg, #1e293b 25%, #334155 50%, #1e293b 75%);
  background-size: 200% 100%;
  animation: esp-shimmer 1.4s linear infinite;
  border-radius: 6px;
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
