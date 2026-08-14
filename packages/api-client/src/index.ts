// ESP 三端统一 API SDK —— 单真源方法签名(超集,以 Web 端真实 API 面为准)。
// 消灭历史漂移:H5 grade/writingRun 硬编码 exam="toefl"、小程序硬编码 prompt+exam、
// reportEvent 三端签名不一。统一后各端 api.ts 为薄委托层。

import type {
  AuthUser,
  ClassAnalytics,
  DifficultyResp,
  GradeResult,
  LessonPlan,
  LoginResp,
  Mastery,
  PolishResult,
  Scenario,
  SessionResp,
  SessionSummary,
  TranslateResult,
  WsFrame,
} from "@esp/types";
import { ApiError, FetchTransport, type Transport } from "./transport";
import type { TokenStorage } from "./storage";

export { ApiError, FetchTransport } from "./transport";
export type { Transport, TransportRequest, TransportResponse } from "./transport";
export type { TokenStorage } from "./storage";
export { LocalStorageTokenStorage } from "./storage";

// 简版 traceparent 生成(00-<32hex>-<16hex>-01),与三端原 trace.ts 行为一致
function traceparent(): string {
  const hex = (n: number) =>
    Array.from({ length: n }, () =>
      Math.floor(Math.random() * 16).toString(16)
    ).join("");
  return `00-${hex(32)}-${hex(16)}-01`;
}

export interface ApiClientOptions {
  /** 空串 = 同源相对路径(Web/H5 经 ingress/dev proxy);小程序传 TARO_APP_API */
  baseUrl?: string;
  transport?: Transport;
  storage: TokenStorage;
  defaultTimeoutMs?: number;
}

export class ApiClient {
  private base: string;
  private transport: Transport;
  private storage: TokenStorage;
  private timeoutMs: number;

  constructor(opts: ApiClientOptions) {
    this.base = (opts.baseUrl ?? "").replace(/\/$/, "");
    this.transport = opts.transport ?? new FetchTransport();
    this.storage = opts.storage;
    this.timeoutMs = opts.defaultTimeoutMs ?? 15000;
  }

