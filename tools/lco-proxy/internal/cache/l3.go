// Package cache — L3 知识图谱(Neo4j,上下文增强:prompt↔response 关系)
package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"lco-proxy/internal/config"
	"lco-proxy/internal/protocol"
)

// L3Knowledge Neo4j 知识图谱层。
// 2026-08-19 实装:Neo4j 5.x REST(/db/neo4j/tx/commit)写 Prompt→Response 关系 +
// 查语义相关历史(基于 prompt 文本模糊匹配,作为 L2 向量命中的图补充)。
// L3 不阻断请求(查询失败=无增强,直穿),仅作为知识沉淀。
type L3Knowledge struct {
	cfg    *config.Config
	logger *slog.Logger
	hc     *http.Client
}

func NewL3Knowledge(cfg *config.Config, logger *slog.Logger) *L3Knowledge {
	return &L3Knowledge{
		cfg:    cfg,
		logger: logger,
		hc:     &http.Client{Timeout: 5 * time.Second},
	}
}

// hashKey 生成 prompt 哈希(与 L1 l1Key 同源:sha256(key.Raw) 前 16 位)。
// CLC 铁律:之前 Record 引用未定义的 hashKey 导致编译失败——补齐。
func hashKey(key protocol.NormalizeKey) string {
	h := sha256.Sum256([]byte(key.Raw))
	return hex.EncodeToString(h[:])[:16]
}

// neo4jTx 执行 Cypher 事务(REST)。失败非致命,返回 error 由调用方降级。
func (l *L3Knowledge) neo4jTx(ctx context.Context, statements []map[string]any) (map[string]any, error) {
	// Neo4j REST 用 HTTP 端口(7474→port-forward 7475);bolt 端口给 driver 用,这里走 REST
	restURL := l.cfg.Neo4jURL
	// bolt://host:7688 → http://host:7474(REST 默认 http 端口 = bolt 端口 - 14)
	// 单节点 port-forward:7688 bolt / 7475 http,需通过 env 显式指定 REST URL
	if r := os.Getenv("LCO_NEO4J_REST_URL"); r != "" {
		restURL = r
	} else {
		// 兜底:bolt://127.0.0.1:7688 → http://127.0.0.1:7475
		restURL = strings.Replace(restURL, "bolt://", "http://", 1)
		restURL = strings.Replace(restURL, ":7688", ":7475", 1)
	}
	endpoint := strings.TrimRight(restURL, "/") + "/db/neo4j/tx/commit"

	body, _ := json.Marshal(map[string]any{"statements": statements})
	req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(l.cfg.Neo4jUser, l.cfg.Neo4jPassword)

	resp, err := l.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("neo4j rest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("neo4j rest status %d: %s", resp.StatusCode, string(rb))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if errs, ok := out["errors"].([]any); ok && len(errs) > 0 {
		return out, fmt.Errorf("neo4j cypher error: %v", errs)
	}
	return out, nil
}

// ensureSchema 幂等建约束 + 索引。启动时调一次(失败仅 warn,降级为无图增强)。
// CLC 铁律:nil 安全。
func (l *L3Knowledge) ensureSchema(ctx context.Context) {
	if l == nil || l.cfg == nil {
		return
	}
	stmts := []map[string]any{
		{"statement": "CREATE CONSTRAINT prompt_hash IF NOT EXISTS FOR (p:Prompt) REQUIRE p.hash IS UNIQUE"},
		{"statement": "CREATE INDEX prompt_text IF NOT EXISTS FOR (p:Prompt) ON (p.text)"},
		{"statement": "CREATE CONSTRAINT response_id IF NOT EXISTS FOR (r:Response) REQUIRE r.id IS UNIQUE"},
	}
	if _, err := l.neo4jTx(ctx, stmts); err != nil {
		l.logger.Warn("L3 Neo4j schema 建立失败(降级,非致命)", "err", err)
	}
}

