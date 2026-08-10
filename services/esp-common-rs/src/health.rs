// health —— 统一 axum 健康检查 handler(会话22 债1+2)。
// 消除 pronunciation-rs / assessment-rs / llm-orchestrator-rs / media-learn-rs 里
// 逐字复制的 /healthz、/actuator/health、/actuator/health/liveness handler。
//
// 设计原则:行为不变红线 —— 返回体 JSON 结构与各服务原 handler 完全一致:
//   /healthz        -> {"status":"UP","service":<name>}
//   /actuator/health-> {"status":"UP","service":<name>}
//   liveness        -> {"status":"UP"}
// readiness 涉及读各自 DB pool(AppState),签名各异,不抽(留在各服务内)。

use axum::Json;
use serde_json::{json, Value};

/// /healthz 与 /actuator/health 通用 handler。返回 {"status":"UP","service":<name>}。
pub async fn health(service_name: &'static str) -> Json<Value> {
    Json(json!({"status": "UP", "service": service_name}))
}

/// /actuator/health/liveness 通用 handler。返回 {"status":"UP"}。
pub async fn liveness() -> Json<Value> {
    Json(json!({"status": "UP"}))
}
