# SDD — 阶段3 排行榜(实时排行榜)详细设计

> 版本:1.0  日期:2026-08-17
> 关联:`sdd-gamification-go.md`、`sdd-content-go.md`、`architecture/togaf-assessment.md`、`architecture/engineering-ironrules.md`、`architecture/tech-debt-register.md`、`01-需求分析文档-SRS.md`
> 服务:gamification-go(扩展,8092) + content-go(喂数源,outbox relay)  语言:Go(gin)  schema:`esp_gamification` / `esp_learning`(读)+ `esp_auth`(读)+ `esp_community`(读)
> 方法论:**SDD 6 节** + **TOGAF ADM 4 架构域** + **DDD(限界上下文/聚合根/领域事件)** + **读写分离** + **多级缓存 L1+L2+L3** + **互联网大厂反刷榜标准**

---

## 0. 背景与诚实声明(为什么有这份补设计)

**铁律违反与补救:** 阶段3 排行榜在功能拓展过程中**违反了"先完整设计再实施"的工程铁律**(engineering-ironrules 第②条"设计-实现-验证三态追踪")—— 先写了 `rankings.go` + `outbox_relay.go` 跑通 E2E,事后才补设计。用户 2026-08-17 明确要求:"要做到 SDD、DDD、读写分离、多级缓存、符合 Togaf 架构方法和互联网大厂标准等和原来所有的服务要求和标准一样"。本文件即按既有服务设计基线(`sdd-gamification-go.md` 等 6 节模板)对该功能做**事后补救式完整设计**,随后据本设计审计/修正实现(任务 #204)。

**用户 2026-08-17 追加要求(本设计硬约束):"缓存全部要实现多级缓存"** —— 排行榜读路径必须 L1(进程内)+ L2(Redis Cluster)+ L3(CRDB 快照)三级,不允许只挂 Redis ZSET 单级。详见 §1.4 / §3.3 / §6。

**已落地实现现状(审计基线,见任务 #204):** `rankings.go`(698 行)轮询喂数 + ZSET 扇出 + top-100 快照 + 读端 opt-out 过滤;`outbox_relay.go`(132 行)content-go 侧 CRDB outbox 自投。E2E 已绿。本设计的目标架构与之的 **5 项偏差** 列于 §8,逐条给出目标态与修正路径。

---

## 1. 模块设计

### 1.1 职责边界

排行榜能力属于 **gamification-go** 的扩展模块(非新服务),复用其 Redis Cluster 连接、读写分离双池、OTel。喂数源侧由 **content-go** 补一个 outbox relay(坑㉙:content 事件经 CRDB outbox 直插 learning_events,不走 Pulsar,故 gamification 必须以落库事件表为完整源)。

```
services/gamification-go/src/
  rankings.go            # 已有(待审计修正):扇出/读榜/快照/opt-out
  rankings_cache.go      # 新增:L1+L2+L3 多级缓存(用户强制)
  rankings_projector.go  # 新增(目标态):Pulsar CQRS 投影器(替代/并行轮询)
  rankings_antifraud.go  # 新增(目标态):反刷增强(频率/设备/可信时间窗)
  rankings_test.go       # 补单测:xpFromEvent/fanout/periodBucket/regionAncestors/cache

services/content-go/src/
  outbox_relay.go        # 已有:CRDB outbox → learning_events 自投(坑㉙)
```

### 1.2 DDD 限界上下文(Context Map)

> ⚠️ **DDD 是本项目新引入的方法论**(既有服务基线未见显式 DDD,见 memory `esp-service-design-baseline`)。此处为阶段3 首次落地 DDD 建模,**需架构治理委员会评审**(architecture-board-charter);评审通过后回填到 `architecture/business-capability-map.md` 与 `business-event-catalog.md`。

| 限界上下文(BC) | 职责 | 聚合根(Aggregate Root) | 领域事件(Domain Event) | 与其他 BC 的关系(Context Map) |
|---|---|---|---|---|
| **排行计算 BC(Ranking)** | XP/进度计算、周期桶、ZSET 扇出、快照、读榜 | `LeaderboardSnapshot`(日切不可变快照)、`RankingScope`(global/region/circle/category × daily/weekly/monthly) | `XPComputed`、`RankingSnapshotTaken`、`OptOutChanged` | 上游依赖"内容事件 BC""学习事件 BC"(事件源);下游被"分享 BC"消费快照 |
| **内容事件 BC(Content Event)** | content-go 作答事件产出 + outbox 中转 | `LearningEvent`(幂等 message_id)、`OutboxMessage`(status pending→done/dead) | `ContentItemAnswered`、`OutboxRelayed` | 下游"排行计算 BC"经事件表消费;与"学习事件 BC"经 shared `learning_events` 表协作(共享内核 Shared Kernel,需显式契约) |
| **用户身份 BC(Identity)** | region_code / ranking_opt_out / name | `UserProfile`(region/optOut/avatar) | `RegionChanged`、`OptOutToggled` | "排行计算 BC"读用户元数据(读模型);opt-out 变更须通知排行 BC 做写端拦截 |
| **圈子社区 BC(Community)** | 圈子成员关系 | `Circle`、`Membership` | `MemberJoined`、`MemberLeft` | "排行计算 BC"读成员关系做圈子维度扇出 |

**Context Map 类型:** 排行计算 BC 与用户身份 BC / 圈子 BC 为 **客户/供应商(C/S)** —— 排行依赖它们的字段契约;与内容/学习事件 BC 为 **遵奉者(Conformist)** —— 排行被动消费既定 schema 的 `learning_events`,不反向施压。`learning_events` 表是 **共享内核(Shared Kernel)**,任何 BC 改其 schema 须架构评审。

### 1.3 读写分离(对标既有 Go 服务基线)

复用 `esp-common-go/db` 双池:`readDB`(follower 读,默认 `crdb-public:26257`)与 `writeDB`(leader 写)。排行榜读写分离矩阵:

| 操作 | 池 | 目标表/键 | 说明 |
|---|---|---|---|
| 拉取喂数事件 `fetchEvents` | readDB | `esp_learning.learning_events` | 按 id 高水位游标,只读 |
| 加载用户元数据 `loadUsers` | readDB | `esp_auth.users`(region_code/ranking_opt_out) | 读模型,5min TTL |
| 圈子成员关系 `memberCircleMap` | readDB | `esp_community.circle_memberships` | 读模型 |
| opt-out 全量 `refreshedOptOut` | readDB | `esp_auth.users` | 读端过滤兜底(目标态改写端拦截) |
| 地区字典 `regions` | readDB | `esp_auth.users` DISTINCT | L1+L2 缓存 |
| 用户名补全 `userNameMap` | readDB | `esp_auth.users` | 读模型 |
| ZSET 扇出 `fanout` | Redis(写) | `esp:gamif:lb:*` | L2 写 |
| 每日 XP cap `dailyUnderCap` | Redis(读写) | `esp:gamif:xpc:*` | 原子 IncrBy |
| 日切快照 `snapshotDay` | writeDB | `esp_gamif.leaderboard_snapshots` | 批量 INSERT,事务 |
| opt-out 切换 `setOptOut` | writeDB | `esp_auth.users` | 写 + 失效缓存 |

> 单节点环境 readDB==writeDB(同一 CRDB),但代码层双池隔离,**多节点扩容时 readDB 切 follower 即可生效,零代码改动**(既有基线保证,见 `sdd-gamification-go.md` §4)。

### 1.4 多级缓存(用户强制:全部实现 L1+L2+L3)

> 既有基线:仅 `conversation-go/src/cache/cache.go` 有真 L1(sync.Map+LRU)+ L2(Redis Cluster)Cache-Aside。gamification-go 当前**只有 L2 ZSET,无 L1** —— 本设计强制补齐 L1,并新增 L3 快照层。对标 conversation-go 的 `GetOrLoad`/`Evict` 语义。

排行榜三级缓存模型:

| 层 | 介质 | 存什么 | TTL / 失效 | 一致性策略 |
|---|---|---|---|---|
| **L1 进程内** | sync.Map + LRU(照 conversation-go `l1Cache`) | 用户元数据(region/optOut/name)、地区字典、opt-out 集、小榜 top-N JSON | 60s~5min | 跨副本靠 L2+TTL 兜底(conversation-go 既有策略);opt-out 变更走 `Evict` 主动失效 |
| **L2 Redis Cluster** | ZSET + String | ZSET 榜(`esp:gamif:lb:*`,主存)、XP cap 计数、feed 高水位/锁 | 周期桶自然翻转(daily/weekly/monthly 按日期 key) | 写端扇出即时;读端 ZREVRANGE |
| **L3 CRDB 快照** | `esp_gamif.leaderboard_snapshots` | 日切全量 rank/total(目标态:不再只 top-100,见 §8 偏差④) | 永久(历史 percentile 用) | 日切定时写,不可变 |

**Cache-Aside 读路径(目标态):** `rankings`/`rankingGap` 读榜 → L1 命中(top-N JSON)?→ L2 ZREVRANGE → 回填 L1 → L3 仅用于历史 percentile 与 L2 不可用时的降级快照读。**opt-out 读端过滤在 L1 层用缓存集完成,避免每请求查 DB**(目标态进一步改写端拦截,见 §8 偏差②)。

**降级:** L2 Redis 不可达 → 读 L3 快照(上一日)+ L1;L1 未命中且 L2 不可达 → 直接读 L3。照 conversation-go "Redis 不可达只 warn 不阻断" 语义。

---

## 2. 接口

| 方法 | 路径 | 入参 | 出参 | 缓存 |
|---|---|---|---|---|
| GET | /api/gamification/rankings | type=xp\|progress, scope=global\|region\|circle\|category, scope_id, period=daily\|weekly\|monthly, limit | {list:[{rank,user_id,name,score}], as_of, mine:{user_id,rank,score,gap_to_top}} | L1(top-N,60s)+L2 ZSET |
| GET | /api/gamification/rankings/gap | type, scope, scope_id, period | {my_rank, my_score, total, gap_to_top, gap_to_prev, percentile} | L1(optOut,60s)+L2;percentile 目标态走 L3 快照 |
| GET | /api/gamification/rankings/regions | - | [{code,label}] | L1(300s)+L2(300s) |
| GET | /api/gamification/rankings/categories | - | [{code,label}] | L1(300s) |
| PUT | /api/gamification/rankings/opt-out | {opt_out:bool}(X-User-Id) | {opt_out} | 写 DB + Evict L1(optOut)+ 目标态写端拦截 |

**喂数源契约(content-go → learning_events):** content-go `outbox_relay.go` 把 `learning_event_outbox(status=pending)` 投到 `esp_learning.learning_events`,字段 `(id, message_id UK, user_id, event_type, payload JSONB, created_at)`。gamification 经 `fetchEvents` 按 `id > hwm` 游标拉取。**幂等保证:** `learning_events.message_id` UNIQUE + `ON CONFLICT (message_id) DO NOTHING`(content-go relay 侧已做);目标态 gamification 侧再加 `SET processed:{msg_id} 1 NX EX 86400` 双保险(见 §8 偏差⑤)。

---

## 3. 数据模型

### 3.1 esp_gamification(读+写)

- `leaderboard_snapshots(id, scope_type, scope_id, period, rank, user_id, score, ts)` —— 日切快照。
  - **现状:** 仅 top-100(`snapshotTopN=100`)。
  - **目标态(偏差④):** 全量 rank + total,支持任意用户历史 percentile;`snapshotDay` 改 `ZREVRANGE 0 -1` 全量写(大榜分批),或增量写 `(rank, total)` 两列。

### 3.2 esp_learning(读,共享内核)

- `learning_events(id, message_id UK, user_id, event_type, payload JSONB, created_at)` —— 喂数源。
- `learning_event_outbox(id, user_id, event_type, payload, status default 'pending', retry_count default 0, next_retry_at default now(), created_at, message_id)` —— content-go relay 源表。

### 3.3 Redis 键空间(L2)

```
esp:gamif:lb:{xp|prog}:{global|region|circle|category}:{scopeId}:{daily|weekly|monthly bucket}   # ZSET 主存
esp:gamif:xpc:{uid}:{YYYYMMDD}                # 每日 XP cap 计数(IncrBy,当日 TTL)
esp:gamif:feed:hwm                            # 喂数高水位(持久,重启/多副本共享)
esp:gamif:feed:lock                           # 多副本互斥(TTL 10s)
esp:gamif:day:{YYYYMMDD}:scopes               # 当日已写 ZSET key 集合(日切快照用)
esp:gamif:optout                              # opt-out 集合(L1 源,目标态)
esp:gamif:processed:{msg_id}                  # 目标态:幂等去重(NX EX 86400)
# L1(进程内)键:与 L2 同名空间,GetOrLoad 透明回填
```

### 3.4 esp_auth(读,用户身份 BC)

- `users(id, name, region_code, ranking_opt_out, avatar_url, bio)` —— region_code 形如 `CN/Guangdong/Shenzhen`(斜杠分层,`regionAncestors` 拆祖先前缀)。

### 3.5 esp_community(读,圈子 BC)

- `circle_memberships(circle_id, user_id, role, joined_at)` —— 圈子维度扇出用。

---

## 4. 依赖

- **外部:** crdb-public(esp_gamification 写 / esp_learning+esp_auth+esp_community 读)、redis-cluster(ZSET + 缓存 + 锁 + cap)、pulsar-broker(目标态 CQRS 投影源,见 §8 偏差①)。
- **公共包:** esp-common-go/db(双池)、esp-common-go/redisx、OTel go sdk。
- **跨服务:** content-go(outbox relay 喂数)、bff-java(RouteTable `/api/gamification` 转发,当前已接)、community-go(circle_memberships 读)。
- **OTel:** feed/扇出/读榜 span,trace 串联 content-go relay → gamification fanout。

---

## 5. 部署

- 沿用 `deploy/k8s/32-gamification-go.yaml`,replicas 1(单节点 DEFERRED,目标 3)。HPA CPU>70%。
- content-go: `deploy/k8s/34-content-go.yaml` image `esp/content-go:1.0.5`(坑㉙ relay 已发)。
- Env: `READ_DB_URL`/`WRITE_DB_URL`(crdb-public:26257)、`REDIS_HOST=redis-cluster`、`PULSAR_URL`(目标态)、`OTEL_EXPORTER_OTLP_ENDPOINT`、`CACHE_ENABLED`(默认 true,L1+L2 开关)。
- 探针:/livez /readyz,压测期 timeout 3s(防探针误杀雪崩,见 memory `esp-loadtest-probe-pitfalls`)。
- **单节点 DEFERRED:** HPA maxReplicas 因 metrics-server 缺失未真生效;多节点步骤=扩容后 minReplicas→3 + metrics-server 到位。如实记录于 `tech-debt-register.md`。

---

## 6. 测试要点(对标既有 SDD §6 + 铁律③契约测试)

- **单测(≥80%):** `xpFromEvent`(各事件类型/正确错误/score≥100 加成/cap 截断)、`periodBucket`(daily/weekly/monthly 边界)、`regionAncestors`(CN/Guangdong/Shenzhen 三级)、`categoryOf`、`fanout`(opt-out 跳过/各类 scope 扇出键数)、`relayBackoffSec`(已有)、L1 LRU 命中/淘汰/`Evict` 失效。
- **集成(testcontainers CRDB + miniredis + mock Pulsar):** 灌 learning_events → fanout → ZSET 出数 → 读榜校验;日切 → 快照入库;opt-out 切换 → 写端不扇出(目标态)/读端过滤(现状)。
- **契约测试(铁律③):** ① content-go relay 产出的 `learning_events` 行 schema 与 gamification `fetchEvents` 读取契约一致(字段/类型/JSONB 形状);② bff-java RouteTable `/api/gamification/rankings` 转发契约;③ Redis ZSET key 命名契约(扇出写键 = 读榜读键)。
- **反刷榜测试:** 日 XP cap 超限拒入榜、单事件 XP cap、目标态频率异常检测。
- **幂等测试:** 同 message_id 重复投递 → 不重复计分(目标态 `processed:{msg_id}` SETNX)。
- **E2E(Playwright):** 已有 `report/phase3-ranking-e2e.sh`(curl 链路),补前端 `/ranking` 页面三端视角一致 + opt-out 生效用例。
- **多级缓存测试:** L1 命中率、L2 不可达降级 L3、`Evict` 后 L1 失效。

---

## 7. TOGAF ADM 对标(4 架构域)

| 域 | 阶段 | 排行榜对应产出物 | 等价度 | GAP |
|---|---|---|---|---|
| **BA 业务** | B | 价值流:学习作答→XP 累积→周期榜→差距反馈→分享/挑战 | 70% | 缺独立价值流图(并入 `architecture/value-streams.md`) |
| **DA 数据** | C | ER:learning_events/leaderboard_snapshots/users/circle_memberships;CRUD 矩阵见 §1.3 | 75% | 补 app-data-matrix 排行榜行 |
| **AA 应用** | C | 本 SDD + DDD 限界上下文 + 接口契约 + 应用-数据矩阵 | 80% | DDD 首次引入,需架构评审(§1.2) |
| **TA 技术** | D | 双池读写分离 + L1L2L3 缓存 + Redis Cluster + CRDB + K8s HPA | 85% | 单节点 DEFERRED 记 tech-debt |
| **E 机会方案** | E | 5 项偏差(§8)→ 修正工作包(任务 #204) | - | 本设计即工作包输入 |
| **G 实施治理** | G | 本设计 → #204 审计修正 → 契约测试 → 回填 tech-debt | - | 闭环 |
| **H 变更治理** | H | DDD 引入、轮询→Pulsar 迁移属架构变更,登记 ADR | - | 需补 ADR |

---

## 8. 目标架构 vs 现状:5 项偏差与修正路径(任务 #204 输入)

> 来源:联网核验最优实践(memory `esp-phase3-rankings-bestpractice-audit`,2026-08-17 用 `curl`+`gh api` 直连核验 redis.io/GitHub 可达)。

| # | 现状(已实现) | 目标态(大厂标准) | 修正路径 | 优先级 |
|---|---|---|---|---|
| ① | 轮询 learning_events(id 高水位,2s) | Pulsar 消费 + 幂等 CQRS 投影 | 新增 `rankings_projector.go` 订阅 `esp.business.*`;轮询作降级兜底(因 content 事件不走 Pulsar,需双源) | P1 |
| ② | opt-out 读端过滤( refreshedOptOut 每请求查 DB) | 写端拦截 + L1 缓存 opt-out 集 | `fanout` 已在写端跳过(现状);读端改用 L1 缓存集,移除每请求 DB 查;GDPR 默认不入榜(产品决策) | P1 |
| ③ | 平局按 member 字典序 | 复合 score `xp*1e10 - ts_offset`,XP 相同早达成优先 | `fanout` 改复合 score;读榜解析时还原 XP | P2 |
| ④ | 快照仅 top-100 | 全量 rank/total,percentile 走快照 | `snapshotDay` 改全量;`rankingGap` percentile 用快照非实时 ZCOUNT | P2 |
| ⑤ | 去重靠 Redis 分布式锁(不可靠) | 幂等 message_id + `SET processed:{msg_id} 1 NX EX 86400` | `fetchEvents` 后逐事件 SETNX 去重;锁仅作多副本批互斥 | P1 |
| ⑥(反刷增强) | 仅日 XP cap + 单事件 cap | 频率/设备指纹/可信时间窗/行为序列合理性/risk-score | 新增 `rankings_antifraud.go`,分阶段引入 | P3 |

---

## 9. 风险与诚实备注

- **DDD 首次引入:** 既有服务基线无显式 DDD,本设计的限界上下文/聚合根建模需架构治理委员会评审(architecture-board-charter),通过后回填 `business-capability-map.md`。
- **Pulsar 双源一致性:** content 事件经 CRDB outbox 不走 Pulsar,若 ① 改 Pulsar 投影须保留轮询兜底,否则 content 榜单断喂数(坑㉙ 重现)。
- **单节点约束:** HPA/maxReplicas 未真生效,L1 跨副本一致性暂靠 TTL;多节点扩容后 L1 失效策略需升级(pub/sub 或 Redis Streams 广播 Evict)。
- **联网受限:** 5 项偏差已于 2026-08-17 用 Bash `curl`+`gh api` 直连核验 redis.io(Leaderboards 官方 use case)、redis-developer/basic-redis-leaderboard-demo-go、arpitbbhayani/leaderboard-go-redis、Strava segment API;Duolingo blog 本环境 curl 404 未取得一手原文,leagues 机制基于通用公开知识+HN 讨论。WebSearch/WebFetch 走 claude.ai 域名校验被沙箱拦,Bash 出口正常。
- **内容种子缺口:** 1342 items vs 3400 词汇"丰富起步"目标,记 tech-debt,不影响本设计。
- **本设计为事后补救:** 违反"设计先行"铁律已是事实,本文件 + 任务 #204 审计修正构成闭环;后续阶段4 必须严格设计先行(不再事后补)。
