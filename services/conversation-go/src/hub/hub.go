// Package hub 提供 WebSocket 连接的会话级路由与跨 Pod 广播。
//
// 设计(P2-1,按 session_id hash 分片广播):
//   - 每个 Pod 维护本地连接注册表 conns,key=sessionID → set(*Conn)。
//     一个 session 可能有多个连接(多端同步:Web + H5 + 小程序同 session),故用 set。
//   - 跨 Pod 广播经 Redis Pub/Sub:channel = shardChannel(sessionID) = "ws:shard:{hash(sessionID)%N}"。
//     每个 Pod 启动时订阅全部 N 个 shard channel(因为任意 Pod 都可能持有该 session 的连接,
//     且 HPA 扩缩容时副本数变化;订阅全部分片比"按副本数取模"更稳定,分片只为降低单 channel 消息量)。
//   - 分片数 N 固定(WS_SHARD_COUNT,默认 8),与副本数解耦:
//     增减副本不改变 channel 数,避免 rebalance 风暴;消息量按 session 均匀分散到 N 个 channel,
//     每个 Pod 只处理自己关心的 session(本地 conns 命中才写 conn),未命中即丢弃。
//   - 发广播:Publish(sessionID, frame) → PUBLISH 到 shard channel,所有 Pod 收到后查本地 conns 命中则写。
//   - 与单连接回写 writeConn 共存:点对点响应仍走 writeConn(低延迟),跨端/跨 Pod 同步走 Hub.Broadcast。
package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"esp/conversation-go/src/ws"
)

// ShardCount 分片 channel 数(与副本数解耦,固定值)。
// 通过环境变量 WS_SHARD_COUNT 覆盖;变更需滚动重启所有 Pod。
const defaultShardCount = 8

// Hub 管理本地 WS 连接注册表 + Redis Pub/Sub 分片广播。
type Hub struct {
	rdb        redis.UniversalClient
	shardCount int

	mu    sync.RWMutex
	conns map[string]map[*websocket.Conn]struct{} // sessionID → conns set

	pubsub *redis.PubSub
}

// New 创建 Hub 并启动 N 个 shard channel 的订阅。
func New(rdb redis.UniversalClient, shardCount int) *Hub {
	if shardCount <= 0 {
		shardCount = defaultShardCount
	}
	h := &Hub{
		rdb:        rdb,
		shardCount: shardCount,
		conns:      make(map[string]map[*websocket.Conn]struct{}),
	}
	return h
}

// Start 订阅全部分片 channel,后台 goroutine 消费广播消息派发到本地连接。
// 调用方应在独立 goroutine 中运行;ctx 取消时停止。
func (h *Hub) Start(ctx context.Context) {
	channels := make([]string, h.shardCount)
	for i := 0; i < h.shardCount; i++ {
		channels[i] = shardChannel(i, h.shardCount)
	}
	h.pubsub = h.rdb.Subscribe(ctx, channels...)
	go h.dispatch(ctx)
	slog.Info("ws hub subscribed shards", "count", h.shardCount)
}

// dispatch 消费 Pub/Sub 消息,解析后派发到本地命中的连接。
func (h *Hub) dispatch(ctx context.Context) {
	defer h.pubsub.Close()
	ch := h.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var m message
			if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
				slog.Warn("ws hub: bad broadcast payload", "err", err)
				continue
			}
			h.sendLocal(m.SessionID, m.Frame)
		}
	}
}

// Register 把 conn 注册到 sessionID 的本地连接集。返回反注册函数(defer 调用)。
func (h *Hub) Register(sessionID string, conn *websocket.Conn) func() {
	h.mu.Lock()
	if h.conns[sessionID] == nil {
		h.conns[sessionID] = make(map[*websocket.Conn]struct{})
	}
	h.conns[sessionID][conn] = struct{}{}
	h.mu.Unlock()
	return func() { h.Unregister(sessionID, conn) }
}

// Unregister 移除连接;session 无连接时清空条目。
func (h *Hub) Unregister(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.conns[sessionID]; ok {
		delete(set, conn)
		if len(set) == 0 {
			delete(h.conns, sessionID)
		}
	}
}

// Broadcast 发布帧到该 session 的分片 channel,所有持有该 session 连接的 Pod 收到后本地派发。
// 用于跨端/跨 Pod 同步(如多端同 session、教师旁听、情绪推送)。
func (h *Hub) Broadcast(ctx context.Context, sessionID string, frame ws.Frame) error {
	payload, err := json.Marshal(message{SessionID: sessionID, Frame: frame})
	if err != nil {
		return err
	}
	return h.rdb.Publish(ctx, shardChannelFor(sessionID, h.shardCount), payload).Err()
}

// sendLocal 向本 Pod 内 sessionID 的所有连接写帧(非阻塞派发,写失败静默跳过)。
func (h *Hub) sendLocal(sessionID string, frame ws.Frame) {
	b, err := frame.Encode()
	if err != nil {
		return
	}
	h.mu.RLock()
	set := h.conns[sessionID]
	conns := make([]*websocket.Conn, 0, len(set))
	for c := range set {
		conns = append(conns, c)
	}
	h.mu.RUnlock()
	for _, c := range conns {
		if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
			slog.Debug("ws hub: write to conn failed (will unregister on close)", "err", err)
		}
	}
}

// message 是 Pub/Sub 上的广播消息信封。
type message struct {
	SessionID string   `json:"session_id"`
	Frame     ws.Frame `json:"frame"`
}

// shardChannelFor 返回 sessionID 哈希后对应的分片 channel 名。
func shardChannelFor(sessionID string, n int) string {
	return shardChannel(int(fnv1a(sessionID)%uint32(n)), n)
}

func shardChannel(i int, n int) string {
	return fmt.Sprintf("ws:shard:%d", i)
}

// fnv1a 32-bit 哈希(快、分布均匀,无需 crypto)。
func fnv1a(s string) uint32 {
	const (
		offsetBasis32 = 2166136261
		prime32       = 16777619
	)
	h := uint32(offsetBasis32)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	return h
}
