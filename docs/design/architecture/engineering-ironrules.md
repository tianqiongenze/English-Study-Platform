# ESP 工程铁律(所有项目强制遵守)

> 本文件是 ESP 及所有派生工程的工程宪法,架构设计、编码、评审、落盘、跨会话恢复一律以此为准。
> 全局记忆副本:`~/.claude/projects/*/memory/engineering-ironrules.md`;Redis 锚点:`esp:ctx:ironrules:engineering`。
> 本项目 CLAUDE.md 与用户全局 ~/.claude/CLAUDE.md 均引用本文件。

---

## 0. 为什么有这份铁律

从 22+ 个会话的实测暴露出一类**系统性缺陷**:设计了没落地、配置了没接线、公共能力在服务里复制、声称完成但无产物。不是偶发 bug,是**工程机制缺失**。最典型证据:

| # | 现象 | 证据 |
|---|---|---|
| 1 | GlobalExceptionHandler 在 bff/learning/teacher **复制 3 份**(envelope 同构,仅 basePackages 不同) | `backend/services/{bff,learning,teacher}-java/.../GlobalExceptionHandler.java` |
| 2 | Dubbo Triple 三服务 `application.yml` 全配 + Nacos registry,但 bff **零 `@DubboReference`**,全链路走同步 RestClient | 架构评审 report/architecture-review-2026-08-15.md 定性"架构债 P0" |
| 3 | learning-worker 拆分设计文档齐全(P2-2),但 worker `WRITE_DB_URL` 指向 `esp` 老库(100M 行)而非 `esp_learning`,deploy replicas=0 | `deploy/k8s/21-learning-worker.yaml`(已修) |
| 4 | `learning_event_outbox` 先建错 schema(esp 而非 esp_learning),TiDB 报"bad SQL grammar" | 会话22-8 修复记录 |
| 5 | README 称 `esp.rw.storage.type` 是"reserved 未实现",实际 #132 已实现 | 文档与代码漂移 |
| 6 | 本会话前期"声称 #131/#132/#133 完成"但核验无产物 | 会话22-32 记录 |

---

## 1. 根因分析(为什么设计与实现会脱节)

| 根因 | 机制解释 | 直接后果 |
|---|---|---|
| **R1 设计无追踪物** | 设计文档写完即终结,没有"设计项→代码覆盖→验证"三态跟踪,没有逐项校验步骤。"配置加进 yml" 就算做了,实际 wiring 无人验证 | Dubbo 配而未接、outbox 落错库、worker 指向老库,全部事后才发现 |
| **R2 无共享包优先治理** | 服务内就地复制比抽公共包"快",没有门禁阻止复制,评审未在合并前拦截 | GlobalExceptionHandler×3、Jwt 散落、DB 池各自 hardcode |
| **R3 验收缺契约测试** | 接线类功能(Dubbo/Pulsar/outbox/读写分离)没有"链路通"的测试证据,靠"看起来能跑"放行 | integration-tests 仅 4 个 IT,大量接线未经证明 |
| **R4 单节点限制被静默吞掉** | worker 多副本/CRDB 全量/Dubbo wire 在单节点做不了,没有显式标 DEFERRED+原因+多节点步骤,后续会话误以为已完成 | 功能"设计了但没落地"且无人知道 |
| **R5 会话碎片化 + 无产物声称** | 22 个会话靠 PROGRESS/Redis 恢复,但恢复锚点没强制"声称完成必须附产物(文件+测试)" | 声称完成≠真完成,跨会话状态失真 |
| **R6 零 CI 门禁** | 无构建期依赖分析/重复检测/覆盖率门禁 | 设计债只能靠人评审,机器不拦截 |

---

## 2. 解决方案(一一对齐根因)

| 方案 | 对齐根因 | 落地物 |
|---|---|---|
| S1 设计项三态追踪 | R1 | 每设计项带状态 DESIGNED→IMPLEMENTED→VERIFIED,落盘含证据;新功能 PR 必须过一遍 |
| S2 公共包优先 + 依赖治理 | R2 | 公共能力先下沉共享包;服务依赖矩阵文档化;禁止服务内复制公共能力 |
| S3 契约测试门禁 | R3 | 接线类功能必须有集成测试证明链路;integration-tests 扩展 |
| S4 显式 DEFERRED | R4 | 单节点做不了的标注 DEFERRED+原因+多节点移植步骤,双轨落盘 |
| S5 无产物不声称完成 | R5 | 声称完成必须附:文件路径 + 测试结果;恢复锚点三态校验 |
| S6 CI 门禁 | R6 | ArchUnit 架构约束测试 + 覆盖率≥80% + 重复代码检测 |

---

## 3. 铁律(强制,所有项目)

