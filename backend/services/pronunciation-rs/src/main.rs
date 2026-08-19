// pronunciation-rs —— 音素级发音评分服务(S3, P0)
// 主干:GOP(Goodness of Pronunciation)骨架 + 音标/重音/语调纠错桩 + 评分落库
// scaffold 级:健康检查 + 评分路由桩 + esp_pronunciation.pronunciation_attempts 写入
// ADR: docs/design/adr-new-services.md / PRD §三 / schema esp_pronunciation
//
// 表 pronunciation_attempts(id,user_id,ref_type,ref_id,audio_url,score,phoneme_scores,asr_text,duration_ms,created_at)
//
// 配置(env):
//   PORT/READ_DB_URL/WRITE_DB_URL/REDIS_CLUSTER

use axum::{extract::State, middleware, response::IntoResponse, routing::post, Json, Router};
use governor::{Quota, RateLimiter as GovernorLimiter};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::env;
use std::net::SocketAddr;
use std::num::NonZeroU32;
use std::sync::{Arc, LazyLock};

const SERVICE_NAME: &str = "pronunciation-rs";

// ---------- governor 令牌桶限流(替代手写 AtomicU64) ----------
type AppRateLimiter = GovernorLimiter<governor::state::NotKeyed, governor::state::InMemoryState, governor::clock::DefaultClock>;
static RATE_LIMITER: LazyLock<AppRateLimiter> = LazyLock::new(|| {
    GovernorLimiter::direct(Quota::per_second(NonZeroU32::new(100).unwrap()))
});

/// Rate-limit middleware: skip health endpoints
async fn rate_limit_middleware(req: axum::http::Request<axum::body::Body>, next: axum::middleware::Next) -> axum::response::Response {
    let path = req.uri().path().to_string();
    if path == "/healthz" || path == "/metrics" || path.starts_with("/actuator/health") {
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

pub struct AppState {
    pub pg_write: sqlx::PgPool,
    pub pg_read: sqlx::PgPool,
    pub redis: Option<redis::cluster_async::ClusterConnection>,
}

#[derive(Clone)]
pub struct AppHandle(Arc<AppState>);

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    esp_common::tracing_init::init(SERVICE_NAME);

    let port: u16 = env::var("PORT").unwrap_or_else(|_| "8088".into()).parse()?;
    tracing::info!(%port, "[{}] starting", SERVICE_NAME);

    // CRDB 读写 pool(env=WRITE_DB_URL/READ_DB_URL,缺省 crdb-public)
    let (pg_write, pg_read) = esp_common::db::connect_rw("esp_pronunciation", SERVICE_NAME).await?;

    // redis:保留各服务原语义(env 缺省=不连,返回 None)。esp-common-rs::redisx::connect
    // 缺省会连 DEFAULT_ADDR,行为不同,故不抽(行为不变红线)。
    let redis = match env::var("REDIS_CLUSTER") {
        Ok(addr) => match redis::cluster::ClusterClient::builder(vec![addr]).build() {
            Ok(client) => client.get_async_connection().await.ok(),
            Err(e) => {
                tracing::warn!("[{}] redis cluster client build failed: {}", SERVICE_NAME, e);
                None
            }
        },
        Err(_) => None,
    };

    let state = AppHandle(Arc::new(AppState { pg_write, pg_read, redis }));

    let app = Router::new()
        .route("/healthz", axum::routing::get(health))
        .route("/actuator/health", axum::routing::get(health))
        .route("/actuator/health/readiness", axum::routing::get(readiness))
        .route("/actuator/health/liveness", axum::routing::get(liveness))
        .route("/api/pronunciation/score", post(score))
        .route("/api/pronunciation/feedback", post(feedback))
        .with_state(state)
        // 全语言统一深度诊断面板(Step2):挂载 actuator 风格 debug 端点(env/loglevel/beans/mappings)。
        .merge(esp_common::debug::debug_router())
        // Phase1 子项 1.2:Prometheus /metrics 抓取端点(进程级 + 后续业务自定义 metric)。
        .merge(esp_common::metrics::metrics_router())
        .layer(middleware::from_fn(rate_limit_middleware));

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    let listener = tokio::net::TcpListener::bind(addr).await?;
    tracing::info!("[{}] listening on {}", SERVICE_NAME, addr);
    axum::serve(listener, app).await?;
    Ok(())
}

async fn health() -> Json<Value> {
    Json(json!({"status": "UP", "service": SERVICE_NAME}))
}
async fn liveness() -> Json<Value> {
    Json(json!({"status": "UP"}))
}
async fn readiness(State(h): State<AppHandle>) -> Json<Value> {
    let ok = sqlx::query("SELECT 1").execute(&h.0.pg_read).await.is_ok();
    Json(json!({"status": if ok {"UP"} else {"DOWN"}, "db_read": ok}))
}

#[derive(Deserialize)]
struct ScoreReq {
    user_id: i64,
    ref_type: String,        // scenario / word / sentence
    ref_id: i64,
    audio_url: String,
    #[serde(default)]
    phoneme_scores: Option<Value>,
    #[serde(default)]
    asr_text: Option<String>,
    #[serde(default)]
    duration_ms: i64,
}

#[derive(Serialize)]
struct ScoreResp {
    attempt_id: u64,
    score: f64,
}

/// 发音评分骨架(scaffold):
/// 1. 写 pronunciation_attempts(本次评测记录)
/// 2. GOP 真实算法待接入(需 ASR/强制对齐模型,桩返回 0.85)
async fn score(State(h): State<AppHandle>, Json(req): Json<ScoreReq>) -> Result<Json<ScoreResp>, (axum::http::StatusCode, String)> {
    let score = 0.85_f64; // DEFERRED-真实音素评分(需ASR引擎接入,见多节点清单#4)
    let phoneme = req.phoneme_scores.unwrap_or(json!({}));
    let asr = req.asr_text.unwrap_or_default();

    // CRDB(PG 协议)支持 RETURNING,INSERT 直接回传 id,无需 LAST_INSERT_ID/二次查询。
    let attempt_id: (i64,) = sqlx::query_as(
        "INSERT INTO pronunciation_attempts (user_id, ref_type, ref_id, audio_url, score, phoneme_scores, asr_text, duration_ms) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id",
    )
    .bind(req.user_id)
    .bind(&req.ref_type)
    .bind(req.ref_id)
    .bind(&req.audio_url)
    .bind(score)
    .bind(serde_json::to_string(&phoneme).unwrap_or_default())
    .bind(&asr)
    .bind(req.duration_ms)
    .fetch_one(&h.0.pg_write)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db insert: {e}")))?;

    let attempt_id = attempt_id.0 as u64;

    Ok(Json(ScoreResp { attempt_id, score }))
}

/// 音标/重音/语调纠错(桩):返回音标提示
async fn feedback(State(_h): State<AppHandle>, Json(req): Json<Value>) -> Json<Value> {
    let word = req.get("word").and_then(|v| v.as_str()).unwrap_or("");
    Json(json!({
        "word": word,
        "ipa": "/ˈsæm.pəl/",
        "stress": "第1音节",
        "intonation": "降调(陈述)",
        "tips": ["注意元音 /æ/ 口型", "重音在第1音节"]
    }))
}
