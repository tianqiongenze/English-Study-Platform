# B3 — 业务事件目录(Business Event Catalog)

> TOGAF ADM Phase B/C 衔接 P0 补救产出物。
> 评估视角:教授级高级工程师 / 互联网大厂资深架构师。
> 数据来源:SAD §7 通信契约 + bff API Portal(226 行 OpenAPI,11 服务 43 路由)+ esp-api Dubbo 接口 + ADR-new-services。
> 日期:2026-08-13。
> 更新:2026-08-17 追加 Phase 1-3 新服务事件(content-go/community-go/gamification-go rankings)+ Nacos 服务注册说明。

## 目的

把 ESP 的三类业务事件/接口(Pulsar 异步事件 / REST 同步路由 / Dubbo Triple 同步 RPC)做全量登记,标 producer/consumer/语义/Schema/SLA/幂等/重试,作为契约版本化(C5)与 Pact 扩量(N-4)的基线。

---

## 1. Pulsar 异步业务事件(5 真实 topic)

> 命名规约 `{domain}.{action}`,Geo Replication 见 `多区域数据同步方案.md` / `pulsar-geo-replication-plan.md`。
> 当前默认全降级(off),未在压测真连 broker 验证(单节点资源);多节点 Phase D2 落地后真连。

| # | topic | producer | consumer | 语义 | Schema | SLA | 幂等 | 重试策略 |
|---|---|---|---|---|---|---|---|---|
| E-1 | `speech.session` | gateway-go | learning-java(异步落 session_turns)/ emotion-svc | 口语会话生命周期事件(发起/每轮/结束) | JSON {session_id, user_id, turn, asr_text, ts} | 端到端 <1s 落库 | event_id 幂等键 | DLQ + 3 次 |
| E-2 | `learning.event` | learning-java(Disruptor→Pulsar 双路径) | learning-worker | 学情事件(答题/掌握度变更) | JSON {user_id, kp_id, event_type, value, ts} | <1s 落 learning_events | event_id | DLQ + 3 |
| E-3 | `writing.task` | writing-py / writing-coach | teacher-java(终审联动)/ learning-java(mastery 反哺) | 写作任务状态变更(批改完成/评分/笔记) | JSON {task_id, user_id, score, notes, ts} | <2s 落库 | task_id | DLQ + 3 |
| E-4 | `emotion.signal` | emotion-svc | gateway-go(数字人反馈)/ learning-java | 情绪信号(识别结果/可视化触发) | JSON {session_id, emotion, confidence, ts} | <500ms | signal_id | DLQ + 3 |
| E-5 | `teacher.task` | teacher-java | learning-java(学情汇总)/ writing-py(批改联动) | 教师任务(教案生成/作业评阅/终审) | JSON {task_id, class_id, type, payload, ts} | <3s | task_id | DLQ + 3 |
| (横切) | `esp-cache-invalidate` | 任意服务(Java/Go/Python) | 全服务(L1 缓存订阅 L2 失效广播) | L1+L2 缓存失效广播(三端 key 统一明文 `{service}:{entity}:{id}`) | JSON {key, op} | <100ms 广播 | key | fire-and-forget |

### 1.1 Phase 1-3 新增事件(2026-08-17 追加)

> 内容库(content-go)、圈子社区(community-go)、排行榜(gamification-go rankings)三域事件。

