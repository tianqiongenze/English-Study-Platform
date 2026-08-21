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

// 运维操作面板类型(Phase3 · 2026-08-21):资源监控 + 日志 + scale/restart。
export interface AdminOpsMetric {
  query: string;
  value: string;
  status: string;
}
export interface AdminOpsResources {
  kind: string;
  checked_at: string;
  metrics: Record<string, AdminOpsMetric>;
  note?: string;
}
export interface AdminOpsResult {
  action: string;
  service: string;
  replicas?: string;
  status: string;
  code: number;
  response?: string;
}

// 各级缓存监控类型(Phase4 · 2026-08-21):L1 Redis + L2 Milvus + L3 Neo4j。
export interface AdminCacheLayerStat {
  layer: string;
  name: string;
  hits: string;
  misses: string;
  errors: string;
  hit_rate: string;
  status: string;
  extra?: Record<string, string>;
  msg?: string;
}
export interface AdminCacheStatsResponse {
  kind: string;
  checked_at: string;
  layers: AdminCacheLayerStat[];
  lco_stats?: unknown;
  note?: string;
}
export interface AdminMilvusResponse {
  kind: string;
  checked_at: string;
  collection: string;
  num_entities: string;
  status: string;
  msg?: string;
}
export interface AdminNeo4jResponse {
  kind: string;
  checked_at: string;
  nodes: Record<string, string>;
  relationships: Record<string, string>;
  status: string;
  msg?: string;
}

// Pulsar MQ 消息查看类型(Phase4 · 2026-08-21)。
export interface AdminPulsarSubscription {
  name: string;
  msg_backlog: number;
  msg_rate_out: string;
  msg_rate_redeliver: string;
  type: string;
}
export interface AdminPulsarTopic {
  topic: string;
  fqn: string;
  subscriptions?: AdminPulsarSubscription[];
  backlog: number;
  producers: number;
  msg_rate_in: string;
  msg_rate_out: string;
  status: string;
}
export interface AdminPulsarTopicsResponse {
  kind: string;
  checked_at: string;
  namespace: string;
  topics: AdminPulsarTopic[];
  note?: string;
}
export interface AdminPulsarMessage {
  message_id: string;
  publish_time: string;
  producer: string;
  payload: unknown;
  payload_raw: string;
}
export interface AdminPulsarMessagesResponse {
  kind: string;
  topic: string;
  subscription: string;
  messages: AdminPulsarMessage[];
  note?: string;
}

// 服务运行时健康类型(Phase4 · 2026-08-21):HPA + 熔断 + 限流 + API 错误率。
export interface AdminHPAStatus {
  name: string;
  minReplicas: number;
  maxReplicas: number;
  currentReplicas: number;
  desiredReplicas: number;
  targetCpu: string;
  currentCpu: string;
  scaling: boolean;
  status: string;
}
export interface AdminHPAResponse {
  kind: string;
  checked_at: string;
  items: AdminHPAStatus[];
  note?: string;
  error?: string;
}
export interface AdminResilienceStatus {
  service: string;
  cb_state: string;
  cb_trips: string;
  rl_allowed: string;
  rl_rejected: string;
  status: string;
}
export interface AdminResilienceResponse {
  kind: string;
  checked_at: string;
  items: AdminResilienceStatus[];
  note?: string;
}
export interface AdminApiStatus {
  service: string;
  status: string;
  req_count: string;
  err_5xx: string;
  err_rate: string;
}
export interface AdminApisResponse {
  kind: string;
  checked_at: string;
  items: AdminApiStatus[];
  note?: string;
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

  // ---- admin:运维操作面板(Phase3 · 2026-08-21) ----
  // 资源监控 + 实时日志 + 可管理操作(scale/restart)。
  // 资源监控走 Prometheus 查询(admin-go 聚合);日志/scale/restart 走 K8s API(需 esp-admin-sa RBAC)。
  /** 资源监控快照:节点 CPU/内存/磁盘/负载 + 容器级聚合 + pod 就绪(5s 刷新)。 */
  adminOpsResources(): Promise<AdminOpsResources> {
    return this.request("GET", "/api/admin/ops/resources");
  }
  /** 实时日志(tail=N,默认 200 行,kubectl logs 等价)。 */
  adminLogs(service: string, tail = 200): Promise<string> {
    return this.request("GET", `/api/admin/logs/${service}?tail=${tail}`, undefined, { timeoutMs: 30000 }) as Promise<string>;
  }
  /** scale deploy(L2 不可逆,前端需二次确认)。 */
  adminScale(service: string, replicas: number): Promise<AdminOpsResult> {
    return this.request("POST", `/api/admin/scale/${service}?replicas=${replicas}`);
  }
  /** rollout restart(L2 不可逆,前端需二次确认)。 */
  adminRestart(service: string): Promise<AdminOpsResult> {
    return this.request("POST", `/api/admin/restart/${service}`);
  }

