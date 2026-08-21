package main

// HTTP handler 单测(用 httptest + 真实下游 httptest.Server)。
// CLC 铁律:测真实执行路径——probeOne 真拨 httptest.Server、handleProm 真转发、
// 缓存 handler 在下游不可达时走 RWT 降级路径(status=down 仍返 200 不 500)。
// 非"语义该返回什么"而是逐行走 handler 内每个解引用/分支。

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------- main.go: probeOne / handleProm / handleCatalog / requireAdmin handleServices ----------

func TestProbeOne_HTTP_Up(t *testing.T) {
	// 真实 httptest.Server 返 200 → probeOne 标 UP
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	// 解析 httptest 地址 host:port
	host, port := splitHostPort(srv.URL, t)
	tgt := Target{Name: "test-svc", Kind: "service", Lang: "Go", Svc: host, Port: port, Path: "/healthz", Type: "http"}
	r := probeOne(tgt)
	if r.Status != "UP" {
		t.Errorf("Status: got %q, want UP", r.Status)
	}
	if r.Code != 200 {
		t.Errorf("Code: got %d, want 200", r.Code)
	}
	if r.LatencyMS < 0 {
		t.Errorf("LatencyMS negative: %d", r.LatencyMS)
	}
}

func TestProbeOne_HTTP_Down_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	host, port := splitHostPort(srv.URL, t)
	tgt := Target{Name: "broken-svc", Kind: "service", Lang: "Go", Svc: host, Port: port, Path: "/", Type: "http"}
	r := probeOne(tgt)
	if r.Status != "DOWN" {
		t.Errorf("Status: got %q, want DOWN", r.Status)
	}
	if r.Code != 500 {
		t.Errorf("Code: got %d, want 500", r.Code)
	}
	if !strings.Contains(r.Msg, "http 500") {
		t.Errorf("Msg: got %q, want contain 'http 500'", r.Msg)
	}
}

func TestProbeOne_HTTP_Unreachable(t *testing.T) {
	// 用不可达端口(127.0.0.1:1 一般被拒) → DOWN + err Msg
	tgt := Target{Name: "dead-svc", Kind: "service", Lang: "Go", Svc: "127.0.0.1", Port: 1, Path: "/", Type: "http"}
	r := probeOne(tgt)
	if r.Status != "DOWN" {
		t.Errorf("Status: got %q, want DOWN", r.Status)
	}
	if r.Msg == "" {
		t.Error("Msg empty, want error detail")
	}
	if r.Code != 0 {
		t.Errorf("Code: got %d, want 0 (no response)", r.Code)
	}
}

func TestProbeOne_TCP(t *testing.T) {
	// 起 TCP listener(不响应 HTTP),Type=tcp 应只测连通标 UP
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	host, port := splitHostPort("http://"+ln.Addr().String(), t)
	tgt := Target{Name: "redis-svc", Kind: "infra", Lang: "redis", Svc: host, Port: port, Path: "", Type: "tcp"}
	r := probeOne(tgt)
	if r.Status != "UP" {
		t.Errorf("tcp Status: got %q, want UP", r.Status)
	}
	if r.Code != 0 {
		t.Errorf("tcp Code: got %d, want 0 (tcp 无 HTTP code)", r.Code)
	}
}

func TestProbeAll_Concurrent(t *testing.T) {
	srvUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srvUp.Close()
	srvDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srvDown.Close()
	hUp, pUp := splitHostPort(srvUp.URL, t)
	hDown, pDown := splitHostPort(srvDown.URL, t)
	targets := []Target{
		{Name: "up1", Kind: "service", Lang: "Go", Svc: hUp, Port: pUp, Path: "/", Type: "http"},
		{Name: "up2", Kind: "service", Lang: "Go", Svc: hUp, Port: pUp, Path: "/", Type: "http"},
		{Name: "down1", Kind: "service", Lang: "Go", Svc: hDown, Port: pDown, Path: "/", Type: "http"},
	}
	out := probeAll(targets)
	if len(out) != 3 {
		t.Fatalf("len: got %d, want 3", len(out))
	}
	// 顺序应保持(probeAll 用 index 写回,非 append)
	upCount := 0
	for _, r := range out {
		if r.Status == "UP" {
			upCount++
		}
	}
	if upCount != 2 {
		t.Errorf("up count: got %d, want 2", upCount)
	}
	// 名字对齐(验证非乱序)
	if out[0].Name != "up1" || out[2].Name != "down1" {
		t.Errorf("order mismatch: %s %s %s", out[0].Name, out[1].Name, out[2].Name)
	}
}

