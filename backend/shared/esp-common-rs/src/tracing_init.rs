// tracing_init —— 统一 tracing_subscriber 初始化(会话22 债1+2)。
// 对齐 4 个 Rust 服务原 main 里的 fmt + EnvFilter("info" 默认) 行为。

use tracing_subscriber::EnvFilter;

/// 初始化 tracing:fmt 输出到 stdout,EnvFilter 取 RUST_LOG,缺省 info。
/// 对齐各服务原 `tracing_subscriber::fmt().with_env_filter(...).init()` 行为。
pub fn init(service_name: &str) {
    let filter =
        EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));
    tracing_subscriber::fmt()
        .with_env_filter(filter)
        .try_init()
        .ok(); // 已初始化时忽略(测试场景可能重复调用)
    tracing::info!("[{}] tracing initialized", service_name);
}
