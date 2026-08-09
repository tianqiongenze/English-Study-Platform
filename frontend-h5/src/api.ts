import { useAuth, authHeader } from "./auth";
import { traceparent } from "./trace";
import type { Scenario, SessionResp, SessionSummary, Mastery, GradeResult, PolishResult, TranslateResult, DifficultyResp, AuthUser } from "./types";

async function req<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = useAuth.getState().token;
  const headers: Record<string, string> = { "Content-Type": "application/json", traceparent: traceparent(), ...((init.headers as Record<string,string>) || {}), ...authHeader(token) } as Record<string, string>;
  const res = await fetch(path, { ...init, headers });
  const text = await res.text();
  let body: unknown = null; if (text) { try { body = JSON.parse(text); } catch { body = text; } }
  if (!res.ok) throw new Error(`${res.status}`);
  return body as T;
}

export const api = {
  login: (email: string, password: string) => req<{ token: string; user: AuthUser }>("/api/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  register: (email: string, password: string, name: string, role: string) => req<{ token: string; user: AuthUser }>("/api/auth/register", { method: "POST", body: JSON.stringify({ email, password, name, role }) }),
  scenarios: () => req<Scenario[]>("/api/scenarios"),
  createSession: (code: string) => req<SessionResp>("/api/sessions", { method: "POST", body: JSON.stringify({ scenario_code: code }) }),
  // 会话历史(gateway-go ListHistory,会话22-3 C 批次三端贯通)
  sessionHistory: () => req<SessionSummary[]>("/api/sessions/history"),
  reportEvent: (uid: number, t: string, p: string) => req<{ accepted: boolean }>("/api/learning/event", { method: "POST", body: JSON.stringify({ user_id: uid, event_type: t, payload: p }) }),
  review: (uid: number) => req<Mastery[]>(`/api/learning/review?user_id=${uid}`),
  weak: (uid: number) => req<Mastery[]>(`/api/learning/mastery/weak?user_id=${uid}`),
  // FR-2.1 初测定级
  placement: (uid: number, kp_ids: number[] = [], score = 0) => req<{ initialized: number }>("/api/learning/placement", { method: "POST", body: JSON.stringify({ user_id: uid, kp_ids, score }) }),
  // FR-2.2 动态难度
  difficulty: (current: number, correct: boolean) => req<DifficultyResp>("/api/learning/difficulty", { method: "POST", body: JSON.stringify({ current, correct }) }),
  grade: (prompt: string, essay: string) => req<GradeResult>("/api/writing/grade", { method: "POST", body: JSON.stringify({ prompt, essay, exam: "toefl" }) }),
  // 润色 / 翻译
  polish: (text: string) => req<PolishResult>("/api/writing/polish", { method: "POST", body: JSON.stringify({ text }) }),
  translate: (text: string) => req<TranslateResult>("/api/writing/translate", { method: "POST", body: JSON.stringify({ text }) }),
  // FR-5.2 视觉辅助 + FR-6.2 LangGraph 写作智能体(human-in-loop)
  visualWord: (word: string, context = "") => req<{ image_url: string; alt: string }>("/api/emotion/visual", { method: "POST", body: JSON.stringify({ word, context }) }),
  senseEmotion: (text: string) => req<{ emotion: string; encouragement: string }>("/api/emotion/sense", { method: "POST", body: JSON.stringify({ text }) }),
  writingRun: (prompt: string, essay: string, human = false) => req<{ thread_id: string; status: string; notes?: string }>("/api/writing/run", { method: "POST", body: JSON.stringify({ prompt, essay, exam: "toefl", human }) }),
  writingReview: (thread_id: string, teacher_feedback: string) => req<{ thread_id: string; status: string; notes: string }>("/api/writing/review", { method: "POST", body: JSON.stringify({ thread_id, teacher_feedback }) }),
  writingState: (thread_id: string) => req<{ status: string; notes?: string }>(`/api/writing/state/${thread_id}`),
  // 用户行为上报 + 推荐(ADR-observability-rebuild §4)
  reportBehavior: (uid: number, action: string, target?: string, duration_ms?: number, context?: Record<string, unknown>) =>
    req<{ accepted: boolean }>("/api/learning/behavior", { method: "POST", body: JSON.stringify({ user_id: uid, action, target, duration_ms, context }) }),
  recommend: (uid: number) => req<{ review: unknown[]; weak_points: unknown[]; similar_scenarios: unknown[] }>(`/api/learning/recommend?user_id=${uid}`),
};
