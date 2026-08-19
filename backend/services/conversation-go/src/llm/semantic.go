// Package llm 的 L2 语义缓存(Phase2,Milvus 向量检索)。
//
// SemanticCache 包装 Milvus REST(esp_llm_semantic collection,384 维 minilm,COSINE)做语义模糊缓存:
//  1. Search(prompt):embedding → Milvus /v1/vector/search,cosine≥0.92(LLM 语义等价阈值,见 LCO 铁律)视为命中。
//  2. Store(prompt,response):embedding → Milvus /v1/vector/insert,写 prompt_sha16 主键 + response + 元数据 + vector。
//
// 与 CachedProvider(L1 exact)正交:L1 做精确 SHA256 命中省 100%,L1 MISS 才查 L2 语义命中省 80-95%。
// collection 字段:prompt_sha16(VarChar PK)、response、model、scene、ts(Int64)、vector(FloatVector 384)。
//   注:FloatVector 字段名固定为 `vector`(Milvus REST insert/search 用字段名映射向量,见 ASV 实测)。
//
// 降级铁律(CLC+LCO):Milvus/embedding 不可达时 warn+skip,绝不阻断 LLM 主链路(与 cached.go L1 同模式)。
//   所有 HTTP 调用 3s 超时;nil receiver/nil client 必须安全返回 (false, false),不 panic。
// 端点从 env 读:MILVUS_ADDR(默认 milvus:19530)、EMBEDDING_URL(默认 http://host.lima.internal:15730/embed)。
// 开关:ESP_LLM_L2_ENABLED(默认 true),false 时 Search/Store 直接返回未命中(不报错)。
//
// ASV 实测 Milvus REST v1(2026-08-19):
//   - /v1/vector/collections/create  body{collectionName,dimension,metricType,schema.fields[],indexParams[]} → 200{code,data}
//   - /v1/vector/collections          GET → 200{code,data:["name",...]}(存在性检查用 list,describe 对不存在返回 HTTP 404 不可靠)
//   - /v1/vector/collections/describe 对已存在返回 200,但 list 更可靠作存在性判断
//   - /v1/vector/insert  body{collectionName,data:[{prompt_sha16,...,vector:[float32...]}]} → 200{insertCount}
//   - /v1/vector/search  body{collectionName,vector:[float32...](扁平,非嵌套),limit,outputFields,params:{radius}} → 200{data:[{distance,...}]}
//     注:COSINE metric 下 distance 即 cosine 相似度(1.0=相同);radius 过滤 + 应用层 threshold 双保险。
package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// L2 语义缓存配置常量。
const (
	l2DefaultMilvusAddr     = "milvus:19530"
	l2DefaultEmbeddingURL   = "http://host.lima.internal:15730/embed"
	l2EmbeddingModel        = "minilm"          // 固定 minilm 384 维,避免 zh/en 维度不一致
	l2VectorDim             = 384              // minilm all-MiniLM-L6-v2 输出维度
	l2SemanticThreshold     = 0.92            // LCO 铁律:cosine≥0.92 视为语义等价
	l2CollectionName        = "esp_llm_semantic"
	l2HTTPTimeout           = 3 * time.Second // 降级不阻断:超时即 skip
	l2ShaLen                = 16
)

// SemanticCache 是 L2 语义缓存(Milvus)。
// client 为 nil(降级模式)时所有方法安全返回未命中。
// enabled=false(ESP_LLM_L2_ENABLED=false)时 Search/Store 短路。
// 注意:Lazy collection 创建在第一次 Store 或 Search 时触发(initCollectionOnce);
//   若 Milvus 不可达,init 失败置 ready=false,后续调用降级 skip。
type SemanticCache struct {
	client        *http.Client
	milvusAddr    string // host:port,不含 scheme
	embeddingURL  string
	enabled       bool
	collectionOK  bool // collection 是否已确认存在(成功 create 后置 true)
	checkedOnce   bool
}

// NewSemanticCache 构造 L2 语义缓存装饰器。
// client 为 nil 时降级(Search 返回未命中,Store no-op,均不 panic)。
// env MILVUS_ADDR / EMBEDDING_URL 可覆盖默认端点。
func NewSemanticCache(client *http.Client) *SemanticCache {
	milvusAddr := os.Getenv("MILVUS_ADDR")
	if milvusAddr == "" {
		milvusAddr = l2DefaultMilvusAddr
	}
	embeddingURL := os.Getenv("EMBEDDING_URL")
	if embeddingURL == "" {
		embeddingURL = l2DefaultEmbeddingURL
	}
	enabled := os.Getenv("ESP_LLM_L2_ENABLED") != "false" // 默认 true
	if !enabled {
		log.Printf("[llm-cache] L2 disabled by ESP_LLM_L2_ENABLED=false")
	}
	if client == nil && enabled {
		log.Printf("[llm-cache] L2 enabled but client nil, will degrade (skip)")
	}
	return &SemanticCache{
		client:       client,
		milvusAddr:   milvusAddr,
		embeddingURL: embeddingURL,
		enabled:      enabled,
	}
}