func TestHandleProm_Happy(t *testing.T) {
	// 真实 Prometheus mock 返 200 + body,handleProm 应原样透传
	promSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			t.Errorf("prom path: got %q, want /api/v1/ prefix", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"success","data":"ok"}`))
	}))
	defer promSrv.Close()
	srv := &Server{prom: promSrv.URL}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/metrics/query?query=up", nil)
	req.SetPathValue("x", "") // 不影响,路径已含
	w := httptest.NewRecorder()
	srv.handleProm(w, req)
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	if !strings.Contains(w.Body.String(), `"success"`) {
		t.Errorf("body: got %q, want contain success", w.Body.String())
	}
}

func TestHandleProm_BadPath(t *testing.T) {
	srv := &Server{prom: "http://does-not-exist:9090"}
	// validPromPath 只认 query/query_range/alerts/rules,"delete" 应 400
	req := httptest.NewRequest(http.MethodGet, "/api/admin/metrics/delete", nil)
	w := httptest.NewRecorder()
	srv.handleProm(w, req)
	if w.Code != 400 {
		t.Errorf("bad path code: got %d, want 400", w.Code)
	}
}

func TestHandleProm_Unreachable(t *testing.T) {
	// prom 指向不可达 → 502
	srv := &Server{prom: "http://127.0.0.1:1"}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/metrics/query?query=up", nil)
	w := httptest.NewRecorder()
	srv.handleProm(w, req)
	if w.Code != 502 {
		t.Errorf("unreachable code: got %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "prometheus unreachable") {
		t.Errorf("body: got %q, want contain 'prometheus unreachable'", w.Body.String())
	}
}

func TestHandleCatalog(t *testing.T) {
	// 真 catalog.json(//go:embed)应可返 Services/Infra
	var cat Catalog
	if err := json.Unmarshal(catalogData, &cat); err != nil {
		t.Fatalf("parse catalogData: %v", err)
	}
	srv := &Server{catalog: &cat, pubKey: &testPrivateKey(t).PublicKey}
	w := httptest.NewRecorder()
	srv.handleCatalog(w, httptest.NewRequest(http.MethodGet, "/api/admin/catalog", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var got Catalog
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, w.Body.String())
	}
	if len(got.Services) != len(cat.Services) {
		t.Errorf("services len: got %d, want %d", len(got.Services), len(cat.Services))
	}
	if len(got.Infra) != len(cat.Infra) {
		t.Errorf("infra len: got %d, want %d", len(got.Infra), len(cat.Infra))
	}
	// catalog.json 应非空(embed 已加载)
	if len(cat.Services)+len(cat.Infra) == 0 {
		t.Error("catalog.json has no services or infra entries (embed failed?)")
	}
}

// ---------- debug_proxy.go ----------

func TestHandleDebugCapabilities(t *testing.T) {
	srv := &Server{catalog: &Catalog{}, pubKey: &testPrivateKey(t).PublicKey}
	w := httptest.NewRecorder()
	srv.handleDebugCapabilities(w, httptest.NewRequest(http.MethodGet, "/api/admin/debug/capabilities", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	// 响应结构:{"capabilities":{lang:[endpoints],...},"endpoints":[...],"note":"..."}
	var got struct {
		Capabilities map[string][]string `json:"capabilities"`
		Endpoints    []string           `json:"endpoints"`
		Note         string             `json:"note"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}
	// 应含 Java/Go 至少
	if _, ok := got.Capabilities["Java"]; !ok {
		t.Errorf("Java missing: %+v", got.Capabilities)
	}
	if _, ok := got.Capabilities["Go"]; !ok {
		t.Errorf("Go missing: %+v", got.Capabilities)
	}
	// Java 应有 threaddump/heapdump,Python 应没有(NA)
	javaHasHeap := false
	for _, e := range got.Capabilities["Java"] {
		if e == "heapdump" {
			javaHasHeap = true
		}
	}
	if !javaHasHeap {
		t.Errorf("Java capabilities missing heapdump: %v", got.Capabilities["Java"])
	}
	pyHasThread := false
	for _, e := range got.Capabilities["Python"] {
		if e == "threaddump" {
			pyHasThread = true
		}
	}
	if pyHasThread {
		t.Errorf("Python should not have threaddump (NA): %v", got.Capabilities["Python"])
	}
	// endpoints 全集应有 6 项
	if len(got.Endpoints) != 6 {
		t.Errorf("endpoints len: got %d, want 6", len(got.Endpoints))
	}
}

