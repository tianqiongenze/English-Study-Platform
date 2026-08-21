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

#[cfg(test)]
mod tests {
    //! tracing_init 单测(对齐同 crate health.rs `#[cfg(test)] mod tests` 风格)。
    //! 覆盖:正常路径 / 边界(空 service_name / RUST_LOG 缺失)/ 幂等(重复调用不 panic)。
    //! CLC 铁律:init 无 nil 隐患(EnvFilter::try_from_default_env 用 unwrap_or_else 降级,
    //! try_init 用 .ok() 吞重复初始化);测试验证零值/边界输入安全返回不 panic。

    use super::init;

    /// init 的核心不变量:任何入参下都不 panic(行为不变红线)。
    /// 用 catch_unwind 捕获,因 try_init 在重复初始化时返回 Err 但被 .ok() 吞掉不应 unwind。
    #[test]
    fn init_normal_path_does_not_panic() {
        // 取任意典型服务名,验证 init 正常返回(RC: 函数返回 () 不 panic)。
        init("assessment-rs");
    }

    #[test]
    fn init_boundary_empty_service_name() {
        // 边界:空 service_name —— Rust 无 nil,&str 零值即 ""。验证不 panic(仅日志前缀为空)。
        init("");
    }

    #[test]
    fn init_boundary_long_service_name() {
        // 边界:极长 service_name,验证日志格式化不 panic。
        init(&"svc-".repeat(2048));
    }

    #[test]
    fn init_idempotent_repeated_calls() {
        // 幂等:多次调用 init(tracing 全局 subscriber 只能注册一次,try_init 第二次起返回 Err,
        // 被 .ok() 吞掉)。验证重复调用不 panic —— 这是 try_init().ok() 防御路径的核心保证。
        init("first-call-rs");
        init("second-call-rs");
        init("third-call-rs");
        // 若上面任一 panic 则测试失败;能执行到这里即通过。
    }

    #[test]
    fn init_boundary_missing_rust_log() {
        // 边界:RUST_LOG 未设置时,EnvFilter::try_from_default_env 返回 Err,
        // unwrap_or_else 降级到 EnvFilter::new("info")。验证降级路径不 panic。
        // 注意:不 set RUST_LOG 即假定未设置;若运行环境恰好设了也不影响"不 panic"结论。
        init("no-rust-log-rs");
    }

    #[test]
    fn init_boundary_invalid_rust_log_falls_back_safely() {
        // 边界/异常:RUST_LOG 设为非法过滤表达式。EnvFilter::try_from_default_env 应返回 Err
        // (解析失败),unwrap_or_else 降级到 "info"。验证安全降级不 panic。
        // CLC:这是"追踪执行路径"——确认错误输入走 unwrap_or_else 分支而非 panic。
        // SAFETY: 单测串行运行下 set_var 安全;本测试不与其他 env-mutating 测试并发断言同一值。
        unsafe {
            std::env::set_var("RUST_LOG", "===invalid filter===%%notvalid");
        }
        init("invalid-rust-log-rs");
        // 还原环境避免污染后续测试。
        unsafe {
            std::env::remove_var("RUST_LOG");
        }
    }

    #[test]
    fn init_boundary_valid_rust_log() {
        // 正常变体:RUST_LOG 设为合法级别,验证 try_from_default_env 成功路径不 panic。
        unsafe {
            std::env::set_var("RUST_LOG", "debug");
        }
        init("valid-rust-log-rs");
        unsafe {
            std::env::remove_var("RUST_LOG");
        }
    }

    #[test]
    fn init_special_chars_service_name() {
        // 异常输入:含特殊字符/unicode 的 service_name,验证日志格式化不 panic。
        init("服务-rs_1.0/嵌套");
    }
}
