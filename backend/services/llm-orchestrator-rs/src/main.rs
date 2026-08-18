// llm-orchestrator-rs —— LLM 编排网关 + dialogue 子模块(S1, P0)
// 主干:provider 路由/降级 + 语义缓存(Redis L2)+ 成本治理(llm_call_logs)
// dialogue:session/turn/persona CRUD + SSE 流式 /api/dialogue/chat
// ADR: docs/design/adr-new-services.md / PRD §一 / schema-per-service-phase0.sql esp_llm

pub mod cache;
pub mod cost;
pub mod dialogue;
pub mod provider;

use axum::{
    body::Body,
    extract::State,
    http::{HeaderMap, HeaderValue},
    middleware,
    response::{IntoResponse, Response},
    routing::{get, post},
    Json, Router,
};
use governor::{Quota, RateLimiter as GovernorLimiter};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::env;
use std::net::SocketAddr;
use std::num::NonZeroU32;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, LazyLock};
use tokio::sync::Semaphore;

const SERVICE_NAME: &str = "llm-orchestrator-rs";

// ---------- governor 令牌桶限流(替代手写 AtomicU64) ----------
type AppRateLimiter = GovernorLimiter<governor::state::NotKeyed, governor::state::InMemoryState, governor::clock::DefaultClock>;
static RATE_LIMITER: LazyLock<AppRateLimiter> = LazyLock::new(|| {
    GovernorLimiter::direct(Quota::per_second(NonZeroU32::new(100).unwrap()))
});

/// Rate-limit middleware: skip health endpoints
async fn rate_limit_middleware(req: axum::http::Request<Body>, next: axum::middleware::Next) -> axum::response::Response {
    let path = req.uri().path().to_string();
    if path == "/healthz" || path == "/readyz" || path == "/livez" || path.starts_with("/actuator/health") {
        return next.run(req).await;
    }
    if !RATE_LIMITER.check().is_ok() {
        return (
            axum::http::StatusCode::TOO_MANY_REQUESTS,
            Json(json!({"error": "rate limit exceeded", "retry_after": 1})),
        ).into_response();
    }
    next.run(req).await
}

// ---------- 自适应并发限制器(gradient2 思路) ----------
/// 低 RTT 放量(+1),高 RTT 保持。进程内 tokio Semaphore,无需 Redis 共享。
pub struct AdaptiveLimiter {
    semaphore: Arc<Semaphore>,
    initial: usize,
    max_concurrency: AtomicUsize,
    min_rtt_ms: AtomicUsize, // 整数化避免 atomic f64(精度足够:0.1ms)
}

impl AdaptiveLimiter {
    fn new(initial: usize) -> Self {
        Self {
            semaphore: Arc::new(Semaphore::new(initial)),
            initial,
            max_concurrency: AtomicUsize::new(initial),
            min_rtt_ms: AtomicUsize::new(u32::MAX as usize),
        }
    }

    /// 获取 permit(阻塞直到有空位)
    pub async fn acquire(&self) -> tokio::sync::OwnedSemaphorePermit {
        self.semaphore.clone().acquire_owned().await
            .unwrap_or_else(|_| {
                tracing::warn!("adaptive limiter semaphore closed");
                self.semaphore.clone().try_acquire_owned().unwrap()
            })
    }

    /// 释放 permit + 根据 RTT 调整并发上限
    fn release_with_rtt(&self, rtt: std::time::Duration) {
        let rtt_ms = (rtt.as_secs_f64() * 1000.0) as usize;
        if rtt_ms == 0 { return; }
        let prev_min = self.min_rtt_ms.load(Ordering::Relaxed);
        if prev_min == u32::MAX as usize || rtt_ms < prev_min {
            self.min_rtt_ms.store(rtt_ms, Ordering::Relaxed);
        }
        // RTT < min * 1.5 → 低负载信号,尝试扩容(最多 2x)
        let min = self.min_rtt_ms.load(Ordering::Relaxed);
        if rtt_ms < min * 3 / 2 {
            let cur = self.max_concurrency.load(Ordering::Relaxed);
            let target = (self.initial * 2).min(cur + 1);
            if target > cur {
                self.max_concurrency.store(target, Ordering::Relaxed);
                // 重建更大 Semaphore(旧的等释放后 GC;新增 permit 立即可用)
                let old = self.semaphore.clone();
                self.semaphore = Arc::new(Semaphore::new(target));
                tracing::info!("adaptive limiter: max_concurrency {} -> {} (rtt={}ms, min={}ms)", cur, target, rtt_ms, min);
                // 注:旧 Semaphore 仍持有可能的等待者,不丢弃;新 Semaphore 新 permit 可立即获取
                let _ = old; // prevent drop warning; old permits still valid
            }
        }
    }

