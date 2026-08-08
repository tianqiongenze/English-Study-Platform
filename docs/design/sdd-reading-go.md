# SDD — 分级阅读服务(reading-go)详细设计

> 版本:1.0  日期:2026-08-06
> 关联:`srs-reading-go.md`、`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`、`03-详细设计文档-SDD.md`
> 端口:8091  语言:Go(gin + Disruptor)  schema:`esp_reading`

## 1. 模块设计

```
services/reading-go/
  src/
    api/         # routes: list/attempt/mark-word/parse-sentence/progress/healthz
    parse/       # 长难句 AST 解析(调 llm-orchestrator) + AST 缓存
    vocab/       # 生词联动:查 vocabulary / 调 llm-orchestrator 生成 / 调 vocab-srs-go mark
    score/       # 客观题判分
    db/          # esp-db-go: reading_passages/reading_attempts
    cache/       # esp-cache-go: 文章热点 + AST 缓存
    disruptor/   # 批量写 reading_attempts
    otel/        # OTel go sdk
    main.go
```

关键类型:`parse::Parser`(fn parse(sentence) -> AST,先查 Redis 后调 llm-orchestrator)、`vocab::Linker`(fn lookup(word) -> Vocab / fn mark(user_id, word_id) 调 vocab-srs-go)。`score::Grader` 客观题判分。

## 2. 接口

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | /api/reading/list | level, category, page | [{id, level, category, title, word_count}] |
| POST | /api/reading/attempt | {user_id, passage_id, answers, time_spent} | {score, correct_answers, explanations} |
| POST | /api/reading/mark-word | {user_id, word} | {word_id, marked} |
| POST | /api/reading/parse-sentence | {sentence} | {ast, translation} |
| GET | /api/reading/progress | user_id | {read, total_sec, avg_score} |
| GET | /healthz | - | 200 |

parse-sentence:Redis GET ast → 命中返回;未命中调 llm-orchestrator `/api/llm/chat` → SETex 缓存 → 返回。mark-word:vocab.lookup(word) → 不存在调 llm-orchestrator 生成释义 → INSERT vocabulary → 调 vocab-srs-go `/api/vocab/review`(quality=0,due=now)。

## 3. 数据模型(esp_reading)

- reading_passages(id, level, category, title, content, word_count, questions JSON, created_at; INDEX level_cat)。schema.sql L320-330。
- reading_attempts(id, user_id, passage_id, answers JSON, score, time_spent_sec, created_at; INDEX user/passage)。schema.sql L331-341。

## 4. 依赖

- 外部:tidb-primary/follower(esp_reading)、redis-cluster(文章 + AST 缓存)、llm-orchestrator-rs(释义/解析)、vocab-srs-go(生词标记,HTTP RPC)。
- 公共包:esp-db-go、esp-cache-go、OTel go sdk。
- Pulsar:`esp.business.reading.completed` → gamification。
- OTel:esp-observability,trace bff → reading-go → llm-orchestrator / vocab-srs-go。

## 5. 部署

- Dockerfile:多阶段(golang:1.22 + alpine),ENV GOPROXY=https://goproxy.cn,direct。
- K8s:deploy/reading-go,replicas 1/目标 3,requests 100m/128Mi,limits 500m/512Mi。
- Env:WRITE_DB_SCHEMA=esp_reading、READ_DB_SCHEMA=esp_reading、LLM_ENDPOINT、VOCAB_ENDPOINT、REDIS_URL、OTEL_EXPORTER_OTLP_ENDPOINT。
- 探针:/healthz,压测期 timeout 3s。HPA CPU>70%。

## 6. 测试要点

- 单测:score 判分、parse 缓存命中/未命中、vocab lookup/mark 流程、disruptor 批量写,≥80%。
- 集成:testcontainers TiDB + miniredis + mock llm-orchestrator + mock vocab,/attempt + /mark-word + /parse-sentence 端到端。
