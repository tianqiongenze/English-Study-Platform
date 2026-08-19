// Package cache — L1 Redis 多 pod 连接池(MOVED 重路由)
//
// 解决 Redis Cluster + port-forward 的固有矛盾:
// - ClusterClient CLUSTER SLOTS 拿到 FQDN(集群内 svc),宿主 DNS 解析不了
// - 单节点 Client 遇 MOVED 不重试,命中率 ~33%(仅连的 pod 主辖 slot 命中)
//
// 方案:维护 6 个单节点 Client(port-forward 6380-6385),捕获 MOVED 错误,
// 从 FQDN 提取 pod 索引,重试到对应 Client。命中率 → 100%。
package cache

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// L1Pool 6 pod 单节点 Client 池 + MOVED 重路由。
type L1Pool struct {
	clients []*redis.Client // index 0-5 对应 redis-cluster-new-0..5
}

// NewL1Pool 从逗号分隔的 port-forward 地址构造池。
// 期望格式:127.0.0.1:6380,127.0.0.1:6381,...,127.0.0.1:6385
func NewL1Pool(addrs string, password string) *L1Pool {
	parts := strings.Split(addrs, ",")
	clients := make([]*redis.Client, len(parts))
	for i, a := range parts {
		a = strings.TrimSpace(a)
		clients[i] = redis.NewClient(&redis.Options{
			Addr:         a,
			Password:     password,
			DB:           0,
			PoolSize:     4,
			MinIdleConns: 1,
		})
	}
	return &L1Pool{clients: clients}
}

// Get 优先用 pod-0,遇 MOVED 重路由到正确 pod。
func (p *L1Pool) Get(ctx context.Context, key string) (string, error) {
	return p.withMovedRetry(ctx, key, func(c *redis.Client) (string, error) {
		return c.Get(ctx, key).Result()
	})
}

// Set 优先用 pod-0,遇 MOVED 重路由。
func (p *L1Pool) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	_, err := p.withMovedRetry(ctx, key, func(c *redis.Client) (string, error) {
		return c.Set(ctx, key, value, ttl).Result()
	})
	return err
}

// withMovedRetry 执行操作,遇 MOVED 解析 FQDN 提取 pod 索引重试。
func (p *L1Pool) withMovedRetry(ctx context.Context, key string, fn func(*redis.Client) (string, error)) (string, error) {
	// 先尝试 pod-0(或任意可用)
	for startIdx := 0; startIdx < len(p.clients); startIdx++ {
		c := p.clients[startIdx]
		val, err := fn(c)
		if err == nil {
			return val, nil
		}
		// 检查是否 MOVED
		if moved, ok := parseMoved(err); ok {
			// 从 FQDN 提取 pod 索引
			podIdx := extractPodIndex(moved.host)
			if podIdx >= 0 && podIdx < len(p.clients) && podIdx != startIdx {
				// 重试到正确 pod
				val2, err2 := fn(p.clients[podIdx])
				if err2 == nil {
					return val2, nil
				}
			}
		}
		// 非 MOVED 或重试失败:继续尝试下一个 pod(尽力而为)
		if startIdx == len(p.clients)-1 {
			return "", err
		}
	}
	return "", fmt.Errorf("no client available")
}

// parseMoved 从 go-redis 错误提取 MOVED 信息。
// go-redis 遇 MOVED 时 err 是 *redis.Nil 或 *redis.MovedError(v9 有 redis.MovedError)。
// 兼容:string 错误含 "MOVED <slot> <host:port>"。
func parseMoved(err error) (struct{ slot, host string }, bool) {
	if err == nil {
		return struct{ slot, host string }{}, false
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "MOVED ") {
		return struct{ slot, host string }{}, false
	}
	// MOVED <slot> <host:port>
	parts := strings.Fields(msg)
	if len(parts) < 3 {
		return struct{ slot, host string }{}, false
	}
	host := parts[2]
	// 去掉端口
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return struct{ slot, host string }{parts[1], host}, true
}

// extractPodIndex 从 FQDN(如 redis-cluster-new-3.redis-cluster-new...)提取 pod 索引 3。
func extractPodIndex(fqdn string) int {
	// 找 redis-cluster-new-<N> 模式
	prefix := "redis-cluster-new-"
	idx := strings.Index(fqdn, prefix)
	if idx < 0 {
		return -1
	}
	rest := fqdn[idx+len(prefix):]
	// 提取数字
	var numStr strings.Builder
	for _, ch := range rest {
		if ch >= '0' && ch <= '9' {
			numStr.WriteRune(ch)
		} else {
			break
		}
	}
	if numStr.Len() == 0 {
		return -1
	}
	n, err := strconv.Atoi(numStr.String())
	if err != nil || n < 0 {
		return -1
	}
	return n
}

// Ping 健康检查(ping 所有 client,任一通即 ok)。
func (p *L1Pool) Ping(ctx context.Context) error {
	for _, c := range p.clients {
		if err := c.Ping(ctx).Err(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("all redis clients down")
}

// Close 关闭所有 client。
func (p *L1Pool) Close() {
	for _, c := range p.clients {
		_ = c.Close()
	}
}
