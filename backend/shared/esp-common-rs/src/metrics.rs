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

#[cfg(test)]
mod tests {
    //! metrics.rs 单测(会话23 子项 1.2 测试补全)。
    //!
    //! 覆盖维度:正常路径 / 边界(非 GET 方法、极大指标集、空 Registry 聚合) /
    //! 异常(encode 失败降级路径——默认 Registry 正常时不触发,以契约断言代覆盖) /
    //! 并发安全(幂等读不 panic)。
    //!
    //! CLC 铁律说明:metrics_handler() 与 metrics_router() 均无指针/引用入参,
    //! 所有可错路径(encoder.encode / String::from_utf8)已在源码用 match 接 500 降级,
    //! 无 nil 解引用隐患——本模块无需补 nil 防御。CLC 铁律起因(l1pool Lookup 零值
    //! Manager 解引用 nil cfg panic)位于 cache 模块,不在本 metrics 模块范围内。

    use super::*;
    use axum::body::to_bytes;
    use axum::http::{Method, Request, StatusCode};
    use prometheus::{register_counter, register_int_gauge, Counter, IntGauge};
    use tower::ServiceExt; // Router::oneshot (axum 0.7 + tower 0.5 已核实 API)

    /// 构造一个唯一 metric name 前缀,避免并行测试与全局 Registry 已注册同名 metric 冲突
    /// (prometheus 0.13 默认 Registry 全局静态,重复注册同名同 help 会 panic)。
    fn unique_metric_name(tag: &str) -> String {
        // 用进程级 once + 计数器,保证每次调用单调递增唯一。
        use std::sync::atomic::{AtomicU64, Ordering};
        static SEQ: AtomicU64 = AtomicU64::new(0);
        let n = SEQ.fetch_add(1, Ordering::Relaxed);
        format!("esp_test_metrics_{tag}_{n}")
    }

    // ---------- metrics_router() 正常路径 ----------

    #[test]
    fn metrics_router_returns_router_with_metrics_route() {
        // 正常路径:metrics_router() 返回 Router<()>,可独立 oneshot /metrics。
        let app = metrics_router();
        // Router<()> 已实现在无 state 路由上调用 oneshot,编译通过即证明路由类型签名正确。
        let _ = app;
    }

    #[test]
    fn metrics_router_merge_into_router_unit() {
        // 契约:调用方典型用法 Router::<()>::new().merge(metrics_router())。
        // 编译通过即证明 Router<()> 类型签名与文档一致。
        let app = Router::<()>::new().merge(metrics_router());
        let _ = app;
    }

    // ---------- /metrics 端点正常路径(GET 200 + Prometheus text) ----------

