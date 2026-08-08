# SDD — 听力训练服务(listening-rs)详细设计

> 版本:1.0  日期:2026-08-06
> 关联:`srs-listening-rs.md`、`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`、`03-详细设计文档-SDD.md`
> 端口:8087  语言:Rust(axum + tokio)  schema:`esp_listening`

## 1. 模块设计

```
services/listening-rs/
  src/
    api/        # routes: list/stream/attempt/transcribe/progress/healthz
    stream/     # chunked 音频流 + resample(rubato)
    asr/        # AsrProvider trait(Mock + 外部)
    score/      # 客观题判分 + 听写文本相似度(编辑距离/BLEU)
    db/         # esp-db-rs: listening_exercises/listening_attempts
    cache/      # esp-cache-rs: 素材热点缓存
    otel/       # esp-otel-rs
    main.rs
```

关键 trait:`asr::AsrProvider`(fn transcribe(audio_uri) -> String)、`stream::Resampler`(fn resample(samples, speed) -> Vec<f32>)。`score::Grader` 区分 objective/dictation 两条路径。

## 2. 接口

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | /api/listening/list | level, category, page | [{id, level, category, duration_sec}] |
| GET | /api/listening/stream/{id} | speed | chunked audio bytes |
| POST | /api/listening/attempt | {user_id, exercise_id, answers, mode, time_spent} | {score, correct_answers, transcript} |
| POST | /api/listening/transcribe | {audio_uri} | {text} |
| GET | /api/listening/progress | user_id | {practiced, total_sec, avg_score} |
| GET | /healthz | - | 200 |

stream:从对象存储读 audio → resample(by speed)→ tokio chunked writer。attempt:判分 → db.insert(listening_attempts) → Pulsar `esp.business.listening.practiced`。

## 3. 数据模型(esp_listening)

- listening_exercises(id, level, category, audio_uri, transcript, questions JSON, duration_sec, created_at; INDEX level_cat)。schema.sql L206-216。
- listening_attempts(id, user_id, exercise_id, answers JSON, score, mode, time_spent_sec, created_at; INDEX user/exercise)。schema.sql L217-228。

## 4. 依赖

- 外部:tidb-primary/follower(esp_listening)、redis-cluster(素材缓存)、对象存储(audio_uri 解析)、ASR provider。
- 公共 crate:esp-db-rs、esp-cache-rs、esp-otel-rs。
- Pulsar:`esp.business.listening.practiced` → gamification。
- OTel:esp-observability,trace bff → listening-rs。

## 5. 部署

- Dockerfile:cargo-chef 多阶段,rust:1.81-slim。
- K8s:deploy/listening-rs,replicas 1/目标 3,requests 200m/256Mi,limits 1000m/1Gi。
- Env:WRITE_DB_SCHEMA=esp_listening、READ_DB_SCHEMA=esp_listening、ASR_ENDPOINT、OBJSTORE_BASE、REDIS_URL、OTEL_EXPORTER_OTLP_ENDPOINT。
- 探针:/healthz,压测期 timeout 调 3s。HPA CPU>70%。

## 6. 测试要点

- 单测:score objective/dictation、stream resample、asr mock 降级、cache 命中,≥80%。
- 集成:testcontainers TiDB,/stream + /attempt 端到端(mock ASR + 本地音频)。