### 铁律一:公共能力共享包优先
- 任何被 ≥2 个服务共用的能力(异常处理/错误码/鉴权/JWT/健康检查/追踪/事件发布/幂等/读写分离/缓存/DB 连接池),**必须先下沉共享包,再被服务依赖引用**。
- 禁止在服务内复制公共能力。新增类似 `GlobalExceptionHandler` 的重复实现 = 架构违规。
- Java 共享包规范:`platform/esp-web-common`(web 通用:全局异常+错误 envelope)、`esp-rw-java`(DAL+读写分离+存储选型)、`esp-cache`(缓存/事件/幂等)、`esp-disruptor`(调度队列)、`esp-api`(跨服务契约)。
- 共享包必须自带单测(覆盖率≥80%),零单测的共享包禁止发布。
- 依赖治理:服务 pom/清单显式声明共享包版本;禁止使用各自实现绕过共享包。

### 铁律二:设计-实现-验证三态追踪
- 每个设计项(ADR/P2 项/优化项)必须有状态机:DESIGNED → IMPLEMENTED → VERIFIED。
- 状态迁移必须落盘:DESIGNED(设计文档)→ IMPLEMENTED(代码文件路径)→ VERIFIED(测试/压测/冒烟证据)。
- 不允许存在"配置了但没接线"的隐形状态 —— 配置(Dubbo/Nacos/env)与代码引用必须双向核对。
- 新会话恢复时,对 PROGRESS 中每个 IMPLEMENTED/VERIFIED 项先核验产物存在,再继续。

### 铁律三:契约测试证明接线
- 跨服务/跨组件接线(Dubbo Triple/Pulsar/outbox/读写分离/多级缓存/存储选型)必须有集成测试证明"链路通"。
- 每新增一条接线,必加一条 IT/E2E:集成到 `integration-tests`(Java IT)或对应语言契约测试。
- 压测声称的吞吐/CCU 数字必须可复现(报告含脚本+日期+环境),禁止凭印象写数字。

### 铁律四:无产物不声称完成
- 声称"完成/闭合/✅"必须附两类产物:①改动文件路径;②验证证据(测试 RC、压测 JSON、curl 输出)。
- 无产物的一律视为未完成;跨会话恢复时"声称完成"仅作线索,以产物核验为准。
- 任务清单(PROGRESS/TaskList)与产物必须一致,不一致时以产物为准并修正记录。

### 铁律五:单节点限制显式 DEFERRED
- 受单节点 Rancher Desktop 约束而无法落地/验证的项(多副本、CRDB 全量、Dubbo wire、异地多活),必须显式标注 **DEFERRED-单节点限制 + 原因 + 多节点移植步骤**,写入 PROGRESS 与 Redis 锚点。
- 禁止静默省略:未落地≠已完成,必须可见、可追溯、有 owner。

### 铁律六:CI 门禁
- 所有共享包与业务服务必须有自动化门禁(可先本地脚本,后接 CI):架构约束测试(ArchUnit 禁止重复实现/禁止反向依赖)、单元测试覆盖率≥80%、重复代码检测。
- 关键服务(learning-java/gateway-go/bff-java)覆盖率不达标禁止上线。

### 铁律七:双轨落盘(引用既有)
- 上下文达 10% 起、每里程碑,更新 `docs/PROGRESS.md` + Redis `esp:ctx:*` 锚点,两源互为恢复。
- 恢复锚点必含:当前精确子步骤 + 已确认根因 + 已改文件 + 下一步 + **本会话"设计项三态"快照**。

### 铁律八:共享基础设施红线(引用既有)
- Redis/Pulsar/TiDB/Neo4j/Milvus/CRDB 是多项目共用底座,禁止 scale down/删除/破坏;高风险操作前口头提示。

### 铁律九:上下文工程(引用既有)
- 80% 主动 /compact、90% 强制止损;subagent 隔离读多/跑长;输出限流;文件系统=长期记忆。

### 铁律十:诚实优先
- 一切落盘内容(进度/报告/声称)以"当前可核验事实"为准;对未知明确说未知,对未做明确说未做,宁可暴露缺口也不粉饰。
- 审查报告的评分、压测数字、覆盖率必须真实,不许四舍五入到"达标"。

---

## 4. 证据附录

- 架构审查报告:`report/architecture-review-2026-08-15.md`(综合 5.2/10,Top5 重构建议)
- 全服务应共享/未接线代码扫描清单:`docs/design/architecture/shared-code-audit-2026-08-15.md`(由 #138 扫描产出)
- 重构计划:`~/.claude/plans/indexed-honking-spring.md`(P0~P2 19 项,三波次)
- 本铁律全局副本:memory `engineering-ironrules`(feedback 类型)
