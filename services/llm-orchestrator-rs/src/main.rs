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
    response::{IntoResponse, Response},
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::env;
use std::net::SocketAddr;
use std::sync::Arc;
use tracing_subscriber::EnvFilter;

const SERVICE_NAME: &str = "llm-orchestrator-rs";

/// 全局应用状态:连接池 + provider 注册表
pub struct AppState {
    // Redis Cluster 异步连接(支持 MOVED/ASK 自动重定向)。
    // ClusterConnection 实现 ConnectionLike + Clone,可直接 query_async。
    pub redis: redis::cluster_async::ClusterConnection,
    pub mysql: sqlx::MySqlPool,
    pub providers: provider::ProviderRegistry,
}

#[derive(Clone)]
pub struct AppHandle(Arc<AppState>);

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")))
        .init();

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
        .with_state(handle);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    let listener = tokio::net::TcpListener::bind(addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}

async fn init_state() -> AppState {
    let redis_addr = env::var("REDIS_ADDR").unwrap_or_else(|_| "redis://redis-cluster:6379".into());
    // ESP 用真 Redis Cluster(3 master 分片)。单连接 ConnectionManager 不处理 MOVED 重定向,
    // 导致 SETEX/ZADD/SET NX 写到非本节点 key 时静默失败。改用 ClusterClient,自动跟随 MOVED/ASK。
    // ClusterClient 接受单个 seed 节点 URL,会自动发现集群拓扑。
    let redis_client = redis::cluster::ClusterClient::new(vec![redis_addr.as_str()])
        .expect("redis cluster client");
    let redis = redis_client
        .get_async_connection()
        .await
        .expect("redis cluster async connection");

    let mysql_url = env::var("TIDB_DSN").unwrap_or_else(|_| {
        "mysql://root@tidb-primary:4000/esp_llm".into()
    });
    let mysql = sqlx::MySqlPool::connect(&mysql_url)
        .await
        .expect("tidb esp_llm pool");

    let providers = provider::ProviderRegistry::from_env();

    AppState { redis, mysql, providers }
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

    // TiDB ping
    let tidb_ok = sqlx::query("SELECT 1").execute(&h.0.mysql).await.is_ok();
    checks.insert("tidb".into(), json!(if tidb_ok { "UP" } else { "DOWN" }));

    let ok = redis_ok && tidb_ok;
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
            &h.0.mysql,
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

    // 调 provider
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
    let (provider, model, status, error_msg) = match &result {
        Ok(r) => (r.provider.clone(), r.model.clone(), "ok".to_string(), None),
        Err(e) => {
            let (p, m) = h.0.providers.fallback_names(req.provider.as_deref(), req.model.as_deref());
            (p, m, "error".to_string(), Some(e.to_string()))
        }
    };

    // 记成本(失败也记)
    let _ = cost::record(
        &h.0.mysql,
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
