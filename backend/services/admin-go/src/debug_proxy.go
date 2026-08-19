package main

// 全语言统一深度诊断面板(Step3 · admin-go 代理路由)。
//
// 路由:/api/admin/debug/{service}/{endpoint}
//   service = catalog.services[].name(bff-java / gateway-go / writing-py / assessment-rs ...)
//   endpoint ∈ {env, loggers, threaddump, heapdump, beans, mappings}
//
// 语义映射(actuator 风格全语言统一):
//   Java  : env→/actuator/env, loggers→/actuator/loggers, threaddump→/actuator/threaddump,
//           heapdump→/actuator/heapdump(二进制,流式透传), beans→/actuator/beans, mappings→/actuator/mappings
//   Go    : env→/debug/env, loggers→/debug/loglevel, threaddump→/debug/pprof/goroutine,
//           heapdump→/debug/pprof/heap, beans→/debug/beans, mappings→/debug/mappings
//   Python: env→/debug/env, loggers→/debug/loglevel, beans→/debug/beans, mappings→/debug/mappings
//           threaddump/heapdump → 404 NA(Python 无原生等价,标 NA)
//   Rust  : env→/debug/env, loggers→/debug/loglevel, beans→/debug/beans, mappings→/debug/mappings
//           threaddump/heapdump → 404 NA
//
// 鉴权:全 6 个敏感 endpoint 必走 admin JWT(requireAdmin 守门)。
// 代理转发时透传原始 admin token(让下游服务侧 DebugEndpointSecurityFilter/RequireAdminDebug 二次校验)。
// 防 SSRF:仅转发到 catalog 内已登记服务 + 白名单 endpoint + 白名单路径。

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// debugEndpointMap 各 endpoint → 各语言真实路径。NA=该语言无原生等价。
var debugEndpointMap = map[string]map[string]string{
	"Java": {
		"env":        "/actuator/env",
		"loggers":    "/actuator/loggers",
		"threaddump": "/actuator/threaddump",
		"heapdump":   "/actuator/heapdump",
		"beans":      "/actuator/beans",
		"mappings":   "/actuator/mappings",
	},
	"Go": {
		"env":        "/debug/env",
		"loggers":    "/debug/loglevel",
		"threaddump": "/debug/pprof/goroutine",
		"heapdump":   "/debug/pprof/heap",
		"beans":      "/debug/beans",
		"mappings":   "/debug/mappings",
	},
	"Python": {
		"env":        "/debug/env",
		"loggers":    "/debug/loglevel",
		"beans":      "/debug/beans",
		"mappings":   "/debug/mappings",
		// threaddump/heapdump: NA
	},
	"Rust": {
		"env":        "/debug/env",
		"loggers":    "/debug/loglevel",
		"beans":      "/debug/beans",
		"mappings":   "/debug/mappings",
		// threaddump/heapdump: NA
	},
}

// debugSupportedEndpoints 前端能力矩阵:某语言支持哪些 endpoint。
func debugSupportedEndpoints(lang string) []string {
	m, ok := debugEndpointMap[lang]
	if !ok {
		return nil
	}
	// 固定顺序(与 actuator 全 6 对齐),便于前端展示。
	order := []string{"env", "loggers", "threaddump", "heapdump", "beans", "mappings"}
	out := make([]string, 0, len(order))
	for _, e := range order {
		if _, ok := m[e]; ok {
			out = append(out, e)
		}
	}
	return out
}

// handleDebugProxy /api/admin/debug/{service}/{endpoint} 代理到对应服务 debug/actuator 端点。
// 方法:GET(查询)/ POST(动态调日志级别:?level= 或 body)。
func (s *Server) handleDebugProxy(w http.ResponseWriter, r *http.Request) {
	// 路径:/api/admin/debug/{service}/{endpoint}
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/debug/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, `{"error":"bad path, expect /api/admin/debug/{service}/{endpoint}"}`, http.StatusBadRequest)
		return
	}
	svcName, endpoint := parts[0], parts[1]

	// 查 catalog 定位服务语言 + 地址(防 SSRF:仅 catalog 内服务可达)。
	target, ok := s.findService(svcName)
	if !ok {
		http.Error(w, `{"error":"service not in catalog"}`, http.StatusNotFound)
		return
	}
	langMap, ok := debugEndpointMap[target.Lang]
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"lang %s not supported for debug"}`, target.Lang), http.StatusNotFound)
		return
	}
	path, ok := langMap[endpoint]
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"endpoint %s not available for %s (NA)"}`, endpoint, target.Lang), http.StatusNotFound)
		return
	}

	// 构造下游 URL(透传 query,如 ?level=DEBUG / ?seconds=30)。
	u := fmt.Sprintf("http://%s:%d%s", target.Svc, target.Port, path)
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}

	// 复制下游请求(透传方法 + 原始 admin token,让服务侧二次校验)。
	req, err := http.NewRequestWithContext(r.Context(), r.Method, u, r.Body)
	if err != nil {
		http.Error(w, `{"error":"build downstream request failed"}`, http.StatusInternalServerError)
		return
	}
	// 透传 admin token(让下游 DebugEndpointSecurityFilter/RequireAdminDebug 二次校验 role=admin)。
	if authz := r.Header.Get("Authorization"); authz != "" {
		req.Header.Set("Authorization", authz)
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	// 透传可信身份头:admin-go requireAdmin 已校验 role==admin(line 207-213),
	// 此处注入 X-Role=admin / X-User-Id=<uid> 供服务侧 RequireAdminDebug 二次守门(校验 X-Role==admin),
	// 与 BFF EdgeAuthHandler 注入语义一致(外网 X-Role 已被 BFF/admin-go 剥离,此值为代理可信注入)。
	// requireAdmin 已 decode 出 uid(verifyToken 返回),但 handler 链未携带;这里从 JWT claims 再取一次。
	if uid, role, err := verifyToken(tokenFromRequest(r), s.pubKey); err == nil && role == "admin" {
		req.Header.Set("X-Role", "admin")
		req.Header.Set("X-User-Id", fmt.Sprintf("%d", uid))
	}
	// 标识来源(便于服务侧审计:经 admin-go 代理)。
	req.Header.Set("X-Debug-Via", "admin-go")

	client := &http.Client{Timeout: 15 * 1000000000} // 15s
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"downstream unreachable","detail":"%s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 透传响应(heapdump=二进制流,env/loggers/beans/mappings=JSON)。
	// heapdump 含 Content-Disposition: attachment;透传让浏览器下载。
	for _, h := range []string{"Content-Type", "Content-Disposition", "Content-Length"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleDebugCapabilities /api/admin/debug/capabilities — 返回各语言支持的 endpoint 清单(前端能力矩阵)。
func (s *Server) handleDebugCapabilities(w http.ResponseWriter, _ *http.Request) {
	caps := make(map[string][]string, len(debugEndpointMap))
	for lang := range debugEndpointMap {
		caps[lang] = debugSupportedEndpoints(lang)
	}
	writeJSON(w, map[string]any{
		"capabilities": caps,
		"endpoints":     []string{"env", "loggers", "threaddump", "heapdump", "beans", "mappings"},
		"note":          "threaddump/heapdump: Java actuator 原生;Go pprof 等价;Python/Rust NA(无原生等价,标 NA)",
	})
}

// findService 在 catalog 中按 name 查服务(语言 + 地址)。
func (s *Server) findService(name string) (Target, bool) {
	for _, t := range s.catalog.Services {
		if t.Name == name {
			return t, true
		}
	}
	return Target{}, false
}
