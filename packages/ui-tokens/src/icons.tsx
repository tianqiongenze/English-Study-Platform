// 内联 SVG 图标集(零依赖,stroke=currentColor 可随主题/品牌染色)。
// 供 Web/H5/Portal 使用;小程序(Taro weapp 不支内联 <svg>)自行用 emoji/iconfont。

import * as React from "react";

export function Icon({
  size = 20,
  children,
  className,
  style,
}: {
  size?: number;
  children: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
}) {
  return React.createElement(
    "svg",
    {
      width: size,
      height: size,
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      strokeWidth: 2,
      strokeLinecap: "round",
      strokeLinejoin: "round",
      className,
      style,
      "aria-hidden": true,
    },
    children
  );
}

export function SunIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4l1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4m11.4-11.4l1.4-1.4" />
    </Icon>
  );
}

export function MoonIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
    </Icon>
  );
}

export function MonitorIcon(props: { size?: number; className?: string }) {
  // 跟随系统
  return (
    <Icon {...props}>
      <rect x="2" y="3" width="20" height="14" rx="2" />
      <path d="M8 21h8m-4-4v4" />
    </Icon>
  );
}

export function GlobeIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18" />
    </Icon>
  );
}

export function LogoIcon(props: { size?: number; className?: string }) {
  // 对话气泡 + 闪电(AI)
  return (
    <Icon {...props}>
      <path d="M21 12a8 8 0 0 1-8 8H4l2-3a8 8 0 1 1 15-5z" />
      <path d="M13.5 7l-3 4.5h3L11.5 16" />
    </Icon>
  );
}

export function TrophyIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <path d="M8 21h8m-4-4v-3m0-1c-3.5 0-6-2.5-6-6V5h12v2c0 3.5-2.5 6-6 6z" />
      <path d="M6 5H3v2a3 3 0 0 0 3 3m12-5h3v2a3 3 0 0 1-3 3" />
    </Icon>
  );
}

export function MedalIcon({
  tier,
  size = 20,
  className,
}: {
  tier: "gold" | "silver" | "bronze";
  size?: number;
  className?: string;
}) {
  return (
    <Icon {...props(size)}>
      <circle cx="12" cy="14" r="6" />
      <path d="M9 3l1.5 5m4.5-5l-1.5 5" />
      <path d="M6.5 8l-2.8 2.4 1.2 4.6m13.6-7l2.8 2.4-1.2 4.6" />
    </Icon>
  );
  function props(s: number) {
    return { size: s, className };
  }
}

export function HomeIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <path d="M3 11l9-7 9 7v9a1 1 0 0 1-1 1h-5v-6h-6v6H4a1 1 0 0 1-1-1v-9z" />
    </Icon>
  );
}

export function BookIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <path d="M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2V5zM4 19a2 2 0 0 1 2-2h13" />
    </Icon>
  );
}

export function MicIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <rect x="9" y="3" width="6" height="11" rx="3" />
      <path d="M5 11a7 7 0 0 0 14 0M12 18v3" />
    </Icon>
  );
}

export function PenIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <path d="M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
    </Icon>
  );
}

export function UsersIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <circle cx="9" cy="8" r="4" />
      <path d="M2 21v-1a6 6 0 0 1 12 0v1M16 3.1a4 4 0 0 1 0 7.8M22 21v-1a6 6 0 0 0-4-5.6" />
    </Icon>
  );
}

export function GridIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <rect x="3" y="3" width="7" height="7" rx="1" />
      <rect x="14" y="3" width="7" height="7" rx="1" />
      <rect x="3" y="14" width="7" height="7" rx="1" />
      <rect x="14" y="14" width="7" height="7" rx="1" />
    </Icon>
  );
}

export function SettingsIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.09a1.7 1.7 0 0 0-1-1.55 1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.7 1.7 0 0 0 .34-1.87 1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.09a1.7 1.7 0 0 0 1.55-1 1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.7 1.7 0 0 0 1.87.34h.01a1.7 1.7 0 0 0 1-1.55V3a2 2 0 1 1 4 0v.09a1.7 1.7 0 0 0 1 1.55 1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.7 1.7 0 0 0-.34 1.87v.01a1.7 1.7 0 0 0 1.55 1H21a2 2 0 1 1 0 4h-.09a1.7 1.7 0 0 0-1.55 1z" />
    </Icon>
  );
}

export function ChevronRightIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <path d="M9 6l6 6-6 6" />
    </Icon>
  );
}

export function SparklesIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <path d="M12 3l1.6 4.4L18 9l-4.4 1.6L12 15l-1.6-4.4L6 9l4.4-1.6L12 3z" />
      <path d="M19 14l.8 2.2L22 17l-2.2.8L19 20l-.8-2.2L16 17l2.2-.8L19 14zM5 14l.6 1.7L7.3 16l-1.7.6L5 18.3l-.6-1.7L2.7 16l1.7-.6L5 14z" />
    </Icon>
  );
}

export function ImageOffIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M3 3l18 18M9.5 9.5l-5 8M14 10l5 7" />
    </Icon>
  );
}

export function RocketIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <path d="M5 13c-1.5 1-2 2.5-2 4.5 2 0 3.5-.5 4.5-2M14.5 4.5c2.5-1.5 5-2 8-1.5-.5 3-1 5.5-2.5 7.5l-4 5-4-1-2 2-1.5-4.5 5-4c1-.5 1.5-2 1-3.5z" />
      <circle cx="14.5" cy="9.5" r="1.6" />
      <path d="M8.5 15.5L4 20" />
    </Icon>
  );
}

export function HeartIcon(props: { size?: number; className?: string }) {
  return (
    <Icon {...props}>
      <path d="M12 21C6.5 16 3 13 3 8.5 3 6 5 4 7.5 4c1.7 0 3.3.9 4.5 2.3C13.2 4.9 14.8 4 16.5 4 19 4 21 6 21 8.5 21 13 17.5 16 12 21z" />
    </Icon>
  );
}