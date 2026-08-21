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

	"llm-cache-proxy/internal/cache"
	"llm-cache-proxy/internal/config"
	"llm-cache-proxy/internal/protocol"
)

// Server LCO 代理 HTTP 服务器。
type Server struct {
	cfg    *config.Config
	cache  *cache.Manager
	logger *slog.Logger
	hc     *http.Client
}

// New 构造 LCO 代理服务器。
//
// upstream http.Client 超时策略(RWT 铁律:超时分级):
// 用 Transport.ResponseHeaderTimeout 控制首字节超时(60s),不设 http.Client.Timeout。
// 原因:http.Client.Timeout 是"请求开始到 body 读完"的总超时,会中断长流式 SSE body;
// 改为只控首字节,body 读取由 forwardUpstream 内独立 context(600s)控制。
func New(cfg *config.Config, cm *cache.Manager, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		cache:  cm,
		logger: logger,
		hc: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: 60 * time.Second, // 首字节超时(上游多久开始吐第一个响应头)
				IdleConnTimeout:       90 * time.Second,
			},
			// 无 Timeout:流式 body 由 forwardUpstream 的独立 upCtx 控制,不被总超时切断
		},
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
//
// 流式透传(2026-08-20):SSE 流式响应边读边 flush,不全量缓冲(Claude Code 100% 流式请求,
// 全量缓冲会致首字节超时→重试→context canceled)。响应头阶段用 isStreamByHeader 分流:
// 流式→streamProxy 边读边 flush 不缓存;非流式→保持 io.ReadAll+缓存回写。
//
// upstream context 解耦(RWT+BRR):不绑 r.Context(),改用独立 600s context。
// 原因:防短超时客户端断开致 context canceled 误杀(误杀根因是全量缓冲致首字节超时,
// 流式改造已消除主因,解耦是额外防御)。代价:客户端断开后 upstream 继续烧 token,
// 由 600s 独立超时回收。WriteTimeout 1200s 兜底。
func (s *Server) forwardUpstream(w http.ResponseWriter, r *http.Request, body []byte, cacheKeyRaw string) {
	upstreamURL := s.cfg.UpstreamURL + r.URL.Path
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	// 独立 upstream context(不绑 r.Context),600s 超时回收防泄漏
	upCtx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	// G704 SSRF 评估:upstreamURL 的 host 部分固定来自 env 配置(LCO_UPSTREAM_URL,配置信任域);
	// 路径/查询虽源自请求,但受 ServeMux 注册路由白名单约束(/v1/messages 等),且 net/http 已做 URL 解析,
	// 不会被注入新 host(r.URL.Path 是路径而非完整 URL)。代理转发语义本就如此,非 SSRF 漏洞。
	req, err := http.NewRequestWithContext(upCtx, r.Method, upstreamURL, bytes.NewReader(body)) //nolint:gosec // G704: 见上注释,host 来自配置信任域,路径受路由白名单约束。
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

	resp, err := s.hc.Do(req) //nolint:gosec // G704: req.URL host 来自配置信任域,见上方 build 处注释。
	if err != nil {
		s.logger.Error("upstream 请求失败", "url", upstreamURL, "err", err)
		writeErr(w, 502, "upstream unreachable")
		return
	}
	defer resp.Body.Close()

	// 流式响应:SSE 边读边 flush,不全量缓冲,不回写缓存
	if isStreamByHeader(resp) {
		s.streamProxy(w, resp, cacheKeyRaw)
		return
	}

	// ─── 非流式:全量读 + 缓存回写 + 一次性写(原逻辑不变)───
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20)) // 20MB 上限

	// 回写缓存(仅 2xx + 非流式)
	if cacheKeyRaw != "" && resp.StatusCode == 200 && !isStreamBody(respBody) {
		key, _ := protocol.Normalize(r.URL.Path, body)
		if key.Protocol != protocol.Unknown {
			scene := cache.ClassifyScene(key)
			storeCtx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
			s.cache.Store(storeCtx, key, respBody, scene)
			scancel()
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

// streamProxy SSE 流式透传:边读边 flush,不全量缓冲,不回写缓存。
//
// 设计(LCO 铁律:流式则不缓存):SSE token 流无单一可缓存值,跳过 cache.Store,
// 与 conversation-go/llm-orchestrator-rs 的"流式不缓存/聚合后缓存"设计共识一致。
func (s *Server) streamProxy(w http.ResponseWriter, resp *http.Response, cacheKeyRaw string) {
	// 透传响应头,剔除 Content-Length/Transfer-Encoding(chunked 冲突,由 ResponseWriter 自管)
	for k, vs := range resp.Header {
		if k == "Content-Length" || k == "Transfer-Encoding" {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if cacheKeyRaw != "" {
		w.Header().Set("X-LCO-Cache-Hit", "MISS")
	}
	w.WriteHeader(resp.StatusCode)

	// CLC:Flusher 类型断言用 , ok 形式(errcheck check-type-assertions:true)
	// 不支持 Flush 的 ResponseWriter(如 httptest.ResponseRecorder):ok=false,降级为纯 Write 不 Flush
	flusher, ok := w.(http.Flusher)

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				// 客户端写失败(已断开):return,不继续读 upstream 省 token
				return
			}
			if ok {
				flusher.Flush() // ok=true 时 flusher 必非 nil,nilnesserr 满足
			}
		}
		if err != nil {
			return // io.EOF 或读取错误
		}
	}
}

// handlePassthrough 非 LLM 端点透传(/v1/models 等)。
func (s *Server) handlePassthrough(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	s.forwardUpstream(w, r, body, "")
}

// isStreamByHeader 仅看响应头 Content-Type 判断是否 SSE 流式(不读 body)。
// 流式分流必须在读 body 前判断(流式一旦开始读不能"退回"给客户端)。
func isStreamByHeader(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return strings.Contains(ct, "text/event-stream")
}

// isStreamBody 看 body 前缀判断是否流式(兜底,防响应头未带 content-type 的伪流式)。
// 仅用于非流式路径的缓存回写互斥守卫(读完全量 body 后再判断)。
func isStreamBody(body []byte) bool {
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
	_, _ = fmt.Fprintf(w, `{"error":"%s"}`, msg)
}
