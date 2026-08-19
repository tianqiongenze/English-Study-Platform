package main

// 中间件统一访问面板后端(BFF 代理层,工业级——不暴露宿主 IP 到前端浏览器)。
//
// 现状背景(2026-08-19 修复):admin 前端 GovernanceDashboard.tsx 硬编码
//   href="http://192.168.5.15:30300/d/esp-overview"(Grafana)
//   href="http://192.168.5.15:8858"(Sentinel)
// 问题:192.168.5.15 是 VM IP,前端用户浏览器(集群外/Mac 宿主)访问不到 VM IP:端口;
//       且 30300 是旧下沉前端口,下沉后宿主 Grafana 在 :3000;Sentinel 在集群内 svc:8858。
//
// 方案(ADR: 中间件 BFF 代理):浏览器只访问 BFF(经 nginx /api/admin/ 已可达),
//   admin-go 在集群内经 ExternalName svc 透明转发到宿主中间件 UI,前端不持宿主 IP。
//   鉴权:复用 requireAdmin(role=admin RS256 JWT),与其它 admin API 对齐。
//   防 SSRF:仅转发到固定白名单目标(FQDN),不接受任意 host 参数。
//
// 路由(enhanced ServeMux,Go 1.22+ 模式匹配):
//   GET /api/admin/middleware/health          → 各中间件在线状态聚合
//   ALL /api/admin/middleware/{name}/{tail...}→ 反向代理到 {name} 对应 UI(剥前缀)
//
// ASV 铁律:httputil.ReverseProxy 用 Rewrite(func(*httputl.ProxyRequest){r.SetURL(u)})
//   已在 Go 1.26.2 实测确认签名(非臆造);ServeMux `{name}/` 前缀匹配 + r.PathValue 实测确认。

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// MiddlewareTarget 中间件代理目标(白名单,防 SSRF)。
type MiddlewareTarget struct {
	Name string // 前端展示名 + 路由 {name}
	Addr string // 集群内可解析 FQDN:port(不暴露宿主 IP)
	Port int    // 仅用于 health 探活的端口展示
	Kind string // observability|governance
	// HealthPath:探活路径(空串=TCP 探活)。UI 类用 /api/health 或 /-/ready。
	HealthPath string
	// HealthType: http|tcp(对齐 catalog.json 的 Target.Type 语义)。
	HealthType string
}

// middlewareTargets 白名单(防 SSRF:前端只能访问此表内的 {name})。
// 地址均为集群内可解析的 FQDN(经 ExternalName svc → host.lima.internal 或跨 ns svc)。
// Sentinel 在集群内容器(nsentinel.ekb-m2-aws-us-east-1),非宿主进程。
var middlewareTargets = []MiddlewareTarget{
	{Name: "grafana", Addr: "grafana.esp-observability.svc.cluster.local:3000", Port: 3000, Kind: "observability", HealthPath: "/api/health", HealthType: "http"},
	{Name: "prometheus", Addr: "prometheus.esp-observability.svc.cluster.local:9090", Port: 9090, Kind: "observability", HealthPath: "/-/ready", HealthType: "http"},
	{Name: "loki", Addr: "loki.esp-observability.svc.cluster.local:3100", Port: 3100, Kind: "observability", HealthPath: "/ready", HealthType: "http"},
	{Name: "alertmanager", Addr: "alertmanager.esp-observability.svc.cluster.local:9093", Port: 9093, Kind: "observability", HealthPath: "/-/healthy", HealthType: "http"},
	{Name: "tempo", Addr: "tempo.esp-observability.svc.cluster.local:3200", Port: 3200, Kind: "observability", HealthPath: "/ready", HealthType: "http"},
	{Name: "nacos", Addr: "nacos:8848", Port: 8848, Kind: "governance", HealthPath: "/nacos/v1/console/health/readiness", HealthType: "http"},
	{Name: "sentinel", Addr: "sentinel.ekb-m2-aws-us-east-1.svc.cluster.local:8858", Port: 8858, Kind: "governance", HealthPath: "", HealthType: "tcp"},
	{Name: "otel-collector", Addr: "otel-collector.esp-observability.svc.cluster.local:8889", Port: 8889, Kind: "observability", HealthPath: "/metrics", HealthType: "http"},
}

// middlewareByName O(1) 查找。name 在路由 PathValue 取得,白名单外的 name 不代理。
func middlewareByName(name string) (MiddlewareTarget, bool) {
	for _, t := range middlewareTargets {
		if t.Name == name {
			return t, true
		}
	}
	return MiddlewareTarget{}, false
}

