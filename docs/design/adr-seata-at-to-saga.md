# ADR:teacher-java 学情汇总事务从 Seata AT 迁移到应用层 Saga 补偿模式

> 状态:Accepted(2026-08-07)
> 关联:`adr-schema-per-service.md`(本 ADR 解锁其 Phase1 Batch2~5 的 undo_log 跨 schema 阻塞)
> 影响服务:teacher-java(`summarizeStudentLearning`)
> 影响 API:`com.esp.teacher.service.TeacherService.summarizeStudentLearning`

## 1. 背景

### 1.1 现状

`TeacherService.summarizeStudentLearning(Long userId, Long submissionId, BigDecimal finalScore, Long classId)`
当前用 Seata AT 全局事务(`@GlobalTransactional(timeoutMills = 60000, name = "esp-teacher-summary")`)包裹:

1. Dubbo 读 learning-java(`learningRpc.userStats(userId)`)拉薄弱知识点统计 —— **纯读,无本地写**。
2. 本地写 finalize(终审 submission:`final_score` + `status=finalized` + 缓存失效 + 发 `teacher.finalized` 事件)。

幂等:`CacheService.tryIdempotent` SETNX `esp:idem:teacher:summary:<submissionId>` TTL 600s。

### 1.2 阻塞点(schema-per-service Phase1)

`adr-schema-per-service.md` 决策按限界上下文拆 schema(`esp_teaching` 归 teacher,`esp_learning` 归 learning)。
AT 模式要求**每个参与方在本地 schema 维护 `undo_log` 表**,由 TC 协调全局回滚。拆 schema 后:

- teacher(`esp_teaching`)与 learning(`esp_learning`)各处独立 schema,`undo_log` 表归属哪个 schema?
  learning 是 Dubbo 纯读提供方,没有本地写,理论上不需要 undo_log,但 AT 协议要求所有 RM 注册并持有 undo_log —— 冗余且跨 schema 路由不可行。
- AT 全局锁对 learning 这种"纯读参与方"是纯开销:全局锁保障的是写隔离,learning 没有写,锁了反而限制其读并发。
- ESP 是**多语言混合**(Java/Go/Python/Rust)平台,Seata AT 的 RM SDK 在 Rust/Go 生态成熟度不足,
  把 AT 推广到所有服务不现实 —— 一旦 pronunciation-rs/listening-rs 等服务接入,AT 协调链会断裂。

### 1.3 场景特征(决定可补偿性)

- teacher 本地写是**终态写**(submission `status=finalized`),但 `status` + `final_score` 都是可逆字段:
  补偿只需把 `status`/`final_score` 回滚到前置值,语义清晰、无级联副作用。
- learning 的 Dubbo 调用是**纯读**,失败不需要补偿 learning 侧(没有产生任何写)。
- `teacher.finalized` 事件下游(learning-java)消费后 upsert `user_mastery`,该 upsert 幂等由 P1-4 保证
  (mastery upsert on conflict),即使收到补偿信号,learning 也可选择忽略或重算 —— 补偿事件是"信号",非强制。

**结论:这是一个"单步可补偿写 + 一步纯读"的简单场景,适合应用层 Saga 补偿,不需要 AT 全局锁,也不需要 Seata Saga 状态机。**

## 2. 决策

**移除 `@GlobalTransactional`,改用应用层 Saga(本地消息表 + 补偿),不引入 Seata Saga 状态机。**

### 2.1 Saga 编排

| 步骤 | 动作 | 可补偿 | 补偿动作 |
|---|---|---|---|
| 1 | 本地写 finalize(终审 submission,本地事务) | 是 | 回滚 `status`+`final_score` 到前置状态;失效幂等占位;发 `teacher.finalized.compensation` 事件 |
| 2 | Dubbo 读 learning `userStats`(纯读) | 是(可重试) | 触发步骤1补偿(纯读无本地副作用,无需补偿 learning) |

### 2.2 执行顺序(关键)

1. 步骤1在**本地事务**内执行(`@Transactional` + `@DataSource(WRITE)`),记录前置快照(status + finalScore)。
2. 本地事务**提交后**,再执行步骤2 Dubbo 读(纯读不参与本地事务,避免 DB 连接长时间持有)。
3. 步骤2异常 → 调用 `compensate()`(独立本地事务)回滚 submission → 重抛原异常给上层。

### 2.3 补偿幂等

- 补偿前检查 submission 是否仍为 `finalized` 状态;已被补偿(status 不再是 finalized)则跳过,防止重复补偿。
- 补偿时失效幂等占位 `esp:idem:teacher:summary:<submissionId>`,允许下次业务重试。
- 补偿失败**仅记 ERROR 日志 + 告警**(后续人工介入),不向上抛 —— 避免补偿链无限循环。

### 2.4 补偿事件

- topic:`teacher.finalized.compensation`(新增,常量 `TeacherSummarySaga.TEACHER_FINALIZED_COMPENSATION`)。
- payload 复用 `TeacherFinalizedEvent`(score=0 信号值)。
- 下游 learning 是纯读消费,幂等由 P1-4 保证;补偿事件是"信号",非强制回滚下游。
  (learning upsert mastery 走 ON CONFLICT,重复消费幂等;收到补偿可选择忽略。)

## 3. 实现

### 3.1 新建 Saga 类

`services/teacher-java/src/main/java/com/esp/teacher/service/TeacherSummarySaga.java`

