// pronunciation-rs —— 音素级发音评分服务(S3, P0)
// 主干:GOP(Goodness of Pronunciation)骨架 + 音标/重音/语调纠错桩 + 评分落库
// scaffold 级:健康检查 + 评分路由桩 + esp_pronunciation.pronunciation_attempts 写入
// ADR: docs/design/adr-new-services.md / PRD §三 / schema esp_pronunciation
//
// 表 pronunciation_attempts(id,user_id,ref_type,ref_id,audio_url,score,phoneme_scores,asr_text,duration_ms,created_at)
//
// 配置(env):
//   PORT/READ_DB_URL/WRITE_DB_URL/REDIS_CLUSTER

use axum::{extract::State, routing::post, Json, Router};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::env;
use std::net::SocketAddr;
use std::sync::Arc;
use tracing_subscriber::EnvFilter;

const SERVICE_NAME: &str = "pronunciation-rs";

pub struct AppState {
    pub mysql_write: sqlx::MySqlPool,
    pub mysql_read: sqlx::MySqlPool,
    pub redis: Option<redis::cluster_async::ClusterConnection>,
}

#[derive(Clone)]
pub struct AppHandle(Arc<AppState>);

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")))
        .init();

    let port: u16 = env::var("PORT").unwrap_or_else(|_| "8088".into()).parse()?;
    tracing::info!(%port, "[{}] starting", SERVICE_NAME);

    let write_url = env::var("WRITE_DB_URL").unwrap_or_else(|_| {
        format!("mysql://root:@tidb-primary:4000/esp_pronunciation?ssl-mode=DISABLED")
    });
    let read_url = env::var("READ_DB_URL").unwrap_or_else(|_| {
        format!("mysql://root:@tidb-follower:4000/esp_pronunciation?ssl-mode=DISABLED")
    });

    let mysql_write = sqlx::MySqlPool::connect(&write_url).await?;
    let mysql_read = sqlx::MySqlPool::connect(&read_url).await?;
    tracing::info!("[{}] TiDB pools ready (write+read)", SERVICE_NAME);

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

    let state = AppHandle(Arc::new(AppState { mysql_write, mysql_read, redis }));

    let app = Router::new()
        .route("/actuator/health", axum::routing::get(health))
        .route("/actuator/health/readiness", axum::routing::get(readiness))
        .route("/actuator/health/liveness", axum::routing::get(liveness))
        .route("/api/pronunciation/score", post(score))
        .route("/api/pronunciation/feedback", post(feedback))
        .with_state(state);

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
    let ok = sqlx::query("SELECT 1").execute(&h.0.mysql_read).await.is_ok();
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
    let score = 0.85_f64; // scaffold 占位
    let phoneme = req.phoneme_scores.unwrap_or(json!({}));
    let asr = req.asr_text.unwrap_or_default();

    // TiDB AUTO_RANDOM 主键:INSERT 不会回传 id,需单独 LAST_INSERT_ID()。
    // 注:fetch_one 要求 RETURNING,改 execute + 查 LAST_INSERT_ID 两步。
    let insert_res = sqlx::query(
        "INSERT INTO pronunciation_attempts (user_id, ref_type, ref_id, audio_url, score, phoneme_scores, asr_text, duration_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
    )
    .bind(req.user_id)
    .bind(&req.ref_type)
    .bind(req.ref_id)
    .bind(&req.audio_url)
    .bind(score)
    .bind(serde_json::to_string(&phoneme).unwrap_or_default())
    .bind(&asr)
    .bind(req.duration_ms)
    .execute(&h.0.mysql_write)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db insert: {e}")))?;

    // 注:pronunciation_attempts.id 列 EXTRA=''(非 AUTO_RANDOM/AUTO_INCREMENT),
    // LAST_INSERT_ID() 对非自增列返回 0。改用 ORDER BY id DESC LIMIT 1 取真实 id。
    let attempt_id: (i64,) = sqlx::query_as("SELECT id FROM pronunciation_attempts ORDER BY id DESC LIMIT 1")
        .fetch_one(&h.0.mysql_write)
        .await
        .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db last_id: {e}")))?;

    let attempt_id = attempt_id.0 as u64;
    let _ = insert_res;

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
