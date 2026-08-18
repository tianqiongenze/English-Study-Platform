// 小程序 i18n 入口:detector(esp_locale storage + 系统语言)+ createI18n(ui-tokens 共享词典)。
// 共享词典尚未收录的新 key 在此本地补全(miniprogram.* 命名空间);
// ui-tokens 扩展吸收后,共享词典命中优先,本地 shim 自动失效(见 t 的查找顺序)。
import Taro from "@tarojs/taro";
import { createI18n, type Locale, type MessageKey } from "@esp/ui-tokens";

/* ---------- 本地补全词典(待吸收进 packages/ui-tokens/src/i18n.ts) ---------- */
const extraEn = {
  "miniprogram.login.wechatHint": "One-tap login with your WeChat account",
  "miniprogram.login.wechat": "WeChat Log In",
  "miniprogram.login.failed": "Login failed",
  "miniprogram.index.welcome": "Welcome to {app}",
  "miniprogram.index.sessionHistory": "Session history",
  "miniprogram.index.loadFailed": "Load failed:{err}",
  "miniprogram.index.status": "Status",
  "miniprogram.index.startedAt": "Started",
  "miniprogram.index.endedAt": "Ended",
  "miniprogram.index.ongoing": "Ongoing",
  "miniprogram.scenario": "Scenario",
  "miniprogram.speech.session": "Session",
  "miniprogram.speech.correction": "Correction",
  "miniprogram.speech.guidance": "Hint",
  "miniprogram.speech.end": "Ended",
  "miniprogram.speech.error": "Error",
  "miniprogram.speech.me": "Me",
  "miniprogram.speech.inputPlaceholder": "Say something (English)...",
  "miniprogram.speech.send": "Send",
  "miniprogram.speech.hintButton": "Hint",
  "miniprogram.speech.endSession": "End",
  "miniprogram.speech.scenarioAirport": "Airport check-in",
  "miniprogram.failed": "Failed",
  "miniprogram.avatar.happy": "Happy",
  "miniprogram.avatar.neutral": "Neutral",
  "miniprogram.avatar.encouraging": "Encouraging",
  "miniprogram.avatar.sad": "Concerned",
  "miniprogram.avatar.speaking": "(speaking)",
  "miniprogram.writing.tabGrade": "Grade",
  "miniprogram.writing.tabAgent": "Agent",
  "miniprogram.writing.aiGrade": "AI Grade",
  "miniprogram.writing.polishPlaceholder": "Paste English to polish...",
  "miniprogram.writing.translatePlaceholder": "Paste English to translate...",
  "miniprogram.writing.agentRun": "AI Agent Grading (LangGraph)",
  "miniprogram.writing.checkStatus": "Check status",
  "miniprogram.learning.adaptive": "Adaptive Learning",
  "miniprogram.learning.userId": "User ID",
  "miniprogram.learning.placementDone": "Placement done:{n} mastery baselines initialized",
  "miniprogram.learning.placementSkip": "Placement skipped:{e}",
  "miniprogram.learning.reportEvent": "Report learning",
  "miniprogram.learning.reviewToday": "Today's review",
  "miniprogram.learning.weakPoints": "Weak points",
  "miniprogram.learning.difficulty": "Adjust difficulty",
  "miniprogram.learning.nextDifficulty": "Next difficulty:{d} speed:{v} scaffolding:{s}",
} as const;

type ExtraKey = keyof typeof extraEn;