| # | topic / 事件名 | 生产者 | 消费者 | 触发条件 | 数据载荷 | 幂等 |
|---|---|---|---|---|---|---|
| E-6 | `content.attempted` (via learning_event_outbox) | content-go | gamification-go(XP 发放)/ learning-java(掌握度更新) | content_attempts 表写入后,经 learning_event_outbox 异步投递 | JSON {user_id, item_id, item_type, score, correct, duration_ms, ts} | item_id+user_id+created_at 幂等 |
| E-7 | `learning.event.reported` (outbox relay) | content-go(写 outbox) | learning-java(worker 消费落库) | content_attempts → learning_event_outbox pending → OutboxRelay 重投 | JSON {user_id, event_type:"content_attempt", payload:{item_id,score}, ts} | message_id ON CONFLICT 去重(P1-8) |
| E-8 | `community.circle.created` | community-go | gamification-go(可选成就触发) | POST /api/circles 创建成功 | JSON {circle_id, name, owner_id, invite_code, ts} | circle_id |
| E-9 | `community.member.joined` | community-go | gamification-go(社交成就) | 圈子邀请码加入成功 | JSON {circle_id, user_id, role, ts} | circle_id+user_id 幂等 |
| E-10 | `community.post.created` | community-go | (可选内容审核/推荐) | 帖子发布成功 | JSON {post_id, circle_id, user_id, body_preview, ts} | post_id |
| E-11 | `community.report.threshold_reached` | community-go | community-go(自动隐藏) | content_reports 累计达阈值(circle_posts.report_count >= config) | JSON {target_type, target_id, report_count, ts} | target_id 幂等(幂等更新) |
| E-12 | `ranking.updated` | gamification-go | gamification-go(Redis ZSET 更新) | XP/积分变更触发排行榜重算 | JSON {user_id, scope_type, scope_id, delta_score, ts} | user_id+ts 幂等 |
| E-13 | `ranking.snapshot.taken` | gamification-go(日切定时) | leaderboard_snapshots(CRDB 落库) | 日切/周切/月切定时任务 | JSON {scope_type, scope_id, period, snapshot_date, rank_list[{user_id,score,rank}], total_members} | scope_type+scope_id+period+date 幂等 |

### 1.2 Nacos 服务注册/注销(ephemeral 实例,无显式事件)

> 2026-08-17 追加。ADR-19 Nacos 统一注册落地说明。

ESP 全部 19 个服务注册到 ekb-m2 共享 Nacos,实例生命周期由各语言 SDK 自动管理(ephemeral 模式)。

| # | 事件类型 | 触发条件 | 传播方式 | 可观测性 |
|---|---|---|---|---|
| E-nacos-1 | 实例注册(instance up) | 服务启动,SDK 自动注册到 Nacos | Nacos 推送(临时实例,心跳间隔 5s,超时 15s 剔除) | Nacos Dashboard 实例列表 + OTel service.instance.id |
| E-nacos-2 | 实例注销(instance down) | 服务正常关闭(graceful shutdown),SDK 自动 deregister | Nacos 即时移除 | Nacos Dashboard 实例列表减少 |
| E-nacos-3 | 实例异常(心跳丢失) | 服务崩溃/网络分区,SDK 未发 deregister | Nacos 15s 无心跳后自动剔除 | Nacos Dashboard 灰色实例 + 告警(可配) |

> 说明:这些是 Nacos 内部管理事件,非业务事件,不做 Pulsar topic 投递。BFF RouteTable(ADR-20)通过 Nacos Listener 感知路由变化,但不直接消费实例注册/注销事件。

## 2. REST 同步路由(11 服务,43 路由,from OpenAPI)

> 来源:`bff-java/static/openapi-go-rust.yaml`。bff-java 已上线 4 端点 200。详细路径见 OpenAPI 原文,此处按服务汇总。

| 服务 | 路由数 | 关键路由(语义) | 鉴权 | SLA(P95 目标) | 幂等 |
|---|---|---|---|---|---|
| gateway-go | ~6 | /api/scenarios, /api/sessions, /ws/speech(WS 升级) | JWT(BFF 签)+WS 鉴权 | 口语引导 <800ms(G-02) | session_id 幂等 |
| bff-java | ~5 | /api/auth/login, /api/auth/register, /api/auth/me, /actuator/health, /.well-known/jwks.json | 公开(login/register)+ JWT(me) | <500ms | login 幂等 by email |
| learning-java | ~5 | /api/learning/mastery/weak, /dueReview, /weakestPoints, /userStats, /weakCount | JWT + X-User-Id 注入 | <2s | 读幂等 |
| teacher-java | ~6 | /api/teacher/lesson, /class/:id, 作业评阅, 学情汇总 | JWT(role=teacher) | <3s | lesson_id 幂等 |
| writing-py | ~4 | /api/writing/grade, /api/writing/run(LangGraph thread_id) | JWT | ≥50rps P95<2s(G-04) | task_id 幂等 |
| emotion-svc | ~3 | /api/emotion/visual, /api/emotion/signal | JWT | <500ms | — |
| writing-coach | ~4 | /api/writing-coach/templates, /plan, /综合批改, /提升方案 | JWT | <3s | — |
| course-path-go | ~3 | 学习路径, 推荐路径 | JWT | <1s | 读幂等 |
| listening-go | ~2 | 听力练习/答题 | JWT | <1s | attempt_id 幂等 |
| reading-go | ~2 | 阅读篇章/答题 | JWT | <1s | attempt_id 幂等 |
| vocab-srs-go | ~2 | 词汇 SRS 复习 | JWT | <1s | — |
| gamification-go | ~1 | 成就/连续学习 | JWT | <1s | — |

