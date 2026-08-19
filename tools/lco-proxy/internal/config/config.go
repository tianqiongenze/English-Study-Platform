// Package config — LCO 代理配置(全 env 驱动,零硬编码)
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config LCO 代理运行配置。
type Config struct {
	// LCO 代理监听地址(本机工具接入点:Claude Code ANTHROPIC_BASE_URL 指向此)
	Listen string

	// 上游真实 LLM 代理(CC Switch,默认 127.0.0.1:15721)
	UpstreamURL string

	// L1 精确缓存(Redis,经 port-forward 到集群 redis-cluster-new)
	RedisAddr     string
	RedisPassword string
	CacheEnabled  bool
	// TTL 分级(秒):chat=1h, code=24h, knowledge=7d
	TTLChat     int
	TTLCode     int
	TTLKnowledge int

	// L2 语义缓存(embedding + Milvus)
	EmbeddingURL    string // 宿主 python venv,127.0.0.1:15730/embed
	MilvusAddr      string // 127.0.0.1:19531(port-forward)
	L2Enabled       bool
	L2Threshold     float64 // cosine 相似度阈值,默认 0.92
	L2Collection    string

	// L3 知识图谱(Neo4j,经 port-forward)
	Neo4jURL      string // bolt://127.0.0.1:7688
	Neo4jUser     string
	Neo4jPassword string
	L3Enabled     bool

	// 指标
	MetricsEnabled bool
}

// Load 从 env 加载配置,缺失项用安全默认值。
func Load() (*Config, error) {
	c := &Config{
		Listen:        getenv("LCO_LISTEN", "127.0.0.1:15720"),
		UpstreamURL:   strings.TrimRight(getenv("LCO_UPSTREAM_URL", "http://127.0.0.1:15721"), "/"),
		RedisAddr:     getenv("LCO_REDIS_ADDR", "127.0.0.1:6380"),
		RedisPassword: getenv("LCO_REDIS_PASSWORD", ""),
		CacheEnabled:  getenv("LCO_CACHE_ENABLED", "true") == "true",
		TTLChat:       getenvInt("LCO_TTL_CHAT", 3600),
		TTLCode:       getenvInt("LCO_TTL_CODE", 86400),
		TTLKnowledge:  getenvInt("LCO_TTL_KNOWLEDGE", 604800),
		EmbeddingURL:  getenv("LCO_EMBEDDING_URL", "http://127.0.0.1:15730/embed"),
		MilvusAddr:    getenv("LCO_MILVUS_ADDR", "127.0.0.1:19531"),
		L2Enabled:     getenv("LCO_L2_ENABLED", "true") == "true",
		L2Threshold:   getenvFloat("LCO_L2_THRESHOLD", 0.92),
		L2Collection:  getenv("LCO_L2_COLLECTION", "lco_prompts"),
		Neo4jURL:      getenv("LCO_NEO4J_URL", "bolt://127.0.0.1:7688"),
		Neo4jUser:     getenv("LCO_NEO4J_USER", "neo4j"),
		Neo4jPassword: getenv("LCO_NEO4J_PASSWORD", ""),
		L3Enabled:     getenv("LCO_L3_ENABLED", "true") == "true",
		MetricsEnabled: getenv("LCO_METRICS_ENABLED", "true") == "true",
	}
	if c.UpstreamURL == "" {
		return nil, fmt.Errorf("LCO_UPSTREAM_URL 不能为空")
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
