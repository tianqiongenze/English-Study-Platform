package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// DebugHandlers 全语言统一深度诊断面板(Step2 · Go 侧)。
//
// Spring Boot Actuator 的 Go 等价映射(admin 前端按同语义展示):
//
//	env        → /debug/env        环境变量(脱敏:KEY/PASSWORD/SECRET/TOKEN 之键值打码)
//	loggers    → /debug/loglevel   日志级别动态查询/调整(等价 loggers;按 level 返回运行期级别)
//	threaddump → /debug/pprof/*    pprof goroutine profile(Go 无 JVM 线程,goroutine dump 等价)
//	heapdump   → /debug/pprof/heap pprof heap profile(Go 无 hprof,heap profile 等价)
//	beans      → /debug/beans      路由/handler 清单(等价 Spring beans;列已注册 mux 路由)
//	mappings   → /debug/mappings   路由表(等价 Spring mappings;与 beans 同源,结构化呈现)
//
// 鉴权:本 package 仅注册 handler;敏感端点鉴权由调用方在 mux 外层包 admin-role JWT 中间件
// (与 admin-go requireAdmin 一致)。ESP 约定:debug 端点仅经 admin-go 代理可达,
// admin-go 侧 requireAdmin 把关,X-User-Id/X-Role 透传后服务侧二次校验(见 RequireAdminDebug)。
//
// 安全:pprof 暴露运行时栈/堆,生产仅 admin 可达。RegisterDebugHandlers 不绑定独立端口,
// 挂载到服务主 mux,由 RequireAdminDebug 守门。

// debugEnvSensitive 匹配需脱敏的环境变量名(大小写不敏感包含即脱敏)。
var debugEnvSensitive = []string{
	"KEY", "PASSWORD", "PASSWD", "SECRET", "TOKEN", "CREDENTIAL", "PRIVATE",
}

// RegisterDebugHandlers 在给定 mux 上注册全 6 个等价深度诊断端点。
// 调用方负责对 /debug/* 加 admin-role JWT 鉴权(见 RequireAdminDebug)。
func RegisterDebugHandlers(mux *http.ServeMux, routes []string) {
	mux.HandleFunc("/debug/env", debugEnvHandler)
	mux.HandleFunc("/debug/loglevel", debugLogLevelHandler)
	mux.HandleFunc("/debug/beans", func(w http.ResponseWriter, r *http.Request) {
		debugBeansHandler(w, r, routes)
	})
	mux.HandleFunc("/debug/mappings", func(w http.ResponseWriter, r *http.Request) {
		debugMappingsHandler(w, r, routes)
	})

	// pprof:等价 threaddump(goroutine)+ heapdump(heap)。
	// 使用 pprof.Handler 精确注册各 profile,Index 覆盖 /debug/pprof/ 列表页。
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile) // CPU profile
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine")) // 等价 threaddump
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))           // 等价 heapdump
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
}

// debugEnvSensitiveMask 对敏感 key 的值打码,仅保留首尾 1 字符判断长度。
func debugEnvMask(k, v string) string {
	ku := strings.ToUpper(k)
	for _, s := range debugEnvSensitive {
		if strings.Contains(ku, s) {
			if len(v) <= 2 {
				return strings.Repeat("*", len(v))
			}
			return string(v[0]) + strings.Repeat("*", len(v)-2) + string(v[len(v)-1])
		}
	}
	return v
}

func debugEnvHandler(w http.ResponseWriter, _ *http.Request) {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			continue
		}
		k, v := kv[:idx], kv[idx+1:]
		env[k] = debugEnvMask(k, v)
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	writeDebugJSON(w, map[string]any{
		"activeProfiles": []string{runtime.GOOS + "/" + runtime.GOARCH, "go" + runtime.Version()},
		"propertySources": []map[string]any{
			{"name": "systemEnvironment", "properties": env},
		},
		"_sanitized": true,
		"_note":      "env values masked for keys matching: " + strings.Join(debugEnvSensitive, ","),
	})
}

// runtimeLogLevel 运行期日志级别(Go 无 SLF4J 全局级别,用包级变量模拟动态调级)。
var (
	debugMu       sync.RWMutex
	debugLogLevel = "INFO"
)

func debugLogLevelHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeDebugJSON(w, map[string]any{
			"configuredLevel": debugLogLevel,
			"effectiveLevel":  debugLogLevel,
			"loggers": map[string]string{
				"root":  debugLogLevel,
				"http":  debugLogLevel,
				"db":    debugLogLevel,
				"redis": debugLogLevel,
			},
		})
	case http.MethodPost, http.MethodPut:
		lvl := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("level")))
		if lvl == "" {
			// 尝试 JSON body
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) == nil {
				if v, ok := body["configuredLevel"]; ok {
					lvl = strings.ToUpper(v)
				}
			}
		}
		switch lvl {
		case "DEBUG", "INFO", "WARN", "ERROR", "TRACE":
			debugMu.Lock()
			debugLogLevel = lvl
			debugMu.Unlock()
			writeDebugJSON(w, map[string]string{"configuredLevel": lvl, "effectiveLevel": lvl, "status": "updated"})
		default:
			http.Error(w, `{"error":"invalid level, one of DEBUG/INFO/WARN/ERROR/TRACE"}`, http.StatusBadRequest)
		}
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func getDebugLogLevel() string {
	debugMu.RLock()
	defer debugMu.RUnlock()
	return debugLogLevel
}

// debugBeansHandler /debug/beans:列出已注册路由(等价 Spring beans:服务内 handler 清单)。
func debugBeansHandler(w http.ResponseWriter, _ *http.Request, routes []string) {
	beans := make([]map[string]string, 0, len(routes))
	for _, r := range routes {
		beans = append(beans, map[string]string{
			"bean":  "muxHandler:" + r,
			"route": r,
			"type":  "http.Handler",
			"scope": "singleton",
		})
	}
	writeDebugJSON(w, map[string]any{
		"context": "main",
		"beans":   beans,
		"_note":   "Go 无 Spring Bean 容器;此处等价为已注册 HTTP 路由(handler 清单)",
	})
}

// debugMappingsHandler /debug/mappings:路由表(等价 Spring mappings:结构化呈现)。
func debugMappingsHandler(w http.ResponseWriter, _ *http.Request, routes []string) {
	mappings := make([]map[string]string, 0, len(routes))
	for _, r := range routes {
		mappings = append(mappings, map[string]string{
			"pattern": r,
			"method":  "ANY",
			"handler": "muxHandler",
		})
	}
	writeDebugJSON(w, map[string]any{
		"contexts": map[string]any{
			"application": map[string]any{
				"mappings": mappings,
				"_note":    "Go http.ServeMux 路由表(等价 Spring RequestMappingHandlerMapping)",
			},
		},
	})
}

func writeDebugJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// RequireAdminDebug 对 /debug/* 端点强制 admin-role JWT 的简易中间件(可选,用于服务侧二次校验)。
// ESP 约定主鉴权在 admin-go requireAdmin;本中间件提供给不经过 admin-go 直连服务的管理路径兜底。
// 用法:mux.Handle("/debug/", RequireAdminDebug(http.StripPrefix("/debug", subMux)))
//
// 注意:本函数为占位守门(校验 X-Role header);完整 RS256 校验由各服务已有 JWT filter 负责。
// 不在此重复实现 RS256 解析,避免与各服务 JwtValidator 公钥注入重复。
func RequireAdminDebug(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := r.Header.Get("X-Role")
		if role != "admin" {
			http.Error(w, `{"error":"forbidden: admin role required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// DebugRoutes 返回标准 debug 路由清单,供调用方同时注册到 beans/mappings。
func DebugRoutes() []string {
	return []string{
		"/debug/env",
		"/debug/loglevel",
		"/debug/beans",
		"/debug/mappings",
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
		"/debug/pprof/goroutine",
		"/debug/pprof/heap",
		"/debug/pprof/allocs",
		"/debug/pprof/threadcreate",
		"/debug/pprof/mutex",
		"/debug/pprof/block",
	}
}

// GinDebugGroup 返回一个 http.Handler,可挂到 gin.RouterGroup("/debug/")。
// 用法(各 Gin 服务 main.go,一行接线):
//
//	r.Any("/debug/*path", gin.WrapH(middleware.GinDebugGroup(middleware.DebugRoutes())))
//
// debugRoutes 可用 DebugRoutes() 获取。鉴权由 RequireAdminDebug 内置(X-Role==admin 守门,
// admin-go handleDebugProxy 代理时透传 X-Role)。
func GinDebugGroup(routes []string) http.Handler {
	mux := http.NewServeMux()
	RegisterDebugHandlers(mux, routes)
	return RequireAdminDebug(mux)
}
