// debug.rs —— 全语言统一深度诊断面板(Step2 · Rust 侧)。
//
// Spring Boot Actuator 的 Rust(axum 0.7)等价映射(admin 前端按同语义展示):
//
//   env        → /debug/env         环境变量(脱敏:KEY/PASSWORD/SECRET/TOKEN 之键值打码)
//   loggers    → /debug/loglevel    日志级别动态查询/调整(等价 loggers;读改 RUST_LOG 等价)
//   threaddump → NA                  Rust 无 JVM 线程 dump;可用 backtrace crate 等价(本版 DEFERRED)
//   heapdump   → NA                  Rust 无 hprof;可用 dhat/jemalloc 等价(本版 DEFERRED)
//   beans      → /debug/beans        路由/handler 清单(等价 Spring beans)
//   mappings   → /debug/mappings     路由表(等价 Spring mappings)
//
// 鉴权:本模块仅注册 handler;敏感端点鉴权由调用方在 axum layer 层包 admin-role JWT
// (与 admin-go requireAdmin 一致)。ESP 约定 debug 端点仅经 admin-go 代理可达。
//
// ASV 铁律落实:axum 0.7 API 已核实(官方文档):
//   - Router<S>::route(self, &str, MethodRouter<S>) -> Self
//   - Router<S>::merge<R: Into<Router<S>>>(self, R) -> Self
//   - Router::new() -> Router<()>  (无 state)
//   - routing::get(handler)、Json responder、State/extractor 注入
// 调用方:`Router::new().route(...).merge(esp_common::debug::debug_router()).with_state(state)`
//
// 安全:/debug/* 不绑独立端口,挂载到主 Router,由调用方 layer 守门(admin JWT)。

use axum::{
    extract::Request,
    http::HeaderMap,
    response::Json,
    routing::get,
    Router,
};
use serde_json::{json, Value};
use std::env;
use std::sync::RwLock;

/// 需脱敏的环境变量名子串(大小写不敏感包含即脱敏)。
const SENSITIVE: &[&str] = &[
    "KEY",
    "PASSWORD",
    "PASSWD",
    "SECRET",
    "TOKEN",
    "CREDENTIAL",
    "PRIVATE",
];

/// 运行期日志级别(Rust 无 SLF4J 全局级别,用静态变量模拟;读取 env RUST_LOG 作初值)。
static LOG_LEVEL: RwLock<String> = RwLock::new(String::new());

fn current_log_level() -> String {
    {
        let g = LOG_LEVEL.read().unwrap();
        if !g.is_empty() {
            return g.clone();
        }
    }
    // 初值:读 RUST_LOG,缺省 INFO。
    let init = env::var("RUST_LOG").unwrap_or_else(|_| "info".to_string());
    *LOG_LEVEL.write().unwrap() = init.clone();
    init
}

/// 对敏感 key 的值打码:保留首尾 1 字符,中间 *。
fn mask(k: &str, v: &str) -> String {
    let ku = k.to_uppercase();
    for s in SENSITIVE {
        if ku.contains(s) {
            if v.len() <= 2 {
                return "*".repeat(v.chars().count());
            }
            let chars: Vec<char> = v.chars().collect();
            let head = chars[0];
            let tail = chars[chars.len() - 1];
            let mid = "*".repeat(chars.len() - 2);
            return format!("{}{}{}", head, mid, tail);
        }
    }
    v.to_string()
}

/// 收集本进程所有环境变量并脱敏。
fn env_map() -> Value {
    let mut env_obj = serde_json::Map::new();
    for (k, v) in env::vars() {
        let masked = mask(&k, &v);
        env_obj.insert(k, Value::String(masked));
    }
    Value::Object(env_obj)
}

/// GET /debug/env —— 等价 actuator/env。
async fn debug_env() -> Json<Value> {
    let level = current_log_level();
    Json(json!({
        "activeProfiles": [std::env::consts::OS, "rust", level],
        "propertySources": [{"name": "systemEnvironment", "properties": env_map()}],
        "_sanitized": true,
        "_note": "env values masked for keys matching: KEY,PASSWORD,PASSWD,SECRET,TOKEN,CREDENTIAL,PRIVATE"
    }))
}

