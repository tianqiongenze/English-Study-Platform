// media-learn-rs —— 真实语料学习(N11,P1,scaffold 级)
// 单节点 RD:scaffold + 字幕同步 API,音视频对齐用预录样本离线验证,不在线转码。
// 完整 ADR/PRD/schema 见 docs/design/adr-new-services.md / prd-new-services.md / adr-schema-per-service.md

use axum::{routing::get, Router};
use serde_json::json;
use std::env;
use std::net::SocketAddr;
use tracing_subscriber::EnvFilter;

const SERVICE_NAME: &str = "media-learn-rs";

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")))
        .init();

    let port: u16 = env::var("PORT")
        .unwrap_or_else(|_| "8095".into())
        .parse()
        .unwrap_or(8095);
    tracing::info!("[{}] starting on :{}", SERVICE_NAME, port);

    let app = Router::new()
        .route("/actuator/health", get(health))
        .route("/actuator/health/readiness", get(readiness))
        .route("/actuator/health/liveness", get(liveness))
        .route("/api/media/clips", get(list_clips))         // 片段列表(桩)
        .route("/api/media/clips/:id/subtitles", get(subtitles)) // 字幕同步(桩)
        .route("/api/media/clips/:id/cards", get(cards));   // 表达卡片(桩)

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
