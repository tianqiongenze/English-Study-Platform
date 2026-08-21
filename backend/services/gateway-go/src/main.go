package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"esp/common-go/config"
	"esp/common-go/db"
	"esp/common-go/middleware"
	"esp/common-go/redisx"
	"esp/common-go/registry"

	"esp/conversation-go/src/asr"
	"esp/conversation-go/src/dialog"
	"esp/conversation-go/src/hub"
	"esp/conversation-go/src/llm"
	gwlog "esp/conversation-go/src/log"
	"esp/conversation-go/src/scenario"
	"esp/conversation-go/src/session"
	"esp/conversation-go/src/tts"
	"esp/conversation-go/src/ws"
	espjwt "esp/gateway-go/src/jwt"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
	// G-04:WS 握手超时,防慢速连接占满 goroutine。
	HandshakeTimeout: 10 * time.Second,
}

const serviceName = "gateway-go"

// WS 心跳/超时参数(G-04 加固)。
const (
	wsWriteTimeout = 10 * time.Second // 单次写 deadline(普通帧 + 控制帧)
	wsPongWait     = 60 * time.Second // 读 deadline:超过未收到任何帧/控制帧则判死
	wsPingPeriod   = 30 * time.Second // 服务端 ping 周期(须 < wsPongWait)
)

type Server struct {
	rdb       *redis.ClusterClient // P1-8: 写接口幂等 SETNX
	sessions  *session.Manager
	scenarios *scenario.Repository
	engine    *dialog.Engine
	asr       asr.Provider
	tts       tts.Provider
	jwt       *espjwt.Validator
	hub       *hub.Hub
}

