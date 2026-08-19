// media-learn-rs —— 真实语料学习(N11,P1,scaffold 级)
// 单节点 RD:scaffold + 字幕同步 API,音视频对齐用预录样本离线验证,不在线转码。
// 完整 ADR/PRD/schema 见 docs/design/adr-new-services.md / prd-new-services.md / adr-schema-per-service.md

use axum::{routing::get, Router, http::StatusCode, middleware, response::IntoResponse, Json};
use governor::{Quota, RateLimiter as GovernorLimiter};
use serde_json::json;
use std::env;
use std::net::SocketAddr;
use std::num::NonZeroU32;
use std::sync::LazyLock;

const SERVICE_NAME: &str = "media-learn-rs";

// ---------- governor 令牌桶限流(替代手写 AtomicU64) ----------
type AppRateLimiter = GovernorLimiter<governor::state::NotKeyed, governor::state::InMemoryState, governor::clock::DefaultClock>;
static RATE_LIMITER: LazyLock<AppRateLimiter> = LazyLock::new(|| {
    GovernorLimiter::direct(Quota::per_second(NonZeroU32::new(100).unwrap()))
});

/// Rate-limit middleware for all non-healthz requests
async fn rate_limit_middleware(req: axum::http::Request<axum::body::Body>, next: axum::middleware::Next) -> axum::response::Response {
    let path = req.uri().path().to_string();
    if path == "/healthz" || path == "/metrics" || path.starts_with("/actuator/health") {
        return next.run(req).await;
    }
    if !RATE_LIMITER.check().is_ok() {
        return (
            StatusCode::TOO_MANY_REQUESTS,
            Json(json!({"error": "rate limit exceeded", "retry_after": 1})),
        ).into_response();
    }
    next.run(req).await
}

#[tokio::main]
async fn main() {
    esp_common::tracing_init::init(SERVICE_NAME);

    let port: u16 = env::var("PORT")
        .unwrap_or_else(|_| "8095".into())
        .parse()
        .unwrap_or(8095);
    tracing::info!("[{}] starting on :{}", SERVICE_NAME, port);

    let app = Router::new()
        .route("/healthz", get(health))
        .route("/actuator/health", get(health))
        .route("/actuator/health/readiness", get(readiness))
        .route("/actuator/health/liveness", get(liveness))
        .route("/api/media/clips", get(list_clips))         // 片段列表(桩)
        .route("/api/media/clips/:id/subtitles", get(subtitles)) // 字幕同步(桩)
        .route("/api/media/clips/:id/cards", get(cards))   // 表达卡片(桩)
        // 全语言统一深度诊断面板(Step2):挂载 actuator 风格 debug 端点(env/loglevel/beans/mappings)。
        .merge(esp_common::debug::debug_router())
        // Phase1 子项 1.2:Prometheus /metrics 抓取端点(进程级 + 后续业务自定义 metric)。
        .merge(esp_common::metrics::metrics_router())
        .layer(middleware::from_fn(rate_limit_middleware));

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    let listener = tokio::net::TcpListener::bind(addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}

async fn health() -> axum::Json<serde_json::Value> {
    axum::Json(json!({"status": "UP", "service": SERVICE_NAME}))
}
async fn readiness() -> axum::Json<serde_json::Value> {
    axum::Json(json!({"status": "UP"}))
}
async fn liveness() -> axum::Json<serde_json::Value> {
    axum::Json(json!({"status": "UP"}))
}

// list_clips —— 片段列表(桩:实际查 esp_media.media_clips)
async fn list_clips() -> axum::Json<serde_json::Value> {
    axum::Json(json!({
        "clips": [],
        "status": "scaffold",
        "message": "media-learn list_clips: scaffold stub, see PRD §4"
    }))
}

// subtitles —— 字幕同步(桩)
async fn subtitles(axum::extract::Path(id): axum::extract::Path<String>) -> axum::Json<serde_json::Value> {
    axum::Json(json!({"clip_id": id, "subtitles": [], "status": "scaffold"}))
}

// cards —— 表达卡片(桩:实际经 S1 LLM 抽取)
async fn cards(axum::extract::Path(id): axum::extract::Path<String>) -> axum::Json<serde_json::Value> {
    axum::Json(json!({"clip_id": id, "cards": [], "status": "scaffold"}))
}
