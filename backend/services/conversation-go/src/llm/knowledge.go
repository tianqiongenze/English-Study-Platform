// Package llm 的 L3 知识图谱(Phase3,Neo4j Prompt→Response 关系)。
//
// KnowledgeGraph 记录 LLM 问答对到 Neo4j 知识图谱,供后续 Enhance 做上下文增强(替代文件搜索)。
// 节点/关系:(:Prompt{sha16,text,model,scene,ts})-[:ANSWERED]->(:Response{sha16,text,model,ts})
// 唯一约束:Prompt.sha16 UNIQUE(init 时 CREATE CONSTRAINT IF NOT EXISTS)。
//
// 与 L1/L2 正交:L1 exact / L2 semantic 省调用;L3 是知识沉淀(供 Enhance 增强未来 prompt 的上下文),
//   Record 在 inner.Chat 成功后异步调用(失败降级 skip,不阻断主链路)。
// Enhance 当前最小实现:按同 scene 找最近 Prompt→Response 返回作为 context(可选,本阶段先实现 Record)。
//
// 降级铁律(CLC+LCO):Neo4j 不可达时 warn+skip;nil receiver/nil driver 必须安全返回,不 panic。
// 端点从 env 读:NEO4J_URL(默认 bolt://neo4j:7687)、NEO4J_USER(默认 neo4j)、
//   NEO4J_PASSWORD(默认 neo4j-password-1 —— 单节点 dev 默认,生产须 env/secret 注入,见 deploy YAML)。
// 开关:ESP_LLM_L3_ENABLED(默认 true),false 时 Record/Enhance 直接 no-op。
//
// ASV 实测 neo4j-go-driver v5.28.4(2026-08-19,查源码,非记忆臆造):
//   - NewDriverWithContext(target string, auth auth.TokenManager, configurers ...func(*Config)) (DriverWithContext, error)
//     neo4j.BasicAuth(user,pass,realm) 返回 AuthToken = auth.Token,满足 auth.TokenManager 接口
//     (经 deprecated NewDriver 委托 NewDriverWithContext(target, auth,...) 验证)。
//   - DriverWithContext.NewSession(ctx, neo4j.SessionConfig{DatabaseName:"neo4j"}) SessionWithContext
//   - SessionWithContext.Run(ctx, cypher, params) (ResultWithContext, error) —— 最简单路径,不用 ExecuteWrite 闭包
//   - ResultWithContext.Consume(ctx) (ResultSummary, error) —— 执行并释放(写语句用 Consume)
//   - SessionWithContext.Close(ctx) error
//   注:public config 不含 WithConnectionPoolSize/WithMaxConnectionLifetime(只有 WithTxTimeout/WithTxMetadata),
//     不臆造不存在的 configurer(ASV 铁律:禁止凭记忆写 API)。
package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// L3 知识图谱配置常量。
const (
	l3DefaultURL      = "bolt://neo4j:7687"
	l3DefaultUser     = "neo4j"
	l3DefaultPassword = "neo4j-password-1" // 单节点 dev 默认,生产走 env/secret 注入
	l3QueryTimeout    = 3 * time.Second
	l3Database        = "neo4j"
	l3ShaLen          = 16
)

// KnowledgeGraph 是 L3 知识图谱(Neo4j Prompt→Response)。
// driver 为 nil(降级模式)时所有方法安全 no-op。
// enabled=false(ESP_LLM_L3_ENABLED=false)时 Record/Enhance 短路。
// constraintOK 标记唯一约束是否已建(幂等,首次 Record 时建)。
type KnowledgeGraph struct {
	driver       neo4j.DriverWithContext // nil = 降级
	enabled      bool
	constraintOK bool
}