func main() {
	middleware.SetRateLimitService(serviceName)
	// Task B-pilot(2026-08-21):Sentinel-golang 动态限流,规则经 Nacos dataId=gateway-go-flow-rules 下发。
	// 初始化失败→降级放行(RWT),不阻断启动。NACOS_ADDR 复用 registry 同款 env(默认 nacos:8848)。
	middleware.InitSentinel(serviceName, config.Getenv("NACOS_ADDR", "nacos:8848"))
	level := slog.LevelInfo
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	// OpenTelemetry tracing -> OTel Collector (OTLP gRPC).
	shutdownTracing := setupTracing()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(ctx)
	}()

	redisAddr := config.Getenv("REDIS_ADDR", "redis-cluster:6379")
	// Redis Cluster 模式(经 ExternalName redis-cluster → redis-cluster-new,3主3从)
	rdb := redisx.MustNewCluster(redisAddr, "gateway-go")

	// #140 全服务切 CRDB:初始化全局读写池(连 esp_speaking,供 conversation-go 的
	// scenarios/sessions 归档读写;app_logs 在 esp_shared,由 gwlog.Init 独立池连接)。
	// DB_ENABLED=false 时 no-op(回退纯 Redis/LLM 模式)。
	readDSN, writeDSN := db.ReadWriteDSN("esp_speaking")
	db.InitGlobal(readDSN, writeDSN, "gateway-go")

	// P2-1: WS 分片广播 Hub(按 session_id hash 分 Redis Pub/Sub channel)。
	// 跨 Pod 同步多端同 session / 教师旁听 / 情绪推送;点对点响应仍走 writeConn。
	shardCount := config.GetenvInt("WS_SHARD_COUNT", 8)
	wsHub := hub.New(rdb, shardCount)
	wsHub.Start(context.Background())

	// RS256 JWT validator(ADR-12):公钥校验 BFF 签发的 JWT
	jwtValidator, err := espjwt.NewValidatorFromEnv()
	if err != nil {
		slog.Error("failed to init JWT validator", "err", err)
		os.Exit(1)
	}

	srv := &Server{
		rdb:       rdb,
		sessions:  session.NewManager(rdb, 2*time.Hour),
		scenarios: scenario.NewRepository(),
		// L1 精确缓存(Phase1):NewWithCache 在 llm.New() 外包 CachedProvider,
		// Redis 可达且未设 ESP_LLM_CACHE_ENABLED=false 时生效;否则降级直穿(语义不变)。
		engine: &dialog.Engine{LLM: llm.NewWithCache(), HintIdle: 8 * time.Second},
		asr:    asr.New(),
		tts:    tts.New(),
		jwt:    jwtValidator,
		hub:    wsHub,
	}
	// AppLog 异步写 TiDB app_logs(ADR-observability-rebuild):业务事件结构化落库,
	// trace_id 关联 Tempo/Loki。降级:TiDB 不可用时仅 slog.Warn 不阻断业务。
	gwlog.Init()

	// Nacos 服务注册(优雅退出时注销)
	nacosCli := registry.RegisterNacos(serviceName, 8080)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	// 探针三分离(OSW/RWT,2026-08-21 P1-D12):
	// readyz=依赖就绪(Redis PING 失败返 503 让流量不打进来,避免 Redis 抖动时假 500 雪崩);
	// livez=进程存活(只查进程不查依赖,失败才重启,readyz 失败不致 liveness 重启)。
	// Redis 是 gateway 强依赖(会话幂等 SETNX + WS Hub Pub/Sub),脑裂/未起时 readyz 必失败。
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			slog.Warn("readyz redis ping failed", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/api/scenarios", srv.handleScenarios)
	mux.HandleFunc("/api/sessions", srv.handleCreateSession)
	mux.HandleFunc("/api/sessions/history", srv.handleListSessionHistory)
	mux.HandleFunc("/ws/speech/", srv.handleSpeech)
	// AI 助手对话端点(2026-08-21):供前端 AIAssistant 浮层调用,LLM 经 gateway 的
	// llm.NewWithCache()(LCO 三层缓存:L1 精确 Redis + L2 语义 Milvus + L3 Neo4j)。
	// 鉴权:复用 userIDFromRequest(JWT)。响应带 cache_hit/cache_layer(经 LCO 元数据)。
	mux.HandleFunc("/api/llm/chat", srv.handleLLMChat)
	mux.Handle("/metrics", promhttp.Handler())
	// /debug/* —— actuator 风格诊断端点(admin-go debug_proxy 代理;全语言统一面板消费)。
	// mappings:返回路由表;beans:返回运行时信息(go 版,非 Spring Bean);pprof 走标准库。
	mux.HandleFunc("/debug/mappings", srv.handleDebugMappings)
	mux.HandleFunc("/debug/beans", srv.handleDebugBeans)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	addr := ":" + config.Getenv("PORT", "8080")
	slog.Info("gateway-go starting", "addr", addr)
	// Wrap mux with rate limit + circuit breaker + otelhttp tracing.
	// Rate limit first (reject early), then circuit breaker, then tracing.
	rateLimited := middleware.HTTPRateLimit(mux, 100, 200)                                    // 100 rps,桶容量200
	cbProtected := middleware.HTTPCircuitBreaker(rateLimited, 5, 30*time.Second, serviceName) // 5次失败触发30s熔断
	tracedHandler := otelhttp.NewHandler(cbProtected, "gateway-go")
	httpSrv := &http.Server{Addr: addr, Handler: tracedHandler}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")
	registry.DeRegister(serviceName, 8080, nacosCli)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	// 排空 AppLog 异步队列后关闭 DB 连接(graceful drain)。
	gwlog.Shutdown()
}

