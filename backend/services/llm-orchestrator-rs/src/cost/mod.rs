// cost/mod.rs —— 成本治理:llm_call_logs 写入(esp_llm 库)
// 每次调用记 provider/model/tokens/latency/cost/trace_id,失败也记
// schema: docs/design/schema-per-service-phase0.sql esp_llm.llm_call_logs

use sqlx::mysql::MySqlPool;

/// 单次调用成本记录
pub struct CallLog {
    pub trace_id: String,
    pub user_id: Option<i64>,
    pub provider: String,
    pub model: String,
    pub scene: String,
    pub prompt_tokens: i32,
    pub completion_tokens: i32,
    pub total_tokens: i32,
    pub latency_ms: i32,
    pub cost_usd: f64,
    pub cache_hit: i8,
    pub status: String,
    pub error_msg: Option<String>,
}

/// 写入 llm_call_logs。失败仅记日志,不阻断主流程(调用方已 .ok() 兜底)
pub async fn record(pool: &MySqlPool, log: &CallLog) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"INSERT INTO llm_call_logs
           (trace_id, user_id, provider, model, scene, prompt_tokens, completion_tokens,
            total_tokens, latency_ms, cost_usd, cache_hit, status, error_msg, created_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())"#,
    )
    .bind(&log.trace_id)
    .bind(log.user_id)
    .bind(&log.provider)
    .bind(&log.model)
    .bind(&log.scene)
    .bind(log.prompt_tokens)
    .bind(log.completion_tokens)
    .bind(log.total_tokens)
    .bind(log.latency_ms)
    .bind(log.cost_usd)
    .bind(log.cache_hit)
    .bind(&log.status)
    .bind(&log.error_msg)
    .execute(pool)
    .await?;
    Ok(())
}