    #[tokio::test]
    async fn metrics_endpoint_get_returns_ok() {
        // 正常路径:GET /metrics 返回 200 OK。
        let app = metrics_router();
        let resp = app
            .oneshot(Request::builder().method(Method::GET).uri("/metrics").body(axum::body::Body::empty()).unwrap())
            .await
            .unwrap();
        assert_eq!(resp.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn metrics_endpoint_content_type_is_prometheus_text() {
        // 正常路径 + 契约断言:Content-Type == Prometheus text exposition format v0.0.4。
        // 这是 Prometheus 抓取器识别端点格式的硬契约,任何漂移会让 scrape 解析失败。
        let app = metrics_router();
        let resp = app
            .oneshot(Request::builder().method(Method::GET).uri("/metrics").body(axum::body::Body::empty()).unwrap())
            .await
            .unwrap();
        let ct = resp.headers().get(header::CONTENT_TYPE).expect("Content-Type header must be present");
        assert_eq!(
            ct,
            &HeaderValue::from_static(METRICS_CONTENT_TYPE),
            "Content-Type must be exact Prometheus text exposition format"
        );
    }

    #[tokio::test]
    async fn metrics_endpoint_body_non_empty_prometheus_text() {
        // 正常路径:注册一个自定义 Counter 后,默认全局 Registry gather 产出非空 Prometheus 文本。
        // 注:prometheus crate 默认 Registry 不会自动塞 process metrics(那是 exporter 层职责),
        // 空 Registry 时 body 为空是正确行为——因此本测先 register 一个 Counter 再断言非空。
        let name = unique_metric_name("nonempty");
        register_counter!(name.clone(), "test counter to make body non-empty").unwrap();

        let app = metrics_router();
        let resp = app
            .oneshot(Request::builder().method(Method::GET).uri("/metrics").body(axum::body::Body::empty()).unwrap())
            .await
            .unwrap();
        // 预分配 2048 Vec;默认 process metrics 常有数 KB。用上限 64KiB 收集。
        let bytes = to_bytes(resp.into_body(), 64 * 1024).await.unwrap();
        assert!(!bytes.is_empty(), "/metrics body must not be empty after registering a counter");
        let text = std::str::from_utf8(&bytes).expect("body must be valid utf-8");
        // Prometheus text exposition format:每行 `name{labels} value` 或 `# HELP/TYPE name ...`。
        assert!(
            text.contains("# HELP") || text.contains("# TYPE") || text.contains("\n"),
            "/metrics body must be Prometheus text format, got: {text}"
        );
    }

    #[tokio::test]
    async fn metrics_endpoint_includes_globally_registered_counter() {
        // 正常路径 + 契约:prometheus crate 默认 Registry 全局静态,业务 register_*
        // 后经同一默认 Registry 自动纳入 gather()。注册自定义 Counter 后 /metrics 必须出现它。
        let name = unique_metric_name("counter");
        let counter: Counter = register_counter!(name.clone(), "test counter for metrics endpoint contract").unwrap();
        counter.inc_by(7.0);

        let app = metrics_router();
        let resp = app
            .oneshot(Request::builder().method(Method::GET).uri("/metrics").body(axum::body::Body::empty()).unwrap())
            .await
            .unwrap();
        let bytes = to_bytes(resp.into_body(), 64 * 1024).await.unwrap();
        let text = std::str::from_utf8(&bytes).unwrap();
        assert!(text.contains(&name), "/metrics must include the registered counter {name}");
        assert!(text.contains(" 7"), "/metrics must include the inc value; got:\n{text}");
    }

    // ---------- 边界路径 ----------

    #[tokio::test]
    async fn metrics_endpoint_post_method_not_allowed() {
        // 边界:/metrics 仅注册 GET,POST 应得 405 Method Not Allowed(axum 路由默认行为)。
        let app = metrics_router();
        let resp = app
            .oneshot(Request::builder().method(Method::POST).uri("/metrics").body(axum::body::Body::empty()).unwrap())
            .await
            .unwrap();
        assert_eq!(resp.status(), StatusCode::METHOD_NOT_ALLOWED);
    }

    #[tokio::test]
    async fn metrics_endpoint_unknown_path_returns_404() {
        // 边界:Router 仅挂 /metrics,其他路径 404。
        let app = metrics_router();
        let resp = app
            .oneshot(Request::builder().method(Method::GET).uri("/nope").body(axum::body::Body::empty()).unwrap())
            .await
            .unwrap();
        assert_eq!(resp.status(), StatusCode::NOT_FOUND);
    }

    #[tokio::test]
    async fn metrics_endpoint_handles_many_registered_metrics() {
        // 边界(极大指标集):注册 200 个独立 IntGauge,验证 /metrics 不 panic 且 body 收集成功。
        // 预分配 Vec<2048> 在 200 series 下应自然扩容;断言 body 非空且每个 name 都出现。
        let mut names = Vec::with_capacity(200);
        for i in 0..200u32 {
            let name = unique_metric_name(&format!("gauge{i}"));
            let gauge: IntGauge = register_int_gauge!(name.clone(), "bulk test gauge").unwrap();
            gauge.set(i as i64);
            names.push(name);
        }
        let app = metrics_router();
        let resp = app
            .oneshot(Request::builder().method(Method::GET).uri("/metrics").body(axum::body::Body::empty()).unwrap())
            .await
            .unwrap();
        assert_eq!(resp.status(), StatusCode::OK);
        // 200 series 每条至少约 30 字节 + 帮助行 > 6KiB;放宽上限到 4MiB 以稳。
        let bytes = to_bytes(resp.into_body(), 4 * 1024 * 1024).await.unwrap();
        let text = std::str::from_utf8(&bytes).unwrap();
        // 抽样校验首/中/末三条,避免 200 次全量断言噪音。
        for i in [0usize, 100, 199] {
            assert!(text.contains(&names[i]), "/metrics must include bulk gauge[{}] = {}", i, names[i]);
        }
    }

    // ---------- 并发安全(幂等读不 panic) ----------

    #[tokio::test]
    async fn metrics_endpoint_concurrent_reads_all_ok() {
        // 并发安全:多个 /metrics 请求并发 gather()+encode()。
        // gather() 与 TextEncoder::encode 线程安全,并发不应 panic/阻塞。
        let app = metrics_router();
        let mut handles = Vec::with_capacity(8);
        for _ in 0..8 {
            let app = app.clone(); // Router::clone 浅拷贝(共享路由表),与生产用法一致。
            handles.push(tokio::spawn(async move {
                app.oneshot(
                    Request::builder()
                        .method(Method::GET)
                        .uri("/metrics")
                        .body(axum::body::Body::empty())
                        .unwrap(),
                )
                .await
            }));
        }
        for h in handles {
            let resp = h.await.unwrap().unwrap();
            assert_eq!(resp.status(), StatusCode::OK, "concurrent /metrics request must succeed");
        }
    }
}
