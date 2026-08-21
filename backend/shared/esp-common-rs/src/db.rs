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

#[cfg(test)]
mod tests {
    //! db 模块单测(覆盖 DSN 拼装纯函数 + 连接错误/降级路径)。
    //! 说明:无活体 CRDB 可用,connect/connect_rw 只能测错误路径(Err),
    //! env 覆盖是否真正被读取在无活体 DB 下无法从结果消歧(两条路径都 Err),
    //! 故 env 相关测试仅断言"不 panic + 返回 Err",覆盖证明需活体 CRDB。

    use super::*;

    // ---- default_write_dsn / default_read_dsn(纯函数,可完全测试)----

    #[test]
    fn test_default_write_dsn_typical() {
        // 正常路径:典型 db 名拼出预期 DSN。
        let dsn = default_write_dsn("esp_pronunciation");
        assert_eq!(
            dsn,
            "postgres://root@crdb-public:26257/esp_pronunciation?sslmode=disable"
        );
    }

    #[test]
    fn test_default_read_dsn_typical() {
        // 正常路径:典型 db 名拼出预期 DSN。
        let dsn = default_read_dsn("esp_writing");
        assert_eq!(
            dsn,
            "postgres://root@crdb-public:26257/esp_writing?sslmode=disable"
        );
    }

    #[test]
    fn test_default_write_and_read_dsn_equivalent() {
        // 边界/契约:CRDB 单集群读写同地址,两者格式必须一致。
        let db = "esp";
        assert_eq!(default_write_dsn(db), default_read_dsn(db));
        // 且都指向同一 crdb-public:26257 主机。
        let dsn = default_write_dsn(db);
        assert!(dsn.contains("crdb-public:26257"));
        assert!(dsn.ends_with("?sslmode=disable"));
        assert!(dsn.starts_with("postgres://root@"));
    }

    #[test]
    fn test_default_write_dsn_empty_db() {
        // 边界:空 db 名(零值输入)——函数仅 format,不 panic,安全返回。
        let dsn = default_write_dsn("");
        assert_eq!(dsn, "postgres://root@crdb-public:26257/?sslmode=disable");
    }

    #[test]
    fn test_default_read_dsn_empty_db() {
        // 边界:空 db 名。
        let dsn = default_read_dsn("");
        assert_eq!(dsn, "postgres://root@crdb-public:26257/?sslmode=disable");
    }

    #[test]
    fn test_default_write_dsn_special_chars() {
        // 边界:含特殊字符的 db 名——函数不做 SQL 转义(仅拼串),验证不 panic、原样插值。
        // 注:这是 DSN 拼装函数而非 SQL 构造器,特殊字符原样出现在 DSN 是符合当前契约的行为。
        let dsn = default_write_dsn("a'b;c");
        assert_eq!(dsn, "postgres://root@crdb-public:26257/a'b;c?sslmode=disable");
    }

    #[test]
    fn test_default_write_dsn_unicode() {
        // 边界:非 ASCII db 名——验证不 panic、UTF-8 原样插值。
        let dsn = default_write_dsn("测试库");
        assert!(dsn.contains("测试库"));
    }

    // ---- connect(异步,无活体 CRDB → 测错误路径)----

    /// 生成一个"几乎不可能被宿主环境预设"的 env key,避免并行测试间环境变量串扰。
    fn unique_env_key(tag: &str) -> String {
        // line!() 保证本文件内唯一,process-全局 env 命名空间下不与他测试冲突。
        format!("ESP_TEST_DB_UNSET_{}_{}", tag, line!())
    }

    #[tokio::test]
    async fn test_connect_unreachable_returns_err() {
        // 异常:不可达 DSN(端口 1 立即 ECONNREFUSED)→ Err,不 panic。
        // env key 未设 → 落到 default_dsn(不可达)。
        let key = unique_env_key("unreachable");
        let dsn = "postgres://root@127.0.0.1:1/esp?sslmode=disable".to_string();
        let res = connect(&key, dsn, "svc-test").await;
        assert!(res.is_err(), "不可达 DSN 必须返回 Err, got Ok");
    }

    #[tokio::test]
    async fn test_connect_empty_dsn_returns_err() {
        // 异常/边界:空 DSN → sqlx 解析失败 → Err,不 panic。
        let key = unique_env_key("empty");
        let res = connect(&key, String::new(), "svc-test").await;
        assert!(res.is_err(), "空 DSN 必须返回 Err, got Ok");
    }

    #[tokio::test]
    async fn test_connect_invalid_dsn_returns_err() {
        // 异常:格式错误 DSN(非 postgres scheme)→ sqlx 解析失败 → Err,不 panic。
        let key = unique_env_key("invalid");
        let dsn = "not-a-valid-dsn://garbage".to_string();
        let res = connect(&key, dsn, "svc-test").await;
        assert!(res.is_err(), "非法 DSN 必须返回 Err, got Ok");
    }

    #[tokio::test]
    async fn test_connect_env_set_does_not_panic() {
        // 降级安全:设了 env(指向不可达),connect 读 env 路径不应 panic、应安全返回 Err。
        // 注:无活体 CRDB 无法从结果消歧"env 被读取 vs default 被读取"(两者都 Err),
        // 本测试仅保证 env 已设场景不 panic,覆盖证明需活体 DB(诚实记录)。
        let key = unique_env_key("envset");
        // SAFETY(2021 edition set_var 安全):key 唯一,不与并行测试读写同一 key。
        std::env::set_var(&key, "postgres://root@127.0.0.1:1/esp?sslmode=disable");
        let res = connect(&key, "postgres://root@127.0.0.1:2/esp?sslmode=disable".to_string(), "svc-test").await;
        // 还原环境,避免污染后续测试。
        std::env::remove_var(&key);
        assert!(res.is_err(), "设了不可达 env 也必须返回 Err, got Ok");
    }

    // ---- connect_rw(内部固定读 WRITE_DB_URL/READ_DB_URL,唯一测试避免并行竞争)----

    #[tokio::test]
    async fn test_connect_rw_unreachable_returns_err() {
        // 异常:connect_rw 设不可达读写地址 → Err(写库先失败)。
        // 本测试是本文件唯一读写 WRITE_DB_URL/READ_DB_URL 的测试,无并行竞争。
        // SAFETY(2021 edition set_var 安全):本文件仅此处读写这两个 key。
        std::env::set_var("WRITE_DB_URL", "postgres://root@127.0.0.1:1/esp?sslmode=disable");
        std::env::set_var("READ_DB_URL", "postgres://root@127.0.0.1:1/esp?sslmode=disable");
        let res = connect_rw("esp", "svc-test").await;
        // 还原环境。
        std::env::remove_var("WRITE_DB_URL");
        std::env::remove_var("READ_DB_URL");
        assert!(res.is_err(), "不可达读写 DSN 必须返回 Err, got Ok");
    }
}
