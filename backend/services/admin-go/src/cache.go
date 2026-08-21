package main

// 各级缓存实时情况监控(Phase4 · 2026-08-21)。
// 三源双轨聚合,仿 ops.go:92 handleOpsResources 的并发聚合范式:
//   ① LCO 命中率(宿主 15720):GET /stats(LCO server.go:63 原样 JSON)+ GET /metrics(prometheus exposition 正则提 lco_cache_hits_total{layer}/lco_l2_*)
//   ② Milvus collection 记录数(集群 milvus:19530 REST):POST /v2/vectordb/collections/describe(仿 l2.go:164),解析 numEntities
//   ③ Neo4j 节点/关系数(宿主 REST 7474):POST /db/neo4j/tx/commit Cypher(仿 l3.go:49),BasicAuth 用 secret
//
// 鉴权:requireAdmin(role=admin RS256 JWT)。
// RWT 铁律:三源各 5s 独立超时 + 失败标 status:"down" 返 200 不 500,不阻塞其它层。
// 防 SSRF:基址全从 env 配置(信任域),非外部输入。
//
// 路由(main.go 注册):
//   GET /api/admin/cache/stats   —— 三源聚合(L1/L2/L3 命中率 + Milvus/Neo4j 结构数)
//   GET /api/admin/cache/milvus   —— Milvus collection 直查
//   GET /api/admin/cache/neo4j    —— Neo4j Cypher 直查

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"esp/common-go/middleware"
)

// CacheLayerStat 单层缓存统计。
type CacheLayerStat struct {
	Layer   string            `json:"layer"` // L1|L2|L3
	Name    string            `json:"name"`  // Redis|Milvus|Neo4j
	Hits    string            `json:"hits"`  // 累计命中数
	Misses  string            `json:"misses"`
	Errors  string            `json:"errors"`
	HitRate string            `json:"hit_rate"` // 命中率 hits/(hits+misses),"0-1"
	Status  string            `json:"status"`   // ok|down|na
	Extra   map[string]string `json:"extra,omitempty"`
	Msg     string            `json:"msg,omitempty"`
}

// CacheStatsResponse 缓存监控聚合响应。
type CacheStatsResponse struct {
	Kind      string           `json:"kind"` // "cache_stats"
	CheckedAt string           `json:"checked_at"`
	Layers    []CacheLayerStat `json:"layers"`
	LCOStats  json.RawMessage  `json:"lco_stats,omitempty"` // LCO /stats 原样透传
	Note      string           `json:"note"`
}

