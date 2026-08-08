# SDD — 词汇记忆服务(vocab-srs-go)详细设计

> 版本:1.0  日期:2026-08-06
> 关联:`srs-vocab-srs-go.md`、`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`、`03-详细设计文档-SDD.md`
> 端口:8088  语言:Go(gin + Disruptor)  schema:`esp_vocab`

## 1. 模块设计

```
services/vocab-srs-go/
  src/
    api/        # gin routes: words/word/due/review/estimate/progress/healthz
    srs/        # SM-2+ 算法:Schedule(quality, card) -> Card
    estimate/   # 词汇量自适应测评(IRT 简化/折半)
    db/         # esp-db-go: vocabulary/user_vocabulary CRUD
    cache/      # esp-cache-go: 到期队列 L1 + L2 Redis
    disruptor/  # 批量写 user_vocabulary(异步落库)
    otel/       # OTel go sdk
    main.go
```

关键类型:`srs.Card`(word_id, interval, ease, due_at, review_count, lapse_count, mastery_score)、`srs.Schedule(quality int, c Card) Card`(SM-2+:quality<3 重置 interval=1、ease-=0.2 下限 1.3;quality>=3 interval*=ease)。Disruptor 消费 review 事件批量 UPSERT user_vocabulary。

## 2. 接口

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | /api/vocab/words | cefr, freq_min, page | [{id, word, pos, cefr, frequency_rank}] |
| GET | /api/vocab/word/{id} | - | {word, definition, phonetic, root, affix, example} |
| GET | /api/vocab/due | user_id, limit | [{word_id, word, due_at}] |
| POST | /api/vocab/review | {user_id, word_id, quality} | {interval, ease, due_at, mastery_score} |
| GET | /api/vocab/estimate | user_id | {estimated_size, cefr} |
| GET | /api/vocab/progress | user_id | {learned, mastered, due_count} |
| GET | /healthz | - | 200 |

review 流程:srs.Schedule → Disruptor.publish(异步 UPSERT user_vocabulary)→ 同步返回新调度 → Pulsar `esp.business.vocab.reviewed`。

## 3. 数据模型(esp_vocab)

- vocabulary(id, word UK, pos, definition, phonetic, frequency_rank, cefr, root, affix, example, created_at; INDEX freq/cefr)。schema.sql L231-246。
- user_vocabulary(id, user_id, word_id, srs_interval, srs_ease, due_at, review_count, lapse_count, last_review_at, mastery_score; UK user_word, INDEX due)。schema.sql L247-260。

## 4. 依赖

- 外部:tidb-primary/follower(esp_vocab)、redis-cluster(到期队列 + 词库缓存)。
- 公共包:esp-db-go(sqlx + 读写分离)、esp-cache-go(go-redis cluster)、OTel go sdk。
- Pulsar:`esp.business.vocab.reviewed` → gamification;订阅 learning-java mastery 回写(可选)。
- OTel:esp-observability,trace bff → vocab-srs-go。

## 5. 部署

- Dockerfile:多阶段(builder golang:1.22 + runtime alpine),ENV GOPROXY=https://goproxy.cn,direct(见 esp-go-dockerfile-goproxy)。
- K8s:deploy/vocab-srs-go,replicas 1/目标 3,requests 100m/128Mi,limits 500m/512Mi。
- Env:WRITE_DB_SCHEMA=esp_vocab、READ_DB_SCHEMA=esp_vocab、REDIS_URL、OTEL_EXPORTER_OTLP_ENDPOINT。
- 探针:/healthz,压测期 timeout 3s。HPA CPU>70%。

## 6. 测试要点

- 单测:srs.Schedule quality 0-5 全路径、Disruptor 批量写、cache 命中/穿透、estimate 折半,≥80%。
- 集成:testcontainers TiDB + miniredis,/review → /due 端到端。
- 契约:Pact 对 bff(/review consumer)。
