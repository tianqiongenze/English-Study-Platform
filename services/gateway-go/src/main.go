package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"esp/common-go/config"
	"esp/common-go/redisx"

	"esp/conversation-go/src/asr"
	"esp/conversation-go/src/dialog"
	"esp/conversation-go/src/hub"
	espjwt "esp/gateway-go/src/jwt"
	"esp/conversation-go/src/llm"
	gwlog "esp/conversation-go/src/log"
	"esp/conversation-go/src/scenario"
	"esp/conversation-go/src/session"
	"esp/conversation-go/src/tts"
	"esp/conversation-go/src/ws"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

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
		engine:    &dialog.Engine{LLM: llm.New(), HintIdle: 8 * time.Second},
		asr:       &asr.MockProvider{},
		tts:       &tts.MockProvider{},
		jwt:       jwtValidator,
		hub:       wsHub,
	}
	// AppLog 异步写 TiDB app_logs(ADR-observability-rebuild):业务事件结构化落库,
	// trace_id 关联 Tempo/Loki。降级:TiDB 不可用时仅 slog.Warn 不阻断业务。
	gwlog.Init()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/api/scenarios", srv.handleScenarios)
	mux.HandleFunc("/api/sessions", srv.handleCreateSession)
	mux.HandleFunc("/api/sessions/history", srv.handleListSessionHistory)
	mux.HandleFunc("/ws/speech/", srv.handleSpeech)
	mux.Handle("/metrics", promhttp.Handler())

	addr := ":" + config.Getenv("PORT", "8080")
	slog.Info("gateway-go starting", "addr", addr)
	// Wrap mux with otelhttp so every HTTP request gets a span.
	// /ws/speech upgrade happens after the handler-level span is created,
	// so WS connections are traced at the handshake level; inner WS
	// message loops keep the original logic untouched.
	tracedHandler := otelhttp.NewHandler(mux, "gateway-go")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	// 排空 AppLog 异步队列后关闭 DB 连接(graceful drain)。
	gwlog.Shutdown()
}

func (s *Server) handleScenarios(w http.ResponseWriter, r *http.Request) {
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
	// WS 鉴权(防越权连他人会话):浏览器 WS API 不能设 Authorization header,
	// 故用 ?token= 传 JWT;校验 token 解析出的 userID 必须等于会话归属 UserID。
	sess, err := s.sessions.Get(r.Context(), sessionID)
	if err != nil || sess == nil {
		http.Error(w, "session not found", 404)
		return
	}
	if uid := s.parseUserID(r.URL.Query().Get("token")); uid == 0 || uid != sess.UserID {
		http.Error(w, "forbidden", 403)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// P2-1: 注册到广播 Hub,使跨端/跨 Pod 同 session 连接能收到广播帧。
	// 返回的反注册函数在连接关闭时(defer)清理本地连接表。
	unregister := s.hub.Register(sessionID, conn)
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
			_ = writeConn(conn, ws.Frame{Type: ws.TypeError, Message: "bad frame"})
			continue
		}
		switch frame.Type {
		case ws.TypeAudioChunk:
			text := s.asr.Recognize([]string{frame.Data})
			if text == "" {
				continue
			}
			s.handleUserText(r.Context(), conn, sess, sessionID, text)
		case ws.TypeTextInput:
			if frame.Text == "/hint" {
				_ = writeConn(conn, s.engine.Hint(sess))
				continue
			}
			s.handleUserText(r.Context(), conn, sess, sessionID, frame.Text)
		case ws.TypeEnd:
			_ = writeConn(conn, ws.Frame{Type: ws.TypeEnd, Summary: "Session ended."})
			_ = s.sessions.End(r.Context(), sessionID)
			return
		}
	}
}

func (s *Server) handleUserText(ctx context.Context, conn *websocket.Conn, sess *session.Session, sessionID, text string) {
	_, _ = s.sessions.AppendTurn(ctx, sessionID, session.Turn{Role: "user", Content: text})
	res, err := s.engine.Process(ctx, sess, text)
	if err != nil {
		slog.Error("process error", "err", err)
		_ = writeConn(conn, ws.Frame{Type: ws.TypeError, Message: "ai error"})
		return
	}
	for _, f := range res.Frames {
		if f.Type == ws.TypeAIReply {
			// 附加 TTS 音频
			f.Data = s.tts.Synthesize(ctx, f.Text)
		}
		if err := writeConn(conn, f); err != nil {
			return
		}
	}
	_, _ = s.sessions.AppendTurn(ctx, sessionID, res.Turn)
}

func writeConn(conn *websocket.Conn, f ws.Frame) error {
	b, err := f.Encode()
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// parseUserID 解析 JWT(RS256 + 公钥校验,ADR-12)取 userID。
// 空/非法/无效/签名不符 token 返回 0(调用方应拒绝:HTTP 返 401,WS 返 403)。
func (s *Server) parseUserID(tokenStr string) int64 {
	return s.jwt.ParseUserID(tokenStr)
}

// userIDFromRequest 从 Authorization: Bearer <jwt> 解析 userID(HTTP 路径用)。
func (s *Server) userIDFromRequest(r *http.Request) int64 {
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		return 0
	}
	return s.parseUserID(strings.TrimPrefix(authz, "Bearer "))
}

func env(k, def string) string {
	// 已迁移至 esp/common-go/config.Getenv(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return config.Getenv(k, def)
}

func envInt(k string, def int) int {
	// 已迁移至 esp/common-go/config.GetenvInt(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return config.GetenvInt(k, def)
}

func newID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixMilli())
}
