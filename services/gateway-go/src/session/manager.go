// Package session 管理口语会话状态,持久化到 Redis,使网关无状态。
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"esp/gateway-go/src/db"
	"esp/gateway-go/src/pulsar"
	"github.com/redis/go-redis/v9"
)

// Session 是一次口语会话的内存态。
type Session struct {
	ID         string        `json:"id"`
	UserID     int64         `json:"user_id"`
	Scenario   string        `json:"scenario"`
	System     string        `json:"system"`
	Difficulty int           `json:"difficulty"`
	History    []Turn        `json:"history"`
	StartedAt  time.Time     `json:"started_at"`
	TTL        time.Duration `json:"-"`
}

// Turn 是一轮对话。
type Turn struct {
	Role    string `json:"role"` // user | assistant | system
	Content string `json:"content"`
}

// Manager 管理 Redis 中的会话。
type Manager struct {
	rdb    redis.UniversalClient
	prefix string
	ttl    time.Duration
}

func NewManager(rdb redis.UniversalClient, ttl time.Duration) *Manager {
	return &Manager{rdb: rdb, prefix: "session:", ttl: ttl}
}

func (m *Manager) key(id string) string { return m.prefix + id }

// Create 创建并持久化会话。
func (m *Manager) Create(ctx context.Context, s *Session) error {
	if s.TTL == 0 {
		s.TTL = m.ttl
	}
	s.StartedAt = time.Now()
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return m.rdb.Set(ctx, m.key(s.ID), data, s.TTL).Err()
}

// Get 读取会话。
func (m *Manager) Get(ctx context.Context, id string) (*Session, error) {
	data, err := m.rdb.Get(ctx, m.key(id)).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("session %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// AppendTurn 追加一轮并刷新 TTL。
func (m *Manager) AppendTurn(ctx context.Context, id string, t Turn) (*Session, error) {
	s, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.History = append(s.History, t)
	if len(s.History) > 50 { // 防止历史无限增长
		s.History = s.History[len(s.History)-50:]
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if err := m.rdb.Set(ctx, m.key(id), data, m.ttl).Err(); err != nil {
		return nil, err
	}
	return s, nil
}

// SessionSummary 是会话历史的归档摘要(从 TiDB esp_speaking.sessions 读)。
type SessionSummary struct {
	ID         string `json:"id"`
	UserID     int64  `json:"user_id"`
	Scenario   string `json:"scenario"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	EndedAt    string `json:"ended_at"`
}

// ListHistory 按 user_id 列出会话历史(从 TiDB esp_speaking.sessions 读从库,最近 N 条)。
// End 时归档写入 sessions 表(id BIGINT AUTO_RANDOM,started_at/ended_at/status),
// 此处读取供前端"会话历史"页展示。读从库 tidb-follower,forcedPrimary=false。
func ListHistory(ctx context.Context, userID int64, limit int) ([]SessionSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Read(ctx,
		"SELECT id, user_id, scenario_id, status, started_at, IFNULL(ended_at, '') FROM esp_speaking.sessions WHERE user_id = ? ORDER BY started_at DESC LIMIT ?",
		false, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionSummary
	for rows.Next() {
		var s SessionSummary
		var scenarioID int64
		if err := rows.Scan(&s.ID, &s.UserID, &scenarioID, &s.Status, &s.StartedAt, &s.EndedAt); err != nil {
			slog.Warn("session: ListHistory scan failed", "err", err)
			continue
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// End 结束会话(删除 Redis 键),并异步把会话记录持久化到 TiDB(写主库 tidb-primary)。
// TiDB 写失败只 warn 不阻断(会话状态已在 Redis,丢失只是无历史归档)。
func (m *Manager) End(ctx context.Context, id string) error {
	// 先读会话快照(用于归档),再删 Redis 键
	if sess, err := m.Get(ctx, id); err == nil && sess.UserID > 0 {
		// EDA: 发 session.ended 给 learning-java(归档/更新 mastery)。
		// 默认 build 为 no-op stub;启用 -tags pulsar_eda 后异步发 Pulsar,失败只 warn 不阻断 End。
		pulsar.PublishSessionEnded(map[string]any{
			"session_id": sess.ID,
			"user_id":    sess.UserID,
		})
		go func(s *Session) {
			archiveCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := db.Write(archiveCtx,
				"INSERT INTO sessions (id, user_id, scenario_id, status, started_at, ended_at) VALUES (?,?,?,?,?,NOW())",
				s.ID, s.UserID, 0, "ended", s.StartedAt)
			if err != nil {
				slog.Warn("session: archive to TiDB primary failed (non-fatal)", "id", s.ID, "err", err)
			}
		}(sess)
	}
	return m.rdb.Del(ctx, m.key(id)).Err()
}
