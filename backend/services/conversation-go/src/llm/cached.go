// Package llm 的 L1 精确缓存装饰器(Phase1)。
//
// CachedProvider 包装一个 inner Provider,对 Chat 请求做精确缓存(exact-match):
//  1. 规范化 messages(canonical)—— 去除首尾空白,保留语义内容;
//     注:conversation-go 的 messages 是真实对话文本(无 traceId/时间戳注入,见 mock.go/anthropic.go),
//     故规范化仅做 trim 防御,不做语义抽取(语义模糊匹配放 Phase2 L2 Milvus)。
//  2. SHA256(canonical + model) 作为缓存键分片,避免长 key。
//  3. Redis key: esp:cache:llm:exact:{model}:{sha256前16位}
//  4. GET 命中 → 直接返回缓存回复(记 cache_hits_total);MISS → 委托 inner.Chat() → 回写 SETEX TTL=1h(记 cache_misses_total)。
//
// 降级:ESP_LLM_CACHE_ENABLED=false 或 Redis 不可达时,直穿 inner(只 warn 不阻断),
// 与 cache 包 L1+L2 语义一致(Redis 挂了不影响功能正确性)。
//
// 设计要点:
//   - 零改 dialog.Engine:Engine.LLM 仍是 llm.Provider,CachedProvider 实现同接口,构造时包一层即可。
//   - Provider.Chat 只返回 (string, error),无 metadata 字段;cache_hit 通过 Prometheus 计数器 + 结构化日志体现,
//     不臆造带 metadata 的签名(会破坏 Engine 注入,违反 ASV 铁律)。
//   - 不复用 cache 包的 client():那是 gateway-go 场景数据 L1+L2,语义/TTL 不同;LLM 缓存独立 L2 client
//     (依赖 redis.Cmdable 接口,生产注入 *redis.ClusterClient,测试注入 miniredis 的 *redis.Client),
//     但复用相同 Redis Cluster 短名 redis-cluster(ExternalName → redis-cluster-new,3主3从)。
package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
)

// 缓存配置常量。
const (
	cacheKeyPrefix = "esp:cache:llm:exact:"
	cacheTTL       = 1 * time.Hour // conversation 类回复缓存 1h
	cacheShaLen    = 16             // sha256 前 16 位,碰撞空间 2^128 足够
)

// Prometheus 指标(layer 标签区分 L1 exact / 未来 Phase2 L2 semantic)。
var (
	cacheHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_cache_hits_total",
		Help: "LLM cache hits by layer.",
	}, []string{"layer"})

	cacheMissesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_cache_misses_total",
		Help: "LLM cache misses by layer.",
	}, []string{"layer"})
)

// CachedProvider 包装 inner Provider,加 L1 精确缓存(Redis L2)。
// store 依赖 redis.Cmdable 接口(go-redis v9 commands.go:173 真实存在,ClusterClient 与 Client 均实现),
// 便于测试注入 miniredis 的 *redis.Client;生产注入 *redis.ClusterClient。
// store 为 nil 时降级为直穿(仍实现接口,日志 warn)。
// model 用于缓存键隔离(不同模型回复不可混用)。
type CachedProvider struct {
	inner   Provider
	store   redis.Cmdable
	model   string
	enabled bool
}

// NewCached 构造带缓存的 Provider 装饰器。
// store 为 nil 时降级为直穿(仍实现接口,日志 warn)。
// model 用于缓存键隔离(不同模型回复不可混用)。
func NewCached(inner Provider, store redis.Cmdable, model string) *CachedProvider {
	enabled := os.Getenv("ESP_LLM_CACHE_ENABLED") != "false" // 默认 true
	if !enabled {
		log.Printf("[llm-cache] disabled by ESP_LLM_CACHE_ENABLED=false, pass-through")
	}
	if store == nil && enabled {
		log.Printf("[llm-cache] enabled but store nil, will pass-through on miss")
	}
	return &CachedProvider{inner: inner, store: store, model: model, enabled: enabled}
}

// Chat 实现 Provider 接口:精确缓存查询 → MISS 委托 inner → 回写。
func (c *CachedProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	// 降级:缓存禁用 或 store 缺失 → 直穿 inner
	if !c.enabled || c.store == nil {
		return c.inner.Chat(ctx, messages)
	}

	key := c.cacheKey(messages)

	// 1) 查缓存
	if raw, err := c.store.Get(ctx, key).Result(); err == nil {
		cacheHitsTotal.WithLabelValues("exact").Inc()
		log.Printf("[llm-cache] HIT key=%s", key)
		return raw, nil
	} else if !redisErrIsNil(err) {
		// 非 Nil 错误(网络/集群问题)降级 warn,不阻断
		log.Printf("[llm-cache] GET warn key=%s err=%v, pass-through", key, err)
	}

	// 2) MISS → 委托 inner
	cacheMissesTotal.WithLabelValues("exact").Inc()
	reply, err := c.inner.Chat(ctx, messages)
	if err != nil {
		return "", err
	}

	// 3) 回写 SETEX(失败不阻断,下次仍可重试回写)
	if sErr := c.store.Set(ctx, key, reply, cacheTTL).Err(); sErr != nil {
		log.Printf("[llm-cache] SET warn key=%s err=%v (non-fatal)", key, sErr)
	}
	return reply, nil
}

// cacheKey 构造规范化后的缓存键:esp:cache:llm:exact:{model}:{sha256前16位}。
// 规范化:每条 message trim 首尾空白后拼接 role|content,作为 SHA256 输入。
func (c *CachedProvider) cacheKey(messages []Message) string {
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(m.Role)
		sb.WriteByte('|')
		sb.WriteString(strings.TrimSpace(m.Content))
		sb.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return fmt.Sprintf("%s%s:%s", cacheKeyPrefix, c.model, hex.EncodeToString(sum[:])[:cacheShaLen])
}

// redisErrIsNil 判断 Redis 错误是否为 key 不存在(正常 MISS),go-redis v9 用 redis.Nil。
func redisErrIsNil(err error) bool {
	return err == redis.Nil
}