// NewKnowledgeGraph 构造 L3 知识图谱装饰器。
// driver 为 nil 时降级(Record/Enhance no-op,不 panic)。
// env NEO4J_URL / NEO4J_USER / NEO4J_PASSWORD 可覆盖默认端点与凭据。
// 连接失败返回 nil driver(降级),不 panic(CLC)。
func NewKnowledgeGraph() *KnowledgeGraph {
	enabled := os.Getenv("ESP_LLM_L3_ENABLED") != "false" // 默认 true
	if !enabled {
		log.Printf("[llm-cache] L3 disabled by ESP_LLM_L3_ENABLED=false")
		return &KnowledgeGraph{driver: nil, enabled: false}
	}
	url := os.Getenv("NEO4J_URL")
	if url == "" {
		url = l3DefaultURL
	}
	user := os.Getenv("NEO4J_USER")
	if user == "" {
		user = l3DefaultUser
	}
	pass := os.Getenv("NEO4J_PASSWORD")
	if pass == "" {
		pass = l3DefaultPassword
	}
	driver, err := neo4j.NewDriverWithContext(url, neo4j.BasicAuth(user, pass, ""))
	if err != nil {
		log.Printf("[llm-cache] L3 Neo4j driver init fail (degrade): %v", err)
		return &KnowledgeGraph{driver: nil, enabled: true}
	}
	log.Printf("[llm-cache] L3 Neo4j driver init OK: %s", url)
	return &KnowledgeGraph{driver: driver, enabled: true}
}

// Record 把 (prompt,response) 沉淀到 Neo4j 知识图谱。
// MERGE Prompt(sha16) + MERGE Response(sha16) + MERGE (Prompt)-[:ANSWERED]->(Response)。
// 失败 warn+skip,不阻断主链路。nil receiver/nil driver 安全 no-op(CLC)。
func (k *KnowledgeGraph) Record(ctx context.Context, prompt, response, model, scene string) {
	if k == nil || !k.enabled || k.driver == nil {
		return
	}
	if prompt == "" || response == "" {
		return
	}
	pSha := sha16Hex(prompt)
	rSha := sha16Hex(response)
	ts := time.Now().Unix()
	cypher := `MERGE (p:Prompt {sha16: $psha})
SET p.text = $text, p.model = $model, p.scene = $scene, p.ts = $ts
MERGE (r:Response {sha16: $rsha})
SET r.text = $rtext, r.model = $model, r.ts = $ts
MERGE (p)-[:ANSWERED]->(r)`
	params := map[string]any{
		"psha":  pSha,
		"rsha":  rSha,
		"text":  prompt,
		"rtext": response,
		"model": model,
		"scene": scene,
		"ts":    ts,
	}
	if err := k.runWrite(ctx, cypher, params); err != nil {
		log.Printf("[llm-cache] L3 Record skip (non-fatal): %v", err)
	}
}

// Enhance 按当前 prompt 从知识图谱取相关上下文(同 scene 最近一条 Prompt→Response)做增强。
// 本阶段最小实现:OPTIONAL MATCH 找同 scene 的 Prompt + 其 Response,拼成 context 字符串。
// 无命中/失败返回 ("", false)。nil-safe。
func (k *KnowledgeGraph) Enhance(ctx context.Context, prompt, scene string) (string, bool) {
	if k == nil || !k.enabled || k.driver == nil {
		return "", false
	}
	cypher := `MATCH (p:Prompt)-[:ANSWERED]->(r:Response)
WHERE p.scene = $scene
RETURN p.text AS ptext, r.text AS rtext
ORDER BY p.ts DESC
LIMIT 1`
	params := map[string]any{"scene": scene}
	ptext, rtext, ok, err := k.readPair(ctx, cypher, params)
	if err != nil {
		log.Printf("[llm-cache] L3 Enhance skip (non-fatal): %v", err)
		return "", false
	}
	if !ok || ptext == "" || rtext == "" {
		return "", false
	}
	return fmt.Sprintf("[prior Q] %s\n[prior A] %s", ptext, rtext), true
}

// Close 关闭 driver(进程退出时调)。nil-safe。
func (k *KnowledgeGraph) Close(ctx context.Context) {
	if k == nil || k.driver == nil {
		return
	}
	_ = k.driver.Close(ctx)
}

