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

// ---- content:统一内容库(功能拓展阶段1) ----
export interface ContentCourse {
  id: number;
  code: string;
  title: string;
  exam_tag: string;
  cefr_range: string;
  description: string;
  status: string;
}
export interface ContentLesson {
  id: number;
  course_id: number;
  seq: number;
  title: string;
  skill: string;
  cefr: string;
}
export interface ContentPrompt {
  word?: string;
  phonetic?: string;
  pos?: string;
  meaning_en?: string;
  meaning_zh?: string;
  example?: string;
  title?: string;
  text?: string;
  script?: string;
  question?: string;
  options?: string[];
}
export interface ContentItem {
  id: number;
  lesson_id: number;
  item_type: string;
  prompt: ContentPrompt;
  difficulty: number;
  cefr: string;
  tags: string[];
}
export interface ContentAnswerResult {
  correct: boolean;
  score: number;
  explanation: string;
}

// ---- community:圈子社区(功能拓展阶段2) ----
export interface CommunityCircle {
  id: number;
  name: string;
  description: string;
  invite_code: string;
  owner_id: number;
  owner_name: string;
  member_limit: number;
  member_count: number;
  status: string;
  created_at: string;
  joined: boolean;
  my_role?: string;
}
export interface CommunityMember {
  user_id: number;
  name: string;
  role: string;
  joined_at: string;
}
export interface CommunityPost {
  id: number;
  user_id: number;
  user_name: string;
  body: string;
  status: string;
  report_count: number;
  comment_count: number;
  created_at: string;
}
export interface CommunityComment {
  id: number;
  user_id: number;
  user_name: string;
  body: string;
  created_at: string;
}
export interface CommunityReportResult {
  report_count: number;
  auto_hidden: boolean;
}
export interface CommunityCreateResult {
  id: number;
  invite_code: string;
}

// ---- rankings:排行榜(功能拓展阶段3) ----
export type RankType = "xp" | "prog";
export type RankScope = "global" | "region" | "circle" | "category";
export type RankPeriod = "daily" | "weekly" | "monthly";

export interface RankingEntry {
  rank: number;
  user_id: number;
  name: string;
  score: number;
}
export interface RankingMine {
  user_id: number;
  rank: number;
  score: number;
  gap_to_top: number;
}
export interface RankingResponse {
  list: RankingEntry[];
  mine: RankingMine | Record<string, never>;
  as_of: string;
}
export interface RankingGap {
  my_rank: number;
  my_score: number;
  total: number;
  gap_to_top: number;
  gap_to_prev: number;
  percentile: number;
  opted_out?: boolean;
}
export interface RegionOption {
  code: string;
  label: string;
}