// Record 异步记录 prompt→response 到知识图谱(非阻断)。
// CLC 铁律:nil 安全(l 或 cfg 为零值时不 panic);用调用方 ctx(继承超时/取消)。
func (l *L3Knowledge) Record(ctx context.Context, key protocol.NormalizeKey, body []byte) {
	if l == nil || l.cfg == nil {
		return
	}
	hash := hashKey(key)
	// 截断长文本避免 Cypher 语句过大
	promptText := truncate(key.Raw, 4096)
	respBody := truncate(string(body), 32768)
	respID := hash + "-" + truncate(key.Model, 32)

	stmt := map[string]any{
		"statement": `MERGE (p:Prompt {hash: $hash})
SET p.text = $text, p.protocol = $proto, p.model = $model, p.ts = $ts
MERGE (r:Response {id: $rid})
SET r.body = $body, r.ts = $ts
MERGE (p)-[:ANSWERED_BY]->(r)`,
		"parameters": map[string]any{
			"hash":  hash,
			"text":  promptText,
			"proto": string(key.Protocol),
			"model": truncate(key.Model, 128),
			"ts":    time.Now().Unix(),
			"rid":   respID,
			"body":  respBody,
		},
	}
	if _, err := l.neo4jTx(ctx, []map[string]any{stmt}); err != nil {
		l.logger.Warn("L3 记录失败(非致命)", "err", err)
	}
}

// Enhance 上下文增强:查知识图谱找相关历史 prompt(基于 text contains 模糊匹配)。
// 返回最近相关 response body(若有),作为 L2 未命中时的图补充。
// CLC 铁律:nil 安全;用调用方 ctx(继承超时/取消)。
func (l *L3Knowledge) Enhance(ctx context.Context, key protocol.NormalizeKey) string {
	if l == nil || l.cfg == nil {
		return ""
	}
	// 用 prompt 关键词片段查(取前 64 字符做 contains 匹配,避免全表扫)
	frag := truncate(strings.TrimSpace(key.Raw), 64)
	if frag == "" {
		return ""
	}
	stmt := map[string]any{
		"statement": `MATCH (p:Prompt)-[:ANSWERED_BY]->(r:Response)
WHERE p.text CONTAINS $frag AND p.protocol = $proto
RETURN r.body AS body, p.ts AS ts
ORDER BY p.ts DESC LIMIT 1`,
		"parameters": map[string]any{"frag": frag, "proto": string(key.Protocol)},
	}
	out, err := l.neo4jTx(ctx, []map[string]any{stmt})
	if err != nil {
		l.logger.Debug("L3 增强查询失败(非致命,无增强)", "err", err)
		return ""
	}
	results, _ := out["results"].([]any)
	if len(results) == 0 {
		return ""
	}
	first, _ := results[0].(map[string]any)
	data, _ := first["data"].([]any)
	if len(data) == 0 {
		return ""
	}
	row, _ := data[0].(map[string]any)
	rowArr, _ := row["row"].([]any)
	if len(rowArr) > 0 {
		if body, ok := rowArr[0].(string); ok {
			return body
		}
	}
	return ""
}

func (l *L3Knowledge) Health(ctx context.Context) string {
	// REST 探活:RETURN 1
	out, err := l.neo4jTx(ctx, []map[string]any{{"statement": "RETURN 1 AS v"}})
	if err != nil || out == nil {
		return "down"
	}
	if errs, _ := out["errors"].([]any); len(errs) > 0 {
		return "down"
	}
	return "ok"
}

// StatsJSON L3 统计。
func (l *L3Knowledge) StatsJSON() string {
	b, _ := json.Marshal(map[string]any{
		"status": "L3 Neo4j 实装(REST 写 Prompt→Response + 模糊查询增强)",
		"url":    l.cfg.Neo4jURL,
	})
	return string(b)
}

// truncate 截断字符串到 maxLen 字节(UTF-8 安全:按 rune 截)。
// CLC 铁律:参数名 maxLen 避免遮蔽 Go 1.21+ 内置 max(revive redefines-builtin-id)。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen])
}
