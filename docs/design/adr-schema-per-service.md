# ADR:Schema-per-Service(按限界上下文拆 schema,不拆 database)

> 状态:Accepted(2026-08-06)
> 关联:探查映射表见本 ADR §3;迁移方案见 §5;替代方案否决见 §6。

## 1. 背景

ESP 现有 29~30 张表全部挤在 TiDB 单一 `esp` 库。表归属探查(2026-08-06)实证 4 处隐性跨服务耦合:

1. **跨服务写同一表**:`learning_events` 被 learning(Java Disruptor)+ emotion(Python persist_emotion)双写;`app_logs` 被 6 服务(bff/learning/gateway-go/writing-py/emotion-svc/teacher)共写。
2. **跨服务直读他人表**:learning 的 `BehaviorController` 推荐查询直接 JOIN gateway 拥有的 `scenarios`+`sessions` 表 + `user_mastery`。
3. **跨服务事件触发写**:teacher 经 Pulsar `teacher.finalized` 事件触发 learning 更新 `user_mastery`(已有 EDA,边界已对齐,非债)。
4. **13 张新表已占坑但无服务**:pronunciation/listening/vocab/assessment/gamification/reading/llm 的表 DDL 已建但服务代码零引用。

**债的核心不是"表多"(TiDB 单库承载 30 张表是小儿科),而是"边界越界"——服务间通过共享库直连他人表形成隐性耦合。**

## 2. 决策

**按限界上下文拆 schema,不拆 database。**

- **拆 schema(逻辑隔离)**:每个服务/限界上下文一个独立 schema,服务只授权自己 schema 的读写 + 公共 schema 的 app_logs 写。跨服务直连他人表在 DB 层被 RBAC 拒绝,从机制上根治隐性耦合。
- **不拆 database(物理隔离)**:单节点 Rancher Desktop + 单 TiDB 集群,物理拆库的收益(独立扩缩容/独立备份/故障爆炸半径/独立运维)在单集群下全部为零,只剩多库连接池 + 多份 RBAC 成本。

## 3. 表 → schema 映射

| schema | 归属服务 | 表 | 备注 |
|---|---|---|---|
| `esp_auth` | bff-java | users | 鉴权主表 |
| `esp_learning` | learning-java | user_mastery, knowledge_points, learning_events*, session_turns | *learning_events 见 §4 治理后归 learning 独占 |
| `esp_speaking` | gateway-go | scenarios, sessions | 会话与场景入口 |
| `esp_writing` | writing-py + writing-coach | writing_tasks, writing_templates, writing_essays, writing_plans, writing_progress | 写作域 |
| `esp_teaching` | teacher-java | classes, assignments, submissions, lesson_plans | 教师域 |
| `esp_shared` | 公共 | app_logs | append-only 公共写表(Fowler 认可非反模式) |
| `esp_pronunciation` | pronunciation-rs(待落地) | pronunciation_attempts | 新服务窗口期 |
| `esp_listening` | listening-rs(待落地) | listening_exercises, listening_attempts | 新服务窗口期 |
| `esp_vocab` | vocab-srs-go(待落地) | vocabulary, user_vocabulary | 新服务窗口期 |
| `esp_assessment` | assessment-rs(待落地) | assessment_sessions, assessment_answers | 新服务窗口期 |
| `esp_gamification` | gamification-go(待落地) | user_streaks, achievements, user_achievements | 新服务窗口期 |
| `esp_reading` | reading-go(待落地) | reading_passages, reading_attempts | 新服务窗口期 |
| `esp_llm` | llm-orchestrator-rs(待落地) | llm_call_logs | 新服务窗口期 |

## 4. 4 处强耦合治理

