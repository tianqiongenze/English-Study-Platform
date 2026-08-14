// Package tracing —— ESP Go 服务统一 OTel 初始化(会话22-3 债1+2)。
// 分层:轻量(仅 propagator,如 course-path-go/live-tutor-go)/ 完整(OTLP exporter,如 gateway-go)。
// vocab-srs-go 自定义 X-Trace-Id 迁移到此(单列评估下游依赖后切换,本包先提供 OTel propagator 中间件)。
package tracing

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// InitPropagator 设置 W3C TraceContext 复合 propagator(轻量,无 exporter)。
// 对齐 course-path-go / live-tutor-go 原 SetTextMapPropagator(TraceContext{}) 行为。
func InitPropagator() {
	otel.SetTextMapPropagator(propagation.TraceContext{})
}

// TraceparentFromRequest 从入站请求提取 traceparent 字符串(供日志关联,非 OTel 全链路)。
// 兼容 vocab-srs-go 原 X-Trace-Id 透传场景的过渡:有 traceparent 头则透传,否则返回空(由调用方决定生成)。
func TraceparentFromRequest(r *http.Request) string {
	return r.Header.Get("traceparent")
}

// InjectTraceparent 向出站请求注入当前 context 的 traceparent(供 HTTP 客户端跨服务传播)。
func InjectTraceparent(ctx context.Context, r *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(r.Header))
}
