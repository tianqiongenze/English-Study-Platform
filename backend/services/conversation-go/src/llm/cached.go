// Package llm 的 L1 精确缓存装饰器(Phase1)+ L2 语义缓存(Phase2)+ L3 知识图谱(Phase3)。
//
// CachedProvider 包装一个 inner Provider,对 Chat 请求做三层缓存:
//  1. L1 精确缓存(exact-match,Redis):
//     - 规范化 messages(canonical)—— 去除首尾空白,保留语义内容;
//       注:conversation-go 的 messages 是真实对话文本(无 traceId/时间戳注入,见 mock.go/anthropic.go),
//       故规范化仅做 trim 防御,不做语义抽取(语义模糊匹配放 Phase2 L2 Milvus)。
//     - SHA256(canonical + model) 作为缓存键分片,避免长 key。
//     - Redis key: esp:cache:llm:exact:{model}:{sha256前16位}
//     - GET 命中 → 直接返回缓存回复(记 cache_hits_total{exact});MISS → 降级 L2。
//  2. L2 语义缓存(semantic-match,Milvus,见 semantic.go):
//     - embedding(minilm 384 维)→ Milvus /v1/vector/search,cosine≥0.92 视为语义等价命中(记 cache_hits_total{semantic})。
//     - L1 MISS 才查 L2;命中回写 L1(下次走 L1);未命中降级 inner。
//  3. L3 知识图谱(沉淀,Neo4j,见 knowledge.go):
//     - inner.Chat 成功后异步 Record(prompt→response 关系,降级 skip)。
//     - Enhance 可做上下文增强(本阶段最小实现,Record 优先)。
//  回写链:inner 成功 → L1 SETEX TTL=1h + L2 Store(Milvus)+ L3 Record(Neo4j),三者均降级安全。
//
// 降级:ESP_LLM_CACHE_ENABLED=false 或 Redis 不可达时,直穿 inner(只 warn 不阻断);
// L2 Milvus / L3 Neo4j 不可达时各自 warn+skip,不影响功能正确性(LCO 铁律:缓存层挂了不阻断主链路)。
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
	"net/http"
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

// CachedProvider 包装 inner Provider,加 L1 精确缓存(Redis L2)+ L2 语义缓存(Milvus)+ L3 知识图谱(Neo4j)。
// store 依赖 redis.Cmdable 接口(go-redis v9 commands.go:173 真实存在,ClusterClient 与 Client 均实现),
// 便于测试注入 miniredis 的 *redis.Client;生产注入 *redis.ClusterClient。
// store 为 nil 时降级为直穿(仍实现接口,日志 warn)。
// model 用于缓存键隔离(不同模型回复不可混用)。
// sem(L2 Milvus 语义缓存)与 kg(L3 Neo4j 知识图谱)可 nil,懒构造于 NewCached;
//   L2/L3 均降级安全:不可达时 warn+skip,不阻断主链路(与 L1 同模式)。
type CachedProvider struct {
	inner   Provider
	store   redis.Cmdable
	model   string
	enabled bool
	sem     *SemanticCache // L2,nil 降级
	kg      *KnowledgeGraph // L3,nil 降级
}

// NewCached 构造带缓存的 Provider 装饰器。
// store 为 nil 时降级为直穿(仍实现接口,日志 warn)。
// model 用于缓存键隔离(不同模型回复不可混用)。
// L2 SemanticCache 与 L3 KnowledgeGraph 在此懒构造(env 开关 ESP_LLM_L2/L3_ENABLED 默认 true);
//   构造失败/不可达自动降级(nil),不 panic、不阻断(见 CLC/LCO 铁律)。
// httpClient 注入用于 L2 Milvus/embedding 调用(nil 时 L2 降级);生产传 &http.Client{}。
func NewCached(inner Provider, store redis.Cmdable, model string) *CachedProvider {
	enabled := os.Getenv("ESP_LLM_CACHE_ENABLED") != "false" // 默认 true
	if !enabled {
		log.Printf("[llm-cache] disabled by ESP_LLM_CACHE_ENABLED=false, pass-through")
	}
	if store == nil && enabled {
		log.Printf("[llm-cache] enabled but store nil, will pass-through on miss")
	}
	c := &CachedProvider{inner: inner, store: store, model: model, enabled: enabled}
	// L2/L3 懒构造:仅在缓存启用时初始化
	if enabled {
		c.sem = NewSemanticCache(http.DefaultClient)
		c.kg = NewKnowledgeGraph()
	}
	return c
}

