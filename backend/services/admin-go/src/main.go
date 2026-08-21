package main

// esp-admin-go — 统一管理员监测门户后端。
// 聚合三块:①全服务健康探活 ②共享基础设施探活 + Prometheus 查询代理 ③统一 API 目录。
// 所有 /api/admin/* 需 role=admin 的 RS256 JWT(与 gateway-go 同公钥,esp-rsa-jwt-verify)。
// 零外部依赖(stdlib only),build 无需网络,规避镜像源 403。

import (
	"context"
	"crypto/rsa"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"esp/common-go/middleware"
	"esp/common-go/registry"
)

//go:embed catalog.json
var catalogData []byte

type Target struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // service|infra
	Lang string `json:"lang"`
	Svc  string `json:"svc"`
	Port int    `json:"port"`
	Path string `json:"path"`
	Type string `json:"type"` // http|tcp
}

type ProbeResult struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Lang      string `json:"lang"`
	Svc       string `json:"svc"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
	Status    string `json:"status"` // UP|DOWN
	Code      int    `json:"code,omitempty"`
	Msg       string `json:"msg,omitempty"`
	LatencyMS int    `json:"latency_ms"`
}

type Catalog struct {
	Services []Target `json:"services"`
	Infra    []Target `json:"infra"`
}

const probeTimeout = 2500 * time.Millisecond

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func probeOne(t Target) ProbeResult {
	r := ProbeResult{Name: t.Name, Kind: t.Kind, Lang: t.Lang, Svc: t.Svc, Port: t.Port, Path: t.Path}
	addr := net.JoinHostPort(t.Svc, strconv.Itoa(t.Port))
	if t.Type == "tcp" {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, probeTimeout)
		r.LatencyMS = int(time.Since(start).Milliseconds())
		if err != nil {
			r.Status = "DOWN"
			r.Msg = err.Error()
			return r
		}
		_ = conn.Close()
		r.Status = "UP"
		return r
	}
	u := fmt.Sprintf("http://%s%s", addr, t.Path)
	start := time.Now()
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get(u)
	r.LatencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		r.Status = "DOWN"
		r.Msg = err.Error()
		return r
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	r.Code = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		r.Status = "UP"
	} else {
		r.Status = "DOWN"
		r.Msg = fmt.Sprintf("http %d", resp.StatusCode)
	}
	return r
}

func probeAll(targets []Target) []ProbeResult {
	out := make([]ProbeResult, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t Target) {
			defer wg.Done()
			out[i] = probeOne(t)
		}(i, t)
	}
	wg.Wait()
	return out
}

func summary(results []ProbeResult) map[string]int {
	m := map[string]int{"total": len(results), "up": 0, "down": 0}
	for _, r := range results {
		if r.Status == "UP" {
			m["up"]++
		} else {
			m["down"]++
		}
	}
	return m
}

type Server struct {
	catalog *Catalog
	pubKey  *rsa.PublicKey
	prom    string
	// Phase4(2026-08-21):缓存监控 + Pulsar MQ 各下游基址,从 env 注入(信任域配置,防 SSRF)。
	lcoStats    string // env LCO_STATS_URL,LCO /stats+/metrics(宿主 15720)
	milvusAddr  string // env MILVUS_ADDR,Milvus REST(集群 19530)
	neo4jRest   string // env NEO4J_REST_URL,Neo4j REST(宿主 7474)
	neo4jUser   string // env NEO4J_USER
	neo4jPass   string // env NEO4J_PASSWORD
	pulsarAdmin string // env PULSAR_ADMIN_URL,Pulsar Admin REST(broker 8080)
}

const adminServiceName = "admin-go"

func main() {
	// Task B-pilot(2026-08-21):限流指标 service label + Sentinel-golang 动态限流。
	// Sentinel 规则经 Nacos dataId=admin-go-flow-rules 下发;初始化失败→降级放行(RWT)。
	middleware.SetRateLimitService(adminServiceName)
	middleware.InitSentinel(adminServiceName, env("NACOS_ADDR", "nacos:8848"))

	var cat Catalog
	if err := json.Unmarshal(catalogData, &cat); err != nil {
		log.Fatalf("bad catalog.json: %v", err)
	}
	pemStr := os.Getenv("JWT_RSA_PUBLIC_KEY_PEM")
	if pemStr == "" {
		log.Fatal("JWT_RSA_PUBLIC_KEY_PEM env not set")
	}
	pub, err := loadPubKey(pemStr)
	if err != nil {
		log.Fatalf("load pub key: %v", err)
	}
	srv := &Server{
		catalog: &cat,
		pubKey:  pub,
		prom:    env("PROMETHEUS_URL", "http://prometheus.esp-observability.svc.cluster.local:9090"),
		// Phase4 缓存/MQ 各下游基址(env 未设则对应 handler 返 503 降级,RWT 不阻塞)。
		lcoStats:    env("LCO_STATS_URL", ""),
		milvusAddr:  env("MILVUS_ADDR", "milvus:19530"),
		neo4jRest:   env("NEO4J_REST_URL", ""),
		neo4jUser:   env("NEO4J_USER", "neo4j"),
		neo4jPass:   env("NEO4J_PASSWORD", ""),
		pulsarAdmin: env("PULSAR_ADMIN_URL", "http://pulsar-broker-0.pulsar-broker:8080"),
	}

	// Nacos 服务注册(优雅退出时注销)
	nacosCli := registry.RegisterNacos(adminServiceName, 8099)

	mux := http.NewServeMux()
	// 根路径 admin 信息页(无需 token,展示服务状态+端点清单,避免 404 不友好)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"service":"esp-admin","status":"ok","endpoints":["/healthz","/readyz","/api/admin/services","/api/admin/infra","/api/admin/catalog","/api/admin/metrics/query","/api/admin/middleware/health"],"note":"Admin API service. All /api/admin/* require role=admin RS256 JWT. Visit /api/admin/catalog with token for service catalog."}`)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/api/admin/services", srv.requireAdmin(srv.handleServices))
	mux.HandleFunc("/api/admin/infra", srv.requireAdmin(srv.handleInfra))
	mux.HandleFunc("/api/admin/catalog", srv.requireAdmin(srv.handleCatalog))
	mux.HandleFunc("/api/admin/metrics/query", srv.requireAdmin(srv.handleProm))
	mux.HandleFunc("/api/admin/metrics/query_range", srv.requireAdmin(srv.handleProm))
	mux.HandleFunc("/api/admin/metrics/alerts", srv.requireAdmin(srv.handleProm))
	mux.HandleFunc("/api/admin/metrics/rules", srv.requireAdmin(srv.handleProm))
	mux.HandleFunc("/api/admin/governance/nacos", srv.requireAdmin(srv.handleNacosServices))
	mux.HandleFunc("/api/admin/governance/sentinel", srv.requireAdmin(srv.handleSentinelRules))
	mux.HandleFunc("/api/admin/governance/health", srv.requireAdmin(srv.handleGovernanceHealth))

	// 中间件统一访问面板(BFF 代理层,2026-08-19):替换前端硬编码 VM IP 链接。
	// health:聚合各中间件在线状态;{name}/:反向代理到 UI(剥前缀,白名单防 SSRF)。
	// 鉴权:requireAdmin(role=admin),与其它 admin API 对齐。
	mux.HandleFunc("/api/admin/middleware/health", srv.requireAdmin(srv.handleMiddlewareHealth))
	mux.HandleFunc("/api/admin/middleware/{name}/", srv.requireAdmin(srv.handleMiddlewareProxy))
	// 全语言统一深度诊断面板(Step3):代理到各服务 debug/actuator 端点 + 能力矩阵。
	mux.HandleFunc("/api/admin/debug/capabilities", srv.requireAdmin(srv.handleDebugCapabilities))
	mux.HandleFunc("/api/admin/debug/", srv.requireAdmin(srv.handleDebugProxy))

	// 运维操作面板(Phase3 · 2026-08-21):资源监控 + 实时日志 + 可管理操作(scale/restart)。
	// 资源监控走 Prometheus 查询(只读,L0);日志/scale/restart 走 K8s API(需 esp-admin-sa RBAC)。
	// 写操作 scale/restart 为 L2 不可逆(BRR 铁律):requireAdmin 守门 + 前端 Popconfirm 二次确认。
	mux.HandleFunc("/api/admin/ops/resources", srv.requireAdmin(srv.handleOpsResources))
	mux.HandleFunc("/api/admin/logs/", srv.requireAdmin(srv.handleOpsLogs))
	mux.HandleFunc("/api/admin/scale/", srv.requireAdmin(srv.handleOpsScale))
	mux.HandleFunc("/api/admin/restart/", srv.requireAdmin(srv.handleOpsRestart))

	// 各级缓存监控 + Pulsar MQ 消息查看(Phase4 · 2026-08-21)。
	// 缓存:LCO 命中率(宿主 /stats+/metrics)+ Milvus collection(REST)+ Neo4j 节点(Cypher REST);三源独立超时降级(RWT)。
	// Pulsar:Admin REST 8080 代理(无认证)+ 白名单 tenant=esp/ns=esp{business,cache}(SCS);peek 只读不 ack(BRR)。
	mux.HandleFunc("/api/admin/cache/stats", srv.requireAdmin(srv.handleCacheStats))
	mux.HandleFunc("/api/admin/cache/milvus", srv.requireAdmin(srv.handleCacheMilvus))
	mux.HandleFunc("/api/admin/cache/neo4j", srv.requireAdmin(srv.handleCacheNeo4j))
	mux.HandleFunc("/api/admin/pulsar/topics", srv.requireAdmin(srv.handlePulsarTopics))
	mux.HandleFunc("/api/admin/pulsar/messages", srv.requireAdmin(srv.handlePulsarMessages))

	// 服务运行时健康(Phase4 · 2026-08-21):HPA + 熔断 + 限流 + API 错误率。
	// HPA 走 K8s API autoscaling/v2(最稳,不依赖 Prometheus);熔断/限流 admin-go 直抓各 Go 服务 /metrics(绕开 host→podIP 不通)。
	mux.HandleFunc("/api/admin/runtime/hpa", srv.requireAdmin(srv.handleRuntimeHPA))
	mux.HandleFunc("/api/admin/runtime/resilience", srv.requireAdmin(srv.handleRuntimeResilience))
	mux.HandleFunc("/api/admin/runtime/apis", srv.requireAdmin(srv.handleRuntimeApis))

	// /metrics:Prometheus 抓取端点(治理面板自身指标 + middleware 包注册的限流/熔断 gauge)。
	mux.Handle("/metrics", promhttp.Handler())

	addr := ":" + env("PORT", "8099")
	// 治理铁律:限流(100rps/桶200)+ 熔断(5 次失败触发 30s)统一包裹整 mux,
	// 复用 esp-common-go 共享中间件(与 gateway-go/course-path-go 同构,禁止服务内复制)。
	// 链序:限流先拒(早失败)→ 熔断(保护下游)→ mux 分发。
	rateLimited := middleware.HTTPRateLimit(mux, 100, 200)
	cbProtected := middleware.HTTPCircuitBreaker(rateLimited, 5, 30*time.Second, adminServiceName)
	httpSrv := &http.Server{Addr: addr, Handler: cbProtected}
	go func() {
		log.Printf("esp-admin starting %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("[%s] shutting down", adminServiceName)
	registry.DeRegister(adminServiceName, 8099, nacosCli)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, role, err := verifyToken(tokenFromRequest(r), s.pubKey)
		if err != nil || role != "admin" {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleServices(w http.ResponseWriter, _ *http.Request) {
	results := probeAll(s.catalog.Services)
	writeJSON(w, map[string]any{
		"kind":       "service",
		"checked_at": time.Now().UTC().Format(time.RFC3339),
		"summary":    summary(results),
		"items":      results,
	})
}

func (s *Server) handleInfra(w http.ResponseWriter, _ *http.Request) {
	results := probeAll(s.catalog.Infra)
	writeJSON(w, map[string]any{
		"kind":       "infra",
		"checked_at": time.Now().UTC().Format(time.RFC3339),
		"summary":    summary(results),
		"items":      results,
	})
}

func (s *Server) handleCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.catalog)
}

// handleProm 白名单代理 Prometheus 只读端点:query / query_range / alerts / rules。
// 防 SSRF:仅转发到固定 PROM 基址 + 4 个白名单路径,透传 query 参数。
func (s *Server) handleProm(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/metrics/")
	if !validPromPath(path) {
		http.Error(w, "bad path", 400)
		return
	}
	u := fmt.Sprintf("%s/api/v1/%s?%s", s.prom, path, r.URL.RawQuery)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Get(u)
	if err != nil {
		http.Error(w, `{"error":"prometheus unreachable","detail":"`+err.Error()+`"}`, 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func validPromPath(p string) bool {
	switch p {
	case "query", "query_range", "alerts", "rules":
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ---------- Governance: Nacos + Sentinel ----------

type NacosInstance struct {
	ServiceName string `json:"name"`
	GroupName   string `json:"group"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Healthy     string `json:"healthy"` // true|false
}

type NacosServiceEntry struct {
	Name      string `json:"name"`
	Group     string `json:"group"`
	Instances int    `json:"instances"`
	Healthy   int    `json:"healthy"`
}

type SentinelFlowRule struct {
	Resource        string `json:"resource"`
	Grade           int    `json:"grade"`           // 1=QPS, 0=并发线程数
	Count           int    `json:"count"`           // 阈值
	Strategy        int    `json:"strategy"`        // 0=直接, 1=关联, 2=链路
	ControlBehavior int    `json:"controlBehavior"` // 0=快速失败, 1=WarmUp, 2=匀速排队
}

func (s *Server) nacosAddr() string {
	return env("NACOS_ADDR", "nacos:8848")
}

// handleNacosServices 从 Nacos OpenAPI 查询所有已注册服务及其实例健康状态。
// 防 SSRF: 仅转发到固定 NACOS 基址 + 白名单 OpenAPI 路径。
func (s *Server) handleNacosServices(w http.ResponseWriter, r *http.Request) {
	addr := s.nacosAddr()
	u := fmt.Sprintf("http://%s/nacos/v1/ns/catalog/services?pageNo=1&pageSize=200&namespaceId=", addr)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		writeJSON(w, map[string]any{"error": "nacos unreachable", "detail": err.Error(), "services": []NacosServiceEntry{}})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB cap

	// Nacos 返回: {"count":N, "serviceList":[{"name":"x","groupName":"DEFAULT_GROUP",...}]}
	var parsed struct {
		ServiceList []struct {
			Name                 string `json:"name"`
			GroupName            string `json:"groupName"`
			ClusterCount         int    `json:"clusterCount"`
			InstanceCount        int    `json:"instanceCount"`
			HealthyInstanceCount int    `json:"healthyInstanceCount"`
		} `json:"serviceList"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Nacos 可能未就绪或返回非JSON(如 HTML error page)
		writeJSON(w, map[string]any{"error": "nacos parse error", "detail": err.Error(), "services": []NacosServiceEntry{}})
		return
	}
	services := make([]NacosServiceEntry, 0, len(parsed.ServiceList))
	for _, svc := range parsed.ServiceList {
		services = append(services, NacosServiceEntry{
			Name:      svc.Name,
			Group:     svc.GroupName,
			Instances: svc.InstanceCount,
			Healthy:   svc.HealthyInstanceCount,
		})
	}
	writeJSON(w, map[string]any{"services": services})
}

// handleSentinelRules 从 Nacos 配置中心读取各服务的 Sentinel 限流规则。
// dataId 约定: <svc>-flow-rules, group=DEFAULT_GROUP。
func (s *Server) handleSentinelRules(w http.ResponseWriter, r *http.Request) {
	addr := s.nacosAddr()
	knownSvcs := []string{
		"gateway-go", "learning-java", "writing-java",
		"conversation-go", "emotion-svc", "bff-java",
	}
	client := &http.Client{Timeout: 3 * time.Second}
	result := make(map[string][]SentinelFlowRule)

	for _, svc := range knownSvcs {
		dataId := svc + "-flow-rules"
		u := fmt.Sprintf("http://%s/nacos/v1/cs/configs?dataId=%s&group=DEFAULT_GROUP&tenant=", addr, dataId)
		resp, err := client.Get(u)
		if err != nil {
			continue // Nacos 不可达或超时,该服务规则跳过
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10)) // 512KB
		resp.Body.Close()
		content := strings.TrimSpace(string(body))
		if content == "" {
			continue // 无配置
		}
		// 规则存为 JSON array 或逗号分隔, 尝试 JSON 解析
		var rules []SentinelFlowRule
		if err := json.Unmarshal([]byte(content), &rules); err == nil && len(rules) > 0 {
			result[svc] = rules
		}
		// 非 JSON 格式则忽略(后续扩展支持 YAML)
	}
	writeJSON(w, map[string]any{"rules": result})
}

// handleGovernanceHealth 聚合:服务探活 + Nacos注册状态 + Sentinel规则数,提供一站式治理概览。
func (s *Server) handleGovernanceHealth(w http.ResponseWriter, r *http.Request) {
	// 1) 服务探活(复用现有 probeAll)
	probeResults := probeAll(s.catalog.Services)
	probeSummary := summary(probeResults)

	// 2) Nacos 服务数(快速查询)
	nacosAddr := s.nacosAddr()
	nacosClient := &http.Client{Timeout: 3 * time.Second}
	nacosSvcCount := -1
	nResp, err := nacosClient.Get(fmt.Sprintf("http://%s/nacos/v1/ns/catalog/services?pageNo=1&pageSize=1&namespaceId=", nacosAddr))
	if err == nil {
		defer nResp.Body.Close()
		var nParsed struct {
			Count int `json:"count"`
		}
		if body, _ := io.ReadAll(io.LimitReader(nResp.Body, 64<<10)); err == nil {
			_ = json.Unmarshal(body, &nParsed)
			nacosSvcCount = nParsed.Count
		}
	}

	// 3) Sentinel 规则数(快速遍历,只计数)
	sentinelClient := &http.Client{Timeout: 2 * time.Second}
	sentinelRuleCount := 0
	knownSvcs := []string{"gateway-go", "learning-java", "writing-java", "conversation-go", "emotion-svc", "bff-java"}
	for _, svc := range knownSvcs {
		u := fmt.Sprintf("http://%s/nacos/v1/cs/configs?dataId=%s-flow-rules&group=DEFAULT_GROUP&tenant=", nacosAddr, svc)
		resp, err := sentinelClient.Get(u)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
		resp.Body.Close()
		if len(strings.TrimSpace(string(body))) > 0 {
			sentinelRuleCount++
		}
	}

	writeJSON(w, map[string]any{
		"probes":              probeResults,
		"probe_summary":       probeSummary,
		"nacos_service_count": nacosSvcCount,
		"sentinel_rules_svc":  sentinelRuleCount,
		"checked_at":          time.Now().UTC().Format(time.RFC3339),
	})
}
