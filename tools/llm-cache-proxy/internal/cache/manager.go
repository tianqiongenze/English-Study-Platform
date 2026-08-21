// Package cache — L1 精确缓存(Redis)+ L2 语义缓存(Milvus)+ L3 知识图谱(Neo4j)
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"

	"llm-cache-proxy/internal/config"
	"llm-cache-proxy/internal/protocol"
)

// L1 key 前缀(协议分键:用户决策 L1分键+L2跨协议)
const l1Prefix = "lco:1:"

var (
	hitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "lco_cache_hits_total", Help: "LCO cache hits by layer.",
	}, []string{"layer"})
	missesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "lco_cache_misses_total", Help: "LCO cache misses by layer.",
	}, []string{"layer"})
	errorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "lco_cache_errors_total", Help: "LCO cache errors by layer.",
	}, []string{"layer"})
)

// Manager 三层缓存管理器。
type Manager struct {
	cfg    *config.Config
	pool   *L1Pool        // L1 Redis 多 pod 池(MOVED 重路由)
	l2     *L2Semantic
	l3     *L3Knowledge
	logger *slog.Logger
}

// New 构造缓存管理器。L2/L3 不可达时降级(只 warn 不阻断),与 ESP cache 包语义一致。
func New(cfg *config.Config, logger *slog.Logger) (*Manager, error) {
	// Redis Cluster 经 port-forward 接入,ClusterClient 会 CLUSTER SLOTS 拿到 pod FQDN
	// (如 redis-cluster-new-4...svc.cluster.local),宿主 DNS 解析不了 → i/o timeout。
	// 单节点 Client 遇 MOVED 不重试,命中率仅 ~33%。
	// L1Pool 维护 6 个单节点 Client,MovedError 时从 FQDN 提取 pod 索引重路由 → 命中率 100%。
	pool := NewL1Pool(cfg.RedisAddr, cfg.RedisPassword)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		logger.Warn("L1 Redis Pool 不可达,缓存降级(直穿上游)", "addr", cfg.RedisAddr, "err", err)
	}

	m := &Manager{cfg: cfg, pool: pool, logger: logger}

	if cfg.L2Enabled {
		m.l2 = NewL2Semantic(cfg, logger)
	}
	if cfg.L3Enabled {
		m.l3 = NewL3Knowledge(cfg, logger)
		// L3 启动建 Neo4j 约束+索引(幂等,失败仅 warn 降级为无图增强)。
		// 2026-08-19 修正:之前 NewL3Knowledge 后未调 ensureSchema,导致
		// MERGE 可跑但 CONTAINS 查询全表扫,且 CONSTRAINT 未建则 MERGE 无法保证唯一。
		schemaCtx, schemaCancel := context.WithTimeout(context.Background(), 5*time.Second)
		m.l3.ensureSchema(schemaCtx)
		schemaCancel()
	}
	return m, nil
}

// NewDisabledForTest 构造禁用缓存的 Manager(仅测试用)。
// CacheEnabled=false 时 Lookup/Store 第一行即返回空(零值安全,不连真实 Redis/Milvus/Neo4j)。
// 用于 server 包测试聚焦验证转发/流式逻辑,不依赖三件套基础设施。
func NewDisabledForTest() *Manager {
	return &Manager{cfg: &config.Config{CacheEnabled: false}}
}

// LookupResult 缓存查询结果。
type LookupResult struct {
	Hit      bool
	Layer    string // "L1" / "L2" / "" (miss)
	Body     []byte // 缓存的响应体(已是目标协议格式)
	SourceProto protocol.Protocol // L2 命中时记录源协议(用于跨协议重生成判断)
	// L3 上下文增强(L1/L2 miss 时从知识图谱查到的相关历史 response)。
	// 非空时调用方可作为 RAG 上下文前缀注入上游请求;本轮仅通过响应头暴露,不修改请求体。
	EnhancedContext string
}