func (s *Server) handleScenarios(w http.ResponseWriter, r *http.Request) {
	// Task B-pilot:Sentinel 动态流控(resource=api/scenarios)。被限流返 429。
	if !middleware.AllowOrLimit(serviceName, "api/scenarios") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"flow limited","retry_after":1}`))
		return
	}
	userID := s.userIDFromRequest(r)
	if userID == 0 {
		http.Error(w, "unauthorized", 401)
		return
	}
	gwlog.AppLog("INFO", "api.scenarios.list", userID, nil, r.Context())
	writeJSON(w, s.scenarios.List())
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScenarioCode string `json:"scenario_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	sc, ok := s.scenarios.Get(req.ScenarioCode)
	if !ok {
		http.Error(w, "unknown scenario", 400)
		return
	}
	userID := s.userIDFromRequest(r)
	if userID == 0 {
		http.Error(w, "unauthorized", 401)
		return
	}
	// P1-8 写接口幂等:POST /api/sessions 带 Idempotency-Key 时,SETNX 占键;命中则 replay 上次响应。
	// 缺 header 时不拦截(向后兼容)。TTL 300s,降级(Redis 不可用)直接放行。
	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idemKey != "" && s.rdb != nil {
		redisKey := fmt.Sprintf("esp:idem:gateway:sessions:%s", idemKey)
		if cached, err := s.rdb.Get(r.Context(), redisKey).Result(); err == nil && cached != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(cached))
			return
		}
	}
	sess := &session.Session{
		ID:         newID(),
		UserID:     userID,
		Scenario:   sc.Code,
		System:     sc.System,
		Difficulty: sc.Difficulty,
	}
	if err := s.sessions.Create(r.Context(), sess); err != nil {
		gwlog.AppLog("ERROR", "api.sessions.create", userID, map[string]any{"scenario": sc.Code, "err": err.Error()}, r.Context())
		http.Error(w, "create session failed", 500)
		return
	}
	gwlog.AppLog("INFO", "api.sessions.create", userID, map[string]any{"session_id": sess.ID, "scenario": sc.Code}, r.Context())
	resp, _ := json.Marshal(map[string]any{"session_id": sess.ID, "scenario": sc})
	// P1-8: 写入幂等缓存(nx=true 仅当 key 不存在,防并发双写;ex=300s)
	if idemKey != "" && s.rdb != nil {
		redisKey := fmt.Sprintf("esp:idem:gateway:sessions:%s", idemKey)
		_ = s.rdb.SetNX(r.Context(), redisKey, string(resp), 300*time.Second).Err()
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

// handleListSessionHistory 列出当前用户会话历史(GET /api/sessions/history?limit=)。
// P0-3 IDOR 修复:userID 必须来自 JWT(BFF 注入 X-User-Id),移除 query user_id 回落(原"便于 Swagger 测试"导致可查任意用户历史)。
func (s *Server) handleListSessionHistory(w http.ResponseWriter, r *http.Request) {
	userID := s.userIDFromRequest(r)
	if userID == 0 {
		http.Error(w, "unauthorized", 401)
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	list, err := session.ListHistory(r.Context(), userID, limit)
	if err != nil {
		gwlog.AppLog("ERROR", "api.sessions.history", userID, map[string]any{"err": err.Error()}, r.Context())
		http.Error(w, "list history failed", 500)
		return
	}
	if list == nil {
		list = []session.SessionSummary{}
	}
	gwlog.AppLog("INFO", "api.sessions.history", userID, map[string]any{"count": len(list)}, r.Context())
	writeJSON(w, map[string]any{"user_id": userID, "sessions": list})
}

func (s *Server) handleSpeech(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Path[len("/ws/speech/"):]
	if sessionID == "" {
		http.Error(w, "missing session id", 400)
		return
	}
	// WS 鉴权(防越权连他人会话):token 取 ?token= 查询参数或 esp_token cookie
	// (P1-13 cookie 模式:浏览器 WS 握手同源自动带 httpOnly cookie,JS 无需也无法读 token);
	// 校验 token 解析出的 userID 必须等于会话归属 UserID。
	sess, err := s.sessions.Get(r.Context(), sessionID)
	if err != nil || sess == nil {
		http.Error(w, "session not found", 404)
		return
	}
	wsToken := r.URL.Query().Get("token")
	if wsToken == "" {
		if c, err := r.Cookie("esp_token"); err == nil {
			wsToken = c.Value
		}
	}
	if uid := s.parseUserID(wsToken); uid == 0 || uid != sess.UserID {
		http.Error(w, "forbidden", 403)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	// G-04 加固:写互斥包装(与 Hub 广播 sendLocal 并发写同一连接安全)。
	safeConn := ws.NewSafeConn(conn)
	defer conn.Close()

	// G-04 加固:心跳保活 —— 读 deadline 由 pong 重置;服务端每 wsPingPeriod 发 ping。
	conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	stopPing := make(chan struct{})
	defer close(stopPing)
	go wsPingLoop(safeConn, stopPing)

	// P2-1: 注册到广播 Hub,使跨端/跨 Pod 同 session 连接能收到广播帧。
	// 返回的反注册函数在连接关闭时(defer)清理本地连接表。
	unregister := s.hub.Register(sessionID, safeConn)
	defer unregister()

	// 会话已在 upgrade 前鉴权时 Get 并校验归属,sess 非空,直接复用。
	slog.Info("ws connected", "session", sessionID, "scenario", sess.Scenario)

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		frame, err := ws.DecodeFrame(raw)
		if err != nil {
			_ = writeConn(safeConn, ws.Frame{Type: ws.TypeError, Message: "bad frame"})
			continue
		}
		switch frame.Type {
		case ws.TypeAudioChunk:
			text := s.asr.Recognize([]string{frame.Data})
			if text == "" {
				continue
			}
			s.handleUserText(r.Context(), safeConn, sess, sessionID, text)
		case ws.TypeTextInput:
			if frame.Text == "/hint" {
				_ = writeConn(safeConn, s.engine.Hint(sess))
				continue
			}
			s.handleUserText(r.Context(), safeConn, sess, sessionID, frame.Text)
		case ws.TypeEnd:
			_ = writeConn(safeConn, ws.Frame{Type: ws.TypeEnd, Summary: "Session ended."})
			_ = s.sessions.End(r.Context(), sessionID)
			return
		}
	}
}

// wsPingLoop 周期性发 ping,驱动对端回 pong 以续读 deadline(配合 wsPongWait 判死僵尸连接)。
func wsPingLoop(c *ws.SafeConn, stop <-chan struct{}) {
	t := time.NewTicker(wsPingPeriod)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if err := c.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteTimeout)); err != nil {
				// 连接已关闭/写失败:退出心跳,连接由读循环或 defer Close 收尾。
				return
			}
		}
	}
}

