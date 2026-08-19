// assessment-rs —— CEFR 定级 + 雅思/托福模考 + 自适应测试(IRT)+ 诊断报告(S5, P1)
// scaffold 级:健康检查 + 会话创建/答题/结算路由桩 + esp_assessment 表读写
// ADR: docs/design/adr-new-services.md / PRD §五 / schema esp_assessment
//
// 注意:TiDB 实际表结构(esp_assessment)为旧 schema,与 docs/design/schema.sql 不一致:
// 表 assessment_sessions(id,user_id,goal,cefr_result,status,score,started_at,finished_at)
// 表 assessment_answers(id,session_id,question_id,answer_json,is_correct,skill_tag,created_at)

use axum::{extract::{Path, State}, http::StatusCode, middleware, response::IntoResponse, routing::{get, post}, Json, Router};
use governor::{Quota, RateLimiter as GovernorLimiter};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::env;
use std::net::SocketAddr;
use std::num::NonZeroU32;
use std::sync::{Arc, LazyLock};

const SERVICE_NAME: &str = "assessment-rs";

// ---------- governor 令牌桶限流(替代手写 AtomicU64) ----------
type AppRateLimiter = GovernorLimiter<governor::state::NotKeyed, governor::state::InMemoryState, governor::clock::DefaultClock>;
static RATE_LIMITER: LazyLock<AppRateLimiter> = LazyLock::new(|| {
    GovernorLimiter::direct(Quota::per_second(NonZeroU32::new(100).unwrap()))
});

/// Rate-limit middleware for all non-healthz requests
async fn rate_limit_middleware(req: axum::http::Request<axum::body::Body>, next: axum::middleware::Next) -> axum::response::Response {
    let path = req.uri().path().to_string();
    // Skip rate limiting for health check endpoints
    if path == "/healthz" || path.starts_with("/actuator/health") {
        return next.run(req).await;
    }
    if !RATE_LIMITER.check().is_ok() {
        return (
            StatusCode::TOO_MANY_REQUESTS,
            axum::Json(json!({"error": "rate limit exceeded", "retry_after": 1})),
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

    let port: u16 = env::var("PORT").unwrap_or_else(|_| "8090".into()).parse()?;
    tracing::info!(%port, "[{}] starting", SERVICE_NAME);

    // CRDB 读写 pool(env=WRITE_DB_URL/READ_DB_URL,缺省 crdb-public)
    let (pg_write, pg_read) = esp_common::db::connect_rw("esp_assessment", SERVICE_NAME).await?;

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
        .route("/healthz", get(health))
        .route("/actuator/health", get(health))
        .route("/actuator/health/readiness", get(readiness))
        .route("/actuator/health/liveness", get(liveness))
        .route("/api/assessment/sessions", post(create_session))
        .route("/api/assessment/sessions/:id", get(get_session))
        .route("/api/assessment/sessions/:id/answer", post(submit_answer))
        .route("/api/assessment/sessions/:id/finalize", post(finalize_session))
        .route("/api/assessment/report/:user_id", get(diagnostic_report))
        .with_state(state)
        // 全语言统一深度诊断面板(Step2):挂载 actuator 风格 debug 端点(env/loglevel/beans/mappings)。
        // threaddump/heapdump Rust 无原生等价(标 NA,可扩展 backtrace/dhat)。
        // 鉴权:debug 端点仅经 admin-go 代理可达,admin-go 侧 requireAdmin(admin JWT)把关。
        .merge(esp_common::debug::debug_router())
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
/// CRDB(PG 协议)支持 RETURNING,INSERT 直接回传 id。
async fn create_session(
    State(h): State<AppHandle>,
    Json(req): Json<CreateSessionReq>,
) -> Result<Json<SessionResp>, (axum::http::StatusCode, String)> {
    let row: (i64,) = sqlx::query_as(
        "INSERT INTO assessment_sessions (user_id, goal, status) VALUES ($1, $2, 'in_progress') RETURNING id",
    )
    .bind(req.user_id)
    .bind(req.goal.unwrap_or_else(|| "CEFR".into()))
    .fetch_one(&h.0.pg_write)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db insert: {e}")))?;

    Ok(Json(SessionResp { session_id: row.0 as u64, user_id: req.user_id, status: "in_progress".into() }))
}

/// 读取会话状态
async fn get_session(
    State(h): State<AppHandle>,
    Path(id): Path<i64>,
) -> Result<Json<Value>, (axum::http::StatusCode, String)> {
    let row: Option<(i64, Option<String>, Option<String>, String)> = sqlx::query_as(
        "SELECT id, goal, cefr_result, status FROM assessment_sessions WHERE id = $1",
    )
    .bind(id)
    .fetch_optional(&h.0.pg_read)
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
/// CRDB(PG 协议)支持 RETURNING,INSERT 直接回传 id。
async fn submit_answer(
    State(h): State<AppHandle>,
    Path(session_id): Path<i64>,
    Json(req): Json<AnswerReq>,
) -> Result<Json<Value>, (axum::http::StatusCode, String)> {
    // 坑:pgx v5 对 Rust i8 的编码有缺陷(0 被发成空串 → strconv.ParseInt("") 报错)。
    // is_correct 列为 INT2,i16 是原生支持类型。
    let correct: i16 = if req.is_correct.unwrap_or(false) { 1 } else { 0 };
    let row: (i64,) = sqlx::query_as(
        "INSERT INTO assessment_answers (session_id, question_id, answer_json, is_correct, skill_tag) VALUES ($1, $2, $3, $4, $5) RETURNING id",
    )
    .bind(session_id)
    .bind(req.question_id)
    .bind(serde_json::to_string(&req.answer_json).unwrap_or_else(|_| "{}".into()))
    .bind(correct)
    .bind(req.skill_tag)
    .fetch_one(&h.0.pg_write)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db insert: {e}")))?;

    Ok(json!({"answer_id": row.0 as u64, "is_correct": correct != 0}).into())
}

/// 结算会话(scaffold):status=finished + cefr_result 占位
async fn finalize_session(
    State(h): State<AppHandle>,
    Path(id): Path<i64>,
) -> Result<Json<Value>, (axum::http::StatusCode, String)> {
    let res = sqlx::query(
        "UPDATE assessment_sessions SET status='finished', finished_at=NOW(), cefr_result=$1, score=$2 WHERE id=$3",
    )
    .bind("B2") // scaffold 占位定级
    .bind(75.0_f64)
    .bind(id)
    .execute(&h.0.pg_write)
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
        "SELECT id, goal, cefr_result, status FROM assessment_sessions WHERE user_id=$1 ORDER BY started_at DESC LIMIT 5",
    )
    .bind(user_id)
    .fetch_all(&h.0.pg_read)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, format!("db: {e}")))?;

    let arr: Vec<Value> = sessions
        .into_iter()
        .map(|(id, goal, cefr, st)| json!({"session_id": id, "goal": goal, "cefr_result": cefr, "status": st}))
        .collect();

    Ok(Json(json!({"user_id": user_id, "recent_sessions": arr, "diagnosis": "DEFERRED-真实IRT诊断待接入(需IRT引擎,见多节点清单#6):现状返回结构化空诊断"})))
}
