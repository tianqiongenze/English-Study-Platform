// course-path-go —— 学习路径编排服务(N8,P0,留存引擎)
// scaffold 级:健康检查 + 路径骨架路由 + 契约桩。
// 完整 ADR/PRD/schema 见 docs/design/adr-new-services.md / prd-new-services.md / adr-schema-per-service.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"esp/common-go/config"
	"esp/common-go/health"
	"esp/common-go/middleware"
	"esp/common-go/redisx"
	"esp/common-go/registry"
	"esp/common-go/tracing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const serviceName = "course-path-go"

func main() {
	middleware.SetRateLimitService(serviceName)
	port := config.Getenv("PORT", "8093")
	log.Printf("[%s] starting on :%s", serviceName, port)

	// OTel:仅初始化 propagator(完整 exporter 待接入 esp-observability)
	tracing.InitPropagator()

	// Redis Cluster(路径状态缓存 esp:path:{user_id})
	rdb := redisx.MustNewCluster(config.Getenv("REDIS_CLUSTER", ""), serviceName)
	_ = rdb // scaffold:路径缓存待实装

	// Nacos 服务注册(优雅退出时注销)
	nacosCli := registry.RegisterNacos(serviceName, 8093)

	mux := http.NewServeMux()
	mux.HandleFunc("/actuator/health", health.StdHealthz(serviceName))
	mux.HandleFunc("/actuator/health/readiness", readiness)
	mux.HandleFunc("/actuator/health/liveness", liveness)
	mux.HandleFunc("/api/path/generate", generatePath)     // 首登生成路径(桩)
	mux.HandleFunc("/api/path/unlock", unlockNode)         // 节点解锁判定(桩)
	mux.HandleFunc("/api/path/", getPath)                  // 读取学员路径(桩)
	mux.Handle("/metrics", promhttp.Handler())

	// 限流 + 熔断(net/http 中间件,同 gateway-go 模式)
	rateLimited := middleware.HTTPRateLimit(mux, 100, 200)                                    // 100 rps,桶容量200
	protected := middleware.HTTPCircuitBreaker(rateLimited, 5, 30*time.Second, serviceName)   // 5次失败触发30s熔断

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           protected,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] server error: %v", serviceName, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("[%s] shutting down", serviceName)
	registry.DeRegister(serviceName, 8093, nacosCli)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func readiness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"status": "UP"})
}
func liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"status": "UP"})
}

// generatePath —— 首登生成路径(桩:实际调 S5 assessment 定级 + 生成技能树)
func generatePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// TODO: 调 assessment-rs 定级 → 选 path_template → 生成 path_nodes
	writeJSON(w, map[string]any{
		"path_id":    fmt.Sprintf("path-%d", time.Now().UnixNano()),
		"status":     "scaffold",
		"message":    "course-path generate: scaffold stub, see PRD §2",
		"generated":  false,
	})
}

// unlockNode —— 节点解锁判定(桩)
func unlockNode(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"unlocked": false, "status": "scaffold"})
}

// getPath —— 读取学员路径(桩)
func getPath(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"path": nil, "status": "scaffold"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(k, d string) string {
	// 已迁移至 esp/common-go/config.Getenv(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return config.Getenv(k, d)
}
