// 三端共享 API 契约类型(Web/H5/小程序内联同一份)。

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

// 教师
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
