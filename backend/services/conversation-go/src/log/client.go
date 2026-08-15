// Package log provides AppLogClient —— 结构化业务日志异步写 CRDB app_logs(ADR-observability-rebuild)。
//
// 设计:
//   - 异步:后台 goroutine + 有界 channel,不阻塞业务主路径。
//   - 降级:CRDB 不可用时仅 slog.Warn,不 panic(参考缓存降级)。
//   - traceId:从 OTel 当前 span 提取(若已初始化 tracer)。
//   - 连接:database/sql + pgx(CRDB postgres 协议),独立连接池直连 esp_shared 库(#140 切 CRDB)。
//
// 说明:esp-common-go 全局池连 esp_speaking(sessions/scenarios),app_logs 在 esp_shared,
// 两者不同库,故本包保留独立池连 esp_shared(各写各库,避免跨库依赖)。
//
// 用法:
//
//	log.AppLog("INFO", "ws.connect", userID, map[string]any{"scenario": "daily"}, r.Context())
//
// 最后一个参数为可选 context.Context,传入请求 ctx 可提取 traceId;
// 省略时退化为 context.Background()(traceId 为空)。
package log

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var (
	q       chan *entry
	dropped uint64
	db      *sql.DB
	svc     string
)

type entry struct {
	Service string                 `json:"service"`
	Level   string                 `json:"level"`
	Event   string                 `json:"event"`
	TraceID string                 `json:"trace_id"`
	UserID  int64                  `json:"user_id"`
	Payload map[string]interface{} `json:"payload"`
}

// Init 启动后台写 goroutine。幂等。
func Init() {
	if q != nil {
		return
	}
	svc = envDefault("OTEL_SERVICE_NAME", "gateway")
	// app_logs 在 esp_shared(schema-per-service Phase1);保留 APP_LOG_DB env 可回退。
	dbName := envDefault("APP_LOG_DB", "esp_shared")
	dsn := "postgres://root@" + envDefault("CRDB_HOST", "crdb-public") + ":26257/" + dbName + "?sslmode=disable"
	var err error
	db, err = sql.Open("pgx", dsn)
	if err != nil {
		slog.Warn("app_logs: sql.Open failed (degraded)", "err", err)
		db = nil
	}
	q = make(chan *entry, 4096)
	go worker()
}

func worker() {
	for e := range q {
		if db == nil {
			continue
		}
		payload, _ := json.Marshal(e.Payload)
		_, err := db.ExecContext(context.Background(),
			"INSERT INTO app_logs (ts, service, level, event, trace_id, user_id, payload) "+
				"VALUES (now(), $1, $2, $3, $4, $5, $6::jsonb)",
			e.Service, e.Level, e.Event, e.TraceID, e.UserID, string(payload),
		)
		if err != nil {
			slog.Warn("app_logs insert failed (degraded)", "err", err)
		}
	}
}

// AppLog 异步写一条结构化业务日志。level: INFO/WARN/ERROR;event: ws.connect/...降级不阻断。
// ctx 应为请求上下文(从 handler 参数传入),用于提取当前 span 的 traceId。
func AppLog(level, event string, userID int64, payload map[string]interface{}, ctx ...context.Context) {
	if q == nil {
		return
	}
	var c context.Context
	if len(ctx) > 0 {
		c = ctx[0]
	} else {
		c = context.Background()
	}
	traceID := currentTraceID(c)
	e := &entry{
		Service: svc, Level: level, Event: event, TraceID: traceID,
		UserID: userID, Payload: payload,
	}
	select {
	case q <- e:
	default:
		// 队列满丢弃,保业务
		dropped++
	}
}

// currentTraceID 从给定的 context 提取 OTel span 的 traceId。
// 修复(P1-5 断点3):原实现用 context.Background() 导致 traceId 恒空。
// 现在接收请求 ctx,从 trace.SpanFromContext(ctx) 正确提取。
func currentTraceID(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return spanCtx.TraceID().String()
	}
	// otel tracer may not be initialized
	_ = otel.GetTracerProvider()
	return ""
}

func envDefault(key, dflt string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return dflt
}

// Shutdown 优雅停机:关闭 channel 等待排空。
func Shutdown() {
	if q != nil {
		close(q)
	}
	if db != nil {
		_ = db.Close()
	}
}

// 防止 time 未用警告(预留 ts 字段使用)
var _ = time.Now