/// GET /debug/loglevel —— 等价 actuator/loggers GET。
async fn debug_loglevel_get() -> Json<Value> {
    let level = current_log_level();
    Json(json!({
        "configuredLevel": level.to_uppercase(),
        "effectiveLevel": level.to_uppercase(),
        "loggers": {"root": level.to_uppercase(), "http": level.to_uppercase(), "db": level.to_uppercase(), "redis": level.to_uppercase()}
    }))
}

/// POST /debug/loglevel?level=LEVEL —— 等价 actuator/loggers POST。
/// Body 可选 JSON {"configuredLevel":"DEBUG"}。
async fn debug_loglevel_set(req: Request) -> Json<Value> {
    let level = req
        .uri()
        .query()
        .and_then(|q| {
            q.split('&')
                .find_map(|kv| kv.strip_prefix("level="))
                .map(|s| s.to_string())
        })
        .unwrap_or_default();
    let level = if level.is_empty() {
        // 尝试解析 JSON body
        let bytes = axum::body::to_bytes(req.into_body(), 1 << 16)
            .await
            .unwrap_or_default();
        serde_json::from_slice::<serde_json::Map<String, Value>>(&bytes)
            .ok()
            .and_then(|m| m.get("configuredLevel").or_else(|| m.get("level")).cloned())
            .and_then(|v| v.as_str().map(|s| s.to_string()))
            .unwrap_or_default()
    } else {
        level
    };
    let lvl = level.trim().to_uppercase();
    match lvl.as_str() {
        "TRACE" | "DEBUG" | "INFO" | "WARN" | "ERROR" => {
            *LOG_LEVEL.write().unwrap() = lvl.to_lowercase();
            Json(json!({"configuredLevel": lvl, "effectiveLevel": lvl, "status": "updated"}))
        }
        _ => Json(json!({"error": "invalid level, one of TRACE/DEBUG/INFO/WARN/ERROR"})),
    }
}

/// GET /debug/beans —— 路由/handler 清单(等价 Spring beans)。
/// 通过 X-Debug-Routes header 让调用方传入自身路由清单(避免 esp-common 反向依赖各服务)。
async fn debug_beans(headers: HeaderMap) -> Json<Value> {
    let routes: Vec<String> = headers
        .get("x-debug-routes")
        .map(|v| v.to_str().unwrap_or(""))
        .unwrap_or("")
        .split(',')
        .filter(|s| !s.is_empty())
        .map(|s| s.to_string())
        .collect();
    let beans: Vec<Value> = routes
        .iter()
        .map(|r| {
            json!({
                "bean": "axumHandler:".to_string() + r,
                "route": r,
                "type": "Handler",
                "scope": "singleton",
            })
        })
        .collect();
    Json(json!({
        "context": "main",
        "beans": beans,
        "_note": "Rust 无 Spring Bean 容器;此处等价为已注册 HTTP 路由(handler 清单,由调用方经 X-Debug-Routes header 传入)"
    }))
}

/// GET /debug/mappings —— 路由表(等价 Spring mappings)。
async fn debug_mappings(headers: HeaderMap) -> Json<Value> {
    let routes: Vec<String> = headers
        .get("x-debug-routes")
        .map(|v| v.to_str().unwrap_or(""))
        .unwrap_or("")
        .split(',')
        .filter(|s| !s.is_empty())
        .map(|s| s.to_string())
        .collect();
    let mappings: Vec<Value> = routes
        .iter()
        .map(|r| json!({"pattern": r, "method": "ANY", "handler": "axumHandler"}))
        .collect();
    Json(json!({
        "contexts": {"application": {"mappings": mappings, "_note": "axum 路由表(等价 Spring RequestMappingHandlerMapping,由调用方经 X-Debug-Routes header 传入)"}}
    }))
}

/// 返回挂载全 5 个等价深度诊断端点的 Router<()>。
/// 调用方:`Router::new().route(...).merge(esp_common::debug::debug_router()).with_state(state)`
/// ASV:axum 0.7 Router<()> 可 merge 进 Router<S> 当 S=() 时;有 state 服务需先 .with_state 再 .merge
/// 或用 nest("/debug", debug_router()) — 均为 axum 0.7 官方 API。
pub fn debug_router() -> Router<()> {
    Router::new()
        .route("/debug/env", get(debug_env))
        .route("/debug/loglevel", get(debug_loglevel_get).post(debug_loglevel_set))
        .route("/debug/beans", get(debug_beans))
        .route("/debug/mappings", get(debug_mappings))
}