// Search 查 L2 语义缓存:embedding prompt → Milvus search → cosine≥0.92 返回 (response, true)。
// 任何环节失败(禁用/nil client/embedding 失败/Milvus 不可达/无命中)返回 ("", false),不 panic、不阻断。
func (s *SemanticCache) Search(ctx context.Context, prompt, model, scene string) (string, bool) {
	if s == nil || !s.enabled || s.client == nil {
		return "", false
	}
	if err := s.ensureCollection(ctx); err != nil {
		log.Printf("[llm-cache] L2 Search skip (collection not ready): %v", err)
		return "", false
	}
	vec, err := s.embed(ctx, prompt)
	if err != nil || len(vec) == 0 {
		log.Printf("[llm-cache] L2 Search embed skip: err=%v len=%d", err, len(vec))
		return "", false
	}
	hit, distance, err := s.milvusSearch(ctx, vec)
	if err != nil {
		log.Printf("[llm-cache] L2 Search milvus skip: %v", err)
		return "", false
	}
	if hit == "" {
		cacheMissesTotal.WithLabelValues("semantic").Inc()
		return "", false
	}
	if distance < l2SemanticThreshold {
		// 有结果但相似度不足阈值 → 视为 MISS(不计 inner 调用,由上层记)
		log.Printf("[llm-cache] L2 near-miss distance=%.4f < %.2f", distance, l2SemanticThreshold)
		cacheMissesTotal.WithLabelValues("semantic").Inc()
		return "", false
	}
	cacheHitsTotal.WithLabelValues("semantic").Inc()
	log.Printf("[llm-cache] L2 HIT distance=%.4f", distance)
	return hit, true
}

// Store 写 L2 语义缓存:embedding prompt → Milvus insert。失败 warn+skip,不阻断。
func (s *SemanticCache) Store(ctx context.Context, prompt, response, model, scene string) {
	if s == nil || !s.enabled || s.client == nil {
		return
	}
	if response == "" {
		return // 不缓存空回复
	}
	if err := s.ensureCollection(ctx); err != nil {
		log.Printf("[llm-cache] L2 Store skip (collection not ready): %v", err)
		return
	}
	vec, err := s.embed(ctx, prompt)
	if err != nil || len(vec) == 0 {
		log.Printf("[llm-cache] L2 Store embed skip: err=%v len=%d", err, len(vec))
		return
	}
	if err := s.milvusInsert(ctx, prompt, response, model, scene, vec); err != nil {
		log.Printf("[llm-cache] L2 Store milvus skip (non-fatal): %v", err)
	}
}

// ensureCollection 幂等确保 collection 存在(已确认则短路)。
// 用 list 接口检查存在性(describe 对不存在返回 HTTP 404,不可靠,见 ASV 实测)。
// 不存在则 create(含 schema + index)。
func (s *SemanticCache) ensureCollection(ctx context.Context) error {
	if s.collectionOK {
		return nil
	}
	// 1) list 检查存在性
	exists, err := s.collectionExists(ctx)
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	if !exists {
		if err := s.createCollection(ctx); err != nil {
			return fmt.Errorf("create collection: %w", err)
		}
		log.Printf("[llm-cache] L2 Milvus collection created: %s (dim=%d metric=COSINE)", l2CollectionName, l2VectorDim)
	}
	s.collectionOK = true
	return nil
}

