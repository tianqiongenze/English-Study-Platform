// dialogue/mod.rs —— AI 多轮对话子模块
// session/turn/persona CRUD + SSE 流式 /api/dialogue/chat
// schema: esp_llm.dialogue_sessions / dialogue_turns / dialogue_personas

use axum::{
    extract::{Path, Query, State},
    http::{HeaderMap, HeaderValue},
    response::{IntoResponse, Response},
    Json,
};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sqlx::PgPool;

use crate::AppHandle;

// ---------------- DTO ----------------

#[derive(Debug, Deserialize)]
pub struct CreateSessionReq {
    user_id: i64,
    #[serde(default)]
    scenario_id: Option<i64>,
    #[serde(default)]
    persona_id: Option<i64>,
    #[serde(default)]
    cefr_level: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct SessionDto {
    id: i64,
    user_id: i64,
    scenario_id: Option<i64>,
    persona_id: Option<i64>,
    cefr_level: Option<String>,
    status: String,
}

#[derive(Debug, Deserialize)]
pub struct ChatReq {
    session_id: i64,
    content: String,
    #[serde(default)]
    max_tokens: Option<i32>,
}

#[derive(Debug, Deserialize)]
pub struct ListQuery {
    user_id: Option<i64>,
    #[serde(default)]
    limit: Option<i64>,
}

#[derive(Debug, Deserialize)]
pub struct CreatePersonaReq {
    name: String,
    persona_prompt: String,
    #[serde(default)]
    voice_id: Option<String>,
    #[serde(default)]
    accent: Option<String>,
    #[serde(default)]
    cefr_level: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct PersonaDto {
    id: i64,
    name: String,
    persona_prompt: String,
    voice_id: Option<String>,
    accent: Option<String>,
    cefr_level: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct TurnDto {
    id: i64,
    session_id: i64,
    role: String,
    content: String,
    tokens_in: i32,
    tokens_out: i32,
    latency_ms: i32,
}

// ---------------- session handlers ----------------

pub async fn list_sessions(
    State(h): State<AppHandle>,
    headers: HeaderMap,
    Query(q): Query<ListQuery>,
) -> Response {
    // P1-D16 零信任身份:query.user_id 不可信,以 X-User-Id 头为权威;缺失→401,不一致→403
    let header_uid = match crate::require_user_id(&headers) {
        Ok(v) => v,
        Err(r) => return r,
    };
    let uid = match crate::authorize_user_id(header_uid, q.user_id) {
        Ok(v) => v,
        Err(r) => return r,
    };
    let limit = q.limit.unwrap_or(50).clamp(1, 200);
    // 强制只查权威用户的数据(防越权查他人会话)
    let rows: Result<Vec<(i64, i64, Option<i64>, Option<i64>, Option<String>, String)>, _> =
        sqlx::query_as(
            "SELECT id, user_id, scenario_id, persona_id, cefr_level, status \
             FROM dialogue_sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2",
        )
        .bind(uid)
        .bind(limit)
        .fetch_all(&h.0.pg)
        .await;
    match rows {
        Ok(rs) => {
            let dtos: Vec<SessionDto> = rs
                .into_iter()
                .map(|r| SessionDto {
                    id: r.0,
                    user_id: r.1,
                    scenario_id: r.2,
                    persona_id: r.3,
                    cefr_level: r.4,
                    status: r.5,
                })
                .collect();
            Json(json!({"items": dtos})).into_response()
        }
        Err(e) => err_response(500, &e.to_string()),
    }
}

pub async fn create_session(
    State(h): State<AppHandle>,
    headers: HeaderMap,
    Json(mut req): Json<CreateSessionReq>,
) -> Response {
    // P1-D16 零信任身份:body.user_id 不可信,以 X-User-Id 头为权威;缺失→401,不一致→403
    let header_uid = match crate::require_user_id(&headers) {
        Ok(v) => v,
        Err(r) => return r,
    };
    let uid = match crate::authorize_user_id(header_uid, Some(req.user_id)) {
        Ok(v) => v,
        Err(r) => return r,
    };
    req.user_id = uid; // 覆盖为权威头值,防冒充他人建会话
    let res: Result<(i64,), _> = sqlx::query_as(
        "INSERT INTO dialogue_sessions (user_id, scenario_id, persona_id, cefr_level, status) \
         VALUES ($1, $2, $3, $4, 'active') RETURNING id",
    )
    .bind(req.user_id)
    .bind(req.scenario_id)
    .bind(req.persona_id)
    .bind(&req.cefr_level)
    .fetch_one(&h.0.pg)
    .await;
    match res {
        Ok((id,)) => Json(json!({"id": id, "status": "active"})).into_response(),
        Err(e) => err_response(500, &e.to_string()),
    }
}

pub async fn get_session(
    State(h): State<AppHandle>,
    headers: HeaderMap,
    Path(id): Path<i64>,
) -> Response {
    // P1-D16 纵深 IDOR:取权威 X-User-Id;session 不存在→404,不属于调用者→403(防越权读他人会话)
    let uid = match crate::require_user_id(&headers) {
        Ok(v) => v,
        Err(r) => return r,
    };
    let res: Result<(i64, i64, Option<i64>, Option<i64>, Option<String>, String), _> =
        sqlx::query_as(
            "SELECT id, user_id, scenario_id, persona_id, cefr_level, status \
             FROM dialogue_sessions WHERE id = $1",
        )
        .bind(id)
        .fetch_one(&h.0.pg)
        .await;
    match res {
        Ok(r) => {
            if r.1 != uid {
                return err_response(403, "session does not belong to caller (forbidden)");
            }
            Json(json!(SessionDto {
                id: r.0,
                user_id: r.1,
                scenario_id: r.2,
                persona_id: r.3,
                cefr_level: r.4,
                status: r.5
            }))
            .into_response()
        }
        Err(e) => err_response(404, &e.to_string()),
    }
}

pub async fn list_turns(
    State(h): State<AppHandle>,
    headers: HeaderMap,
    Path(session_id): Path<i64>,
) -> Response {
    // P1-D16 纵深 IDOR:取权威 X-User-Id;session 不存在→404,不属于调用者→403(防越权读他人轮次)
    let uid = match crate::require_user_id(&headers) {
        Ok(v) => v,
        Err(r) => return r,
    };
    if let Err(r) = verify_session_owner(&h.0.pg, session_id, uid).await {
        return r;
    }
    let rows: Result<Vec<(i64, i64, String, String, i32, i32, i32)>, _> = sqlx::query_as(
        "SELECT id, session_id, role, content, tokens_in, tokens_out, latency_ms \
         FROM dialogue_turns WHERE session_id = $1 ORDER BY created_at ASC",
    )
    .bind(session_id)
    .fetch_all(&h.0.pg)
    .await;
    match rows {
        Ok(rs) => {
            let dtos: Vec<TurnDto> = rs
                .into_iter()
                .map(|r| TurnDto {
                    id: r.0,
                    session_id: r.1,
                    role: r.2,
                    content: r.3,
                    tokens_in: r.4,
                    tokens_out: r.5,
                    latency_ms: r.6,
                })
                .collect();
            Json(json!({"items": dtos})).into_response()
        }
        Err(e) => err_response(500, &e.to_string()),
    }
}

// ---------------- SSE chat ----------------

/// POST /api/dialogue/chat —— SSE 流式对话
/// 写入 user turn → 调 provider 流式生成 → 逐 token SSE 推送 → 落库 assistant turn
pub async fn chat_sse(
    State(h): State<AppHandle>,
    headers: HeaderMap,
    Json(req): Json<ChatReq>,
) -> Response {
    // P1-D16 纵深 IDOR:取权威 X-User-Id;写 turn 前先校验 session 归属,不属于调用者→403(防越权写他人会话)
    let uid = match crate::require_user_id(&headers) {
        Ok(v) => v,
        Err(r) => return r,
    };
    if let Err(r) = verify_session_owner(&h.0.pg, req.session_id, uid).await {
        return r;
    }
    let trace_id = headers
        .get("x-trace-id")
        .and_then(|v| v.to_str().ok())
        .map(|s| s.to_string())
        .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());

    // 1. 落 user turn
    let _ = sqlx::query(
        "INSERT INTO dialogue_turns (session_id, role, content, tokens_in, tokens_out, latency_ms) \
         VALUES ($1, 'user', $2, 0, 0, 0)",
    )
    .bind(req.session_id)
    .bind(&req.content)
    .execute(&h.0.pg)
    .await;

    // 2. 调 provider(自适应限流保护;非流式简化实现;流式见 provider SSE 解析)
    let _permit = h.0.llm_limiter.acquire().await;
    let started = std::time::Instant::now();
    let result =
        h.0.providers
            .call(None, None, &req.content, req.max_tokens, false, &trace_id)
            .await;
    let rtt = started.elapsed();
    let latency_ms = rtt.as_millis() as i32;
    // 释放 permit + 自适应调整
    h.0.llm_limiter.release_with_rtt(rtt);
    drop(_permit);

    match result {
        Ok(r) => {
            // 3. 落 assistant turn
            let _ = sqlx::query(
                "INSERT INTO dialogue_turns (session_id, role, content, tokens_in, tokens_out, latency_ms) \
                 VALUES ($1, 'assistant', $2, 0, 0, $3)",
            )
            .bind(req.session_id)
            .bind(&r.content)
            .bind(latency_ms)
            .execute(&h.0.pg)
            .await;

            // 4. SSE 单帧返回(完整内容;真正逐 token 流式可后续从 provider stream 改造)
            let body = format!(
                "event: turn\ndata: {}\n\nevent: done\ndata: [DONE]\n\n",
                json!({
                    "content": r.content,
                    "provider": r.provider,
                    "model": r.model,
                    "trace_id": trace_id,
                    "latency_ms": latency_ms,
                })
            );
            let mut resp = Response::new(axum::body::Body::from(body));
            resp.headers_mut().insert(
                "content-type",
                HeaderValue::from_static("text/event-stream"),
            );
            resp.headers_mut()
                .insert("cache-control", HeaderValue::from_static("no-cache"));
            resp
        }
        Err(e) => err_response(502, &e.to_string()),
    }
}

// ---------------- persona handlers ----------------

pub async fn list_personas(State(h): State<AppHandle>) -> Response {
    let rows: Result<
        Vec<(
            i64,
            String,
            String,
            Option<String>,
            Option<String>,
            Option<String>,
        )>,
        _,
    > = sqlx::query_as(
        "SELECT id, name, persona_prompt, voice_id, accent, cefr_level \
             FROM dialogue_personas ORDER BY id ASC LIMIT 100",
    )
    .fetch_all(&h.0.pg)
    .await;
    match rows {
        Ok(rs) => {
            let dtos: Vec<PersonaDto> = rs
                .into_iter()
                .map(|r| PersonaDto {
                    id: r.0,
                    name: r.1,
                    persona_prompt: r.2,
                    voice_id: r.3,
                    accent: r.4,
                    cefr_level: r.5,
                })
                .collect();
            Json(json!({"items": dtos})).into_response()
        }
        Err(e) => err_response(500, &e.to_string()),
    }
}

pub async fn create_persona(
    State(h): State<AppHandle>,
    Json(req): Json<CreatePersonaReq>,
) -> Response {
    let res: Result<(i64,), _> = sqlx::query_as(
        "INSERT INTO dialogue_personas (name, persona_prompt, voice_id, accent, cefr_level) \
         VALUES ($1, $2, $3, $4, $5) RETURNING id",
    )
    .bind(&req.name)
    .bind(&req.persona_prompt)
    .bind(&req.voice_id)
    .bind(&req.accent)
    .bind(&req.cefr_level)
    .fetch_one(&h.0.pg)
    .await;
    match res {
        Ok((id,)) => Json(json!({"id": id})).into_response(),
        Err(e) => err_response(500, &e.to_string()),
    }
}

// ---------------- helpers ----------------

/// 校验 session 归属:session 不存在→404 Err,不属于 caller→403 Err,属于→Ok(())。
/// 供 list_turns / chat_sse 复用,避免每个端点重复写归属校验(DRY)。
async fn verify_session_owner(pg: &PgPool, session_id: i64, uid: i64) -> Result<(), Response> {
    let row: Option<(i64,)> =
        sqlx::query_as("SELECT user_id FROM dialogue_sessions WHERE id = $1")
            .bind(session_id)
            .fetch_optional(pg)
            .await
            .map_err(|e| err_response(500, &e.to_string()))?;
    match row {
        None => Err(err_response(404, "session not found")),
        Some((owner_uid,)) if owner_uid != uid => Err(err_response(
            403,
            "session does not belong to caller (forbidden)",
        )),
        Some(_) => Ok(()),
    }
}

fn err_response(status: u16, msg: &str) -> Response {
    (
        axum::http::StatusCode::from_u16(status)
            .unwrap_or(axum::http::StatusCode::INTERNAL_SERVER_ERROR),
        Json(json!({"error": msg})),
    )
        .into_response()
}

/// 允许在别处访问 PgPool(预留)
#[allow(dead_code)]
pub fn pool_of(h: &AppHandle) -> &PgPool {
    &h.0.pg
}

#[allow(dead_code)]
pub fn value_of(v: &Value) -> String {
    v.to_string()
}
