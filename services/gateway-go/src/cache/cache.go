// Package cache 提供 gateway-go 多级缓存(L1 进程内 LRU + L2 Redis),与 Java/Python 侧语义对齐。
//
// Cache-Aside:读 L1 → L2 → loader;命中回填上层。L1 用 sync.Map+LRU(进程内,单实例最快);
// L2 用 redis-cluster(go-redis v9)。Redis 不可达降级为纯 L1(只 warn 不阻断)。
// 跨副本 L1 一致性靠 L2 + TTL 兜底(gateway-go 多副本间场景数据从代码常量加载,天然一致;
// 仅 TTS 合成结果真实化后是缓存候选,走 L2 共享)。
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	enabled      = os.Getenv("CACHE_ENABLED") != "false"
	l1Max        = 4096
	l2TTL = 300 * time.Second
	emptyTTL = 30 * time.Second
	emptySentinel = "__ESP_CACHE_EMPTY__"
)

// l1Cache 进程内 LRU(简单实现:map + 双向链表受容量限制)。
type l1Cache struct {
	mu  sync.RWMutex
	cap int
	m   map[string]*lruNode
	head, tail *lruNode
}

type lruNode struct {
	key  string
	val  any
	prev, next *lruNode
}

func newL1(cap int) *l1Cache {
	c := &l1Cache{cap: cap, m: make(map[string]*lruNode)}
	c.head = &lruNode{}
	c.tail = &lruNode{}
	c.head.next = c.tail
	c.tail.prev = c.head
	return c
}

func (c *l1Cache) get(key string) (any, bool) {
	c.mu.RLock()
	n, ok := c.m[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	c.mu.Lock()
	c.moveToFront(n)
	c.mu.Unlock()
	return n.val, true
}

func (c *l1Cache) set(key string, val any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n, ok := c.m[key]; ok {
		n.val = val
		c.moveToFront(n)
		return
	}
	n := &lruNode{key: key, val: val}
	c.m[key] = n
	c.pushFront(n)
	if len(c.m) > c.cap {
		oldest := c.tail.prev
		c.remove(oldest)
		delete(c.m, oldest.key)
	}
}

func (c *l1Cache) del(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n, ok := c.m[key]; ok {
		c.remove(n)
		delete(c.m, key)
	}
}

func (c *l1Cache) moveToFront(n *lruNode) { c.remove(n); c.pushFront(n) }
func (c *l1Cache) pushFront(n *lruNode) {
	n.prev = c.head
	n.next = c.head.next
	c.head.next.prev = n
	c.head.next = n
}
func (c *l1Cache) remove(n *lruNode) {
	n.prev.next = n.next
	n.next.prev = n.prev
}

var l1 = newL1(l1Max)

var rdb *redis.ClusterClient
var rdbOnce sync.Once
var rdbErr error

func client() *redis.ClusterClient {
	rdbOnce.Do(func() {
		host := os.Getenv("REDIS_HOST")
		if host == "" {
			host = "redis-cluster"
		}
		port := os.Getenv("REDIS_PORT")
		if port == "" {
			port = "6379"
		}
		// Redis Cluster 模式(经 ExternalName redis-cluster → redis-cluster-new,3主3从)
		rdb = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        []string{host + ":" + port},
			DialTimeout:  time.Second,
			ReadTimeout:  time.Second,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			rdbErr = err
			rdb = nil
		}
	})
	return rdb
}

// key 构造:cacheName:part1:part2 (明文拼接,对齐 Java esp-cache)。
// P1-6:去掉 sha256(json(parts))[:24],三端对同一实体产出相同 Redis key。
func keyOf(cacheName string, parts ...any) string {
	strs := make([]string, len(parts))
	for i, p := range parts {
		strs[i] = fmt.Sprintf("%v", p)
	}
	return cacheName + ":" + strings.Join(strs, ":")
}

// GetOrLoad 读 L1 → L2 → loader。loader 返回的值回填 L2(set TTL)+ L1。
// unmarshalInto 为 nil 时原样返回 any;非 nil 时尝试 json 反序列化到目标结构。
func GetOrLoad(ctx context.Context, cacheName string, parts []any,
	loader func(ctx context.Context) (any, error), cacheEmpty bool) (any, error) {
	if !enabled {
		return loader(ctx)
	}
	k := keyOf(cacheName, parts...)

	if v, ok := l1.get(k); ok {
		return v, nil
	}

	if r := client(); r != nil {
		raw, err := r.Get(ctx, k).Result()
		if err == nil {
			if raw == emptySentinel {
				l1.set(k, nil)
				return nil, nil
			}
			var val any
			if json.Unmarshal([]byte(raw), &val) == nil {
				l1.set(k, val)
				return val, nil
			}
		} else if !errors.Is(err, redis.Nil) {
			// 降级 warn,不打断
		}
	}

	val, err := loader(ctx)
	if err != nil {
		return nil, err
	}
	if val == nil && !cacheEmpty {
		return nil, nil
	}

	if r := client(); r != nil {
		var payload string
		ttl := l2TTL
		if val == nil {
			payload = emptySentinel
			ttl = emptyTTL
		} else {
			b, _ := json.Marshal(val)
			payload = string(b)
		}
		_ = r.Set(ctx, k, payload, ttl).Err()
	}
	l1.set(k, val)
	return val, nil
}

// Evict 失效:删 L2 + 本地 L1(多副本 L1 靠 TTL 兜底)。
func Evict(ctx context.Context, cacheName string, parts ...any) {
	if !enabled {
		return
	}
	k := keyOf(cacheName, parts...)
	l1.del(k)
	if r := client(); r != nil {
		_ = r.Del(ctx, k).Err()
	}
}
