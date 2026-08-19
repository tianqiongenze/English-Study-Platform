// metrics.rs —— Rust 服务 Prometheus /metrics 端点(Phase1 子项 1.2)。
//
// 4 个 Rust 服务(assessment-rs / llm-orchestrator-rs / media-learn-rs / pronunciation-rs)
// 共用同一实现(工程铁律①公共能力共享包优先),各服务只需在主 Router merge
// `esp_common::metrics::metrics_router()` 即可暴露 GET /metrics。
//
// 设计对齐既有 Go 服务的 promhttp.Handler 模式:
//   - /metrics 端点返回 Prometheus text exposition format。
//   - 进程级 metrics 由 prometheus crate 默认 gather() 提供(等价 Go runtime
//     metrics 的子集:process_resident_memory_bytes / process_cpu_seconds_total 等)。
//   - 本版仅注册端点 + 默认 process metrics;业务自定义 Histogram/Counter 由各服务
//     自行 `prometheus::register_*` 后经同一默认 Registry 自动纳入 gather()(prometheus
//     crate 默认 Registry 是全局静态的,gather() 拉取所有已注册 metric)。后续 Phase 可扩展。
//
// ASV 铁律落实(prometheus 0.13 crate API 已核实):
//   - prometheus::gather() -> Vec<MetricFamily>  (默认全局 Registry)
//   - TextEncoder::new() -> TextEncoder, Encoder trait
//   - Encoder::encode(&self, buf: &mut String, mfs: &[MetricFamily]) -> Result<(), Error>
// 防御性编程(CLC 铁律):
//   - encode 失败不 panic,返回 500 + 错误文本(unwrap 会让 /metrics 整端点崩)。
//   - header 显式 Content-Type: text/plain; version=0.0.4(Prometheus 要求)。
//
// 调用方:`Router::new().route(...).with_state(state).merge(esp_common::metrics::metrics_router())`

use axum::{
    http::{header, HeaderValue, StatusCode},
    response::{IntoResponse, Response},
    routing::get,
    Router,
};
// ASV:Encoder trait 在 scope 才能用 encode()(prometheus 0.13 API 已核实)。
use prometheus::Encoder;

/// Prometheus text format Content-Type(官方 exposition format v0.0.4)。
const METRICS_CONTENT_TYPE: &str = "text/plain; version=0.0.4; charset=utf-8";

/// /metrics handler:拉取默认 Registry 全部 metric families → text format。
///
/// 防御:encode 出错返回 500(Prometheus 抓取失败可视化为 UP==0),不让端点 panic。
async fn metrics_handler() -> Response {
    // ASV:Encoder::encode(&self, mfs: &[MetricFamily], writer: &mut W where W: io::Write)。
    // String 只实现 fmt::Write 不实现 io::Write,故用 Vec<u8> 作目标,再 from_utf8 转 String。
    // encode_utf8 是另一选择,但经 io::Write 路径与 prometheus 官方示例一致且可统一错误处理。
    let encoder = prometheus::TextEncoder::new();
    let metric_families = prometheus::gather();
    let mut buffer: Vec<u8> = Vec::with_capacity(2048);
    match encoder.encode(&metric_families, &mut buffer) {
        Ok(()) => match String::from_utf8(buffer) {
            Ok(body) => {
                let mut resp = body.into_response();
                // Content-Type: Prometheus text exposition format v0.0.4。
                // HeaderValue::from_static 是 const fn 直接返回 HeaderValue(infallible)。
                resp.headers_mut().insert(
                    header::CONTENT_TYPE,
                    HeaderValue::from_static(METRICS_CONTENT_TYPE),
                );
                resp
            }
            Err(e) => {
                tracing::warn!(error = %e, "metrics body not utf8");
                (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    "metrics body utf8 error".to_string(),
                )
                    .into_response()
            }
        },
        Err(e) => {
            tracing::warn!(error = %e, "prometheus encode failed");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("metrics encode error: {e}"),
            )
                .into_response()
        }
    }
}

/// 返回挂载 /metrics 的 Router<()>。
/// 调用方:`Router::new().route(...).with_state(state).merge(esp_common::metrics::metrics_router())`
pub fn metrics_router() -> Router<()> {
    Router::new().route("/metrics", get(metrics_handler))
}
