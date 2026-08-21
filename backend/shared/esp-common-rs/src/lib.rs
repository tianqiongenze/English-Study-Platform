// Package esp_common —— ESP Rust 服务公共库(会话22 债1+2)。
// 消除 4 个 Rust 服务(pronunciation-rs / assessment-rs / llm-orchestrator-rs / media-learn-rs)里
// 逐字复制的:tracing 初始化、TiDB 读写连接池、Redis Cluster 异步连接、axum 健康检查 handler。
//
// 设计原则:行为不变红线 —— 不改 HTTP body 契约、不改 env 命名、不改连接池默认值,
// 仅把重复代码上提。error enum 不抽(改 HTTP body 契约,单列评估)。

pub mod db;
pub mod debug;
pub mod health;
pub mod metrics;
pub mod redisx;
pub mod tracing_init;

#[cfg(test)]
mod tests {
    //! lib(crate 根)单测 —— 模块接线校验(工程铁律②设计-实现-验证三态追踪)。
    //! 覆盖:6 个 pub mod 路径可达(编译期即证明接线,运行期再 smoke 引用核心符号)。
    //! CLC:lib 仅模块声明无 nil 隐患;此测验证"配而未接"反向 —— 确保 pub mod 实际导出目标符号。

    #[test]
    fn modules_are_reachable_from_crate_root() {
        // 编译期已校验 pub mod 存在;运行期 use 引用每个模块路径(若某 mod 未导出目标符号则编译失败)。
        // 仅做"符号存在"smoke,不触发网络/DB(避免依赖外部中间件)。
        use crate::{db, debug, health, metrics, redisx, tracing_init};

        // 引用各模块公开符号(函数项 / const),证明导出且非 placeholder。
        // async fn 的函数项无法写成具体 fn 指针类型(返回 impl Future 不透明),
        // 故用 `let _ = path` 触发符号解析校验导出,再在 tokio 测试里实调用验证。
        let _ = health::health;
        let _ = health::liveness;
        let _ = tracing_init::init;
        // db/redisx/metrics/debug 的公开符号也存在(编译期校验):
        let write_dsn = db::default_write_dsn("esp_test");
        assert!(write_dsn.contains("esp_test"));
        let _router = metrics::metrics_router();
        let _addr: &'static str = redisx::DEFAULT_ADDR;
        assert_eq!(_addr, "redis://redis-cluster:6379");
        let _dbg = debug::debug_router();
    }

    #[tokio::test]
    async fn health_handler_smoke_through_crate_path() {
        // 端到端 smoke:经 crate 根路径调用 health,验证接线后行为正确(非 placeholder)。
        let v = crate::health::health("smoke-rs").await.0;
        assert_eq!(v["status"], "UP");
        assert_eq!(v["service"], "smoke-rs");
    }

    #[tokio::test]
    async fn liveness_handler_smoke_through_crate_path() {
        let v = crate::health::liveness().await.0;
        assert_eq!(v["status"], "UP");
    }

    #[test]
    fn crate_name_constant_matches_cargo_toml() {
        // 行为不变红线:lib name = "esp_common"(Cargo.toml [lib] name)。
        // module_path!() 在测试 mod 内返回 "esp_common::tests",含 crate 名前缀即证明 crate 名正确。
        let root_mod: &str = module_path!();
        assert!(
            root_mod.starts_with("esp_common"),
            "crate root module path should start with esp_common, got {root_mod}"
        );
    }
}
