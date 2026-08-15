// db —— 统一 CockroachDB(sqlx::PgPool)读写连接池(#140 全服务切 CRDB)。
// 消除 pronunciation-rs / assessment-rs 两处 `PgPool::connect` + DSN 拼装重复。
// llm-orchestrator-rs 用单 pool(非读写分离),亦提供 connect_single。
// 历史:原为 MySqlPool(sqlx mysql feature);#140 切 CockroachDB 后改 PgPool(postgres feature)。

use anyhow::{Context, Result};
use sqlx::postgres::PgPoolOptions;
use sqlx::PgPool;

/// 拼装默认写库 DSN(主库 crdb-public)。db 例 "esp_pronunciation"。
/// CockroachDB PG 协议:postgres://root@crdb-public:26257/<db>?sslmode=disable
pub fn default_write_dsn(db: &str) -> String {
    format!("postgres://root@crdb-public:26257/{}?sslmode=disable", db)
}

/// 拼装默认读库 DSN(CRDB 单集群同地址,读写分离由 CRDB 内部 follower read /负载均衡承担;
/// 单节点环境 read/write 同指向 crdb-public,保留双 pool 接口兼容现有调用方)。
pub fn default_read_dsn(db: &str) -> String {
    format!("postgres://root@crdb-public:26257/{}?sslmode=disable", db)
}

/// 连接单个 CockroachDB pool(用 env DSN,空则用默认写库 DSN)。service_name 仅用于日志上下文。
/// 对齐 pronunciation-rs/assessment-rs 原 `PgPool::connect(&url).await?` 行为(默认连接池配置)。
pub async fn connect(env_key: &str, default_dsn: String, service_name: &str) -> Result<PgPool> {
    let url = std::env::var(env_key).unwrap_or(default_dsn);
    let pool = PgPoolOptions::new()
        .connect(&url)
        .await
        .with_context(|| format!("[{}] connect crdb: {}", service_name, url))?;
    tracing::info!("[{}] CockroachDB pool ready ({})", service_name, env_key);
    Ok(pool)
}

/// 一次返回读写分离 pool 对(CRDB 单集群双 pool,read/write 同地址;
/// 多节点可分别指向不同 CRDB 节点做客户端负载分散)。
/// 对齐 pronunciation-rs/assessment-rs 原 write+read 双 pool 初始化。
pub async fn connect_rw(db: &str, service_name: &str) -> Result<(PgPool, PgPool)> {
    let write = connect("WRITE_DB_URL", default_write_dsn(db), service_name).await?;
    let read = connect("READ_DB_URL", default_read_dsn(db), service_name).await?;
    Ok((write, read))
}