  async request<T>(
    method: "GET" | "POST" | "PUT" | "DELETE",
    path: string,
    body?: unknown,
    opts?: { auth?: boolean; timeoutMs?: number }
  ): Promise<T> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      traceparent: traceparent(),
    };
    if (opts?.auth !== false) {
      const token = this.storage.getToken();
      if (token) headers.Authorization = `Bearer ${token}`;
    }
    const res = await this.transport.request({
      method,
      url: `${this.base}${path}`,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      timeoutMs: opts?.timeoutMs ?? this.timeoutMs,
    });
    if (res.status >= 400) {
      throw new ApiError(res.status, res.body, `HTTP ${res.status}: ${path}`);
    }
    return res.body ? (JSON.parse(res.body) as T) : (undefined as T);
  }

  getToken(): string | null {
    return this.storage.getToken();
  }

  // ---- auth ----
  login(email: string, password: string): Promise<LoginResp> {
    return this.request("POST", "/api/auth/login", { email, password }, { auth: false });
  }
  register(
    email: string,
    password: string,
    name: string,
    role?: string
  ): Promise<LoginResp> {
    // role 仅做接口兼容:服务端 P0-4 已强制 learner,传了也被忽略
    return this.request(
      "POST",
      "/api/auth/register",
      { email, password, name, role },
      { auth: false }
    );
  }

  // ---- speech ----
  scenarios(): Promise<Scenario[]> {
    return this.request("GET", "/api/scenarios");
  }
  createSession(scenario_code: string): Promise<SessionResp> {
    return this.request("POST", "/api/sessions", { scenario_code });
  }
  sessionHistory(limit?: number): Promise<SessionSummary[]> {
    return this.request(
      "GET",
      `/api/sessions/history${limit ? `?limit=${limit}` : ""}`
    );
  }

  // ---- learning ----
  reportEvent(user_id: number, event_type: string, payload: string): Promise<{ accepted: boolean }> {
    return this.request("POST", "/api/learning/event", { user_id, event_type, payload });
  }
  review(user_id: number): Promise<Mastery[]> {
    return this.request("GET", `/api/learning/review?user_id=${user_id}`);
  }
  weak(user_id: number): Promise<Mastery[]> {
    return this.request("GET", `/api/learning/mastery/weak?user_id=${user_id}`);
  }
  placement(user_id: number, kp_ids: number[] = [], score = 0): Promise<{ initialized: number }> {
    return this.request("POST", "/api/learning/placement", { user_id, kp_ids, score });
  }
  difficulty(current: number, correct: boolean): Promise<DifficultyResp> {
    return this.request("POST", "/api/learning/difficulty", { current, correct });
  }
  reportBehavior(
    user_id: number,
    action: string,
    target?: string,
    duration_ms?: number,
    context?: Record<string, unknown>
  ): Promise<{ accepted: boolean }> {
    return this.request("POST", "/api/learning/behavior", {
      user_id,
      action,
      target,
      duration_ms,
      context,
    });
  }
  recommend(user_id: number): Promise<{
    review: unknown[];
    weak_points: unknown[];
    similar_scenarios: unknown[];
  }> {
    return this.request("GET", `/api/learning/recommend?user_id=${user_id}`);
  }

  // ---- writing(统一完整签名:prompt/essay/exam 全显式,消灭 H5/小程序硬编码) ----
  grade(prompt: string, essay: string, exam: string): Promise<GradeResult> {
    return this.request("POST", "/api/writing/grade", { prompt, essay, exam });
  }
  writingRun(
    prompt: string,
    essay: string,
    exam: string,
    human = false
  ): Promise<{
    thread_id: string;
    status: string;
    grade_result?: unknown;
    polished?: string;
    notes?: string;
  }> {
    return this.request("POST", "/api/writing/run", { prompt, essay, exam, human });
  }
  writingReview(thread_id: string, teacher_feedback: string): Promise<{
    thread_id: string;
    status: string;
    notes?: string;
  }> {
    return this.request("POST", "/api/writing/review", { thread_id, teacher_feedback });
  }
  writingState(thread_id: string): Promise<{
    status: string;
    grade_result?: unknown;
    polished?: string;
    teacher_feedback?: string;
    notes?: string;
  }> {
    return this.request("GET", `/api/writing/state/${thread_id}`);
  }
  polish(text: string): Promise<PolishResult> {
    return this.request("POST", "/api/writing/polish", { text });
  }
  translate(text: string): Promise<TranslateResult> {
    return this.request("POST", "/api/writing/translate", { text });
  }

  // ---- teacher(仅 Web 消费) ----
  lessonPlan(topic: string, level: string): Promise<LessonPlan> {
    return this.request("POST", "/api/teacher/lesson-plan", { topic, level });
  }
  classAnalytics(id: number): Promise<ClassAnalytics[]> {
    return this.request("GET", `/api/teacher/classes/${id}/analytics`);
  }
  autoGrade(assignmentId: number, classId?: number): Promise<{ graded: number }> {
    return this.request(
      "POST",
      `/api/teacher/assignments/${assignmentId}/auto-grade${classId ? `?classId=${classId}` : ""}`
    );
  }
  finalize(
    submissionId: number,
    finalScore: number,
    classId?: number
  ): Promise<{ finalized: number }> {
    return this.request(
      "POST",
      `/api/teacher/submissions/${submissionId}/finalize`,
      { final_score: finalScore, ...(classId ? { class_id: classId } : {}) }
    );
  }

  // ---- emotion ----
  senseEmotion(text: string): Promise<{ emotion: string; encouragement: string }> {
    return this.request("POST", "/api/emotion/sense", { text });
  }
  visualWord(word: string, context = ""): Promise<{ image_url: string; alt: string }> {
    return this.request("POST", "/api/emotion/visual", { word, context });
  }

  // ---- WS ----
  speechWsUrl(sessionId: string): string {
    const token = this.storage.getToken() ?? "";
    return `${this.base.replace(/^http/, "ws")}/ws/speech/${sessionId}?token=${token}`;
  }
}

export type { WsFrame };
