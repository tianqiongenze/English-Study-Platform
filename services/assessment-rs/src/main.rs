// assessment-rs —— CEFR 定级 + 雅思/托福模考 + 自适应测试(IRT)+ 诊断报告(S5, P1)
// scaffold 级:健康检查 + 会话创建/答题/结算路由桩 + esp_assessment 表读写
// ADR: docs/design/adr-new-services.md / PRD §五 / schema esp_assessment
//
// 注意:TiDB 实际表结构(esp_assessment)为旧 schema,与 docs/design/schema.sql 不一致:
// 表 assessment_sessions(id,user_id,goal,cefr_result,status,score,started_at,finished_at)
// 表 assessment_answers(id,session_id,question_id,answer_json,is_correct,skill_tag,created_at)

use axum::{extract::{Path, State}, routing::{get, post}, Json, Router};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::env;
use std::net::SocketAddr;
use std::sync::Arc;
use tracing_subscriber::EnvFilter;

const SERVICE_NAME: &str = "assessment-rs";

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

    let port: u16 = env::var("PORT").unwrap_or_else(|_| "8090".into()).parse()?;
    tracing::info!(%port, "[{}] starting", SERVICE_NAME);

    let write_url = env::var("WRITE_DB_URL").unwrap_or_else(|_| {
        format!("mysql://root:@tidb-primary:4000/esp_assessment?ssl-mode=DISABLED")
    });
    let read_url = env::var("READ_DB_URL").unwrap_or_else(|_| {
        format!("mysql://root:@tidb-follower:4000/esp_assessment?ssl-mode=DISABLED")
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
        .route("/actuator/health", get(health))
        .route("/actuator/health/readiness", get(readiness))
        .route("/actuator/health/liveness", get(liveness))
        .route("/api/assessment/sessions", post(create_session))
        .route("/api/assessment/sessions/:id", get(get_session))
        .route("/api/assessment/sessions/:id/answer", post(submit_answer))
        .route("/api/assessment/sessions/:id/finalize", post(finalize_session))
        .route("/api/assessment/report/:user_id", get(diagnostic_report))
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
struct CreateSessionReq {
    user_id: i64,
    #[serde(default)]
    goal: Option<String>, // CEFR / IELTS / TOEFL 等
}

#[derive(Serialize)]
struct SessionResp {
    session_id: u64,
    user_id: i64,
    status: String,
}

/// 创建测评会话(scaffold):写 assessment_sessions(status=in_progress)
/// TiDB AUTO_RANDOM 主键:INSERT 不回传 id,需 execute + LAST_INSERT_ID 两步。
async fn create_session(
    State(h): State<AppHandle>,
    Json(req): Json<CreateSessionReq>,
) -> Result<Json<SessionResp>, (axum::http::StatusCode, String)> {
    let _insert_res = sqlx::query(
        "INSERT INTO assessment_sessions (user_id, goal, status) VALUES (?, ?, 'in_progress')",
    )
    .bind(req.user_id)
    .bind(req.goal.unwrap_or_else(|| "CEFR".into()))
    .execute(&h.0.mysql_write)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db insert: {e}")))?;

    let row: (u64,) = sqlx::query_as("SELECT LAST_INSERT_ID()")
        .fetch_one(&h.0.mysql_write)
        .await
        .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db last_id: {e}")))?;

    Ok(Json(SessionResp { session_id: row.0, user_id: req.user_id, status: "in_progress".into() }))
}

/// 读取会话状态
async fn get_session(
    State(h): State<AppHandle>,
    Path(id): Path<i64>,
) -> Result<Json<Value>, (axum::http::StatusCode, String)> {
    let row: Option<(i64, Option<String>, Option<String>, String)> = sqlx::query_as(
        "SELECT id, goal, cefr_result, status FROM assessment_sessions WHERE id = ?",
    )
    .bind(id)
    .fetch_optional(&h.0.mysql_read)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db: {e}")))?;

    match row {
        Some((sid, goal, cefr, st)) => Ok(Json(json!({"session_id": sid, "goal": goal, "cefr_result": cefr, "status": st}))),
        None => Err((axum::http::StatusCode::NOT_FOUND, "session not found".into())),
    }
}

#[derive(Deserialize)]
struct AnswerReq {
    question_id: i64,
    answer_json: Value,
    #[serde(default)]
    is_correct: Option<bool>,
    #[serde(default)]
    skill_tag: Option<String>,
}

/// 提交单题答案(scaffold):写 assessment_answers
/// TiDB AUTO_RANDOM 主键:execute + LAST_INSERT_ID 两步。
async fn submit_answer(
    State(h): State<AppHandle>,
    Path(session_id): Path<i64>,
    Json(req): Json<AnswerReq>,
) -> Result<Json<Value>, (axum::http::StatusCode, String)> {
    let correct = req.is_correct.unwrap_or(false) as i8;
    let _insert_res = sqlx::query(
        "INSERT INTO assessment_answers (session_id, question_id, answer_json, is_correct, skill_tag) VALUES (?, ?, ?, ?, ?)",
    )
    .bind(session_id)
    .bind(req.question_id)
    .bind(serde_json::to_string(&req.answer_json).unwrap_or_else(|_| "{}".into()))
    .bind(correct)
    .bind(req.skill_tag.unwrap_or_default())
    .execute(&h.0.mysql_write)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db insert: {e}")))?;

    let row: (u64,) = sqlx::query_as("SELECT LAST_INSERT_ID()")
        .fetch_one(&h.0.mysql_write)
        .await
        .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db last_id: {e}")))?;

    Ok(json!({"answer_id": row.0, "is_correct": correct != 0}).into())
}

/// 结算会话(scaffold):status=finished + cefr_result 占位
async fn finalize_session(
    State(h): State<AppHandle>,
    Path(id): Path<i64>,
) -> Result<Json<Value>, (axum::http::StatusCode, String)> {
    let res = sqlx::query(
        "UPDATE assessment_sessions SET status='finished', finished_at=NOW(), cefr_result=?, score=? WHERE id=?",
    )
    .bind("B2") // scaffold 占位定级
    .bind(75.0_f64)
    .bind(id)
    .execute(&h.0.mysql_write)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db update: {e}")))?;

    if res.rows_affected() == 0 {
        return Err((axum::http::StatusCode::NOT_FOUND, "session not found".into()));
    }
    Ok(Json(json!({"session_id": id, "status": "finished", "cefr_result": "B2", "score": 75.0})))
}

/// 诊断报告(scaffold):聚合该用户会话
async fn diagnostic_report(
    State(h): State<AppHandle>,
    Path(user_id): Path<i64>,
) -> Result<Json<Value>, (axum::http::StatusCode, String)> {
    let sessions: Vec<(i64, Option<String>, Option<String>, String)> = sqlx::query_as(
        "SELECT id, goal, cefr_result, status FROM assessment_sessions WHERE user_id=? ORDER BY started_at DESC LIMIT 5",
    )
    .bind(user_id)
    .fetch_all(&h.0.mysql_read)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db: {e}")))?;

    let arr: Vec<Value> = sessions
        .into_iter()
        .map(|(id, goal, cefr, st)| json!({"session_id": id, "goal": goal, "cefr_result": cefr, "status": st}))
        .collect();

    Ok(Json(json!({"user_id": user_id, "recent_sessions": arr, "diagnosis": "占位诊断报告(真实 IRT 诊断待接入)"})))
}
