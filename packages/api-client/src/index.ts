// ESP 三端统一 API SDK —— 单真源方法签名(超集,以 Web 端真实 API 面为准)。
// 消灭历史漂移:H5 grade/writingRun 硬编码 exam="toefl"、小程序硬编码 prompt+exam、
// reportEvent 三端签名不一。统一后各端 api.ts 为薄委托层。

import type {
  ClassAnalytics,
  CommunityCircle,
  CommunityComment,
  CommunityCreateResult,
  CommunityMember,
  CommunityPost,
  CommunityReportResult,
  ContentAnswerResult,
  ContentCourse,
  ContentItem,
  ContentLesson,
  DifficultyResp,
  GradeResult,
  LessonPlan,
  LoginResp,
  Mastery,
  PolishResult,
  RankingGap,
  RankingResponse,
  RankPeriod,
  RankScope,
  RankType,
  RegionOption,
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
export { LocalStorageTokenStorage, CookieTokenStorage } from "./storage";

// 管理端类型(探活/Prometheus/治理)
export interface AdminProbe {
  name: string; kind: string; lang: string; svc: string; port: number;
  path: string; status: string; code?: number; msg?: string; latency_ms: number;
}
export interface AdminProbeResponse {
  kind: string; checked_at: string; summary: { total: number; up: number; down: number }; items: AdminProbe[];
}
export interface AdminCatalog {
  services: AdminProbe[]; infra: AdminProbe[];
}
export interface AdminNacosService {
  name: string; group: string; instances: number; healthy: number;
}
export interface AdminSentinelRule {
  resource: string; grade: number; count: number; strategy: number; controlBehavior?: number;
}
// 中间件统一访问面板(BFF 代理,2026-08-19):各中间件在线状态。
export interface AdminMiddlewareHealth {
  name: string; kind: string; addr: string;
  status: string; latency_ms: number; msg?: string;
}
export interface AdminMiddlewareHealthResponse {
  kind: string; checked_at: string;
  summary: { total: number; up: number; down: number };
  items: AdminMiddlewareHealth[];
}

// 全语言统一深度诊断面板类型(Step4 · actuator 风格全 6 endpoint)
export interface AdminDebugCapabilities {
  capabilities: Record<string, string[]>;
  endpoints: string[];
  note: string;
}
export interface AdminDebugLoggers {
  configuredLevel: string;
  effectiveLevel?: string;
  loggers?: Record<string, string>;
  status?: string;
  error?: string;
}

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

  // ---- content:统一内容库(功能拓展阶段1;BFF 注入 X-User-Id/X-Role) ----
  listCourses(examTag?: string, status = "published"): Promise<ContentCourse[]> {
    const q = new URLSearchParams();
    if (examTag) q.set("exam_tag", examTag);
    q.set("status", status);
    return this.request("GET", `/api/content/courses?${q.toString()}`);
  }
  getCourse(id: number | string): Promise<ContentCourse> {
    return this.request("GET", `/api/content/courses/${id}`);
  }
  listLessons(courseId: number | string): Promise<ContentLesson[]> {
    return this.request("GET", `/api/content/courses/${courseId}/lessons`);
  }
  listItems(lessonId: number | string): Promise<ContentItem[]> {
    return this.request("GET", `/api/content/lessons/${lessonId}/items`);
  }
  submitAnswer(
    itemId: number | string,
    answers: Record<string, unknown>,
    durationMs = 0
  ): Promise<ContentAnswerResult> {
    return this.request("POST", `/api/content/items/${itemId}/answer`, {
      answers,
      duration_ms: durationMs,
    });
  }
  // 管理端 CRUD(role=admin 才放行)
  createCourse(body: Record<string, unknown>): Promise<{ id: number }> {
    return this.request("POST", "/api/content/courses", body);
  }
  updateCourse(id: number | string, body: Record<string, unknown>): Promise<{ id: number }> {
    return this.request("PUT", `/api/content/courses/${id}`, body);
  }
  createLesson(body: Record<string, unknown>): Promise<{ id: number }> {
    return this.request("POST", "/api/content/lessons", body);
  }
  createItem(body: Record<string, unknown>): Promise<{ id: number }> {
    return this.request("POST", "/api/content/items", body);
  }
  updateItem(id: number | string, body: Record<string, unknown>): Promise<{ id: number }> {
    return this.request("PUT", `/api/content/items/${id}`, body);
  }
  importContent(items: unknown[]): Promise<{ created: number }> {
    return this.request("POST", "/api/content/import", items);
  }

  // ---- admin:运维监测(探活/Prometheus/治理) ----
  adminServices(): Promise<AdminProbeResponse> {
    return this.request("GET", "/api/admin/services");
  }
  adminInfra(): Promise<AdminProbeResponse> {
    return this.request("GET", "/api/admin/infra");
  }
  adminCatalog(): Promise<AdminCatalog> {
    return this.request("GET", "/api/admin/catalog");
  }
  adminAlerts(): Promise<{ status: string; data: { alerts: unknown[] } }> {
    return this.request("GET", "/api/admin/metrics/alerts");
  }
  adminPromQuery(query: string): Promise<{ data: { result: unknown[] } }> {
    return this.request("GET", `/api/admin/metrics/query?query=${encodeURIComponent(query)}`);
  }
  adminGovernanceNacos(): Promise<{ services: AdminNacosService[] }> {
    return this.request("GET", "/api/admin/governance/nacos");
  }
  adminGovernanceSentinel(): Promise<{ rules: Record<string, AdminSentinelRule[]> }> {
    return this.request("GET", "/api/admin/governance/sentinel");
  }
  // 中间件统一访问面板(BFF 代理,2026-08-19):聚合各中间件在线状态。
  // 控制台链接走 /api/admin/middleware/<name>/... 由 admin-go 反向代理,前端不持宿主 IP。
  adminMiddlewareHealth(): Promise<AdminMiddlewareHealthResponse> {
    return this.request("GET", "/api/admin/middleware/health");
  }

  // ---- admin:全语言统一深度诊断面板(Step4 · actuator 风格全 6 endpoint) ----
  // 全 6 endpoint:env / loggers / threaddump / heapdump / beans / mappings。
  // Java=actuator 原生全 6;Go pprof 等价 threaddump+heapdump;Python/Rust 无原生 dump(标 NA)。
  adminDebugCapabilities(): Promise<AdminDebugCapabilities> {
    return this.request("GET", "/api/admin/debug/capabilities");
  }
  /** 查询某服务某诊断 endpoint(env/loggers/threaddump/beans/mappings 返回 JSON;heapdump 返回二进制)。 */
  adminDebugEndpoint(service: string, endpoint: string): Promise<unknown> {
    return this.request("GET", `/api/admin/debug/${service}/${endpoint}`);
  }
  /** heapdump 下载(Java only):返回 Blob,前端 a 下载。 */
  async adminDebugHeapdump(service: string): Promise<Blob> {
    const token = this.storage.getToken();
    const headers: Record<string, string> = { traceparent: traceparent() };
    if (token) headers.Authorization = `Bearer ${token}`;
    const res = await this.transport.request({
      method: "GET",
      url: `${this.base}/api/admin/debug/${service}/heapdump`,
      headers,
      timeoutMs: 60000,
    });
    if (res.status >= 400) throw new ApiError(res.status, res.body, `HTTP ${res.status}: heapdump`);
    // body 是二进制 dump 的 base64 文本传输(FetchTransport 返 text);此处降级为 JSON 解析失败兜底。
    // 真实二进制下载走浏览器原生 fetch(Blob)以确保字节完整。
    const r = await fetch(`${this.base}/api/admin/debug/${service}/heapdump`, { headers });
    return r.blob();
  }
  /** 动态调日志级别:POST ?level=DEBUG 或 body {configuredLevel}。 */
  adminDebugSetLogLevel(service: string, level: string): Promise<AdminDebugLoggers> {
    return this.request("POST", `/api/admin/debug/${service}/loggers?level=${encodeURIComponent(level)}`);
  }

  // ---- community:圈子社区(功能拓展阶段2) ----
  createCircle(body: { name: string; description?: string; member_limit?: number }): Promise<CommunityCreateResult> {
    return this.request("POST", "/api/community/circles", body);
  }
  listCircles(q = "", mine = false): Promise<CommunityCircle[]> {
    const params = new URLSearchParams();
    if (q) params.set("q", q);
    return this.request("GET", mine ? "/api/community/circles/mine" : `/api/community/circles?${params.toString()}`);
  }
  getCircle(id: number | string): Promise<CommunityCircle> {
    return this.request("GET", `/api/community/circles/${id}`);
  }
  listCircleMembers(id: number | string): Promise<CommunityMember[]> {
    return this.request("GET", `/api/community/circles/${id}/members`);
  }
  joinCircle(id: number | string, inviteCode: string): Promise<{ id: number; joined?: boolean; already_member?: boolean }> {
    return this.request("POST", `/api/community/circles/${id}/join`, { invite_code: inviteCode });
  }
  leaveCircle(id: number | string): Promise<{ id: number; left: boolean }> {
    return this.request("POST", `/api/community/circles/${id}/leave`, {});
  }
  createPost(circleId: number | string, body: string): Promise<{ id: number }> {
    return this.request("POST", `/api/community/circles/${circleId}/posts`, { body });
  }
  listPosts(circleId: number | string): Promise<CommunityPost[]> {
    return this.request("GET", `/api/community/circles/${circleId}/posts`);
  }
  createComment(postId: number | string, body: string): Promise<{ id: number }> {
    return this.request("POST", `/api/community/posts/${postId}/comments`, { body });
  }
  listComments(postId: number | string): Promise<CommunityComment[]> {
    return this.request("GET", `/api/community/posts/${postId}/comments`);
  }
  reportContent(target_type: "post" | "comment", target_id: number, reason: string): Promise<CommunityReportResult> {
    return this.request("POST", "/api/community/reports", { target_type, target_id, reason });
  }

  // ---- gamification 排行榜(功能拓展阶段3;type=xp|prog,scope=global|region|circle|category) ----
  rankings(params: {
    type?: RankType;
    scope?: RankScope;
    scope_id?: string;
    period?: RankPeriod;
    limit?: number;
  }): Promise<RankingResponse> {
    const q = new URLSearchParams();
    if (params.type) q.set("type", params.type);
    if (params.scope) q.set("scope", params.scope);
    if (params.scope_id) q.set("scope_id", params.scope_id);
    if (params.period) q.set("period", params.period);
    if (params.limit) q.set("limit", String(params.limit));
    return this.request("GET", `/api/gamification/rankings?${q.toString()}`);
  }
  rankingGap(params: {
    type?: RankType;
    scope?: RankScope;
    scope_id?: string;
    period?: RankPeriod;
  }): Promise<RankingGap> {
    const q = new URLSearchParams();
    if (params.type) q.set("type", params.type);
    if (params.scope) q.set("scope", params.scope);
    if (params.scope_id) q.set("scope_id", params.scope_id);
    if (params.period) q.set("period", params.period);
    return this.request("GET", `/api/gamification/rankings/gap?${q.toString()}`);
  }
  rankingRegions(): Promise<RegionOption[]> {
    return this.request("GET", "/api/gamification/rankings/regions");
  }
  setRankingOptOut(optOut: boolean): Promise<{ opt_out: boolean }> {
    return this.request("POST", "/api/gamification/users/optout", { opt_out: optOut });
  }
  // 数字人形象偏好(per-user,跨设备持久化;X-User-Id 由 BFF 注入)
  getDigitalHuman(): Promise<{ persona_id: string }> {
    return this.request("GET", "/api/gamification/me/digital-human");
  }
  setDigitalHuman(persona_id: string): Promise<{ persona_id: string }> {
    return this.request("PUT", "/api/gamification/me/digital-human", { persona_id });
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
  // P1-13 cookie 模式:无本地 token 时不拼 ?token=(WS 握手同源自动带 httpOnly cookie,
  // gateway-go 侧已兼容从 esp_token cookie 验签);小程序/Bearer 模式仍拼 ?token=。
  speechWsUrl(sessionId: string): string {
    const base = `${this.base.replace(/^http/, "ws")}/ws/speech/${sessionId}`;
    const token = this.storage.getToken();
    return token ? `${base}?token=${encodeURIComponent(token)}` : base;
  }
}

export type { WsFrame };