// mwProxyTimeout 反向代理整体超时(对齐 nginx proxy_read_timeout 30s)。
const mwProxyTimeout = 30 * time.Second

// handleMiddlewareProxy 反向代理 /api/admin/middleware/{name}/<tail> 到目标 UI。
// 剥前缀:前端 /api/admin/middleware/grafana/d/esp-overview → 目标 /d/esp-overview。
// 鉴权:requireAdmin 包裹(在 main.go 注册时即包,这里只做转发)。
func (s *Server) handleMiddlewareProxy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	t, ok := middlewareByName(name)
	if !ok {
		http.Error(w, `{"error":"unknown middleware"}`, http.StatusNotFound)
		return
	}
	target, err := url.Parse(fmt.Sprintf("http://%s", t.Addr))
	if err != nil {
		http.Error(w, `{"error":"bad upstream"}`, http.StatusBadGateway)
		return
	}

	// 剥前缀:去掉 /api/admin/middleware/{name},保留剩余 path + query。
	// r.URL.Path 形如 /api/admin/middleware/grafana/d/esp-overview;剥到 /d/esp-overview。
	prefix := fmt.Sprintf("/api/admin/middleware/%s", name)
	stripped := strings.TrimPrefix(r.URL.Path, prefix)
	if stripped == "" || stripped == "/" {
		stripped = "/" // 根路径保留 /
	}

	// ReverseProxy:Rewrite 重写出站请求的 URL/Host(现代 API,Go 1.21+)。
	// ASV:Rewrite 接收 *httputil.ProxyRequest,其 SetURL(*url.URL) 设目标,
	//      r.Out.Host 置空让下游按 Host 头自定(转发 UI 时设目标 Host,避免 Grafana 绑定 host 校验)。
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = stripped
			pr.Out.URL.RawQuery = r.URL.RawQuery
			pr.Out.Host = target.Host // 设目标 Host,部分 UI(Grafana/Nacos)按 Host 路由
		},
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConns:        20,
			IdleConnTimeout:     90 * time.Second,
			ResponseHeaderTimeout: mwProxyTimeout,
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, fmt.Sprintf(`{"error":"middleware unreachable","detail":"%s"}`, err.Error()), http.StatusBadGateway)
		},
	}
	rp.ServeHTTP(w, r)
}

// MiddlewareHealthItem 单个中间件健康状态。
type MiddlewareHealthItem struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Addr      string `json:"addr"`
	Status    string `json:"status"` // UP|DOWN
	LatencyMS int    `json:"latency_ms"`
	Msg       string `json:"msg,omitempty"`
}

// handleMiddlewareHealth 并发探活所有中间件,返回聚合状态。
// 复用 probeOne 的语义但不直接调(避免改 catalog 表),独立实现 http/tcp 探活。
func (s *Server) handleMiddlewareHealth(w http.ResponseWriter, _ *http.Request) {
	out := make([]MiddlewareHealthItem, len(middlewareTargets))
	var wg sync.WaitGroup
	for i, t := range middlewareTargets {
		wg.Add(1)
		go func(i int, t MiddlewareTarget) {
			defer wg.Done()
			out[i] = probeMiddleware(t)
		}(i, t)
	}
	wg.Wait()

	up := 0
	for _, h := range out {
		if h.Status == "UP" {
			up++
		}
	}
	writeJSON(w, map[string]any{
		"kind":       "middleware",
		"checked_at": time.Now().UTC().Format(time.RFC3339),
		"summary":    map[string]int{"total": len(out), "up": up, "down": len(out) - up},
		"items":      out,
	})
}

// probeMiddleware 探活单个中间件(http 探活路径返回 2xx=UP;tcp 仅测连通)。
func probeMiddleware(t MiddlewareTarget) MiddlewareHealthItem {
	h := MiddlewareHealthItem{Name: t.Name, Kind: t.Kind, Addr: t.Addr}
	start := time.Now()
	if t.HealthType == "tcp" {
		conn, err := net.DialTimeout("tcp", t.Addr, probeTimeout)
		h.LatencyMS = int(time.Since(start).Milliseconds())
		if err != nil {
			h.Status = "DOWN"
			h.Msg = err.Error()
			return h
		}
		_ = conn.Close()
		h.Status = "UP"
		return h
	}
	u := fmt.Sprintf("http://%s%s", t.Addr, t.HealthPath)
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get(u)
	h.LatencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		h.Status = "DOWN"
		h.Msg = err.Error()
		return h
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		h.Status = "UP"
	} else {
		h.Status = "DOWN"
		h.Msg = fmt.Sprintf("http %d", resp.StatusCode)
	}
	return h
}
