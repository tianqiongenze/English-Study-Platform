package main

// 治理面 handler 单测(handleServices/handleInfra/handleNacosServices/handleGovernanceHealth/
// handlePulsarTopics)。用 httptest.Server mock Nacos/Pulsar admin + 真实 catalog 指向 httptest,
// 走真实执行路径(probeAll→probeOne→httptest、Nacos OpenAPI parse、Pulsar REST stats 聚合)。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// makeUpCatalog 构造一个指向 up httptest server 的 catalog(复用同 server 测多 target)。
func makeUpCatalog(t *testing.T, svcURL, infraURL string) *Catalog {
	t.Helper()
	sh, sp := splitHostPort(svcURL, t)
	ih, ip := splitHostPort(infraURL, t)
	return &Catalog{
		Services: []Target{
			{Name: "up-svc", Kind: "service", Lang: "Go", Svc: sh, Port: sp, Path: "/healthz", Type: "http"},
		},
		Infra: []Target{
			{Name: "up-infra", Kind: "infra", Lang: "redis", Svc: ih, Port: ip, Path: "/", Type: "http"},
		},
	}
}

func TestHandleServices(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer up.Close()
	srv := &Server{catalog: makeUpCatalog(t, up.URL, up.URL), pubKey: &testPrivateKey(t).PublicKey}
	w := httptest.NewRecorder()
	srv.handleServices(w, httptest.NewRequest(http.MethodGet, "/api/admin/services", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}
	if got["kind"] != "service" {
		t.Errorf("kind: got %v, want service", got["kind"])
	}
	summary, _ := got["summary"].(map[string]any)
	if summary["total"].(float64) != 1 {
		t.Errorf("summary.total: got %v, want 1", summary["total"])
	}
	if summary["up"].(float64) != 1 {
		t.Errorf("summary.up: got %v, want 1", summary["up"])
	}
	if summary["down"].(float64) != 0 {
		t.Errorf("summary.down: got %v, want 0", summary["down"])
	}
}

func TestHandleInfra(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer up.Close()
	srv := &Server{catalog: makeUpCatalog(t, up.URL, up.URL), pubKey: &testPrivateKey(t).PublicKey}
	w := httptest.NewRecorder()
	srv.handleInfra(w, httptest.NewRequest(http.MethodGet, "/api/admin/infra", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}
	if got["kind"] != "infra" {
		t.Errorf("kind: got %v, want infra", got["kind"])
	}
}

func TestHandleNacosServices(t *testing.T) {
	// mock Nacos OpenAPI catalog/services 返 serviceList
	ns := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v1/ns/catalog/services" {
			t.Errorf("path: got %q, want /nacos/v1/ns/catalog/services", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":2,"serviceList":[{"name":"gateway-go","groupName":"DEFAULT_GROUP","clusterCount":1,"instanceCount":2,"healthyInstanceCount":2},{"name":"bff-java","groupName":"DEFAULT_GROUP","clusterCount":1,"instanceCount":1,"healthyInstanceCount":0}]}`))
	}))
	defer ns.Close()
	t.Setenv("NACOS_ADDR", ns.Listener.Addr().String()) // nacosAddr() 走 env
	srv := &Server{catalog: &Catalog{}, pubKey: &testPrivateKey(t).PublicKey}
	w := httptest.NewRecorder()
	srv.handleNacosServices(w, httptest.NewRequest(http.MethodGet, "/api/admin/governance/nacos", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}
	services, ok := got["services"].([]any)
	if !ok {
		t.Fatalf("services not array: %T (body=%s)", got["services"], w.Body.String())
	}
	if len(services) != 2 {
		t.Fatalf("services len: got %d, want 2", len(services))
	}
	first := services[0].(map[string]any)
	if first["name"] != "gateway-go" {
		t.Errorf("first name: got %v, want gateway-go", first["name"])
	}
	if first["instances"].(float64) != 2 {
		t.Errorf("first instances: got %v, want 2", first["instances"])
	}
	if first["healthy"].(float64) != 2 {
		t.Errorf("first healthy: got %v, want 2", first["healthy"])
	}
}

func TestHandleNacosServices_Unreachable(t *testing.T) {
	// Nacos 指向不可达 → 返 error + 空 services list,不 500(RWT 降级)
	t.Setenv("NACOS_ADDR", "127.0.0.1:1")
	srv := &Server{catalog: &Catalog{}, pubKey: &testPrivateKey(t).PublicKey}
	w := httptest.NewRecorder()
	srv.handleNacosServices(w, httptest.NewRequest(http.MethodGet, "/api/admin/governance/nacos", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200 (degrade)", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["error"] != "nacos unreachable" {
		t.Errorf("error: got %v, want 'nacos unreachable'", got["error"])
	}
	services, _ := got["services"].([]any)
	if len(services) != 0 {
		t.Errorf("services len on unreachable: got %d, want 0", len(services))
	}
}

func TestHandleGovernanceHealth(t *testing.T) {
	// 端到端:probeAll 指向 up server + Nacos mock 返 count + Sentinel 配置返非空
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer up.Close()
	nacosSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"count":5}`)) // catalog/services 返 count
		// configs 返非空 → sentinelRuleCount++
	}))
	defer nacosSrv.Close()
	t.Setenv("NACOS_ADDR", nacosSrv.Listener.Addr().String())
	srv := &Server{catalog: makeUpCatalog(t, up.URL, up.URL), pubKey: &testPrivateKey(t).PublicKey}
	w := httptest.NewRecorder()
	srv.handleGovernanceHealth(w, httptest.NewRequest(http.MethodGet, "/api/admin/governance/health", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}
	if got["nacos_service_count"].(float64) != 5 {
		t.Errorf("nacos_service_count: got %v, want 5", got["nacos_service_count"])
	}
	probeSummary, _ := got["probe_summary"].(map[string]any)
	if probeSummary["up"].(float64) != 1 {
		t.Errorf("probe_summary.up: got %v, want 1", probeSummary["up"])
	}
	// sentinel_rules_svc:6 个已知服务都返非空 → 6
	if got["sentinel_rules_svc"].(float64) != 6 {
		t.Errorf("sentinel_rules_svc: got %v, want 6", got["sentinel_rules_svc"])
	}
}

func TestHandlePulsarTopics(t *testing.T) {
	// mock Pulsar Admin:topics 列表 + 每个 topic stats
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/admin/v2/namespaces/esp/business/topics" {
			_, _ = w.Write([]byte(`["persistent://esp/business/session.ended","persistent://esp/business/xp.awarded"]`))
			return
		}
		if r.URL.Path == "/admin/v2/persistent/esp/business/session.ended/stats" || r.URL.Path == "/admin/v2/persistent/esp/business/xp.awarded/stats" {
			_, _ = w.Write([]byte(`{"subscriptions":{"sub1":{"msgBacklog":42,"msgRateOut":1.5,"msgRateRedeliver":0,"subscriptionType":"Shared"}},"backlogSize":1024,"producers":[{}],"msgRateIn":2.0,"msgRateOut":1.5}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ps.Close()
	srv := &Server{catalog: &Catalog{}, pubKey: &testPrivateKey(t).PublicKey, pulsarAdmin: ps.URL}
	w := httptest.NewRecorder()
	srv.handlePulsarTopics(w, httptest.NewRequest(http.MethodGet, "/api/admin/pulsar/topics?namespace=esp/business", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var got PulsarTopicsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}
	if got.Kind != "pulsar_topics" {
		t.Errorf("kind: got %q, want pulsar_topics", got.Kind)
	}
	if len(got.Topics) != 2 {
		t.Fatalf("topics len: got %d, want 2", len(got.Topics))
	}
	if got.Topics[0].Topic != "session.ended" {
		t.Errorf("topics[0].Topic: got %q, want session.ended", got.Topics[0].Topic)
	}
	if got.Topics[0].Status != "ok" {
		t.Errorf("topics[0].Status: got %q, want ok", got.Topics[0].Status)
	}
	if len(got.Topics[0].Subscriptions) != 1 {
		t.Fatalf("subs len: got %d, want 1", len(got.Topics[0].Subscriptions))
	}
	if got.Topics[0].Subscriptions[0].MsgBacklog != 42 {
		t.Errorf("sub backlog: got %d, want 42", got.Topics[0].Subscriptions[0].MsgBacklog)
	}
	if got.Topics[0].Producers != 1 {
		t.Errorf("producers: got %d, want 1", got.Topics[0].Producers)
	}
}

func TestHandlePulsarTopics_NamespaceNotAllowed(t *testing.T) {
	// 白名单外的 ns 应 400(SCS 防越权)
	srv := &Server{catalog: &Catalog{}, pubKey: &testPrivateKey(t).PublicKey, pulsarAdmin: "http://x"}
	w := httptest.NewRecorder()
	srv.handlePulsarTopics(w, httptest.NewRequest(http.MethodGet, "/api/admin/pulsar/topics?namespace=evil/ns", nil))
	if w.Code != 400 {
		t.Errorf("code: got %d, want 400", w.Code)
	}
	if !contains(w.Body.String(), "namespace not allowed") {
		t.Errorf("body: got %q, want contain 'namespace not allowed'", w.Body.String())
	}
}

func TestHandlePulsarTopics_NotConfigured(t *testing.T) {
	// pulsarAdmin 空 → 503
	srv := &Server{catalog: &Catalog{}, pubKey: &testPrivateKey(t).PublicKey, pulsarAdmin: ""}
	w := httptest.NewRecorder()
	srv.handlePulsarTopics(w, httptest.NewRequest(http.MethodGet, "/api/admin/pulsar/topics?namespace=esp/business", nil))
	if w.Code != 503 {
		t.Errorf("code: got %d, want 503", w.Code)
	}
}

func TestHandleMiddlewareHealth(t *testing.T) {
	// 各中间件目标都是不可达地址 → 全 DOWN 但返 200(聚合不阻塞)
	srv := &Server{catalog: &Catalog{}, pubKey: &testPrivateKey(t).PublicKey}
	w := httptest.NewRecorder()
	srv.handleMiddlewareHealth(w, httptest.NewRequest(http.MethodGet, "/api/admin/middleware/health", nil))
	if w.Code != 200 {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}
	if got["kind"] != "middleware" {
		t.Errorf("kind: got %v, want middleware", got["kind"])
	}
	summary, _ := got["summary"].(map[string]any)
	// middlewareTargets 8 项,全不可达 → total=8 up=0 down=8
	if summary["total"].(float64) != 8 {
		t.Errorf("summary.total: got %v, want 8", summary["total"])
	}
	if summary["up"].(float64) != 0 {
		t.Errorf("summary.up: got %v, want 0 (all unreachable)", summary["up"])
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
