# B3 — 业务事件目录(Business Event Catalog)

> TOGAF ADM Phase B/C 衔接 P0 补救产出物。
> 评估视角:教授级高级工程师 / 互联网大厂资深架构师。
> 数据来源:SAD §7 通信契约 + bff API Portal(226 行 OpenAPI,11 服务 43 路由)+ esp-api Dubbo 接口 + ADR-new-services。
> 日期:2026-08-13。

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
- **N-4 Pact 扩量**:从 teacher-learning 一对扩到全 11 REST 路由 + 全 Dubbo 接口。
- **L4 Pulsar 事件测点**:发一学习事件→worker 落库,延迟<1s。
- **L3 Dubbo Triple 测点**:teacher→learning 学情汇总调学习。

## 出口条件

5 Pulsar topic + 11 服务 43 REST 路由汇总 + 4 Dubbo Triple 接口 + WS 帧齐(已满足)。每条标 producer/consumer/语义/SLA/幂等/重试。
