# LLM 缓存 Phase1 实现报告 — conversation-go L1 精确缓存

日期:2026-08-18
方法论:go-microservices skill(table-driven test + 构造器注入 + 依赖倒置)+ ASV 铁律(改前 Read 确认真实 API 签名)。

## 1. 实现文件

| 文件 | 说明 |
|---|---|
| `backend/services/conversation-go/src/llm/cached.go` | CachedProvider 装饰器(新增) |
| `backend/services/conversation-go/src/llm/cached_test.go` | table-driven 单测(新增,9 用例) |
| `backend/services/conversation-go/src/llm/provider.go` | 追加 `NewWithCache()` 工厂 + `newLLMRedisClient()` Redis client 构造(New() 原语义不变) |

## 2. Provider 接口签名(读代码确认的真实签名,非臆造)

来源:`src/llm/provider.go:17-20`

```go
type Provider interface {
    Chat(ctx context.Context, messages []Message) (string, error)
}
type Message struct {
    Role    string // "system" | "user" | "assistant"
    Content string
}
```

**关键 ASV 判断**:接口只返回 `(string, error)`,**无 metadata 字段**。任务书提到"返回缓存带 cache_hit=true 元数据"——这无法通过返回值传递(改签名会破坏 dialog.Engine.LLM 注入,违反"零改 Engine")。cache_hit 通过 Prometheus 计数器 + 结构化日志体现,不臆造带 metadata 的签名。

## 3. Redis key 规范 + TTL + 降级开关

- **key 格式**:`esp:cache:llm:exact:{model}:{sha256前16位}`
  - sha256 输入 = 规范化后的 messages(每条 `role|trim(content)\n` 拼接),碰撞空间 2^128 足够
  - model 段隔离不同模型缓存(同输入不同模型不串)
  - 前缀 `esp:cache:llm:exact:` 与 gateway-go 通用 cache(`esp:cache:...`)隔离
- **TTL**:`1 * time.Hour`(conversation 类回复,`cacheTTL` 常量)
- **降级开关 ENV**:`ESP_LLM_CACHE_ENABLED`(默认 true;=false 直穿 inner)
- **Redis 不可达降级**:`newLLMRedisClient()` Ping 失败 → store=nil → CachedProvider 直穿(只 warn 不阻断)
- **Redis 地址**:复用 `REDIS_HOST`(默认 `redis-cluster`)+ `REDIS_PORT`(默认 6379),与 cache 包一致;独立 ClusterClient(不复用 cache 包 client,语义/TTL 不同)

## 4. 依赖倒置(可测性)

CachedProvider.store 类型为 `redis.Cmdable` 接口(go-redis v9 `commands.go:173` 真实存在,ClusterClient 与 Client 均实现)。
- 生产:注入 `*redis.ClusterClient`(NewClusterClient 签名经 osscluster.go:1161 核实)
- 测试:注入 miniredis 的 `*redis.Client`(实现 Cmdable),无需真实 Redis Cluster

## 5. Prometheus 指标

go.mod 已有 `prometheus/client_golang`(indirect,已可用)。两个 counter:
- `llm_cache_hits_total{layer="exact"}`
- `llm_cache_misses_total{layer="exact"}`
(layer 标签预留 Phase2 L2 semantic)

## 6. 编译 / 单测证据

```
go build ./...           → BUILD_RC=0
go vet ./src/llm/        → VET_RC=0
go test ./src/llm/ -v    → 12/12 PASS(RC=0)
  - 9 新增 Cached 用例全过
  - 3 原有 mock/provider 用例全过(含 TestNew_ReturnsMockWhenModeMock,证明 New() 语义未破坏)
go test ./...            → 全仓全绿(asr/dialog/hub/llm/log/scenario/session/tts/ws,RC=0)
```

单测覆盖场景:MISS→inner 调用→回写→第二次 HIT / 不同输入独立缓存 / TTL 过期再 MISS / 规范化 trim 命中 / model 隔离 / ENV 禁用直穿且不写 Redis / store=nil 直穿 / inner error 不回写且错误传播 / key 格式。

## 7. 接线点

**事实**:conversation-go 当前**无 main 包**、**无 `llm.New()` 调用点**(grep 实证)。Engine.LLM 仅在 `dialog/engine_test.go` 用 `&llm.MockProvider{}` 直接构造注入。这说明 WS 入口在仓库外或尚未落地。

接线策略(零改 Engine):
- `New()` 保持不变(测试依赖其语义)
- 新增 `NewWithCache()`:内部 `NewCached(New(), newLLMRedisClient(), model)` —— 未来 WS 入口/外部 wire 注入 Engine.LLM 处,把 `llm.New()` 换成 `llm.NewWithCache()` 即可,Engine 零改动(仍是 llm.Provider)。

**当前接线状态**:工厂已就位,但无调用方(因仓库无 WS 入口)。部署到生产时,WS 入口 main 包(若后续补在 cmd/ 下)调用 `llm.NewWithCache()` 替代 `llm.New()` 即完成接线。

## 8. 部署验证

DEFERRED:conversation-go 无 main 包,镜像构建无目标(且无 Dockerfile 指向 main)。编译 + 单测已过,部署验证推迟到 WS 入口落地后。不碰 k3s/CRDB/集群 Nacos。

## 9. 后续 Phase1.1 / .2 / Phase2 清单

- **P1.1 writing-py 双写迁移**:writing-py 已有旧 L1,需迁移到统一 `esp:cache:llm:exact:` key 规范(双写过渡 + 读新 key 优先)
- **P1.2 llm-orchestrator-rs**:同上迁移,旧 L1 key 对齐新规范
- **Phase2 L2 Milvus**:语义模糊匹配(embedding 近邻),区别于 Phase1 精确匹配;Prometheus layer="semantic"
- **接线补全**:conversation-go 补 WS 入口 main 包(cmd/server),调用 `llm.NewWithCache()` 完成实际注入
- **集成测试**:Phase1 落地后加 IT(真实 Redis Cluster + MockProvider),验证跨副本缓存共享(当前单测用 miniredis 验证单实例语义)