// Lookup 查询缓存:L1 精确(同协议)→ L2 语义(跨协议,需重生成)→ L3 上下文增强。
// L1 命中直接返回(同协议响应可直用);L2 命中返回源协议响应+标记,调用方决定是否重生成。
// CLC 铁律:cfg/rdb/logger nil 安全(零值 Manager 必须安全,不 panic)。
func (m *Manager) Lookup(ctx context.Context, key protocol.NormalizeKey) LookupResult {
	if m == nil || m.cfg == nil || !m.cfg.CacheEnabled || key.Protocol == protocol.Unknown {
		return LookupResult{}
	}
	log := m.logger // 可能 nil,用局部变量 + 判空
	safeLog := func(msg string, args ...any) {
		if log != nil {
			log.Warn(msg, args...)
		}
	}
	if m.pool == nil {
		// Redis 不可达:跳过 L1,尝试 L2(L2 独立降级)
		if m.l2 != nil {
			l2Ctx, l2Cancel := context.WithTimeout(context.Background(), time.Duration(m.cfg.L2TimeoutSec)*time.Second)
			body, srcProto, ok := m.l2.Search(l2Ctx, key)
			l2Cancel()
			if ok {
				hitsTotal.WithLabelValues("L2").Inc()
				if log != nil {
					log.Info("L2 语义命中", "srcProto", srcProto, "dstProto", key.Protocol)
				}
				return LookupResult{Hit: true, Layer: "L2", Body: body, SourceProto: srcProto}
			}
		}
		// Redis 降级路径仍走 L3 上下文增强(L3 独立于 Redis)
		var enhanced string
		if m.l3 != nil {
			l3Ctx, l3Cancel := context.WithTimeout(context.Background(), 5*time.Second)
			enhanced = m.l3.Enhance(l3Ctx, key)
			l3Cancel()
			if enhanced != "" {
				hitsTotal.WithLabelValues("L3").Inc()
			}
		}
		return LookupResult{EnhancedContext: enhanced}
	}

	// L1:协议分键精确匹配(L1Pool 自动 MOVED 重路由)
	l1Key := m.l1Key(key)
	if val, err := m.pool.Get(ctx, l1Key); err == nil && len(val) > 0 {
		hitsTotal.WithLabelValues("L1").Inc()
		safeLog("L1 命中", "key", l1Key, "proto", key.Protocol)
		return LookupResult{Hit: true, Layer: "L1", Body: []byte(val)}
	} else if err != nil && !errors.Is(err, redis.Nil) {
		// CLC:redis.Nil = 正常 cache miss(key 不存在),不计 error;
		// 仅真错误(MOVED 未恢复/io timeout 等)记 error。原 strings.Contains 太脆弱。
		errorsTotal.WithLabelValues("L1").Inc()
		safeLog("L1 读失败(非致命)", "err", err)
	}

	// L2:语义匹配(跨协议)。独立短超时,避免 embedding 慢拖垮主请求(降级直穿)
	if m.l2 != nil {
		l2Ctx, l2Cancel := context.WithTimeout(context.Background(), time.Duration(m.cfg.L2TimeoutSec)*time.Second)
		body, srcProto, ok := m.l2.Search(l2Ctx, key)
		l2Cancel()
		if ok {
			hitsTotal.WithLabelValues("L2").Inc()
			safeLog("L2 语义命中", "srcProto", srcProto, "dstProto", key.Protocol)
			return LookupResult{Hit: true, Layer: "L2", Body: body, SourceProto: srcProto}
		}
	}

	// L3:上下文增强(L1/L2 miss 时查知识图谱找相关历史 response 作为 RAG 上下文)。
	// 2026-08-19 实装:之前 Enhance 写了但 Lookup 未调用→L3 只写不读,违反"完整三层都起"。
	// 独立短超时,失败降级为空上下文(直穿上游)。仅暴露为 EnhancedContext 字段,不修改请求体
	// (真实 RAG 注入需改上游请求体,成本权衡后本轮仅标记 + 响应头暴露,重写 DEFERRED)。
	var enhanced string
	if m.l3 != nil {
		l3Ctx, l3Cancel := context.WithTimeout(context.Background(), 5*time.Second)
		enhanced = m.l3.Enhance(l3Ctx, key)
		l3Cancel()
		if enhanced != "" {
			hitsTotal.WithLabelValues("L3").Inc()
			safeLog("L3 上下文增强命中", "ctxLen", len(enhanced), "proto", key.Protocol)
		}
	}

	missesTotal.WithLabelValues("L1").Inc()
	return LookupResult{EnhancedContext: enhanced}
}

