# ADR:数据库逻辑分库(Database-per-Service 逻辑层)

> 状态:Proposed(2026-08-06)
> 关联:`adr-new-services.md`、`02-系统架构设计文档-SAD.md`、`多区域数据同步方案.md`
> 触发:schema.sql 增至 30 张表(8 旧域 + 7 新服务域),用户要求评估"30 表是否正常"并给架构决策+重构计划。

## 1. 结论先行

- **30 张表本身正常**(11 服务 × 平均 2.7 表/服务,粒度合理;对标 Duolingo 单域即 10-20 表)。
- **真正问题:30 表同处一个 `esp` 库,违反 Database-per-Service 原则**,是工业级基线下的架构债。
- **决策:P0 不动(不阻塞 7 新服务 Phase6 落地);P1~P4 实施"逻辑分库 + 物理共集群"**,按域拆 TiDB database,服务只跨库读不跨库 Join。

## 2. 现状与问题

| 维度 | 现状(单库 esp,30 表) | 问题 |
|---|---|---|
| 服务边界 | 11 服务共享一库 | 跨服务 Join 诱惑,边界模糊 |
| 扩缩容 | learning_events(30亿行)膨胀牵连小表 | 无法按域独立分片/TiFlash |
| 故障域 | 单库 DDL/大事务影响全服务 | blast radius 过大 |
| 多区域同步 | 全库同步 | 带宽/冲突成本高(热表 learning_events 被迫与冷表 reading_passages 同步) |
| 安全 | 任一服务有全库读权限 | 越权风险 |

## 3. 方案抉择

### 方案 A:物理拆多 TiDB 集群(每域独立集群)
- **否决**:单节点 RD 资源不允许;多节点也建议先共集群降运维成本;TiDB 集群本身支持海量库表。

### 方案 B:逻辑分库 + 物理共集群(采纳)
- 同一 TiDB 集群内按域建独立 database,`RENAME TABLE esp.X TO esp_<domain>.X`(在线 DDL,瞬间元数据变更)。
- 跨库读零成本(同 TiKV),但禁止应用层跨库 Join,强制走服务 API。
- 多区域按 database 粒度配 Placement Rules。

### 方案 C:维持单库
- **否决**:架构债持续累积,多区域/扩缩容/故障域无解。

## 4. 分库设计(12 个逻辑 database)

| database | 服务 | 表 |
|---|---|---|
| `esp_identity` | bff/gateway | users |
| `esp_learning` | learning-java | sessions, session_turns, knowledge_points, user_mastery, learning_events |
| `esp_writing` | writing-py, writing-coach | writing_tasks, writing_templates, writing_essays, writing_plans, writing_progress |
| `esp_teacher` | teacher-java | lesson_plans, classes, assignments, submissions |
| `esp_pronunciation` | pronunciation-rs | pronunciation_attempts |
| `esp_listening` | listening-rs | listening_exercises, listening_attempts |
| `esp_vocab` | vocab-srs-go | vocabulary, user_vocabulary |
| `esp_assessment` | assessment-rs | assessment_sessions, assessment_answers |
| `esp_gamification` | gamification-go | user_streaks, achievements, user_achievements |
| `esp_reading` | reading-go | reading_passages, reading_attempts |
| `esp_llm` | llm-orchestrator-rs | llm_call_logs |
| `esp_observability` | 全服务 | app_logs |

## 5. 重构计划(分阶段,不阻断当前 7 新服务)

| 阶段 | 动作 | 风险 | 时机 |
|---|---|---|---|
| **P0** | 30 表保留单库 esp,7 新服务先落地(用户要 Phase6 部署) | 无 | 当前轮 |
| **P1** | 新建 12 database,`RENAME TABLE esp.X TO esp_<domain>.X`(TiDB 跨库 RENAME,在线 DDL) | 低 | 新服务稳定后 |
| **P2** | 各服务连接串改 `database=esp_<domain>`;公共包加 `DB_SCHEMA` env;重建镜像 | 中 | P1 验证后 |
| **P3** | 删 esp 库残留;TiDB Placement Rules 按域配副本/TiFlash(learning_events 多副本+列存) | 中 | 多节点环境 |
| **P4** | 跨域 Join 审计(grep 跨库 SQL),强制改服务调用 | 中 | P3 后 |

## 6. 读写分离与多区域影响

- 读写分离不变:每库共用 tidb-primary 写 / tidb-follower 读。
- 多区域:按 database 粒度同步(热表 esp_learning 高频同步,冷表 esp_reading 单区),较全库同步省带宽/降冲突。
- Seata 分布式事务:跨库事务仍走 Seata AT(undo_log 已建),分库不破坏现有事务链。

## 7. 决策

**采纳方案 B**。P0 不动以保 7 新服务 Phase6 节奏;P1~P4 作为后续重构项排期,优先级低于 7 新服务上线与多节点 5000CCU 压测。本 ADR 作为"技术债已识别 + 路径明确"的备案,不立即执行。