// handleCacheStats GET /api/admin/cache/stats —— 并发查 3 源(LCO/Milvus/Neo4j),独立超时降级(RWT)。
func (s *Server) handleCacheStats(w http.ResponseWriter, _ *http.Request) {
	// Task B-pilot:Sentinel 动态流控(resource=api/admin/cache)。被限流返 429。
	if !middleware.AllowOrLimit(adminServiceName, "api/admin/cache") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"flow limited","retry_after":1}`))
		return
	}
	var (
		mu       sync.Mutex
		layers   []CacheLayerStat
		lcoStats json.RawMessage
	)
	var wg sync.WaitGroup
	add := func(st CacheLayerStat) {
		mu.Lock()
		defer mu.Unlock()
		layers = append(layers, st)
	}

	// ① LCO 命中率(宿主 /stats + /metrics)
	wg.Add(1)
	go func() {
		defer wg.Done()
		st := CacheLayerStat{Layer: "L1+L2+L3", Name: "LCO(llm-cache-proxy)", Status: "ok"}
		client := &http.Client{Timeout: 5 * time.Second}
		// /stats 原样(LCO server.go:63 StatsJSON,含 l1_prefix/l2_enabled/l3_enabled)
		if s.lcoStats != "" {
			resp, err := client.Get(s.lcoStats + "/stats")
			if err != nil {
				st.Status = "down"
				st.Msg = err.Error()
				add(st)
				return
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			if resp.StatusCode == 200 {
				lcoStats = json.RawMessage(body)
			}
		}
		// /metrics 提 lco_cache_hits_total{layer} / lco_l2_hits_total / misses
		if s.lcoStats != "" {
			resp, err := client.Get(s.lcoStats + "/metrics")
			if err == nil {
				mbody, _ := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
				resp.Body.Close()
				st.Hits = extractLCOMetric(mbody, `lco_cache_hits_total\{layer="L1"\}\s+([0-9.]+)`)
				st.Misses = extractLCOMetric(mbody, `lco_cache_misses_total\{layer="L1"\}\s+([0-9.]+)`)
				// L2 独立计数
				l2hits := extractLCOMetric(mbody, `lco_l2_hits_total\s+([0-9.]+)`)
				l2miss := extractLCOMetric(mbody, `lco_l2_misses_total\s+([0-9.]+)`)
				if l2hits != "" || l2miss != "" {
					if st.Extra == nil {
						st.Extra = map[string]string{}
					}
					st.Extra["l2_hits"] = l2hits
					st.Extra["l2_misses"] = l2miss
				}
			}
		}
		st.HitRate = computeHitRate(st.Hits, st.Misses)
		if st.Hits == "" && st.Misses == "" && st.Status == "ok" {
			st.Status = "na"
		}
		add(st)
	}()

	// ② Milvus collection 记录数(REST describe)
	wg.Add(1)
	go func() {
		defer wg.Done()
		st := CacheLayerStat{Layer: "L2", Name: "Milvus", Status: "ok"}
		num, err := s.milvusCollectionCount("llm_cache_prompts")
		if err != nil {
			st.Status = "down"
			st.Msg = err.Error()
		} else {
			if st.Extra == nil {
				st.Extra = map[string]string{}
			}
			st.Extra["collection"] = "llm_cache_prompts"
			st.Extra["num_entities"] = num
		}
		add(st)
	}()

	// ③ Neo4j 节点/关系数(Cypher REST)
	wg.Add(1)
	go func() {
		defer wg.Done()
		st := CacheLayerStat{Layer: "L3", Name: "Neo4j", Status: "ok"}
		nodes, rels, err := s.neo4jCounts()
		if err != nil {
			st.Status = "down"
			st.Msg = err.Error()
		} else {
			if st.Extra == nil {
				st.Extra = map[string]string{}
			}
			for k, v := range nodes {
				st.Extra["nodes_"+k] = v
			}
			for k, v := range rels {
				st.Extra["rels_"+k] = v
			}
		}
		add(st)
	}()

	wg.Wait()
	writeJSON(w, CacheStatsResponse{
		Kind:      "cache_stats",
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Layers:    layers,
		LCOStats:  lcoStats,
		Note:      "L1 Redis + L2 Milvus + L3 Neo4j(三层缓存命中率 + 结构数据);LCO /stats 原样透传;三源独立 5s 超时降级",
	})
}

// handleCacheMilvus GET /api/admin/cache/milvus —— 直查 Milvus collection。
func (s *Server) handleCacheMilvus(w http.ResponseWriter, _ *http.Request) {
	num, err := s.milvusCollectionCount("llm_cache_prompts")
	status := "ok"
	msg := ""
	if err != nil {
		status = "down"
		msg = err.Error()
	}
	writeJSON(w, map[string]any{
		"kind":         "cache_milvus",
		"checked_at":   time.Now().UTC().Format(time.RFC3339),
		"collection":   "llm_cache_prompts",
		"num_entities": num,
		"status":       status,
		"msg":          msg,
	})
}

// handleCacheNeo4j GET /api/admin/cache/neo4j —— 直查 Neo4j 节点/关系数。
func (s *Server) handleCacheNeo4j(w http.ResponseWriter, _ *http.Request) {
	nodes, rels, err := s.neo4jCounts()
	status := "ok"
	msg := ""
	if err != nil {
		status = "down"
		msg = err.Error()
	}
	writeJSON(w, map[string]any{
		"kind":          "cache_neo4j",
		"checked_at":    time.Now().UTC().Format(time.RFC3339),
		"nodes":         nodes,
		"relationships": rels,
		"status":        status,
		"msg":           msg,
	})
}

// milvusCollectionCount 查 Milvus collection 记录数(REST /v2/vectordb/collections/describe)。
// 仿 l2.go:164 的 REST 范式。失败返 error 由调用方降级(RWT)。
func (s *Server) milvusCollectionCount(collection string) (string, error) {
	if s.milvusAddr == "" {
		return "", fmt.Errorf("MILVUS_ADDR not configured")
	}
	body, _ := json.Marshal(map[string]string{"collectionName": collection})
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://%s/v2/vectordb/collections/describe", s.milvusAddr), strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	// Milvus 2.5 返回 {"code":0,"data":{"collectionName":...,"numEntities":N,...}}
	var parsed struct {
		Code int `json:"code"`
		Data struct {
			NumEntities json.Number `json:"numEntities"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(rb, &parsed); err != nil {
		return "", fmt.Errorf("milvus parse: %w (body: %s)", err, truncate(string(rb), 100))
	}
	if parsed.Code != 0 {
		return "", fmt.Errorf("milvus code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return string(parsed.Data.NumEntities), nil
}

// neo4jCounts 查 Neo4j 节点/关系数(REST Cypher,仿 l3.go:49 neo4jTx)。
// 跑 MATCH(n:Prompt)RETURN count(n) + MATCH(n:Response)RETURN count(n) + MATCH()-[r:ANSWERED_BY]->()RETURN count(r)。
func (s *Server) neo4jCounts() (nodes, rels map[string]string, err error) {
	if s.neo4jRest == "" {
		return nil, nil, fmt.Errorf("NEO4J_REST_URL not configured")
	}
	statements := []map[string]any{
		{"statement": "MATCH (n:Prompt) RETURN count(n) AS c"},
		{"statement": "MATCH (n:Response) RETURN count(n) AS c"},
		{"statement": "MATCH ()-[r:ANSWERED_BY]->() RETURN count(r) AS c"},
	}
	body, _ := json.Marshal(map[string]any{"statements": statements})
	endpoint := strings.TrimRight(s.neo4jRest, "/") + "/db/neo4j/tx/commit"
	req, _ := http.NewRequest("POST", endpoint, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.neo4jUser, s.neo4jPass)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	// Neo4j 返回 {"results":[{"columns":["c"],"rows":[[123]]},...]}
	var parsed struct {
		Results []struct {
			Columns []string        `json:"columns"`
			Rows    [][]json.Number `json:"rows"`
		} `json:"results"`
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rb, &parsed); err != nil {
		return nil, nil, fmt.Errorf("neo4j parse: %w (body: %s)", err, truncate(string(rb), 100))
	}
	if len(parsed.Errors) > 0 {
		return nil, nil, fmt.Errorf("neo4j error: %s %s", parsed.Errors[0].Code, parsed.Errors[0].Message)
	}
	nodes = map[string]string{}
	rels = map[string]string{}
	labels := []string{"Prompt", "Response"}
	for i, res := range parsed.Results {
		if i >= 2 {
			break
		}
		val := ""
		if len(res.Rows) > 0 && len(res.Rows[0]) > 0 {
			val = string(res.Rows[0][0])
		}
		nodes[labels[i]] = val
	}
	if len(parsed.Results) >= 3 {
		val := ""
		if len(parsed.Results[2].Rows) > 0 && len(parsed.Results[2].Rows[0]) > 0 {
			val = string(parsed.Results[2].Rows[0][0])
		}
		rels["ANSWERED_BY"] = val
	}
	return nodes, rels, nil
}

// extractLCOMetric 从 LCO /metrics(prometheus exposition 文本)正则提标量值。
// 如 `lco_cache_hits_total{layer="L1"} 42` → "42"。未匹配返空。
func extractLCOMetric(body []byte, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}

// computeHitRate 算命中率 hits/(hits+misses),保留 4 位小数。
func computeHitRate(hits, misses string) string {
	h := parseFloatSafe(hits)
	mm := parseFloatSafe(misses)
	total := h + mm
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("%.4f", h/total)
}

// parseFloatSafe 容错解析浮点,失败返 0。
func parseFloatSafe(s string) float64 {
	var f float64
	_, _ = fmt.Sscanf(s, "%f", &f)
	return f
}

// truncate 字符串截断(用于错误信息)。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