// ensureConstraint 幂等建唯一约束(Prompt.sha16 UNIQUE)。首次 Record 前调。
// 注意:此方法直接用 rawRun 而非 runWrite,避免 runWrite→ensureConstraint→runWrite 无限递归(CLC 铁律:追踪执行路径,防栈溢出)。
func (k *KnowledgeGraph) ensureConstraint(ctx context.Context) error {
	if k.constraintOK {
		return nil
	}
	cypher := `CREATE CONSTRAINT prompt_sha16 IF NOT EXISTS FOR (p:Prompt) REQUIRE p.sha16 IS UNIQUE`
	if err := k.rawRun(ctx, cypher, nil); err != nil {
		return err
	}
	log.Printf("[llm-cache] L3 Neo4j constraint ensured: Prompt.sha16 UNIQUE")
	k.constraintOK = true
	return nil
}

// runWrite 执行写 Cypher(auto-commit,Run+Consume)。建约束也用此路径。
// 幂等建约束在每次写前 ensure(首次)。
func (k *KnowledgeGraph) runWrite(ctx context.Context, cypher string, params map[string]any) error {
	if k == nil || k.driver == nil {
		return fmt.Errorf("driver nil")
	}
	// 非建约束语句前确保约束已建(Record 路径)
	if !k.constraintOK {
		if err := k.ensureConstraint(ctx); err != nil {
			// 约束建失败不阻断写(降级,继续写)
			log.Printf("[llm-cache] L3 constraint ensure warn (continue): %v", err)
			k.constraintOK = true // 标记避免反复重试
		}
	}
	return k.rawRun(ctx, cypher, params)
}

// rawRun 执行写 Cypher(auto-commit,Run+Consume),不做约束检查(避免递归)。
// ensureConstraint 与 runWrite 共用此原语。
func (k *KnowledgeGraph) rawRun(ctx context.Context, cypher string, params map[string]any) error {
	if k == nil || k.driver == nil {
		return fmt.Errorf("driver nil")
	}
	qCtx, cancel := context.WithTimeout(ctx, l3QueryTimeout)
	defer cancel()
	sess := k.driver.NewSession(qCtx, neo4j.SessionConfig{DatabaseName: l3Database})
	defer sess.Close(qCtx)
	result, err := sess.Run(qCtx, cypher, params)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if _, err := result.Consume(qCtx); err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	return nil
}

// readPair 执行读 Cypher,在 session 存活期间取首条 Record 的 ptext/rtext 两个字段。
// 返回 (ptext, rtext, hasRow, error)。nil-safe:driver nil 返回 ("","",false,nil)。
// 注意:必须在 sess.Close 前 Single+取值(ResultWithContext 在 session 关闭后不可用)。
func (k *KnowledgeGraph) readPair(ctx context.Context, cypher string, params map[string]any) (string, string, bool, error) {
	if k == nil || k.driver == nil {
		return "", "", false, nil
	}
	qCtx, cancel := context.WithTimeout(ctx, l3QueryTimeout)
	defer cancel()
	sess := k.driver.NewSession(qCtx, neo4j.SessionConfig{DatabaseName: l3Database})
	defer sess.Close(qCtx)
	result, err := sess.Run(qCtx, cypher, params)
	if err != nil {
		return "", "", false, fmt.Errorf("run: %w", err)
	}
	rec, err := result.Single(qCtx)
	if err != nil {
		// 无行或错误:无命中(不视为致命,Enhance 返回 false)
		return "", "", false, nil
	}
	pt, _ := rec.Get("ptext")
	rt, _ := rec.Get("rtext")
	ps, _ := pt.(string)
	rs, _ := rt.(string)
	return ps, rs, true, nil
}

// sha16Hex 返回 SHA256 前 16 位 hex(Neo4j Prompt/Response 主键 sha16)。
func sha16Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:l3ShaLen]
}