// collectionExists 调 GET /v1/vector/collections,判断目标 collection 是否在列表中。
func (s *SemanticCache) collectionExists(ctx context.Context) (bool, error) {
	url := fmt.Sprintf("http://%s/v1/vector/collections", s.milvusAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 限 1MB
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("list collections HTTP %d: %s", resp.StatusCode, l2Truncate(string(body), 200))
	}
	var lr struct {
		Code int      `json:"code"`
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(body, &lr); err != nil {
		return false, fmt.Errorf("unmarshal list: %w (body=%s)", err, l2Truncate(string(body), 200))
	}
	for _, name := range lr.Data {
		if name == l2CollectionName {
			return true, nil
		}
	}
	return false, nil
}

// createCollection 调 POST /v1/vector/collections/create 建 collection(schema + index)。
func (s *SemanticCache) createCollection(ctx context.Context) error {
	body := map[string]any{
		"collectionName": l2CollectionName,
		"dimension":      l2VectorDim,
		"metricType":     "COSINE",
		"schema": map[string]any{
			"fields": []map[string]any{
				{"name": "prompt_sha16", "dataType": "VarChar", "isPrimary": true, "elementTypeParams": map[string]string{"max_length": "64"}},
				{"name": "response", "dataType": "VarChar", "elementTypeParams": map[string]string{"max_length": "32768"}},
				{"name": "model", "dataType": "VarChar", "elementTypeParams": map[string]string{"max_length": "128"}},
				{"name": "scene", "dataType": "VarChar", "elementTypeParams": map[string]string{"max_length": "64"}},
				{"name": "ts", "dataType": "Int64"},
				{"name": "vector", "dataType": "FloatVector", "elementTypeParams": map[string]string{"dim": fmt.Sprintf("%d", l2VectorDim)}},
			},
		},
		"indexParams": []map[string]any{
			{"fieldName": "vector", "metricType": "COSINE", "indexType": "AUTOINDEX"},
		},
	}
	return s.postJSON(ctx, fmt.Sprintf("http://%s/v1/vector/collections/create", s.milvusAddr), body, nil)
}

// embed 调 EMBEDDING_URL 取 prompt 的 minilm 向量(384 维)。
func (s *SemanticCache) embed(ctx context.Context, text string) ([]float32, error) {
	body := map[string]string{"text": text, "model": l2EmbeddingModel}
	var resp struct {
		Vector []float32 `json:"vector"`
	}
	if err := s.postJSON(ctx, s.embeddingURL, body, &resp); err != nil {
		return nil, err
	}
	return resp.Vector, nil
}

// milvusSearch 调 /v1/vector/search,vector 为扁平 float32 切片(非嵌套,见 ASV 实测)。
// 返回命中的 response 文本 + cosine distance(0 表示无命中)。
func (s *SemanticCache) milvusSearch(ctx context.Context, vec []float32) (string, float64, error) {
	body := map[string]any{
		"collectionName": l2CollectionName,
		"vector":         vec, // 扁平 []float32,Milvus REST 自动解析
		"limit":          1,
		"outputFields":   []string{"response", "model", "scene", "prompt_sha16"},
		"params":         map[string]any{"radius": l2SemanticThreshold},
	}
	var resp struct {
		Code int `json:"code"`
		Data []struct {
			Distance float64 `json:"distance"`
			Response string  `json:"response"`
		} `json:"data"`
	}
	if err := s.postJSON(ctx, fmt.Sprintf("http://%s/v1/vector/search", s.milvusAddr), body, &resp); err != nil {
		return "", 0, err
	}
	if resp.Code != 200 || len(resp.Data) == 0 {
		return "", 0, nil
	}
	return resp.Data[0].Response, resp.Data[0].Distance, nil
}

// milvusInsert 调 /v1/vector/insert 写一条语义缓存记录。
func (s *SemanticCache) milvusInsert(ctx context.Context, prompt, response, model, scene string, vec []float32) error {
	body := map[string]any{
		"collectionName": l2CollectionName,
		"data": []map[string]any{
			{
				"prompt_sha16": sha16(prompt),
				"response":     response,
				"model":        model,
				"scene":        scene,
				"ts":           time.Now().Unix(),
				"vector":       vec,
			},
		},
	}
	return s.postJSON(ctx, fmt.Sprintf("http://%s/v1/vector/insert", s.milvusAddr), body, nil)
}

// postJSON 发 POST JSON 请求,3s 超时(用 ctx 截止时间覆盖),on success 反序列化到 out(可 nil)。
// 任何错误返回 error(调用方降级 skip)。
func (s *SemanticCache) postJSON(ctx context.Context, url string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, l2HTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, l2Truncate(string(respBody), 200))
	}
	if out == nil {
		return nil
	}
	// 兼容 Milvus {code:200,data:{}} —— 反序列化忽略 code 非 200 由调用方判断
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("unmarshal: %w (body=%s)", err, l2Truncate(string(respBody), 200))
	}
	return nil
}

// sha16 返回 SHA256 前 16 位 hex(Milvus 主键 prompt_sha16)。
func sha16(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:l2ShaLen]
}

// l2Truncate 裁剪日志字符串到 maxLen,避免日志爆量(上下文工程铁律)。
// 命名为 l2Truncate 因 mock.go 已有同名 truncate,避免重定义。
func l2Truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}