func (s *Server) handleUserText(ctx context.Context, conn *ws.SafeConn, sess *session.Session, sessionID, text string) {
	_, _ = s.sessions.AppendTurn(ctx, sessionID, session.Turn{Role: "user", Content: text})
	res, err := s.engine.Process(ctx, sess, text)
	if err != nil {
		slog.Error("process error", "err", err)
		_ = writeConn(conn, ws.Frame{Type: ws.TypeError, Message: "ai error"})
		return
	}
	for _, f := range res.Frames {
		if f.Type == ws.TypeAIReply {
			// 附加情绪驱动 TTS:音频 + 情绪标签 + 口型序列(#121 数字人)
			t := s.tts.Synthesize(ctx, f.Text)
			f.Data = t.Audio
			f.Emotion = t.Emotion
			f.Visemes = t.Visemes
		}
		if err := writeConn(conn, f); err != nil {
			return
		}
	}
	_, _ = s.sessions.AppendTurn(ctx, sessionID, res.Turn)
}

// writeConn 写一帧。经 SafeConn 内部互斥 + 写 deadline 保护(gorilla 单并发写者,解 G-04)。
func writeConn(conn *ws.SafeConn, f ws.Frame) error {
	return conn.WriteFrame(f)
}

// handleLLMChat POST /api/llm/chat —— AI 助手对话(前端浮层调用)。
// body: {"message":"用户输入","context":"当前页/场景上下文"}
// 走 srv.engine.LLM(llm.NewWithCache,LCO 三层缓存)。鉴权:JWT userID。
// 超时 90s(AI 推理长尾,RWT 超时分级);失败降级返友好提示不 500。
func (s *Server) handleLLMChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// Task B-pilot:Sentinel 动态流控(resource=api/llm/chat)。被限流返 429。
	// 放在 method 校验后、鉴权前:流控早失败降负,但不对非法 method 计流。
	if !middleware.AllowOrLimit(serviceName, "api/llm/chat") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"flow limited","retry_after":1}`))
		return
	}
	userID := s.userIDFromRequest(r)
	if userID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var req struct {
		Message string `json:"message"`
		Role    string `json:"role"` // 可选:system 角色(如"你是英语学习助手")
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, `{"error":"message required"}`, http.StatusBadRequest)
		return
	}
	// 构造 LLM 消息:system 设英语学习助手角色(可被 req.Role 覆盖)+ user 消息。
	system := "你是洁语(Verity)AI 英语学习助手。简洁专业地用中文回答用户关于英语学习的问题(口语/写作/词汇/语法等),鼓励用户练习。"
	if req.Role != "" {
		system = req.Role
	}
	msgs := []llm.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: req.Message},
	}
	// 90s 超时(AI 推理 8-34s + LCO 缓存命中 ~10ms)。
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	reply, err := s.engine.LLM.Chat(ctx, msgs)
	if err != nil {
		slog.Warn("ai assistant chat failed", "user_id", userID, "err", err)
		// 降级:不 500,返友好提示(RWT 铁律:失败不阻塞主链)。
		writeJSON(w, map[string]any{
			"reply":       "抱歉,AI 助手暂时不可用,请稍后再试。",
			"error":       err.Error(),
			"cache_hit":   false,
			"cache_layer": "",
		})
		return
	}
	// 注:cache_hit/cache_layer 元数据由 LCO provider 注入(若 LLM 走 LCO 代理);
	// gateway 内置 CachedProvider 当前不透传 cache 元数据,留空(前端可后续增强)。
	writeJSON(w, map[string]any{
		"reply":       reply,
		"cache_hit":   false,
		"cache_layer": "",
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// handleDebugMappings 返回路由表(actuator 风格,全语言统一诊断面板消费)。
// admin-go /api/admin/debug/gateway-go/mappings 代理到本端点;前端 DebugPanel 不再 404。
// 返回 JSON: {contexts: {default: {mappings: [{handler, method, pattern}]}}},形似 Spring actuator。
func (s *Server) handleDebugMappings(w http.ResponseWriter, _ *http.Request) {
	type mapping struct {
		Handler string `json:"handler"`
		Method  string `json:"method"`
		Pattern string `json:"pattern"`
	}
	mappings := []mapping{
		{Handler: "handleScenarios", Method: "GET", Pattern: "/api/scenarios"},
		{Handler: "handleCreateSession", Method: "POST", Pattern: "/api/sessions"},
		{Handler: "handleListSessionHistory", Method: "GET", Pattern: "/api/sessions/history"},
		{Handler: "handleSpeech", Method: "GET (WS)", Pattern: "/ws/speech/{sessionId}"},
		{Handler: "promhttp.Handler", Method: "GET", Pattern: "/metrics"},
		{Handler: "pprof.Index", Method: "GET", Pattern: "/debug/pprof/"},
	}
	writeJSON(w, map[string]any{
		"contexts": map[string]any{
			"default": map[string]any{"mappings": mappings},
		},
	})
}

// handleDebugBeans 返回运行时信息(Go 版"beans":无 Spring Bean,用 runtime info 等价)。
// 含 goroutine/mem/GC/版本,供资源监控与诊断。
func (s *Server) handleDebugBeans(w http.ResponseWriter, _ *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	writeJSON(w, map[string]any{
		"service":    "gateway-go",
		"lang":       "go",
		"version":    runtime.Version(),
		"goroutines": runtime.NumGoroutine(),
		"mem": map[string]any{
			"alloc_bytes":  m.Alloc,
			"sys_bytes":    m.Sys,
			"num_gc":       m.NumGC,
			"heap_objects": m.HeapObjects,
		},
		"endpoints": []string{"/healthz", "/readyz", "/livez", "/api/scenarios", "/api/sessions", "/api/sessions/history", "/ws/speech/", "/metrics", "/debug/mappings", "/debug/beans", "/debug/pprof/"},
	})
}

// parseUserID 解析 JWT(RS256 + 公钥校验,ADR-12)取 userID。
// 空/非法/无效/签名不符 token 返回 0(调用方应拒绝:HTTP 返 401,WS 返 403)。
func (s *Server) parseUserID(tokenStr string) int64 {
	return s.jwt.ParseUserID(tokenStr)
}

// userIDFromRequest 解析 userID(HTTP 路径用)。
// P1-13:Authorization Bearer 优先;无 header 时回落 esp_token httpOnly cookie
// (浏览器同源请求自动携带,JS 读不到 → 消灭 XSS 窃取面;小程序仍走 Bearer)。
func (s *Server) userIDFromRequest(r *http.Request) int64 {
	return s.parseUserID(tokenFromRequest(r))
}

// tokenFromRequest 取 JWT:Bearer header 优先,回落 esp_token cookie,都没有返回空串。
func tokenFromRequest(r *http.Request) string {
	authz := r.Header.Get("Authorization")
	if strings.HasPrefix(authz, "Bearer ") {
		return strings.TrimPrefix(authz, "Bearer ")
	}
	if c, err := r.Cookie("esp_token"); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

func env(k, def string) string {
	// 已迁移至 esp/common-go/config.Getenv(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return config.Getenv(k, def)
}

func envInt(k string, def int) int {
	// 已迁移至 esp/common-go/config.GetenvInt(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return config.GetenvInt(k, def)
}

// newID 生成会话 ID。G-04 修复:原 `sess_<UnixMilli>` 毫秒级碰撞导致会话串号。
// 改 `sess_<unixms>_<8位crypto/rand hex>`:零新依赖兜底(首选 UUIDv7 需新增 github.com/google/uuid,
// 当前环境网络受限且 go.sum 本就留空待 tidy,引入新依赖风险高)。unixms 保时序可读,
// 8hex(32bit 随机)在并发 1000 会话/毫秒下碰撞概率可忽略;极端情况 rand 失败回落纳秒保唯一。
func newID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("sess_%d_%d", time.Now().UnixMilli(), time.Now().UnixNano())
	}
	return fmt.Sprintf("sess_%d_%x", time.Now().UnixMilli(), b)
}
