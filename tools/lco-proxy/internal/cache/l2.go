// Package cache — L2 语义缓存(embedding + Milvus,跨协议命中)
package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"lco-proxy/internal/config"
	"lco-proxy/internal/protocol"
)

var (
	l2Hits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lco_l2_hits_total", Help: "L2 semantic cache hits."})
	l2Misses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lco_l2_misses_total", Help: "L2 semantic cache misses."})
)

// L2Semantic L2 语义缓存:prompt 向量化 → Milvus cosine 检索 → 跨协议命中。
// 跨协议:存时记源协议,命中时若与目标协议不同,调用方负责重生成(或直穿)。
type L2Semantic struct {
	cfg    *config.Config
	logger *slog.Logger
	hc     *http.Client
}

// l2Record Milvus 中存的 payload(响应体 + 源协议 + 时间)。
type l2Record struct {
	Body     []byte                `json:"body"`
	Protocol protocol.Protocol     `json:"protocol"`
	Model    string                `json:"model"`
	Ts       int64                 `json:"ts"`
}

func NewL2Semantic(cfg *config.Config, logger *slog.Logger) *L2Semantic {
	return &L2Semantic{
		cfg:    cfg,
		logger: logger,
		hc:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Search 向量化 prompt → Milvus cosine 检索。命中返回(响应体, 源协议, true)。
func (l *L2Semantic) Search(ctx context.Context, key protocol.NormalizeKey) ([]byte, protocol.Protocol, bool) {
	vec, err := l.embed(ctx, key.Raw)
	if err != nil {
		l.logger.Warn("L2 embedding 失败(非致命)", "err", err)
		l2Misses.Inc()
		return nil, "", false
	}

	// Milvus 查询:cosine 相似度 > 阈值
	rec, ok := l.milvusSearch(ctx, vec, l.cfg.L2Threshold)
	if !ok {
		l2Misses.Inc()
		return nil, "", false
	}
	return rec.Body, rec.Protocol, true
}

// Store 向量化 prompt + 存响应体 + 源协议到 Milvus。
func (l *L2Semantic) Store(ctx context.Context, key protocol.NormalizeKey, body []byte) {
	vec, err := l.embed(ctx, key.Raw)
	if err != nil {
		l.logger.Warn("L2 store embedding 失败(非致命)", "err", err)
		return
	}
	rec := l2Record{Body: body, Protocol: key.Protocol, Model: key.Model, Ts: time.Now().Unix()}
	l.milvusInsert(ctx, vec, rec)
}

// embed 调宿主 embedding 服务(127.0.0.1:15730/embed)。
func (l *L2Semantic) embed(ctx context.Context, text string) ([]float32, error) {
	payload, _ := json.Marshal(map[string]string{"text": text})
	req, _ := http.NewRequestWithContext(ctx, "POST", l.cfg.EmbeddingURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("embedding HTTP %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Vector []float32 `json:"vector"`
		Model  string    `json:"model"`
		Dim    int       `json:"dim"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Vector) == 0 {
		return nil, fmt.Errorf("embedding 返回空向量")
	}
	return out.Vector, nil
}

// milvusSearch / milvusInsert 通过 Milvus REST API(2.5 v2/vectordb/entities 端点)。
// 单节点验证期用 REST 简化(避免 Go Milvus SDK 重依赖)。
// 2026-08-19 实测修正:Milvus 2.5 正确端点是 /v2/vectordb/entities/search|insert,
//   原 /v2/vector/* 在 2.5 返回 404 静默失败(WTV 核实目标系统);data 字段需 [[vec]] 二层数组。
func (l *L2Semantic) milvusSearch(ctx context.Context, vec []float32, threshold float64) (*l2Record, bool) {
	body := map[string]any{
		"collectionName": l.cfg.L2Collection,
		"data":           [][]float32{vec},
		"limit":          1,
		"outputFields":   []string{"body", "protocol", "model", "ts"},
		"params":         map[string]any{"radius": threshold},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/v2/vectordb/entities/search", l.cfg.MilvusAddr), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.hc.Do(req)
	if err != nil {
		l.logger.Warn("L2 Milvus search 失败(非致命)", "err", err)
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		// 诊断:读 body 帮助定位(collection 不存在/维度不匹配等),不阻断(降级)
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		l.logger.Warn("L2 Milvus search 非 200(非致命)", "status", resp.StatusCode, "body", string(rb))
		return nil, false
	}
	var out struct {
		Data []struct {
			Body     string              `json:"body"`
			Protocol protocol.Protocol   `json:"protocol"`
			Model    string              `json:"model"`
			Distance float64             `json:"distance"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, false
	}
	if len(out.Data) == 0 {
		return nil, false
	}
	// distance 在 cosine 下 = 相似度(越大越相似)
	if out.Data[0].Distance < threshold {
		return nil, false
	}
	l2Hits.Inc()
	return &l2Record{
		Body:     []byte(out.Data[0].Body),
		Protocol: out.Data[0].Protocol,
		Model:    out.Data[0].Model,
	}, true
}

func (l *L2Semantic) milvusInsert(ctx context.Context, vec []float32, rec l2Record) {
	body := map[string]any{
		"collectionName": l.cfg.L2Collection,
		"data": []map[string]any{{
			"vector":   vec,
			"body":     string(rec.Body),
			"protocol": string(rec.Protocol),
			"model":    rec.Model,
			"ts":       rec.Ts,
		}},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/v2/vectordb/entities/insert", l.cfg.MilvusAddr), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.hc.Do(req)
	if err != nil {
		l.logger.Warn("L2 Milvus insert 失败(非致命)", "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		l.logger.Warn("L2 Milvus insert 非 200(非致命)", "status", resp.StatusCode, "body", string(rb))
	}
}

func (l *L2Semantic) Health(ctx context.Context) string {
	// 2026-08-19 修正:Milvus 2.5 无 /healthz 端点(404),改用 collections/list POST 探活
	rb, _ := json.Marshal(map[string]any{})
	req, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/v2/vectordb/collections/list", l.cfg.MilvusAddr), bytes.NewReader(rb))
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.hc.Do(req)
	if err != nil {
		return "down"
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return "ok"
	}
	return "down"
}
