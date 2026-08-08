# SDD — 能力测评服务(assessment-rs)详细设计

> 版本:1.0  日期:2026-08-06
> 关联:`srs-assessment-rs.md`、`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`、`03-详细设计文档-SDD.md`
> 端口:8089  语言:Rust(axum + tokio)  schema:`esp_assessment`

## 1. 模块设计

```
services/assessment-rs/
  src/
    api/        # routes: start/answer/finish/history/report/healthz
    cat/        # 自适应测试:ItemBank + Selector(最大信息量) + Estimator(EAP 贝叶斯 θ)
    irt/        # 1PL/2PL 模型:probability(theta, a, b) + info(theta)
    report/     # 报告生成:skill_scores + 弱项诊断
    mastery/    # 回写 learning-java(Dubbo Triple / Pulsar)
    db/         # esp-db-rs: assessment_sessions/assessment_answers
    cache/      # esp-cache-rs: 题库缓存
    otel/       # esp-otel-rs
    main.rs
```

关键类型:`irt::Model`(fn prob(theta, a, b) f64 / fn info(theta, a, b) f64)、`cat::Estimator`(fn update(prior, answer, item) -> Posterior,贝叶斯 EAP)、`cat::Selector`(fn next(items, theta) -> Item,最大 Fisher 信息)。收敛:`cat::Convergence`(std < 0.3 或 n >= 25)。

## 2. 接口

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| POST | /api/assessment/start | {user_id, test_type} | {session_id, first_item} |
| POST | /api/assessment/answer | {session_id, item_id, answer} | {correct, next_item?, converged} |
| POST | /api/assessment/finish | {session_id} | {overall_score, skill_scores, irt_theta, report} |
| GET | /api/assessment/history | user_id | [{session_id, test_type, overall_score, completed_at}] |
| GET | /api/assessment/report/{id} | - | {skill_scores, irt_theta, report} |
| GET | /healthz | - | 200 |

answer 流程:判 correct → irt.info → estimator.update(θ) → db.insert(assessment_answers) → 若收敛 next_item=null else selector.next → 返回。finish:聚合 skill_scores → report.gen → db.update(assessment_sessions completed) → mastery.writeback(learning) → Pulsar `esp.business.assessment.completed`。

## 3. 数据模型(esp_assessment)

- assessment_sessions(id, user_id, test_type, status, started_at, completed_at, overall_score, skill_scores JSON, irt_theta, report JSON; INDEX user/status)。schema.sql L263-276。
- assessment_answers(id, session_id, user_id, item_id, answer, correct, irt_theta, responded_at; INDEX session/user)。schema.sql L277-288。

## 4. 依赖

- 外部:tidb-primary/follower(esp_assessment)、redis-cluster(题库缓存)、learning-java(Dubbo Triple mastery 回写)。
- 公共 crate:esp-db-rs、esp-cache-rs、esp-otel-rs。
- Pulsar:`esp.business.assessment.completed` → gamification + learning。模考型触发 writing-coach(`/api/writing-coach/plan` 或 Pulsar)。
- OTel:esp-observability,trace bff → assessment-rs → learning。

## 5. 部署

- Dockerfile:cargo-chef 多阶段,rust:1.81-slim。
- K8s:deploy/assessment-rs,replicas 1/目标 3,requests 200m/256Mi,limits 1000m/1Gi。
- Env:WRITE_DB_SCHEMA=esp_assessment、READ_DB_SCHEMA=esp_assessment、LEARNING_ENDPOINT、REDIS_URL、OTEL_EXPORTER_OTLP_ENDPOINT。
- 探针:/healthz,压测期 timeout 3s。HPA CPU>70%。

## 6. 测试要点

- 单测:irt prob/info 数值、estimator EAP 收敛、selector 最大信息、convergence 判定、report 生成,≥80%。
- 集成:testcontainers TiDB,/start → 多轮 /answer → /finish 端到端(mock 题库 + mock learning)。