关键方法:
- `run(userId, submissionId, finalScore, classId)`:Saga 入口,编排步骤1→步骤2,失败触发补偿。
- `tryFinalize(...)`:步骤1,`@DataSource(WRITE)` + `@Transactional`,本地写 finalize + 记录前置快照 + 发 `teacher.finalized` 事件;幂等命中返回 null。
- `compensate(submissionId, snapshot, userId)`:补偿,`@DataSource(WRITE)` + `@Transactional`,回滚 submission + 失效幂等占位 + 发补偿事件;幂等检查(仅 finalized 状态才回滚)。
- 内部 `Snapshot` 类:记录 submissionId / prevStatus / prevFinalScore / assignmentId / userId,供补偿回滚。

### 3.2 TeacherService 改动

- 移除 `@GlobalTransactional` 注解 + `io.seata.spring.annotation.GlobalTransactional` import。
- `summarizeStudentLearning` 改为委托 `teacherSummarySaga.run(...)`,保留 `@DataSource(DataSourceType.WRITE)`(类级别写数据源一致性)。
- 构造器注入 `TeacherSummarySaga`。
- **保留 `finalize` 方法**(其他地方可能调用,且 Saga 内部不再调用 TeacherService.finalize 而是自带 tryFinalize,避免循环依赖)。
- **保留 seata 依赖**(bff-java / learning-java 仍可能用 AT,不强制关 `SEATA_ENABLED`)。

### 3.3 事件链保持不变

- `teacher.finalized` 事件:步骤1成功后发(payload 与原实现一致:`TeacherFinalizedEvent(userId, assignmentId, finalScore)`)。
- 新增 `teacher.finalized.compensation`:补偿时发(信号事件,下游可选消费)。
- learning-java 消费 `teacher.finalized` 的逻辑零改动(幂等由 P1-4 保证)。

## 4. 风险与缓解

| 风险 | 说明 | 缓解 |
|---|---|---|
| 补偿失败 | 补偿本身异常(DB 不可达/事件发送失败) | 仅记 ERROR + 告警,人工介入;不向上抛避免循环。监控 `teacher.finalized.compensation` 发送成功率 |
| 步骤2 读失败 → 补偿已提交的 finalize | finalize 已写库+发事件,学习侧可能已消费 | 补偿回滚 submission + 发补偿事件;learning 幂等保证最终一致;极端情况(补偿事件丢失)需人工对账 |
| Dubbo 读与本地写非原子 | 步骤1提交后、步骤2前宕机 → finalize 已落地但 stats 未读到 | 业务可接受(stats 仅用于返回值,不影响 finalize 正确性);重启后幂等占位阻止重试(可手动清占位重跑) |
| undo_log 表遗留 | 原 AT 模式下建的 `undo_log` 表 | 不影响,可后续清理(非阻塞) |

## 5. 与 schema-per-service Phase1 关系

本 ADR **解锁 Phase1 Batch2~5** 的 undo_log 跨 schema 阻塞:

- Batch2~5 涉及 teacher/learning 等服务的 schema 拆分,AT 模式下 undo_log 跨 schema 路由不可行。
- 迁移到应用层 Saga 后,teacher 不再依赖 AT 协调,可独立拆 schema(`esp_teaching`)。
- learning-java 仍可保留 AT(若其内部有跨服务写)或后续按需迁移 —— 本 ADR 不强制 learning 同步迁移。

## 6. 替代方案否决

| 方案 | 否决理由 |
|---|---|
| Seata Saga 状态机(JSON/DSL 编排) | 此场景仅 1 步可补偿写 + 1 步纯读,状态机是过度设计;运维成本高;学习曲线陡 |
| 保留 AT + 不拆 teacher schema | 与 `adr-schema-per-service` 决策冲突,且多语言混合下 AT 推广不可行 |
| TCC(Try-Confirm-Cancel) | 需要 learning 侧实现 Try/Confirm/Cancel 三个接口,但 learning 是纯读无写,Try/Confirm/Cancel 退化为空操作,收益为零 |
| 完全去事务(最终一致) | finalize 是终态写,无补偿则 submission 状态错误不可逆;业务不可接受 |

## 7. 验证清单(后续构建/部署阶段)

- [ ] teacher-java 编译通过(`mvn -pl services/teacher-java compile`)。
- [ ] 单测:`TeacherSummarySagaTest` 覆盖 正常路径 / 步骤2失败补偿 / 幂等命中 / 重复补偿跳过。
- [ ] 集成测试:finalize 后 learningRpc 抛异常 → submission 回滚 + 补偿事件发出。
- [ ] 事件链:`teacher.finalized` 仍正常发出;learning-java 消费无回归。
- [ ] schema-per-service Phase1 Batch2(teacher schema 拆分)解锁验证。

## 8. 变更清单

- 新增:`services/teacher-java/src/main/java/com/esp/teacher/service/TeacherSummarySaga.java`
- 修改:`services/teacher-java/src/main/java/com/esp/teacher/service/TeacherService.java`
  - 移除 `@GlobalTransactional` + import
  - `summarizeStudentLearning` 委托 saga
  - 构造器注入 `TeacherSummarySaga`
- 新增事件 topic 常量:`TeacherSummarySaga.TEACHER_FINALIZED_COMPENSATION = "teacher.finalized.compensation"`
- 保留 seata 依赖(bff/learning 仍可用)