// 键位镜像校验:zh 必须与 en 同键集合(缺键/多键 → tsc 报错)
const extraZh: Record<ExtraKey, string> = {
  "miniprogram.login.wechatHint": "使用微信账号一键登录",
  "miniprogram.login.wechat": "微信一键登录",
  "miniprogram.login.failed": "登录失败",
  "miniprogram.index.welcome": "欢迎来到 {app}",
  "miniprogram.index.sessionHistory": "会话历史",
  "miniprogram.index.loadFailed": "加载失败:{err}",
  "miniprogram.index.status": "状态",
  "miniprogram.index.startedAt": "开始",
  "miniprogram.index.endedAt": "结束",
  "miniprogram.index.ongoing": "进行中",
  "miniprogram.scenario": "场景",
  "miniprogram.speech.session": "会话",
  "miniprogram.speech.correction": "纠错",
  "miniprogram.speech.guidance": "引导",
  "miniprogram.speech.end": "结束",
  "miniprogram.speech.error": "错误",
  "miniprogram.speech.me": "我",
  "miniprogram.speech.inputPlaceholder": "说点什么(英文)...",
  "miniprogram.speech.send": "发送",
  "miniprogram.speech.hintButton": "提示",
  "miniprogram.speech.endSession": "结束",
  "miniprogram.speech.scenarioAirport": "机场登机",
  "miniprogram.failed": "失败",
  "miniprogram.avatar.happy": "开心",
  "miniprogram.avatar.neutral": "平静",
  "miniprogram.avatar.encouraging": "鼓励",
  "miniprogram.avatar.sad": "关切",
  "miniprogram.avatar.speaking": "(说话中)",
  "miniprogram.writing.tabGrade": "批改",
  "miniprogram.writing.tabAgent": "智能体",
  "miniprogram.writing.aiGrade": "AI 批改",
  "miniprogram.writing.polishPlaceholder": "输入要润色的英文...",
  "miniprogram.writing.translatePlaceholder": "输入要翻译的英文...",
  "miniprogram.writing.agentRun": "智能体批改(LangGraph)",
  "miniprogram.writing.checkStatus": "查询状态",
  "miniprogram.learning.adaptive": "自适应学习",
  "miniprogram.learning.userId": "用户 ID",
  "miniprogram.learning.placementDone": "能力定级完成:初始化 {n} 个知识点基线",
  "miniprogram.learning.placementSkip": "定级跳过:{e}",
  "miniprogram.learning.reportEvent": "上报学情",
  "miniprogram.learning.reviewToday": "今日复习",
  "miniprogram.learning.weakPoints": "薄弱点",
  "miniprogram.learning.difficulty": "难度调整",
  "miniprogram.learning.nextDifficulty": "下一难度:{d} 倍速:{v} 脚手架:{s}",
};

/* ---------- locale 探测与持久化 ---------- */
function detectLocale(): Locale {
  let stored: unknown = "";
  try {
    stored = Taro.getStorageSync("esp_locale");
  } catch {
    /* 忽略 */
  }
  if (stored === "en" || stored === "zh") return stored;
  let lang = "zh";
  try {
    const info = Taro.getSystemInfoSync();
    if (typeof info.language === "string") lang = info.language;
  } catch {
    /* 忽略 */
  }
  return lang.toLowerCase().startsWith("zh") ? "zh" : "en";
}

export const i18n = createI18n(detectLocale(), (l) => {
  try {
    Taro.setStorageSync("esp_locale", l);
  } catch {
    /* 忽略 */
  }
});

export type MpKey = MessageKey | ExtraKey;

function interpolate(s: string, vars: Record<string, string | number>): string {
  let out = s;
  for (const [k, v] of Object.entries(vars)) {
    out = out.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
  }
  return out;
}

/** 小程序侧 t():先查共享词典,未命中再查本地补全(避免渲染裸 key)。 */
export function t(key: MpKey, vars?: Record<string, string | number>): string {
  const base = i18n.t(key as MessageKey, vars);
  if (base !== key) return base;
  const dict = i18n.locale === "zh" ? extraZh : (extraEn as Record<ExtraKey, string>);
  const raw = dict[key as ExtraKey];
  if (raw == null) return key;
  return vars ? interpolate(raw, vars) : raw;
}

export function setLocale(l: Locale): void {
  i18n.setLocale(l); // persist 回调自动写 esp_locale
  syncNavTitle();
}

export function toggleLocale(): Locale {
  const next: Locale = i18n.locale === "zh" ? "en" : "zh";
  setLocale(next);
  return next;
}

/** 导航标题联动当前语言(app 级;页面级 onShow 可覆盖)。 */
export function syncNavTitle(title?: string): void {
  try {
    Taro.setNavigationBarTitle({ title: title ?? t("app.title") });
  } catch {
    /* 低基础库忽略 */
  }
}