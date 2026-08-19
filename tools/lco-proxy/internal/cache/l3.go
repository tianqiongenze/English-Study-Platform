// Package cache — L3 知识图谱(Neo4j,上下文增强:prompt↔response↔code 关系)
package cache

import (
	"context"
	"encoding/json"
	"log/slog"

	"lco-proxy/internal/config"
	"lco-proxy/internal/protocol"
)

// L3Knowledge Neo4j 知识图谱层。
// MVP 实现:记录 prompt→response 关系(可选上下文增强)。
// L3 不阻断请求(查询失败=无增强,直穿),仅作为知识沉淀。
type L3Knowledge struct {
	cfg    *config.Config
	logger *slog.Logger
	// MVP:用 HTTP REST 调 Neo4j(避免重 Go driver 依赖)
	// Neo4j 5.x 暴露 /db/neo4j/tx/commit REST
}

func NewL3Knowledge(cfg *config.Config, logger *slog.Logger) *L3Knowledge {
	return &L3Knowledge{cfg: cfg, logger: logger}
}

// Record 异步记录 prompt→response 到知识图谱(非阻断)。
func (l *L3Knowledge) Record(ctx context.Context, key protocol.NormalizeKey, body []byte) {
	// MVP:只记日志,Neo4j 实际写入 DEFERRED(需建 schema + Cypher)
	// 避免 MVP 阶段 Neo4j 未初始化导致每请求报错
	l.logger.Debug("L3 记录(DEFERRED 实际写入)", "proto", key.Protocol, "model", key.Model)
	_ = body
}

// Enhance 上下文增强:查知识图谱找相关历史(DEFERRED)。
func (l *L3Knowledge) Enhance(ctx context.Context, key protocol.NormalizeKey) string {
	// DEFERRED:实际 Cypher 查询
	return ""
}

func (l *L3Knowledge) Health(ctx context.Context) string {
	// Neo4j bolt 端口探测(7688 port-forward)
	// MVP:标 ok(port-forward 已验证 7688 起来),实际连通性 DEFERRED
	return "ok-deferred"
}

// StatsJSON L3 统计(占位)。
func (l *L3Knowledge) StatsJSON() string {
	b, _ := json.Marshal(map[string]any{
		"status": "L3 Neo4j MVP(记录 DEFERRED,实际写入待 schema 建立)",
	})
	return string(b)
}
