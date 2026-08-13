# G2 — ADR 索引与编号规范(独立副本)

> TOGAF ADM Phase G(实施治理)P0 补救产出物。
> 本文件是 ADR 索引的独立 Single Source of Truth(与 `architecture-board-charter.md` §G2 内容一致,单独成文便于检索)。
> 日期:2026-08-13。

## 编号规则

- 格式 `ADR-XX`(两位数,01 起,永久不回收)。
- 文件命名 `docs/design/adr-<kebab-topic>.md`。
- 状态:Proposed / Accepted / Superseded / Deprecated。
- Superseded 必注明替代编号,保留原文不删。

## ADR 索引(15 份)

| 编号 | 标题 | 文件 | 状态 | 关键决策 | 日期 |
|---|---|---|---|---|---|
| ADR-01 | Disruptor 学习事件 RingBuffer + worker 拆分 | (SAD 引用) | Accepted | LMAX Disruptor 削峰 + P2-2 worker 独立 Deployment | 会话22 |
| ADR-02 | 数据库逻辑分库 | adr-db-logical-sharding.md | Accepted | 限界上下文拆 schema 不拆 database | — |
| ADR-03 | Schema-per-Service | adr-schema-per-service.md | Accepted | 13 schema 隔离 | — |
| ADR-04 | GitOps ArgoCD | adr-gitops-argocd.md | Accepted | ArgoCD + drift 自愈 + ignoreDifferences | — |
| ADR-05 | 新增服务矩阵(10 服务) | adr-new-services.md | Accepted | Rust4 + Go6 | — |
| ADR-06 | Seata AT → Saga | adr-seata-at-to-saga.md | Accepted | 跨服务事务避全局锁 | — |
| ADR-07 | 安全扫描四件套 | adr-security-scanning.md | Accepted | Semgrep+Trivy+Dependabot+Aikido | — |
| ADR-08 | BFF vs SCG vs Dubbo Triple | (SAD §8) | Accepted | 南北 BFF + 东西 Dubbo Triple | — |
| ADR-09 | FastAPI → Litestar+Granian | (SAD) | Accepted | AI 异步 + Granian | — |
| ADR-10 | writing-coach | adr-writing-coach.md | Accepted | 写作增强聚合 | — |
| ADR-11 | Redis Cluster 迁移 | (redis-cluster-migration-plan.md) | Accepted | 6主6从 + hostname + 各区独立 | — |
| ADR-12 | langgraph 0.2→1.x | (SAD/writing-py) | Accepted | StateGraph/add_edge/MemorySaver | P1-3 |
| ADR-13 | WS 广播分片 shard=8 | (gateway-go hub.go) | Accepted | fnv1a + N=8 + Redis Pub/Sub | P2-1 |
| ADR-14 | 缓存 key 三端统一 | (多级缓存架构对齐.md) | Accepted | `{service}:{entity}:{id}` 明文 | P2-5 |
| ADR-15 | follower lag 防读后写 | (learning-java) | Accepted | @DataSource(consistent=true)+乐观锁 | P2-6 |

## 维护

- 新 ADR 接续编号(下一个 ADR-16),本表追加。
- 状态变更原地更新,不删行。
- 与 `docs/design/adr-*.md` 双向校验,本索引为 SSoT。
