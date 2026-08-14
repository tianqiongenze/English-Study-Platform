// db —— 统一 TiDB(sqlx::MySqlPool)读写连接池(会话22 债1+2)。
// 消除 pronunciation-rs / assessment-rs 两处 `MySqlPool::connect` + DSN 拼装重复。
// llm-orchestrator-rs 用单 pool(非读写分离),亦提供 connect_single。

use anyhow::{Context, Result};
use sqlx::mysql::MySqlPoolOptions;
use sqlx::MySqlPool;

/// 拼装默认写库 DSN(主库 tidb-primary)。db 例 "esp_pronunciation"。
/// 对齐各服务原默认值:mysql://root:@tidb-primary:4000/<db>?ssl-mode=DISABLED
pub fn default_write_dsn(db: &str) -> String {
    format!("mysql://root:@tidb-primary:4000/{}?ssl-mode=DISABLED", db)
}

/// 拼装默认读库 DSN(从库 tidb-follower)。
pub fn default_read_dsn(db: &str) -> String {
    format!("mysql://root:@tidb-follower:4000/{}?ssl-mode=DISABLED", db)
}

/// 连接单个 TiDB pool(用 env DSN,空则用默认写库 DSN)。service_name 仅用于日志上下文。
/// 对齐 pronunciation-rs/assessment-rs 原 `MySqlPool::connect(&url).await?` 行为(默认连接池配置)。
pub async fn connect(env_key: &str, default_dsn: String, service_name: &str) -> Result<MySqlPool> {
    let url = std::env::var(env_key).unwrap_or(default_dsn);
    let pool = MySqlPoolOptions::new()
        .connect(&url)
        .await
        .with_context(|| format!("[{}] connect tidb: {}", service_name, url))?;
    tracing::info!("[{}] TiDB pool ready ({})", service_name, env_key);
    Ok(pool)
}

/// 一次返回读写分离 pool 对(读 tidb-follower / 写 tidb-primary)。
/// 对齐 pronunciation-rs/assessment-rs 原 write+read 双 pool 初始化。
pub async fn connect_rw(db: &str, service_name: &str) -> Result<(MySqlPool, MySqlPool)> {
    let write = connect("WRITE_DB_URL", default_write_dsn(db), service_name).await?;
    let read = connect("READ_DB_URL", default_read_dsn(db), service_name).await?;
    Ok((write, read))
}