    /// 当前最大并发数(监控用)
    pub fn max_permits(&self) -> usize {
        self.max_concurrency.load(Ordering::Relaxed)
    }
}

/// 全局应用状态:连接池 + provider 注册表 + LLM 并发限制器
pub struct AppState {
    // Redis Cluster 异步连接(支持 MOVED/ASK 自动重定向)。
    // ClusterConnection 实现 ConnectionLike + Clone,可直接 query_async。
    pub redis: redis::cluster_async::ClusterConnection,
    pub pg: sqlx::PgPool,
    pub providers: provider::ProviderRegistry,
    /// LLM 调用并发限制器(gradient2 自适应):低 RTT 放量、高 RTT 缩容。
    /// 单节点进程内 tokio::sync::Semaphore,无需 Redis 共享。
    pub llm_limiter: Arc<AdaptiveLimiter>,
}

#[derive(Clone)]
pub struct AppHandle(Arc<AppState>);

#[tokio::main]
async fn main() {
    esp_common::tracing_init::init(SERVICE_NAME);

    let port: u16 = env::var("PORT").unwrap_or_else(|_| "8086".into()).parse().unwrap_or(8086);
    tracing::info!("[{}] starting on :{}", SERVICE_NAME, port);

    let state = init_state().await;
    let handle = AppHandle(Arc::new(state));

    let app = Router::new()
        .route("/healthz", get(health))
        .route("/readyz", get(readiness))
        .route("/livez", get(liveness))
        // LLM 主干:统一入口
        .route("/api/llm/chat", post(llm_chat))
        // dialogue 子模块
        .route("/api/dialogue/chat", post(dialogue::chat_sse))
        .route("/api/dialogue/sessions", get(dialogue::list_sessions).post(dialogue::create_session))
        .route("/api/dialogue/sessions/:id", get(dialogue::get_session))
        .route("/api/dialogue/sessions/:id/turns", get(dialogue::list_turns))
        .route("/api/dialogue/personas", get(dialogue::list_personas).post(dialogue::create_persona))
        .with_state(handle)
        .layer(middleware::from_fn(rate_limit_middleware));

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    let listener = tokio::net::TcpListener::bind(addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}

async fn init_state() -> AppState {
    // Redis:ESP 用真 Redis Cluster(3 master 分片)。ClusterConnection 自动处理 MOVED/ASK。
    // env=REDIS_ADDR,缺省 redis://redis-cluster:6379(对齐原默认值)。
    let redis_addr = env::var("REDIS_ADDR").unwrap_or_else(|_| esp_common::redisx::DEFAULT_ADDR.into());
    let redis = esp_common::redisx::connect_required(&redis_addr, SERVICE_NAME).await;

    // CockroachDB 单 pool(env=LLM_DB_URL,缺省 crdb-public esp_llm)。#140 全服务切 CRDB。
    let default_dsn = "postgres://root@crdb-public:26257/esp_llm?sslmode=disable".to_string();
    let pg = esp_common::db::connect("LLM_DB_URL", default_dsn, SERVICE_NAME)
        .await
        .expect("crdb esp_llm pool");

    let providers = provider::ProviderRegistry::from_env();

    // LLM 并发限制(gradient2 自适应):env LLM_MAX_CONCURRENCY 控制初始值(默认 10)
    let max_concurrency: usize = env::var("LLM_MAX_CONCURRENCY")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(10);
    tracing::info!("[{}] llm adaptive limiter: initial={}", SERVICE_NAME, max_concurrency);

    AppState { redis, pg, providers, llm_limiter: Arc::new(AdaptiveLimiter::new(max_concurrency)) }
}

// ---------------- health ----------------

async fn health() -> Json<Value> {
    Json(json!({"status": "UP", "service": SERVICE_NAME}))
}

async fn readiness(State(h): State<AppHandle>) -> Json<Value> {
    let mut checks = serde_json::Map::new();

    // Redis ping
    let redis_ok = {
        let mut conn = h.0.redis.clone();
        redis::cmd("PING").query_async::<String>(&mut conn).await.is_ok()
    };
    checks.insert("redis".into(), json!(if redis_ok { "UP" } else { "DOWN" }));

    // CRDB ping
    let crdb_ok = sqlx::query("SELECT 1").execute(&h.0.pg).await.is_ok();
    checks.insert("crdb".into(), json!(if crdb_ok { "UP" } else { "DOWN" }));

    let ok = redis_ok && crdb_ok;
    Json(json!({"status": if ok { "UP" } else { "DOWN" }, "checks": checks}))
}

async fn liveness() -> Json<Value> {
    Json(json!({"status": "UP"}))
}

// ---------------- LLM 主干 ----------------

#[derive(Debug, Deserialize)]
struct LlmChatReq {
    prompt: String,
    #[serde(default)]
    model: Option<String>,
    #[serde(default)]
    provider: Option<String>,
    #[serde(default)]
    scene: Option<String>,
    #[serde(default)]
    user_id: Option<i64>,
    #[serde(default)]
    stream: Option<bool>,
    #[serde(default)]
    max_tokens: Option<i32>,
}

#[derive(Debug, Serialize)]
struct LlmChatResp {
    content: String,
    provider: String,
    model: String,
    cache_hit: bool,
    trace_id: String,
}

/// POST /api/llm/chat —— 统一 LLM 入口
/// 流程:traceId 透传 → 幂等(X-Request-Id)→ 语义缓存查 Redis → 命中直返(标 cache_hit) → 否则调 provider → 回写缓存 + 幂等回写 + 记 llm_call_logs
async fn llm_chat(
    State(h): State<AppHandle>,
    headers: HeaderMap,
    Json(req): Json<LlmChatReq>,
) -> Response {
    let trace_id = headers
        .get("x-trace-id")
        .and_then(|v| v.to_str().ok())
        .map(|s| s.to_string())
        .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());

    // 幂等:X-Request-Id(缺失生成 uuid),SETNX esp:idem:llm:chat:<rid> TTL 60s
    // 命中则返回上次缓存的响应体(idempotent_replay=true);异常时降级放行,不阻断业务
    let request_id = headers
        .get("x-request-id")
        .and_then(|v| v.to_str().ok())
        .map(|s| s.to_string())
        .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());
    let idem_key = format!("esp:idem:llm:chat:{}", request_id);
    {
        let mut conn = h.0.redis.clone();
        match redis::cmd("GET").arg(&idem_key).query_async::<Option<String>>(&mut conn).await {
            Ok(Some(cached)) if !cached.is_empty() => {
                tracing::info!(trace_id = %trace_id, request_id = %request_id, "llm idempotent replay");
                if let Ok(v) = serde_json::from_str::<Value>(&cached) {
                    return Json(v).into_response();
                }
                // 解析失败则放行,执行业务
            }
            Ok(_) => {}
            Err(e) => tracing::warn!(trace_id = %trace_id, "llm idem read failed (non-fatal): {}", e),
        }
    }

    let scene = req.scene.clone().unwrap_or_else(|| "default".into());

    // L2 双轨语义缓存查 Redis(L1 精确 → L2 SimHash 近似)
    if let Some(cached) = cache::get(&h.0.redis, &req.prompt, req.model.as_deref(), &scene).await {
        tracing::info!(trace_id = %trace_id, "llm cache hit");
        cost::record(
            &h.0.pg,
            &cost::CallLog {
                trace_id: trace_id.clone(),
                user_id: req.user_id,
                provider: "cache".into(),
                model: req.model.clone().unwrap_or_else(|| "unknown".into()),
                scene: scene.clone(),
                prompt_tokens: 0,
                completion_tokens: 0,
                total_tokens: 0,
                latency_ms: 0,
                cost_usd: 0.0,
                cache_hit: 1,
                status: "ok".into(),
                error_msg: None,
            },
        )
        .await
        .ok();
        let resp = LlmChatResp {
            content: cached,
            provider: "cache".into(),
            model: req.model.unwrap_or_else(|| "unknown".into()),
            cache_hit: true,
            trace_id,
        };
        return Json(resp).into_response();
    }

    // 调 provider:自适应并发限制(acquire → 调用 → release_with_rtt)
    let _permit = h.0.llm_limiter.acquire().await;
    let started = std::time::Instant::now();
    let result = h.0.providers
        .call(
            req.provider.as_deref(),
            req.model.as_deref(),
            &req.prompt,
            req.max_tokens,
            req.stream.unwrap_or(false),
            &trace_id,
        )
        .await;

    let latency_ms = started.elapsed().as_millis() as i32;
    // 释放 permit(自动 drop) + 自适应调整并发上限
    h.0.llm_limiter.release_with_rtt(started.elapsed());
    drop(_permit);
    let (provider, model, status, error_msg) = match &result {
        Ok(r) => (r.provider.clone(), r.model.clone(), "ok".to_string(), None),
        Err(e) => {
            let (p, m) = h.0.providers.fallback_names(req.provider.as_deref(), req.model.as_deref());
            (p, m, "error".to_string(), Some(e.to_string()))
        }
    };

    // 记成本(失败也记)
    let _ = cost::record(
        &h.0.pg,
        &cost::CallLog {
            trace_id: trace_id.clone(),
            user_id: req.user_id,
            provider: provider.clone(),
            model: model.clone(),
            scene: scene.clone(),
            prompt_tokens: 0,
            completion_tokens: 0,
            total_tokens: 0,
            latency_ms,
            cost_usd: 0.0,
            cache_hit: 0,
            status: status.clone(),
            error_msg: error_msg.clone(),
        },
    )
    .await;

    match result {
        Ok(r) => {
            // 回写缓存(L1 精确 + L2 语义索引)
            cache::set(&h.0.redis, &req.prompt, req.model.as_deref(), &scene, &r.content, 3600).await.ok();
            let resp = LlmChatResp {
                content: r.content.clone(),
                provider: r.provider.clone(),
                model: r.model.clone(),
                cache_hit: false,
                trace_id: trace_id.clone(),
            };
            // 幂等回写:存响应体,60s 窗口内同 requestId 直接返回;失败仅 log
            let payload = serde_json::to_string(&resp).unwrap_or_else(|_| "{}".into());
            let mut conn = h.0.redis.clone();
            if let Err(e) = redis::cmd("SET")
                .arg(&idem_key).arg(&payload).arg("NX").arg("EX").arg(60u32)
                .query_async::<()>(&mut conn).await
            {
                tracing::warn!(trace_id = %trace_id, "llm idem write failed (non-fatal): {}", e);
            }
            Json(resp).into_response()
        }
        Err(e) => {
            let body = Json(json!({"error": e.to_string(), "trace_id": trace_id}));
            (axum::http::StatusCode::BAD_GATEWAY, body).into_response()
        }
    }
}

/// SSE 响应 helper:把流式 token 包装成 text/event-stream
#[allow(dead_code)]
pub fn sse_response(stream: impl tokio_stream::Stream<Item = Result<String, std::io::Error>> + Send + 'static) -> Response {
    let body = Body::from_stream(stream);
    let mut resp = Response::new(body);
    resp.headers_mut().insert("content-type", HeaderValue::from_static("text/event-stream"));
    resp.headers_mut().insert("cache-control", HeaderValue::from_static("no-cache"));
    resp
}
