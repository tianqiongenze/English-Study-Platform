# SDD — LLM 统一网关服务(llm-orchestrator-rs)详细设计

> 版本:1.0  日期:2026-08-06
> 关联:`srs-llm-orchestrator-rs.md`、`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`、`03-详细设计文档-SDD.md`
> 端口:8092  语言:Rust(axum + tokio)  schema:`esp_llm`

## 1. 模块设计

```
services/llm-orchestrator-rs/
  src/
    api/        # routes: chat/cost/healthz
    router/     # 路由策略:intent -> provider/model;故障转移
    provider/   # Provider trait(anthropic/openai/mock) + 令牌桶限流
    cache/      # 语义缓存:精确 hash(一期)/ embedding(二期),Redis
    fallback/   # 兜底响应(by intent)
    cost/       # 成本计算(token -> micros) + 落 llm_call_logs + 聚合
    db/         # esp-db-rs: llm_call_logs
    otel/       # esp-otel-rs
    main.rs
```

关键 trait:`provider::LlmProvider`(fn chat(req) -> Resp + usage)、`router::Strategy`(fn pick(intent, providers) -> Provider,主选 + 备选链)。`cache::Cache`(fn key(messages) -> String / fn get/set)。`cost::Calculator`(fn calc(provider, model, prompt_tok, completion_tok) -> i64 micros)。令牌桶:`provider::RateLimiter`(per provider)。

## 2. 接口

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| POST | /api/llm/chat | {service, intent, messages, params} | {content, provider, model, usage, cache_hit} |
| GET | /api/llm/cost | service, provider, from, to | [{service, provider, model, calls, total_cost, cache_hit_rate}] |
| GET | /healthz | - | 200 |

chat 流程:cache.get → 命中返回(cache_hit=1)→ router.pick → rate_limiter.acquire → provider.chat → 失败故障转移下一 provider → 全失败 fallback.gen → cost.calc → db.insert(llm_call_logs) → 返回。落库异步(Disruptor/批写)。

## 3. 数据模型(esp_llm)

- llm_call_logs(id, user_id, service, provider, model, prompt_tokens, completion_tokens, latency_ms, cost_micros, status[ok/fallback/rate_limited/error], cache_hit, trace_id, created_at; INDEX user_time/service_time)。schema.sql L344-359。
- Redis 缓存:key `esp:llm:cache:{intent}:{hash}`,value=Resp+usage,TTL by intent。

## 4. 依赖

- 外部:tidb-primary/follower(esp_llm)、redis-cluster(缓存 + 限流计数)、anthropic/openai API(K8s Secret 存 Key)。
- 公共 crate:esp-db-rs、esp-cache-rs、esp-otel-rs。
- Pulsar:预算超阈值发 `esp.system.llm.budget_alert`(可选)。
- OTel:esp-observability,trace 贯通所有调用方 → llm-orchestrator → provider。

## 5. 部署

- Dockerfile:cargo-chef 多阶段,rust:1.81-slim。
- K8s:deploy/llm-orchestrator-rs,replicas 1/目标 3,requests 200m/256Mi,limits 1000m/1Gi。
- Env:WRITE_DB_SCHEMA=esp_llm、READ_DB_SCHEMA=esp_llm、REDIS_URL、ANTHROPIC_API_KEY(secret)、OPENAI_API_KEY(secret)、OTEL_EXPORTER_OTLP_ENDPOINT。
- 探针:/healthz,压测期 timeout 3s。HPA CPU>70% + 并发。

## 6. 测试要点

- 单测:router 故障转移链、rate_limiter 令牌桶、cache hash 命中/未命中、cost 计算、fallback by intent,≥80%。
- 集成:testcontainers TiDB + miniredis + mock provider,/chat 命中/未命中/故障转移/全降级路径。
- 契约:Pact 对各业务服务 consumer(/chat)。