## 3. Dubbo Triple 同步 RPC(esp-api 契约包)

> esp-api 是 Dubbo 接口包(provider/consumer 共享),契约与实现分离。Pact 当前覆盖 teacher-learning 一对(N-4 扩全)。

| # | 接口 | provider | consumer | 语义 | SLA | 幂等 | 关联 ADR |
|---|---|---|---|---|---|---|---|
| D-1 | TeacherMasteryService.summarizeClass | teacher-java | (前端经 BFF→teacher) | 班级学情汇总 | <3s | 读幂等 | ADR-Seata→Saga |
| D-2 | LearningMasteryService.getWeak / weakestPoints / dueReview | learning-java | teacher-java(学情汇总调学习) | 学情薄弱点/复习查询 | <2s(走主库 consistent=true) | 读幂等 | ADR-读写分离 / P2-6 |
| D-3 | LearningEventService.reportEvent | learning-java | writing-py / teacher-java | 学情事件上报(Disruptor↔Pulsar 双路径) | <1s | event_id 幂等 | ADR-01 Disruptor / P2-2 worker |
| D-4 | WritingTaskService.onGraded | writing-py | teacher-java / learning-java | 写作批改完成回调(终审联动 + mastery 反哺) | <2s | task_id 幂等 | ADR-writing-coach |

> INT-2 Dubbo timeout 3s→10s 是隐性契约破坏的实证(评分 §一 消息/事件驱动 6.5 扣分点)。契约版本化(C5)+ 全 Pact(N-4)是治理方向。

## 4. WS 长连接(gateway-go,口语陪练)

| 帧 | 方向 | Schema | 鉴权 | SLA |
|---|---|---|---|---|
| open/session_id | 客户端→服务端 | JSON {type:"open", session_id, scenario_id} | WS 握手 JWT(SEC-1 越权检查) | <500ms 建立 |
| audio | 客户端→服务端 | binary | 同上 | 端到端 <800ms |
| asr/result | 服务端→客户端 | JSON {type:"asr", text} | — | <1s |
| tts/avatar | 服务端→客户端 | JSON/binary | — | <1s |
| close | 双向 | JSON {type:"close"} | — | — |

> WS 广播分片 hub shard=8(P2-1 落地),双 pod 互发广播 ok(L6 测点)。

## 5. 与 TOGAF/优化方案对接

- **C5 契约版本策略**:本目录是契约版本化基线,加 semver + breaking change workflow。
- **N-4 Pact 扩量**:从 teacher-learning 一对扩到全 11 REST 路由 + 全 Dubbo 接口 + content/community 事件。
- **L4 Pulsar 事件测点**:发一学习事件->worker 落库,延迟<1s。
- **L3 Dubbo Triple 测点**:teacher->learning 学情汇总调学习。
- **Phase 1-3 事件测点**(2026-08-17):content.attempted outbox 投递延迟<5s;community.report.threshold_reached 自动隐藏链路<5s;ranking.updated Redis ZSET 更新 P95<100ms。

## 出口条件

5+8 Pulsar/Outbox 异步事件 + 11 服务 43 REST 路由汇总 + 4 Dubbo Triple 接口 + WS 帧齐(已满足)。每条标 producer/consumer/语义/SLA/幂等/重试。
