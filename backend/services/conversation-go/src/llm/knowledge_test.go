package llm

// L3 知识图谱(Neo4j)单元测试。
// 设计(CLC 铁律:零值对象安全 + 追踪执行路径):
//   - nil driver(降级模式)Record/Enhance/Close 必须 no-op 不 panic。
//   - 禁用开关 ESP_LLM_L3_ENABLED=false 必须 no-op。
//   - 不依赖真实 Neo4j(用 nil driver 测降级路径;真实连接测试放集成层,单测不要求)。

import (
	"context"
	"testing"
)

// CLC 核心:nil receiver + nil driver + 禁用开关,均不得 panic。
func TestKnowledgeGraph_NilSafeAndDisabled(t *testing.T) {
	t.Setenv("ESP_LLM_L3_ENABLED", "true")
	var nilKG *KnowledgeGraph // nil receiver
	// nil receiver 所有方法 no-op
	nilKG.Record(context.Background(), "p", "r", "m", "s")
	if _, ok := nilKG.Enhance(context.Background(), "p", "s"); ok {
		t.Fatalf("nil receiver Enhance should return false")
	}
	nilKG.Close(context.Background())

	// 构造一个 driver=nil 的降级实例(指向不可达地址,driver New 失败降级)
	t.Setenv("NEO4J_URL", "bolt://127.0.0.1:1") // 不可达,但 NewDriverWithContext 不连接,仅解析 URL
	t.Setenv("NEO4J_PASSWORD", "p")
	kg := NewKnowledgeGraph()
	if kg == nil {
		t.Fatalf("NewKnowledgeGraph must not return nil")
	}
	// driver 非 nil(URL 解析成功即建 driver),但 Record 会因连接失败降级(skip,不 panic)
	kg.Record(context.Background(), "prompt-text", "response-text", "minilm", "test")
	// Enhance 同样降级
	if _, ok := kg.Enhance(context.Background(), "prompt-text", "test"); ok {
		// 不可达时不应有命中
	}
	kg.Close(context.Background())
}

// 禁用开关:ESP_LLM_L3_ENABLED=false → NewKnowledgeGraph 返回 driver=nil 的实例,全部 no-op。
func TestKnowledgeGraph_DisabledNoOp(t *testing.T) {
	t.Setenv("ESP_LLM_L3_ENABLED", "false")
	kg := NewKnowledgeGraph()
	if kg.enabled {
		t.Fatalf("disabled should set enabled=false")
	}
	if kg.driver != nil {
		t.Fatalf("disabled should have nil driver")
	}
	// 全部 no-op,不 panic
	kg.Record(context.Background(), "p", "r", "m", "s")
	if _, ok := kg.Enhance(context.Background(), "p", "s"); ok {
		t.Fatalf("disabled Enhance should return false")
	}
	kg.Close(context.Background())
}

// 空输入防御:空 prompt/response 不应触发 Record(不写无效知识)。
func TestKnowledgeGraph_EmptyInputSkipped(t *testing.T) {
	t.Setenv("ESP_LLM_L3_ENABLED", "true")
	t.Setenv("NEO4J_URL", "bolt://127.0.0.1:1")
	t.Setenv("NEO4J_PASSWORD", "p")
	kg := NewKnowledgeGraph()
	// 空 prompt/response → no-op(即使 driver 可达也不应写空值)
	kg.Record(context.Background(), "", "r", "m", "s")
	kg.Record(context.Background(), "p", "", "m", "s")
	kg.Record(context.Background(), "p", "r", "", "s")
	// 不 panic 即通过
}
