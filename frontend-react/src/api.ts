import { useAuth, authHeader } from "./auth";
import { traceparent } from "./trace";

// 统一 API 客户端:自动注入 JWT + W3C traceparent(断点6:前端→BFF trace 贯通)。
// 失败抛 Error(带状态码与消息)。
async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = useAuth.getState().token;
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    traceparent: traceparent(),
    ...(init.headers as Record<string, string> | undefined),
    ...authHeader(token),
  };
  const res = await fetch(path, { ...init, headers });
  const text = await res.text();
  let body: unknown = null;
  if (text) {
    try { body = JSON.parse(text); } catch { body = text; }
  }
  if (!res.ok) {
    const msg = typeof body === "string" ? body : (body as { message?: string })?.message || res.statusText;
    throw new Error(`${res.status} ${msg}`);
  }
  return body as T;
}

export const api = {
  // auth
  login: (email: string, password: string) =>
    request<{ token: string; user: import("./types").AuthUser }>("/api/auth/login", {
      method: "POST", body: JSON.stringify({ email, password }),
    }),
  register: (email: string, password: string, name: string, role: string) =>
    request<{ token: string; user: import("./types").AuthUser }>("/api/auth/register", {
      method: "POST", body: JSON.stringify({ email, password, name, role }),
    }),
  // speech
  scenarios: () => request<import("./types").Scenario[]>("/api/scenarios"),
  createSession: (scenario_code: string) =>
    request<import("./types").SessionResp>("/api/sessions", {
      method: "POST", body: JSON.stringify({ scenario_code }),
    }),
  // learning
  reportEvent: (user_id: number, event_type: string, payload: string) =>
    request<{ accepted: boolean }>("/api/learning/event", {
      method: "POST", body: JSON.stringify({ user_id, event_type, payload }),
    }),
  review: (user_id: number) => request<import("./types").Mastery[]>(`/api/learning/review?user_id=${user_id}`),
  weak: (user_id: number) => request<import("./types").Mastery[]>(`/api/learning/mastery/weak?user_id=${user_id}`),
  // FR-2.1 初测定级(首次进入学习中心灌基线)
  placement: (user_id: number, kp_ids: number[] = [], score: number = 0) =>
    request<{ initialized: number }>("/api/learning/placement", {
      method: "POST", body: JSON.stringify({ user_id, kp_ids, score }),
    }),
  difficulty: (current: number, correct: boolean) =>
    request<import("./types").DifficultyResp>("/api/learning/difficulty", {
      method: "POST", body: JSON.stringify({ current, correct }),
    }),
  // writing
  grade: (prompt: string, essay: string, exam: string) =>
    request<import("./types").GradeResult>("/api/writing/grade", {
      method: "POST", body: JSON.stringify({ prompt, essay, exam }),
    }),
  polish: (text: string) =>
    request<import("./types").PolishResult>("/api/writing/polish", {
      method: "POST", body: JSON.stringify({ text }),
    }),
  translate: (text: string) =>
    request<import("./types").TranslateResult>("/api/writing/translate", {
      method: "POST", body: JSON.stringify({ text }),
    }),
  // teacher
  lessonPlan: (topic: string, level: string) =>
    request<import("./types").LessonPlan>("/api/teacher/lesson-plan", {
      method: "POST", body: JSON.stringify({ topic, level }),
    }),
  classAnalytics: (id: number) =>
    request<import("./types").ClassAnalytics[]>(`/api/teacher/classes/${id}/analytics`),
  autoGrade: (assignmentId: number, classId?: number) =>
    request<{ graded: number }>(`/api/teacher/assignments/${assignmentId}/auto-grade${classId ? `?classId=${classId}` : ""}`, { method: "POST" }),
  finalize: (submissionId: number, finalScore: number, classId?: number) =>
    request<{ finalized: number }>(`/api/teacher/submissions/${submissionId}/finalize`, {
      method: "POST", body: JSON.stringify({ final_score: finalScore, ...(classId ? { class_id: classId } : {}) }),
    }),
  // emotion
  senseEmotion: (text: string) =>
    request<{ emotion: string; encouragement: string }>("/api/emotion/sense", {
      method: "POST", body: JSON.stringify({ text }),
    }),
  // FR-5.2 视觉辅助(三端共用)
  visualWord: (word: string, context: string = "") =>
    request<{ image_url: string; alt: string }>("/api/emotion/visual", {
      method: "POST", body: JSON.stringify({ word, context }),
    }),
  // FR-6.2 LangGraph 写作智能体(human-in-loop)
  writingRun: (prompt: string, essay: string, exam: string, human: boolean = false) =>
    request<{ thread_id: string; status: string; grade_result?: unknown; polished?: string; notes?: string }>("/api/writing/run", {
      method: "POST", body: JSON.stringify({ prompt, essay, exam, human }),
    }),
  writingReview: (thread_id: string, teacher_feedback: string) =>
    request<{ thread_id: string; status: string; notes: string }>("/api/writing/review", {
      method: "POST", body: JSON.stringify({ thread_id, teacher_feedback }),
    }),
  writingState: (thread_id: string) =>
    request<{ status: string; grade_result?: unknown; polished?: string; teacher_feedback?: string; notes?: string }>(
      `/api/writing/state/${thread_id}`),
  // 用户行为上报 + 推荐(ADR-observability-rebuild §4)
  reportBehavior: (user_id: number, action: string, target?: string, duration_ms?: number, context?: Record<string, unknown>) =>
    request<{ accepted: boolean }>("/api/learning/behavior", {
      method: "POST", body: JSON.stringify({ user_id, action, target, duration_ms, context }),
    }),
  recommend: (user_id: number) =>
    request<{ review: unknown[]; weak_points: unknown[]; similar_scenarios: unknown[] }>(
      `/api/learning/recommend?user_id=${user_id}`),
};
