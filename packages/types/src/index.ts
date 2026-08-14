// ESP 三端共享 API 契约类型 —— 单真源。
// 三端(apps/web, apps/h5, apps/miniprogram)从 @esp/types 导入,
// 禁止在应用内复制定义(历史上三份 types.ts 人肉同步已漂移)。

export type Role = "learner" | "teacher" | "admin";

export interface AuthUser {
  id: number;
  email: string;
  name: string;
  role: Role;
}

export interface LoginResp {
  token: string;
  user: AuthUser;
}

// 口语陪练
export interface Scenario {
  code: string;
  title: string;
  system: string;
  difficulty: number;
}
export interface SessionResp {
  session_id: string;
  scenario: string;
}
// 会话历史摘要(gateway-go ListHistory 返回,对齐 backend/services/gateway-go/src/session/manager.go:97-104)
export interface SessionSummary {
  id: number;
  user_id: number;
  scenario: string;
  status: string;
  started_at: string;
  ended_at: string | null;
}
export interface WsFrame {
  type: "ai_reply" | "correction" | "hint" | "end" | "error" | string;
  text?: string;
  original?: string;
  corrected?: string;
  options?: string[];
  summary?: string;
  message?: string;
  data?: string;
  emotion?: string;
}

// 学习
export interface Mastery {
  userId: number;
  kpId: number;
  masteryScore: number;
  nextReviewAt: string;
}
export interface DifficultyResp {
  next_difficulty: number;
  speed_multiplier: number;
  scaffolding: boolean;
}

// 写作
export interface GradeResult {
  score: number;
  feedback: string;
  grammar: { original: string; corrected: string; reason: string }[];
  coherence: { score: number; transitions: string[]; paragraph_flow: string };
}
export interface PolishResult {
  polished: string;
  suggestions: { original: string; polished: string; reason: string }[];
}
export interface TranslateResult {
  translation: string;
  vocab: { word: string; pos: string; meaning: string; example: string }[];
}

// 教师(仅 Web 端使用,H5/小程序不消费但类型保留)
export interface LessonPlan {
  passage: string;
  exercises: unknown[];
  games: unknown[];
  discussion: unknown[];
}
export interface ClassAnalytics {
  id: number;
  content: string;
  avgScore: number;
  count: number;
}

// 事件上报(三端统一签名,消灭 H5 硬编码 toefl / 小程序硬编码 prompt+exam)
export interface ReportEventReq {
  user_id: number;
  event_type: string;
  payload: string;
}
