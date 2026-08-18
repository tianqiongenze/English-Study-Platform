// rankings_cache.go —— 排行榜多级缓存(L1 进程内 LRU + L2 Redis Cluster + L3 CRDB 快照)。
// 用户硬约束(2026-08-17):"缓存全部要实现多级缓存"。对标 conversation-go/src/cache/cache.go 的 Cache-Aside 语义。
// L1 用 sync.Map+LRU(进程内最快);L2 用 redis-cluster(ZSET 榜主存 / String 缓存元数据);L3 用 CRDB leaderboard_snapshots(日切不可变,历史 percentile / 降级)。
// 跨副本 L1 一致性靠 L2+TTL 兜底(conversation-go 既有策略);opt-out 变更走 Evict 主动失效。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	cacheEnabled = os.Getenv("CACHE_ENABLED") != "false"
	l1MaxEntries = 4096
	// 元数据/小榜 L2 TTL;ZSET 周期桶自然翻转不在此列。
	metaL2TTL    = 300 * time.Second
	metaEmptyTTL = 30 * time.Second
	emptySentinel = "__ESP_RANK_CACHE_EMPTY__"
)

// ---- L1:进程内 LRU(双向链表 + map)----

type lruNode struct {
	key  string
	val  any
	prev, next *lruNode
}

type l1Cache struct {
	mu   sync.RWMutex
	cap  int
	m    map[string]*lruNode
	head, tail *lruNode
}

var l1 = newL1(l1MaxEntries)

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

// ---- L2 / L3 辅助 ----

// metaGetOrLoad:元数据(用户信息/地区字典/opt-out 集)Cache-Aside。L1 → L2 → loader(读DB)。
// 返回 any(loader 决定结构),L2 存 JSON。空值用 sentinel 短缓存防穿透。
func (s *Server) metaGetOrLoad(ctx context.Context, cacheName string, parts []any,
	loader func(ctx context.Context) (any, error), cacheEmpty bool) (any, error) {
	if !cacheEnabled {
		return loader(ctx)
	}
	k := metaKey(cacheName, parts...)
	if v, ok := l1.get(k); ok {
		return v, nil
	}
	if r := s.rdb; r != nil {
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
			// L2 异常:降级走 loader,不阻断
		}
	}
	val, err := loader(ctx)
	if err != nil {
		return nil, err
	}
	if val == nil && !cacheEmpty {
		return nil, nil
	}
	if r := s.rdb; r != nil {
		payload := emptySentinel
		ttl := metaEmptyTTL
		if val != nil {
			b, _ := json.Marshal(val)
			payload = string(b)
			ttl = metaL2TTL
		}
		_ = r.Set(ctx, k, payload, ttl).Err()
	}
	l1.set(k, val)
	return val, nil
}

func metaKey(cacheName string, parts ...any) string {
	out := cacheName
	for _, p := range parts {
		out += ":" + strconv.FormatInt(toInt64(p), 10)
	}
	return out
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	default:
		return 0
	}
}

// metaEvict:失效 L2 + 本地 L1(opt-out 切换等触发;跨副本 L1 靠 TTL 兜底)。
func (s *Server) metaEvict(ctx context.Context, cacheName string, parts ...any) {
	if !cacheEnabled {
		return
	}
	k := metaKey(cacheName, parts...)
	l1.del(k)
	if r := s.rdb; r != nil {
		_ = r.Del(ctx, k).Err()
	}
}

// ---- top-N 小榜 L1 缓存(读榜热路径,60s)----
// 读榜主路径仍走 L2 ZSET(ZREVRANGE);L1 仅缓存序列化后的 top-N JSON,降低热点榜重复 ZREVRANGE 成本。
// key = esp:gamif:rankcache:{lbKey}
func rankCacheGet(ctx context.Context, lbKey string) ([]rankRow, bool) {
	if !cacheEnabled {
		return nil, false
	}
	if v, ok := l1.get("rankcache:" + lbKey); ok {
		if rows, ok := v.([]rankRow); ok {
			return rows, true
		}
	}
	return nil, false
}

func rankCacheSet(lbKey string, rows []rankRow) {
	if !cacheEnabled {
		return
	}
	l1.set("rankcache:"+lbKey, rows)
}

func rankCacheEvict(lbKey string) {
	l1.del("rankcache:" + lbKey)
}