func TestFindService(t *testing.T) {
	cat := &Catalog{Services: []Target{
		{Name: "gateway-go", Lang: "Go", Svc: "gateway-go", Port: 8080, Path: "/healthz", Type: "http"},
		{Name: "bff-java", Lang: "Java", Svc: "bff-java", Port: 8084, Path: "/actuator/health", Type: "http"},
	}}
	srv := &Server{catalog: cat}
	// 存在
	tgt, ok := srv.findService("bff-java")
	if !ok {
		t.Fatal("findService(bff-java): not found")
	}
	if tgt.Lang != "Java" {
		t.Errorf("Lang: got %q, want Java", tgt.Lang)
	}
	if tgt.Port != 8084 {
		t.Errorf("Port: got %d, want 8084", tgt.Port)
	}
	// 不存在(防 SSRF:catalog 外不可达)
	_, ok = srv.findService("evil-external-svc")
	if ok {
		t.Error("findService(evil): should not be found (SSRF protection)")
	}
	// 空 catalog
	srv2 := &Server{catalog: &Catalog{}}
	_, ok = srv2.findService("anything")
	if ok {
		t.Error("findService in empty catalog: should not be found")
	}
}

// ---------- cache.go: RWT 降级路径(下游未配置/不可达 handler 返 200 status=down 不 500)----------

func TestHandleCacheMilvus_NotConfigured(t *testing.T) {
	// milvusAddr 空 → milvusCollectionCount 报 "MILVUS_ADDR not configured"
	// → handleCacheMilvus 走 RWT 降级 status=down 返 200(CLC:追 s.milvusAddr=="" 分支)
	srv := &Server{catalog: &Catalog{}, pubKey: &testPrivateKey(t).PublicKey, milvusAddr: ""}
	w := httptest.NewRecorder()
	srv.handleCacheMilvus(w, httptest.NewRequest(http.MethodGet, "/api/admin/cache/milvus", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200 (RWT degrade, not 500)", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}
	if got["status"] != "down" {
		t.Errorf("status: got %v, want down", got["status"])
	}
	if msg, _ := got["msg"].(string); !strings.Contains(msg, "MILVUS_ADDR not configured") {
		t.Errorf("msg: got %q, want contain 'MILVUS_ADDR not configured'", msg)
	}
}

func TestHandleCacheNeo4j_NotConfigured(t *testing.T) {
	// neo4jRest 空 → 报 "NEO4J_REST_URL not configured" → status=down 返 200
	srv := &Server{catalog: &Catalog{}, pubKey: &testPrivateKey(t).PublicKey, neo4jRest: ""}
	w := httptest.NewRecorder()
	srv.handleCacheNeo4j(w, httptest.NewRequest(http.MethodGet, "/api/admin/cache/neo4j", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}
	if got["status"] != "down" {
		t.Errorf("status: got %v, want down", got["status"])
	}
}

func TestMilvusCollectionCount_RealServer(t *testing.T) {
	// 真实 Milvus mock:返 {"code":0,"data":{"numEntities":12345}}
	ms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/vectordb/collections/describe" {
			t.Errorf("path: got %q, want /v2/vectordb/collections/describe", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("method: got %q, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"collectionName":"llm_cache_prompts","numEntities":12345}}`))
	}))
	defer ms.Close()
	host, port := splitHostPort(ms.URL, t)
	srv := &Server{milvusAddr: host + ":" + itoa(port)}
	num, err := srv.milvusCollectionCount("llm_cache_prompts")
	if err != nil {
		t.Fatalf("milvusCollectionCount: %v", err)
	}
	if num != "12345" {
		t.Errorf("num: got %q, want 12345", num)
	}
}

func TestMilvusCollectionCount_ErrorCode(t *testing.T) {
	// Milvus 返 code!=0 → 报错(RWT 由调用方降级)
	ms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":1000,"msg":"collection not found"}`))
	}))
	defer ms.Close()
	host, port := splitHostPort(ms.URL, t)
	srv := &Server{milvusAddr: host + ":" + itoa(port)}
	_, err := srv.milvusCollectionCount("nonexistent")
	if err == nil {
		t.Fatal("expected error for code!=0, got nil")
	}
	if !strings.Contains(err.Error(), "code=1000") {
		t.Errorf("err: got %q, want contain 'code=1000'", err.Error())
	}
}

