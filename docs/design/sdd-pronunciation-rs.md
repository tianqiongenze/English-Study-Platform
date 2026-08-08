# SDD — 发音评分服务(pronunciation-rs)详细设计

> 版本:1.0  日期:2026-08-06
> 关联:`srs-pronunciation-rs.md`、`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`、`03-详细设计文档-SDD.md`
> 端口:8086  语言:Rust(axum + tokio)  schema:`esp_pronunciation`

## 1. 模块设计

```
services/pronunciation-rs/
  src/
    api/        # axum routes: score/history/stats/healthz
    align/      # 音素对齐:Levenshtein + DTW(二期 MFA trait 切换)
    score/      # GOP 评分 + 综合分加权
    feedback/   # 规则引擎:最小对立体/重音/语调建议
    asr/        # AsrProvider trait(Mock + gateway-go RPC)
    db/         # esp-db-rs 封装(pronunciation_attempts CRUD)
    cache/      # esp-cache-rs:音素模型特征缓存
    otel/       # esp-otel-rs:trace 注入
    main.rs
```

关键 trait:`align::Aligner`(fn align(expected: &[Phoneme], actual: &[Phoneme]) -> Aligned)、`asr::AsrProvider`(fn recognize(audio_uri) -> String)。`score::GopScorer` 输出 overall/fluency/completeness/pronunciation + phoneme_scores JSON。

## 2. 接口

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| POST | /api/pronunciation/score | {user_id, target_text, audio_uri} | {overall_score, phoneme_scores[], fluency, completeness, pronunciation, feedback} |
| GET | /api/pronunciation/history | user_id, limit | [{overall_score, created_at, target_text}] |
| GET | /api/pronunciation/stats | user_id | {weak_phonemes:[{phoneme, avg_score, count}]} |
| GET | /healthz | - | 200 |

score 流程:asr.recognize → align.align(target_phonemes, actual_phonemes) → gop.score → feedback.gen → db.insert(pronunciation_attempts) → 返回。ASR 失败走文本对齐降级。

## 3. 数据模型(esp_pronunciation.pronunciation_attempts)

字段:id(BIGINT AUTO_RANDOM PK)、user_id、target_text、audio_uri、overall_score(DECIMAL 5,2)、phoneme_scores(JSON: [{phoneme, score, expected, actual}])、fluency_score、completeness_score、pronunciation_score、feedback(JSON: 重音/语调/最小对立体)、created_at、INDEX(user_id, created_at)。对应 schema.sql L190-203。

## 4. 依赖

- 外部:tidb-primary(写)/tidb-follower(读)经 ExternalName;redis-cluster(音素特征缓存);gateway-go ASR(gRPC/HTTP)。
- 公共 crate:esp-db-rs(sqlx + 读写分离)、esp-cache-rs(redis-cluster)、esp-otel-rs(opentelemetry-otlp)。
- Pulsar:评分完成发 `esp.business.pronunciation.scored`(供 gamification 消费)。
- OTel:接 esp-observability otel-collector,trace 贯通 gateway-go → pronunciation-rs。

## 5. 部署

- Dockerfile:cargo-chef 多阶段(planner → builder → runtime),基础镜像 rust:1.81-slim。
- K8s:deploy/pronunciation-rs, replicas 1(单节点)/目标 3(多节点),requests 200m/256Mi,limits 1000m/1Gi。
- Env:WRITE_DB_SCHEMA=esp_pronunciation、READ_DB_SCHEMA=esp_pronunciation、ASR_ENDPOINT、REDIS_URL=redis-cluster:6379、OTEL_EXPORTER_OTLP_ENDPOINT。
- 探针:readiness /healthz,timeout 1s → 单节点压测期按 esp-loadtest-probe-pitfalls 调 3s。
- HPA:CPU>70% 扩,PDB minAvailable 1。

## 6. 测试要点

- 单测:align Levenshtein/DTW 往返、gop 加权、feedback 规则、asr mock 降级路径,目标 ≥80%。
- 集成:testcontainers TiDB + miniredis,/score 端到端(mock ASR 返回固定文本)。
- 契约:Pact 对 bff/gateway-go(可选,/score consumer)。
