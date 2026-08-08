# SRS — 游戏化服务(gamification-go)需求规格说明书

> 版本:1.0  日期:2026-08-06
> 关联:`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`(user_streaks/achievements/user_achievements)、`01-需求分析文档-SRS.md`

## 1. 背景

ESP 无打卡/排行榜/成就/连胜机制,C 端留存关键缺失。需独立 Go 服务 `gamification-go`,以 Pulsar 事件驱动订阅全服务学习事件,异步计算 XP/连胜/成就,Redis ZSET 维护排行榜,TiDB 周期快照。

## 2. 用户角色

| 角色 | 场景 |
|---|---|
| 学习者 | 学习触发事件 → 累 XP/连胜/解锁成就 → 看排行榜/勋章墙 |
| 教师 | 查看班级排行榜/成就分布 |
| 业务服务 | 发 Pulsar 学习事件(pronunciation/vocab/listening/reading/assessment/writing) |

## 3. 功能需求(FR)

### FR-1 事件接入(Pulsar)
- FR-1.1 订阅 `esp.business.*`(pronunciation.scored/vocab.reviewed/listening.practiced/reading.completed/assessment.completed/writing.submitted)。
- FR-1.2 事件含 user_id + event_type + xp_weight,异步消费。

### FR-2 XP 与连胜
- FR-2.1 每事件累 total_xp(user_streaks),规则:不同 event_type 不同 xp。
- FR-2.2 连胜:当日首事件触发,current_streak+1(若 last_active_date=昨日),否则重置 0;longest_streak 取 max。
- FR-2.3 跨日由 last_active_date 判定,幂等(同日多次不重复+streak)。

### FR-3 成就
- FR-3.1 achievements 表:code/title/description/icon/xp_reward/criteria JSON。
- FR-3.2 事件触发后评估 criteria(如"连续 7 天"/"词汇 1000 词"/"发音 90 分"),解锁落 user_achievements(幂等 UK)。
- FR-3.3 `/api/gamification/achievements?user_id=` 返回已解锁 + 全部成就状态。

### FR-4 排行榜
- FR-4.1 Redis ZSET 按 total_xp/周/月维度维护。
- FR-4.2 `/api/gamification/leaderboard?period=week` 返回 Top-N + 用户排名。
- FR-4.3 TiDB 周期快照(每日)落 user_streaks 快照表(可扩展)。

### FR-5 个人面板
- FR-5.1 `/api/gamification/profile?user_id=` 返回 total_xp/streak/longest_streak/已解锁成就数/排名。

## 4. 非功能需求(NFR)

| 编号 | 维度 | 要求 |
|---|---|---|
| NFR-1 | 性能 | 事件端到端 < 2s(异步);leaderboard/profile P95 < 50ms(Redis) |
| NFR-2 | 可用性 | Pulsar 消费失败进死信队列;降级直接写 DB;单节点 99% |
| NFR-3 | 可观测 | OTel service.name=gamification-go;消费 lag 监控 |
| NFR-4 | 安全 | 经 bff 前置 JWT |
| NFR-5 | 数据 | 写 tidb-primary(esp_gamification)/ 读 tidb-follower;Redis ZSET 主 |
| NFR-6 | 扩展 | gin + 消费者并发可调;HPA 按 CPU + lag 扩 |
| NFR-7 | 容错 | 死信队列 + 幂等消费;Redis 不可达降级读 TiDB |

## 5. 边界

- **不做** 鉴权/用户管理。
- **不做** 积分商城兑换(二期)。
- **不做** 学习事件生产(各业务服务发)。
- 排行榜以 Redis ZSET 为准,TiDB 仅快照。

## 6. 验收标准(AC)

- AC-1:端点(/healthz + /profile + /leaderboard + /achievements)全 200。
- AC-2:发 Pulsar 事件后,profile total_xp/streak 正确更新(幂等)。
- AC-3:成就 criteria 满足时 user_achievements 落库(重复不重复落)。
- AC-4:leaderboard Redis ZSET 排序正确,period 切换正确。
- AC-5:trace Tempo 可见;消费 lag 可观测。
- AC-6:单测 ≥80%(XP/连胜/成就规则/幂等)。
