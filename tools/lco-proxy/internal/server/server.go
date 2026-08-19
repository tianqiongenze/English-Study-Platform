// Package server — LCO 代理 HTTP 服务器(三协议端点 + 缓存拦截 + 上游转发)
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"lco-proxy/internal/cache"
	"lco-proxy/internal/config"
	"lco-proxy/internal/protocol"
)

// Server LCO 代理 HTTP 服务器。
type Server struct {
	cfg    *config.Config
	cache  *cache.Manager
	logger *slog.Logger
	hc     *http.Client
}

func New(cfg *config.Config, cm *cache.Manager, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		cache:  cm,
		logger: logger,
		hc:     &http.Client{Timeout: 300 * time.Second}, // LLM 长尾
	}
}

// Routes 注册所有路由。
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	// 三协议端点(本机工具接入点)
	mux.HandleFunc("/v1/messages", s.handleLLM)
	mux.HandleFunc("/v1/chat/completions", s.handleLLM)
	mux.HandleFunc("/api/chat", s.handleLLM)
	mux.HandleFunc("/api/generate", s.handleLLM) // Ollama 生成端点

	// 健康检查 + 指标 + 统计
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/stats", s.stats)
	if s.cfg.MetricsEnabled {
		mux.Handle("/metrics", promhttp.Handler())
	}

	// 兜底:其他路径透传上游(Claude Code 有时打 /v1/models 等)
	mux.HandleFunc("/", s.handlePassthrough)

	return mux
}

// handleLLM 核心:缓存拦截 → 命中返回 → miss 转发上游 → 回写缓存。
func (s *Server) handleLLM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB 上限
	if err != nil {
		writeErr(w, 400, "read body failed")
		return
	}

	// 归一化解析
	key, err := protocol.Normalize(r.URL.Path, body)
	if err != nil {
		// 无法解析:不缓存,直穿上游(保守,不阻断)
		s.logger.Warn("协议解析失败,直穿不缓存", "path", r.URL.Path, "err", err)
		s.forwardUpstream(w, r, body, "")
		return
	}

	// L1/L2 查询
	result := s.cache.Lookup(r.Context(), key)
	if result.Hit {
		// 缓存命中
		w.Header().Set("X-LCO-Cache-Hit", result.Layer)
		if result.SourceProto != "" {
			w.Header().Set("X-LCO-Source-Protocol", string(result.SourceProto))
		}
		// L2 跨协议命中:响应体是源协议格式,需重生成(用户决策:L1分键+L2跨协议重生成)
		if result.Layer == "L2" && result.SourceProto != key.Protocol {
			w.Header().Set("X-LCO-Cross-Protocol", "true")
			// MVP:跨协议不直接返回源响应,降级为直穿(避免格式串台)
			// 真实重生成需 LLM 二次调用,成本权衡后 DEFERRED
			s.logger.Info("L2 跨协议命中,降级直穿(重生成 DEFERRED)",
				"src", result.SourceProto, "dst", key.Protocol)
			s.forwardUpstream(w, r, body, "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(result.Body)
		s.logger.Info("cache hit", "layer", result.Layer, "proto", key.Protocol, "path", r.URL.Path)
		return
	}

	// miss:转发上游,响应回写缓存。L3 上下文增强(若有)通过响应头暴露(本轮不改请求体)。
	if result.EnhancedContext != "" {
		// 仅截断头值(响应头不宜过长),完整上下文留待后续 RAG 注入阶段使用。
		hint := result.EnhancedContext
		if len(hint) > 128 {
			hint = hint[:128]
		}
		w.Header().Set("X-LCO-Context-Enhanced", hint)
	}
	s.forwardUpstream(w, r, body, key.Raw)
}

// forwardUpstream 转发请求到上游(CC Switch),捕获响应用于缓存回写。
func (s *Server) forwardUpstream(w http.ResponseWriter, r *http.Request, body []byte, cacheKeyRaw string) {
	upstreamURL := s.cfg.UpstreamURL + r.URL.Path
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		writeErr(w, 502, "build upstream request failed")
		return
	}
	// 透传所有头(鉴权/版本/content-type 等)
	for k, vs := range r.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Host = r.Host

	resp, err := s.hc.Do(req)
	if err != nil {
		s.logger.Error("upstream 请求失败", "url", upstreamURL, "err", err)
		writeErr(w, 502, "upstream unreachable")
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20)) // 20MB 上限

	// 回写缓存(仅 2xx + 非流式)
	if cacheKeyRaw != "" && resp.StatusCode == 200 && !isStream(resp, respBody) {
		key, _ := protocol.Normalize(r.URL.Path, body)
		if key.Protocol != protocol.Unknown {
			scene := cache.ClassifyScene(key)
			storeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			s.cache.Store(storeCtx, key, respBody, scene)
			cancel()
		}
	}

	// 回写客户端
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if cacheKeyRaw != "" {
		w.Header().Set("X-LCO-Cache-Hit", "MISS")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// handlePassthrough 非 LLM 端点透传(/v1/models 等)。
func (s *Server) handlePassthrough(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	s.forwardUpstream(w, r, body, "")
}

// isStream 判断是否流式响应(SSE: content-type text/event-stream 或 body 含 "data:" 前缀)。
func isStream(resp *http.Response, body []byte) bool {
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		return true
	}
	return bytes.HasPrefix(bytes.TrimSpace(body), []byte("data:"))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	h := s.cache.HealthCheck(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if h.L1Redis != "ok" {
		w.WriteHeader(503)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": h.L1Redis == "ok",
		"cache":  h,
	})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"cache":   s.cache.HealthCheck(r.Context()),
		"upstream": s.cfg.UpstreamURL,
	})
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, msg)))
}
