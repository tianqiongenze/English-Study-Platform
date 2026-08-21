// ESP 动效变体(framer-motion)—— 互联网大厂标配页面转场/卡片入场/微交互。
// duration/ease 与 packages/ui-tokens/tokens.css --esp-motion-duration-* / --esp-motion-ease 同源。
// 复用:import { fadeUp, stagger, scaleSpring } from "../motion";

import type { Variants } from "framer-motion";

// 与 --esp-motion-duration-base(0.2s)/--esp-motion-ease cubic-bezier(0.645,0.045,0.355,1)同源
export const EASE = [0.645, 0.045, 0.355, 1] as const;
export const DUR_FAST = 0.1;
export const DUR_BASE = 0.2;
export const DUR_SLOW = 0.3;

/** fadeUp:从下淡入(标题/卡片入场常用) */
export const fadeUp: Variants = {
  hidden: { opacity: 0, y: 20 },
  visible: { opacity: 1, y: 0, transition: { duration: DUR_SLOW, ease: EASE } },
};

/** fadeIn:纯淡入 */
export const fadeIn: Variants = {
  hidden: { opacity: 0 },
  visible: { opacity: 1, transition: { duration: DUR_BASE, ease: EASE } },
};

/** scaleSpring:弹性缩放(按钮/卡片悬浮) */
export const scaleSpring: Variants = {
  rest: { scale: 1 },
  hover: { scale: 1.04, transition: { type: "spring", stiffness: 300, damping: 20 } },
};

/** stagger:子元素依次入场容器 */
export const stagger: Variants = {
  hidden: { opacity: 0 },
  visible: {
    opacity: 1,
    transition: { staggerChildren: 0.08, delayChildren: 0.1 },
  },
};

/** slideInRight:从右滑入(Drawer/抽屉) */
export const slideInRight: Variants = {
  hidden: { x: 40, opacity: 0 },
  visible: { x: 0, opacity: 1, transition: { duration: DUR_SLOW, ease: EASE } },
};

/** pageTransition:路由页面转场(Outlet motion.div key=pathname) */
export const pageTransition: Variants = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0, transition: { duration: DUR_BASE, ease: EASE } },
  exit: { opacity: 0, y: -8, transition: { duration: DUR_FAST, ease: EASE } },
};
