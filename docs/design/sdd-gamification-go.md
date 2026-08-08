# SDD — 游戏化服务(gamification-go)详细设计

> 版本:1.0  日期:2026-08-06
> 关联:`srs-gamification-go.md`、`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`、`03-详细设计文档-SDD.md`
> 端口:8090  语言:Go(gin)  schema:`esp_gamification`

## 1. 模块设计

```
services/gamification-go/
  src/
    api/         # routes: profile/leaderboard/achievements/healthz
    consumer/    # Pulsar 消费者 esp.business.*(并发 + 死信)
    xp/          # XP 规则引擎:EventType -> xp
    streak/      # 连胜计算(幂等,基于 last_active_date)
    achievement/ # 成就评估器:criteria JSON -> bool
    leaderboard/ # Redis ZSET 管理(week/month/all)
    db/          # esp-db-go: user_streaks/achievements/user_achievements
    cache/       # esp-cache-go: ZSET + profile 缓存
    otel/        # OTel go sdk
    main.go
```

关键类型:`xp::Rule`(fn award(evt EventType) int)、`streak::Updater`(fn update(s UserStreak, today Date) UserStreak,幂等:同日不+1)、`achievement::Evaluator`(fn check(criteria JSON, profile) bool)。consumer 并发 N,失败 retry → 死信 `esp.business.dlq`。

## 2. 接口

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | /api/gamification/profile | user_id | {total_xp, current_streak, longest_streak, achievements_count, rank} |
| GET | /api/gamification/leaderboard | period, top | [{user_id, score, rank}] |
| GET | /api/gamification/achievements | user_id | {unlocked:[], locked:[]} |
| GET | /healthz | - | 200 |

消费流程:Pulsar msg → 解析 event → xp.award + streak.update → Redis ZINCRBY + DB UPSERT user_streaks(Disruptor 批量) → achievement.check → 解锁则 INSERT IGNORE user_achievements + ZINCRBY xp_reward → ack。降级:Pulsar 失败进死信;Redis 失败只写 DB。

## 3. 数据模型(esp_gamification)

- user_streaks(id, user_id UK, current_streak, longest_streak, last_active_date, total_xp, updated_at; INDEX streak DESC)。schema.sql L291-300。
- achievements(id, code UK, title, description, icon, xp_reward, criteria JSON)。schema.sql L301-309。
- user_achievements(id, user_id, achievement_id, unlocked_at; UK user_ach, INDEX user)。schema.sql L310-317。
- 排行榜:Redis ZSET key `esp:lb:{period}`(week/month/all),score=total_xp。

## 4. 依赖

- 外部:tidb-primary/follower(esp_gamification)、redis-cluster(ZSET + profile 缓存)、pulsar-broker(订阅 esp.business.*)。
- 公共包:esp-db-go、esp-cache-go、OTel go sdk。
- OTel:esp-observability,trace consumer span。

## 5. 部署

- Dockerfile:多阶段(builder golang:1.22 + runtime alpine),ENV GOPROXY=https://goproxy.cn,direct。
- K8s:deploy/gamification-go,replicas 1/目标 3,requests 100m/128Mi,limits 500m/512Mi。
- Env:WRITE_DB_SCHEMA=esp_gamification、READ_DB_SCHEMA=esp_gamification、REDIS_URL、PULSAR_URL、OTEL_EXPORTER_OTLP_ENDPOINT。
- 探针:/healthz,压测期 timeout 3s。HPA CPU>70% + 消费 lag。

## 6. 测试要点

- 单测:xp 规则、streak 幂等(同日/跨日/断连)、achievement evaluator(criteria 解析)、leaderboard ZSET 排序,≥80%。
- 集成:testcontainers TiDB + miniredis + mock Pulsar,发事件 → profile/leaderboard 校验。
- 死信:模拟处理失败,验证进 DLQ。