  // ---- admin:各级缓存监控(Phase4 · 2026-08-21) ----
  // L1 Redis + L2 Milvus + L3 Neo4j 命中率 + 结构数据;三源独立超时降级(RWT)。
  /** 三层缓存命中率聚合(LCO /stats + Milvus collection + Neo4j Cypher,5s 刷新)。 */
  adminCacheStats(): Promise<AdminCacheStatsResponse> {
    return this.request("GET", "/api/admin/cache/stats");
  }
  /** Milvus collection 记录数(llm_cache_prompts)。 */
  adminCacheMilvus(): Promise<AdminMilvusResponse> {
    return this.request("GET", "/api/admin/cache/milvus");
  }
  /** Neo4j 节点/关系数(Cypher 聚合)。 */
  adminCacheNeo4j(): Promise<AdminNeo4jResponse> {
    return this.request("GET", "/api/admin/cache/neo4j");
  }

  // ---- admin:Pulsar MQ 消息查看(Phase4 · 2026-08-21) ----
  // Admin REST 8080 代理(无认证)+ 白名单 tenant=esp/ns=esp{business,cache};peek 只读不 ack(BRR)。
  /** Pulsar topic 列表 + 积压/订阅(esp/business ns)。 */
  adminPulsarTopics(namespace = "esp/business"): Promise<AdminPulsarTopicsResponse> {
    return this.request("GET", `/api/admin/pulsar/topics?namespace=${encodeURIComponent(namespace)}`);
  }
  /** Pulsar peek 消息(只读,不 ack)。 */
  adminPulsarMessages(topic: string, subscription: string, limit = 10): Promise<AdminPulsarMessagesResponse> {
    const q = new URLSearchParams({ topic, subscription, limit: String(limit) });
    return this.request("GET", `/api/admin/pulsar/messages?${q.toString()}`);
  }

  // ---- admin:服务运行时健康(Phase4 · 2026-08-21) ----
  // HPA(K8s API 直查)+ 熔断/限流(admin-go 直抓各服务 /metrics)+ API 错误率(Java actuator)。
  /** HPA 状态(min/max/current/desired/CPU%,19 个全量)。 */
  adminRuntimeHPA(): Promise<AdminHPAResponse> {
    return this.request("GET", "/api/admin/runtime/hpa");
  }
  /** 熔断状态 + 限流计数(各 Go 服务 circuit_breaker_state + rate_limit_total)。 */
  adminRuntimeResilience(): Promise<AdminResilienceResponse> {
    return this.request("GET", "/api/admin/runtime/resilience");
  }
  /** API 错误率(Java http.server.requests 计数 + 服务级 probeAll)。 */
  adminRuntimeApis(): Promise<AdminApisResponse> {
    return this.request("GET", "/api/admin/runtime/apis");
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
  // AI 推理 8-34s(writing-py LangGraph),默认 15s timeout 不够致 AbortError,统一 90s。
  grade(prompt: string, essay: string, exam: string): Promise<GradeResult> {
    return this.request("POST", "/api/writing/grade", { prompt, essay, exam }, { timeoutMs: 90000 });
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
    return this.request("POST", "/api/writing/run", { prompt, essay, exam, human }, { timeoutMs: 90000 });
  }
  writingReview(thread_id: string, teacher_feedback: string): Promise<{
    thread_id: string;
    status: string;
    notes?: string;
  }> {
    return this.request("POST", "/api/writing/review", { thread_id, teacher_feedback }, { timeoutMs: 90000 });
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
    return this.request("POST", "/api/writing/polish", { text }, { timeoutMs: 90000 });
  }
  translate(text: string): Promise<TranslateResult> {
    return this.request("POST", "/api/writing/translate", { text }, { timeoutMs: 90000 });
  }

  // ---- teacher(仅 Web 消费) ----
  lessonPlan(topic: string, level: string): Promise<LessonPlan> {
    return this.request("POST", "/api/teacher/lesson-plan", { topic, level }, { timeoutMs: 90000 });
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

  // ---- AI 助手(全前端 AI 驱动,经 gateway LCO 三层缓存) ----
  // POST /api/llm/chat → gateway-go(srv.engine.LLM = llm.NewWithCache,L1 Redis + L2 Milvus + L3 Neo4j)。
  // 90s 超时(AI 推理 8-34s + 缓存命中 ~10ms)。
  llmChat(message: string, role?: string): Promise<LlmChatResult> {
    return this.request("POST", "/api/llm/chat", { message, role }, { timeoutMs: 90000 });
  }
}

export type { WsFrame };

// AI 助手对话结果(经 gateway LCO 三层缓存)。
export interface LlmChatResult {
  reply: string;
  cache_hit?: boolean;
  cache_layer?: string;
  error?: string;
}