| 耦合 | 治理方式 | 代价 |
|---|---|---|
| `user_mastery`(learning 写 + teacher 事件触发) | **保持 learning 独占**,teacher 只经 Pulsar 事件通知(现有 EDA 已实现)。teacher 需要时走 Dubbo Triple RPC 查 learning | 零代码改,边界对齐已有事件流 |
| `learning_events`(learning + emotion 双写) | **方案 A**:归 `esp_learning`,emotion 写改 Pulsar 事件 `emotion.persisted`→learning 消费落库,emotion 不再直写 | emotion 改发事件(ESP Pulsar EDA 已就绪),根治跨语言双写 |
| `app_logs`(6 服务共写) | 归 `esp_shared`,6 服务保留写权限 + learning 读聚合 | 共享写非反模式(Fowler 认可),保持现状 + RBAC 限定 |
| `scenarios`/`sessions` 跨服务 JOIN | scenarios/sessions 移入 `esp_speaking`(gateway 独占);learning 推荐查询改调 gateway RPC 取数据,内存聚合 | 1 次 SQL JOIN → 2 次 RPC + 内存聚合,延迟略升;推荐非核心链路可接受 |

## 5. 迁移方案(分批,低风险)

### Phase 0(零风险窗口,优先做)
13 张新服务表按 7 schema 建(`CREATE SCHEMA esp_pronunciation` 等 + 建表)。新服务代码落地时直接连自己 schema。**窗口期成本最低,收益最大。**

### Phase 1(老表,分批在线迁移)
TiDB `RENAME TABLE esp.users TO esp_auth.users` 等逐表迁移(TiDB RENAME TABLE 在线操作,秒级不锁表)。每 schema 迁完改对应服务连接串(`WRITE_DB_SCHEMA`/`READ_DB_SCHEMA` env)+ RBAC。按 bff→gateway→writing→teacher→learning 顺序,每批一个服务验证。

### Phase 2(耦合治理,代码改)
- emotion 写 learning_events 改 Pulsar 事件 `emotion.persisted`→learning 消费。
- learning 推荐查询改 RPC 聚合(调 gateway `/api/scenarios/{id}` + `/api/sessions/user/{uid}`)。
- 串行做,与集群操作串行(单节点铁律)。

### Phase 3(多节点上线时评估)
评估是否把 `esp_learning`(学情高频写)拆成独立 TiDB database,享受物理隔离收益。单节点不做。

## 6. 替代方案否决理由

| 方案 | 否决理由 |
|---|---|
| **database-per-service(物理拆库)** | 单节点 RD + 单 TiDB 集群,物理隔离收益全为零,只剩运维成本。多节点上线时再评估 Phase 3。 |
| **保持共享库不动** | 4 处隐性耦合不治,新服务继续往单库堆表,债持续累积。13 张新表窗口期是治理最佳时机。 |
| **按表数机械拆(每服务一张表)** | 不对齐限界上下文,DDD 边界混乱。应以领域边界而非表数划分。 |
| **引入 Vitess/中间件分库** | TiDB 本身分布式 HTAP,跨 schema JOIN 零成本,无需中间件。引入只增复杂度。 |

## 7. 关键工程细节

- **TiDB 跨 schema JOIN**:`SELECT ... FROM esp_learning.user_mastery u JOIN esp_speaking.scenarios s ...` 原生支持,零成本。报表/分析查询不受 schema 拆分影响。这是选 schema 拆而非 database 拆的工程底气。
- **RBAC**:每服务建独立 DB 用户,只授权自己 schema 的读写 + `esp_shared.app_logs` 写。跨服务直连他人表在 DB 层被拒,从机制根治隐性耦合(比文档约定可靠)。
- **读写分离不受影响**:tidb-primary/tidb-follower ExternalName 不动,只是连接串加 schema 参数。
- **单节点铁律**:RENAME TABLE 在线但不并发,串行迁移;Phase 2 代码改与集群操作串行。

## 8. 行业依据

- [microservices.io - Database per service](https://microservices.io/patterns/data/database-per-service.html):database-per-service 是理想态。
- [Martin Fowler - MicroserviceDatabase](https://martinfowler.com/bliki/MicroserviceDatabase.html):共享库在"小团队 + 强事务 + 报表密集"时是合理务实选择,非反模式。
- [Martin Fowler - BoundedContext](https://martinfowler.com/bliki/BoundedContext.html):库的边界应对齐 DDD 限界上下文。
- Sam Newman《Building Microservices》:schema-per-service 享受隔离 80% 收益,物理拆库为最后一步。