// Store 写入缓存:L1(同协议)+ L2(归一化向量,供跨协议检索)+ L3(知识图谱节点)。
// CLC 铁律:cfg/rdb/logger nil 安全。
func (m *Manager) Store(ctx context.Context, key protocol.NormalizeKey, respBody []byte, ttlScene string) {
	if m == nil || m.cfg == nil || !m.cfg.CacheEnabled || key.Protocol == protocol.Unknown {
		return
	}
	log := m.logger
	safeWarn := func(msg string, args ...any) {
		if log != nil {
			log.Warn(msg, args...)
		}
	}

	// L1 写入(协议分键,L1Pool 自动 MOVED 重路由)
	ttl := m.ttlFor(ttlScene)
	l1Key := m.l1Key(key)
	if m.pool != nil {
		if err := m.pool.Set(ctx, l1Key, string(respBody), time.Duration(ttl)*time.Second); err != nil {
			errorsTotal.WithLabelValues("L1").Inc()
			safeWarn("L1 写失败(非致命)", "err", err)
		}
	}

	// L2 写入(归一化向量)
	if m.l2 != nil {
		m.l2.Store(ctx, key, respBody)
	}

	// L3 写入(知识图谱:prompt→response 关系)
	if m.l3 != nil {
		m.l3.Record(ctx, key, respBody)
	}
}

// l1Key 生成 L1 缓存键:lco:1:{protocol}:{sha256前16位}。
func (m *Manager) l1Key(key protocol.NormalizeKey) string {
	h := sha256.Sum256([]byte(key.Raw))
	return fmt.Sprintf("%s%s:%s", l1Prefix, key.Protocol, hex.EncodeToString(h[:])[:16])
}

func (m *Manager) ttlFor(scene string) int {
	switch scene {
	case "code":
		return m.cfg.TTLCode
	case "knowledge":
		return m.cfg.TTLKnowledge
	default:
		return m.cfg.TTLChat
	}
}

// ClassifyScene 粗分场景(影响 TTL):含代码块/长文本→code,短问答→chat。
// 简化实现:按 user 长度 + 关键词。
func ClassifyScene(key protocol.NormalizeKey) string {
	u := strings.ToLower(key.User)
	if strings.Contains(u, "```") || strings.Contains(u, "func ") ||
		strings.Contains(u, "def ") || strings.Contains(u, "class ") ||
		len(key.User) > 2000 {
		return "code"
	}
	if strings.Contains(u, "what is") || strings.Contains(u, "explain") ||
		strings.Contains(u, "定义") || strings.Contains(u, "什么是") {
		return "knowledge"
	}
	return "chat"
}

// HealthStatus 缓存层健康状态(供 /healthz)。
type HealthStatus struct {
	L1Redis  string `json:"l1_redis"`   // "ok" / "down"
	L2Milvus string `json:"l2_milvus"`
	L3Neo4j  string `json:"l3_neo4j"`
}

// HealthCheck 聚合 L1/L2/L3 健康状态。
func (m *Manager) HealthCheck(ctx context.Context) HealthStatus {
	h := HealthStatus{L1Redis: "down", L2Milvus: "disabled", L3Neo4j: "disabled"}
	if m.pool != nil {
		if err := m.pool.Ping(ctx); err == nil {
			h.L1Redis = "ok"
		}
	}
	if m.l2 != nil {
		h.L2Milvus = m.l2.Health(ctx)
	}
	if m.l3 != nil {
		h.L3Neo4j = m.l3.Health(ctx)
	}
	return h
}

// StatsJSON 暴露缓存统计(供 /metrics + /stats)。
func (m *Manager) StatsJSON() string {
	stats := map[string]any{
		"l1_prefix": l1Prefix,
		"l2_enabled": m.l2 != nil,
		"l3_enabled": m.l3 != nil,
	}
	b, _ := json.Marshal(stats)
	return string(b)
}
