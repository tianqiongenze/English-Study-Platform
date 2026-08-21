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

#[cfg(test)]
mod tests {
    //! health handler 单测(对齐 axum 0.7 `Json<Value>` 直接解构 `.0` 风格,零额外 dev-dep)。
    //! 覆盖:正常路径 / 边界(空 service_name / 超长字符串)/ 契约不变量(status=="UP" 结构稳定)。
    //! CLC 铁律:health/liveness 入参为 `&'static str`(Rust 无 nil),边界用空串/极长串验证不 panic。

    use super::{health, liveness};
    use serde_json::Value;

    /// 辅助:断言 health 返回 {"status":"UP","service":<name>} 契约。
    fn assert_health_contract(v: &Value, expected_service: &str) {
        assert_eq!(v["status"], "UP", "status must always be UP");
        assert_eq!(
            v["service"], expected_service,
            "service field must echo the passed service_name"
        );
        // 仅 2 个 key,防止未来误加字段破坏契约(行为不变红线)。
        assert_eq!(
            v.as_object().map(|m| m.len()),
            Some(2),
            "health body must have exactly 2 keys: status + service"
        );
    }

    #[tokio::test]
    async fn health_normal_path() {
        let v = health("pronunciation-rs").await.0;
        assert_health_contract(&v, "pronunciation-rs");
    }

    #[tokio::test]
    async fn health_boundary_empty_service_name() {
        // 边界:空 service_name —— Rust 无 nil,&str 零值即 ""。验证不 panic 且原样回显。
        let v = health("").await.0;
        assert_health_contract(&v, "");
    }

    #[tokio::test]
    async fn health_boundary_long_service_name() {
        // 边界:极长 service_name,验证无截断、无 panic。
        // health 签名要 &'static str;把 String leak 为 &'static str(测试场景,进程退出即回收)。
        let owned = "x".repeat(4096);
        let len = owned.len();
        let leaked: &'static str = Box::leak(owned.into_boxed_str());
        let v = health(leaked).await.0;
        assert_eq!(v["status"], "UP");
        assert_eq!(v["service"].as_str().map(str::len), Some(len));
    }

    #[tokio::test]
    async fn health_contract_status_always_up() {
        // 多 service_name 抽样,强化 status 恒 UP 的契约不变量。
        for svc in ["a", "assessment-rs", "llm-orchestrator-rs", ""] {
            let v = health(svc).await.0;
            assert_eq!(v["status"], "UP", "status must be UP for svc={svc:?}");
            assert_eq!(v["service"], svc);
        }
    }

    #[tokio::test]
    async fn health_special_chars_service_name() {
        // 异常输入:含特殊字符/unicode 的 service_name,验证原样透传不转义错误。
        let v = health("服务- rs_1.0").await.0;
        assert_eq!(v["status"], "UP");
        assert_eq!(v["service"], "服务- rs_1.0");
    }

    #[tokio::test]
    async fn liveness_normal_path() {
        let v = liveness().await.0;
        assert_eq!(v["status"], "UP", "liveness status must be UP");
        assert_eq!(
            v.as_object().map(|m| m.len()),
            Some(1),
            "liveness body must have exactly 1 key: status"
        );
    }

    #[tokio::test]
    async fn liveness_contract_no_service_field() {
        // 契约:liveness 不含 service 字段(与 health 区分,对齐原各服务 handler)。
        let v = liveness().await.0;
        assert!(v.get("service").is_none(), "liveness must not carry service");
    }
}
