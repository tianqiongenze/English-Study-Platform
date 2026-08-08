# SRS — LLM 统一网关服务(llm-orchestrator-rs)需求规格说明书

> 版本:1.0  日期:2026-08-06
> 关联:`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`(llm_call_logs)、`01-需求分析文档-SRS.md`

## 1. 背景

ESP 各服务各自调 LLM,无统一编排/降级/成本管控。这是成本与一致性的核心(P0)。需独立 Rust 服务 `llm-orchestrator-rs` 作为统一 LLM 网关:多 provider 路由/限流/语义缓存/降级/成本治理,所有 LLM 调用经它。

## 2. 用户角色

| 角色 | 场景 |
|---|---|
| 业务服务(pronunciation/listening/assessment/reading/writing/gateway/teacher) | 调 /api/llm/chat 统一入口,声明 service + intent |
| 运营 | 查成本报表(provider/model/service 维度) |
| 系统 | 监控限流/降级/缓存命中率 |

## 3. 功能需求(FR)

### FR-1 统一调用入口
- FR-1.1 `/api/llm/chat` 接收 service + intent + messages + params(model/temp/max_tokens)。
- FR-1.2 路由到 provider(anthropic/openai/...),按 intent 配置默认 model。
- FR-1.3 返回 content + usage(prompt_tokens/completion_tokens) + provider/model/cache_hit。

### FR-2 多 provider 路由与降级
- FR-2.1 按 intent + 成本/延迟策略选 provider;主 provider 失败/超时自动故障转移到备选。
- FR-2.2 provider 限流(令牌桶 per provider),超限返回 429 或排队。
- FR-2.3 全 provider 不可达降级:返回本地兜底(mock 响应 by intent),不阻断调用方。

### FR-3 语义缓存
- FR-3.1 对相同/相似 prompt(intent + 归一化 messages hash)命中 Redis 缓存,直接返回。
- FR-3.2 TTL by intent(解析类长、批改类短);缓存命中率可观测。
- FR-3.3 一期精确 hash 命中,二期语义相似(embedding)命中。

### FR-4 成本治理
- FR-4.1 每次调用落 llm_call_logs(user_id/service/provider/model/prompt_tokens/completion_tokens/latency_ms/cost_micros/status/cache_hit/trace_id)。
- FR-4.2 `/api/llm/cost` 按 service/provider/model/时间维度聚合查询。
- FR-4.3 预算告警:service 超阈值发事件(Pulsar)/日志。

## 4. 非功能需求(NFR)

| 编号 | 维度 | 要求 |
|---|---|---|
| NFR-1 | 性能 | P95 < 1.2s(含上游 LLM);缓存命中 < 50ms |
| NFR-2 | 可用性 | provider 故障自动转移;全失败降级兜底;单节点 99% |
| NFR-3 | 可观测 | OTel service.name=llm-orchestrator-rs;缓存命中/限流/成本指标 |
| NFR-4 | 安全 | 经 bff 前置 JWT;provider API Key 集中管理(K8s Secret) |
| NFR-5 | 数据 | 写 tidb-primary(esp_llm)/ 读 tidb-follower;Redis 语义缓存 |
| NFR-6 | 扩展 | axum + tokio;HPA 按 CPU + 并发扩 |
| NFR-7 | 容错 | provider timeout 10s + 转移;降级兜底 |

## 5. 边界

- **不做** LLM 模型训练/微调。
- **不做** 鉴权(bff 前置)。
- **不做** 业务逻辑(只做路由/缓存/降级/成本)。
- 一期精确缓存,二期语义缓存(embedding)。

## 6. 验收标准(AC)

- AC-1:端点(/healthz + /chat + /cost)全 200。
- AC-2:主 provider 失败时自动转移备选,响应不 5xx。
- AC-3:相同 prompt 二次命中缓存(cache_hit=1,< 50ms)。
- AC-4:每次调用 llm_call_logs 有记录,cost 聚合可查。
- AC-5:全 provider 不可达时降级返回兜底响应。
- AC-6:trace Tempo 可见(service.name=llm-orchestrator-rs);单测 ≥80%。