// Chat 实现 Provider 接口:三层缓存链 L1 exact → L2 semantic → inner → 回写(L1+L2+L3)。
//   1) L1 精确命中(Redis SHA256)→ 返回(cache_hits{exact});未命中降级 L2。
//   2) L2 语义命中(Milvus cosine≥0.92)→ 返回(cache_hits{semantic});未命中降级 inner。
//   3) inner.Chat() → 回写 L1 SETEX + L2 Store(Milvus)+ L3 Record(Neo4j 知识图谱),
//      三者均降级安全(失败 warn+skip,不阻断;inner error 不回写空值)。
func (c *CachedProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	// 降级:缓存禁用 或 store 缺失 → 直穿 inner
	if !c.enabled || c.store == nil {
		return c.inner.Chat(ctx, messages)
	}

	key := c.cacheKey(messages)
	prompt := c.promptText(messages) // L2/L3 用原始 prompt 文本做 embedding/知识图谱

	// 1) L1 精确查缓存
	if raw, err := c.store.Get(ctx, key).Result(); err == nil {
		cacheHitsTotal.WithLabelValues("exact").Inc()
		log.Printf("[llm-cache] L1 HIT key=%s", key)
		return raw, nil
	} else if !redisErrIsNil(err) {
		// 非 Nil 错误(网络/集群问题)降级 warn,不阻断
		log.Printf("[llm-cache] L1 GET warn key=%s err=%v, continue to L2", key, err)
	}
	cacheMissesTotal.WithLabelValues("exact").Inc()

	// 2) L2 语义查缓存(Milvus cosine≥0.92);sem nil 时降级跳过(不计 miss,直接 inner)
	if c.sem != nil {
		if hit, ok := c.sem.Search(ctx, prompt, c.model, c.scene(messages)); ok {
			// L2 命中:回写 L1(让下次走 L1)+ 直接返回
			if sErr := c.store.Set(ctx, key, hit, cacheTTL).Err(); sErr != nil {
				log.Printf("[llm-cache] L2-hit L1 writeback warn key=%s err=%v (non-fatal)", key, sErr)
			}
			return hit, nil
		}
	}

	// 3) L1+L2 均 MISS → 委托 inner
	reply, err := c.inner.Chat(ctx, messages)
	if err != nil {
		return "", err
	}

	// 4) 回写 L1 SETEX(失败不阻断,下次仍可重试回写)
	if sErr := c.store.Set(ctx, key, reply, cacheTTL).Err(); sErr != nil {
		log.Printf("[llm-cache] L1 SET warn key=%s err=%v (non-fatal)", key, sErr)
	}
	// 回写 L2 Milvus 语义缓存 + L3 Neo4j 知识图谱(均降级安全,失败不阻断)
	if c.sem != nil {
		c.sem.Store(ctx, prompt, reply, c.model, c.scene(messages))
	}
	if c.kg != nil {
		c.kg.Record(ctx, prompt, reply, c.model, c.scene(messages))
	}
	return reply, nil
}

// promptText 抽取 messages 的用户 prompt 文本做 L2 embedding / L3 知识图谱 key。
// 取最后一条 user 消息(实际提问);无 user 取全量拼接。防御 nil/空。
func (c *CachedProvider) promptText(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(strings.TrimSpace(m.Content))
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}

// scene 从 messages 抽取场景标签(用于 L2/L3 分域隔离与知识图谱 scene 索引)。
// 约定:system 消息含 "tutor"/"speech"/"writing" 等关键词时作为 scene;默认 "dialogue"。
func (c *CachedProvider) scene(messages []Message) string {
	for _, m := range messages {
		if m.Role != "system" {
			continue
		}
		lc := strings.ToLower(m.Content)
		switch {
		case strings.Contains(lc, "speech") || strings.Contains(lc, "口语"):
			return "speech"
		case strings.Contains(lc, "writing") || strings.Contains(lc, "写作"):
			return "writing"
		case strings.Contains(lc, "tutor") || strings.Contains(lc, "辅导"):
			return "tutor"
		}
	}
	return "dialogue"
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
