// Package llm 提供统一的 LLM Provider 抽象。
// 默认实现 Anthropic Claude;支持 mock 模式(LLM_MODE=mock)用于压测避免费用与限流。
// Phase1 新增 L1 精确缓存装饰器 CachedProvider(见 cached.go):NewWithCache 构造带缓存 Provider,
// New() 保持原语义(无缓存,供 dialog/engine_test.go 直接注入 MockProvider)。
package llm

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Message 是对话消息。
type Message struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

// Provider 是 LLM 提供方抽象。
type Provider interface {
	// Chat 非流式对话,返回完整回复。
	Chat(ctx context.Context, messages []Message) (string, error)
}

// New 根据环境变量构造 Provider。
// LLM_MODE=mock → MockProvider;否则 AnthropicProvider(需 ANTHROPIC_API_KEY)。
func New() Provider {
	if os.Getenv("LLM_MODE") == "mock" {
		return &MockProvider{}
	}
	return &AnthropicProvider{
		APIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		AuthToken: os.Getenv("ANTHROPIC_AUTH_TOKEN"),
		Model:     firstNonEmpty(os.Getenv("ANTHROPIC_MODEL"), "claude-sonnet-4-6"),
		BaseURL:   firstNonEmpty(os.Getenv("ANTHROPIC_BASE_URL"), "https://api.anthropic.com"),
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// llmRedisClient 是 L1 精确缓存专用的 Redis Cluster client(独立于 cache 包的 L2 client,
// 因语义/TTL 不同;但复用相同短名 redis-cluster 与集群拓扑)。
// 为 nil 表示 Redis 不可达,CachedProvider 自动降级直穿。
var (
	llmRedisOnce sync.Once
	llmRdb       *redis.ClusterClient
)

// newLLMRedisClient 构造 LLM 缓存专用 Redis Cluster client。
// 地址约定与 cache 包一致:REDIS_HOST(默认 redis-cluster)+ REDIS_PORT(默认 6379)。
// Ping 失败返回 nil(降级,不 panic)。
func newLLMRedisClient() *redis.ClusterClient {
	llmRedisOnce.Do(func() {
		host := os.Getenv("REDIS_HOST")
		if host == "" {
			host = "redis-cluster"
		}
		port := os.Getenv("REDIS_PORT")
		if port == "" {
			port = "6379"
		}
		cli := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:       []string{host + ":" + port},
			DialTimeout: time.Second,
			ReadTimeout: time.Second,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := cli.Ping(ctx).Err(); err != nil {
			// Redis 不可达:降级,nil store 触发 CachedProvider 直穿
			llmRdb = nil
			return
		}
		llmRdb = cli
	})
	return llmRdb
}

// NewWithCache 构造带 L1 精确缓存的 Provider(装饰 New() 产物)。
// 供 WS 入口/外部 wire 在注入 dialog.Engine.LLM 处包一层(零改 Engine)。
// 降级:ESP_LLM_CACHE_ENABLED=false 或 Redis 不可达 → 退化为 New() 的直穿 Provider(语义不变)。
func NewWithCache() Provider {
	inner := New()
	model := firstNonEmpty(os.Getenv("ANTHROPIC_MODEL"), "claude-sonnet-4-6")
	return NewCached(inner, newLLMRedisClient(), model)
}