func TestNeo4jCounts_RealServer(t *testing.T) {
	// 真实 Neo4j tx/commit mock:返 3 个 result(Prompt/Response 计数 + ANSWED_BY 关系计数)
	ns := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/db/neo4j/tx/commit") {
			t.Errorf("path: got %q, want .../db/neo4j/tx/commit", r.URL.Path)
		}
		// BasicAuth 应带(neo4j/pass)
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Error("BasicAuth not set")
		}
		if user != "neo4j" || pass != "secret-pass" {
			t.Errorf("BasicAuth: got %s:%s, want neo4j:secret-pass", user, pass)
		}
		_, _ = w.Write([]byte(`{"results":[{"columns":["c"],"rows":[[100]]},{"columns":["c"],"rows":[[50]]},{"columns":["c"],"rows":[[7]]}],"errors":[]}`))
	}))
	defer ns.Close()
	srv := &Server{neo4jRest: ns.URL, neo4jUser: "neo4j", neo4jPass: "secret-pass"}
	nodes, rels, err := srv.neo4jCounts()
	if err != nil {
		t.Fatalf("neo4jCounts: %v", err)
	}
	if nodes["Prompt"] != "100" {
		t.Errorf("nodes[Prompt]: got %q, want 100", nodes["Prompt"])
	}
	if nodes["Response"] != "50" {
		t.Errorf("nodes[Response]: got %q, want 50", nodes["Response"])
	}
	if rels["ANSWERED_BY"] != "7" {
		t.Errorf("rels[ANSWERED_BY]: got %q, want 7", rels["ANSWERED_BY"])
	}
}

func TestNeo4jCounts_WithErrors(t *testing.T) {
	// Neo4j 返 errors 非空 → 报错(RWT 由调用方降级)
	ns := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[],"errors":[{"code":"Neo.ClientError","message":"auth failed"}]}`))
	}))
	defer ns.Close()
	srv := &Server{neo4jRest: ns.URL, neo4jUser: "neo4j", neo4jPass: "x"}
	_, _, err := srv.neo4jCounts()
	if err == nil {
		t.Fatal("expected error for neo4j errors, got nil")
	}
	if !strings.Contains(err.Error(), "Neo.ClientError") {
		t.Errorf("err: got %q, want contain 'Neo.ClientError'", err.Error())
	}
}

// ---------- helpers ----------

// splitHostPort 解析 httptest.Server.URL("http://127.0.0.1:PORT")为 host + int port。
func splitHostPort(url string, t *testing.T) (string, int) {
	t.Helper()
	// 去 "http://" 或 "https://" 前缀
	rest := strings.TrimPrefix(url, "http://")
	rest = strings.TrimPrefix(rest, "https://")
	// rest = host:port[/path],取冒号分隔
	idx := strings.LastIndex(rest, ":")
	if idx < 0 {
		t.Fatalf("no port in %q", url)
	}
	host := rest[:idx]
	portStr := rest[idx+1:]
	// 可能带 path,截到第一个 /
	if slash := strings.Index(portStr, "/"); slash >= 0 {
		portStr = portStr[:slash]
	}
	port := 0
	for _, c := range portStr {
		if c < '0' || c > '9' {
			t.Fatalf("bad port %q in %q", portStr, url)
		}
		port = port*10 + int(c-'0')
	}
	return host, port
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// 引用 time 以防 unused(import 在其它函数用 time.Now())
var _ = time.Second
