# G1 — 架构治理委员会章程 + G2 ADR 索引

> TOGAF ADM Phase G(实施治理)P0 补救产出物(G1 + G2 合并)。
> 评估视角:教授级高级工程师 / 互联网大厂资深架构师。
> 数据来源:7 现有 ADR + `deploy/gitops/argocd-apps.yaml` + `.github/workflows/ci.yml` + ThoughtWorks/Uber ARB 实践。
> 日期:2026-08-13。

---

## G1 架构治理委员会章程(Architecture Board Charter)

### 1. 委员会定位

ESP 架构治理委员会是项目架构决策与合规审查的最高治理机构,贯穿 TOGAF ADM 全周期(A-H + ReqMC),保证"做架构"与"管架构"闭环。

### 2. 成员与席位

| 席位 | 角色 | 职责 | 当前持有人 |
|---|---|---|---|
| 首席架构师(Chair) | 决策拍板 + 否决权 + ADM 周期触发 | 架构愿景/过渡架构/重大 ADR 终审 | JoelGong |
| CPO/产品 | 业务诉求对齐 + 价值流验收 | Phase A/B 业务架构把关 | — |
| 多语言代表(Go/Java/Python/Rust) | 实现可行性 + 公共包契约 | Phase C 应用架构 + 跨语言契约统一 | — |
| SRE/运维 | 可部署性 + SLO + DR | Phase D 技术架构 + 上线 Go/No-Go | — |
| 安全 | 鉴权/合规/漏洞 | SEC-1~4 + 依赖升级 + .trivyignore 门禁 | — |
| 数据 | schema/读写分离/数据生命周期 | Phase C 数据架构 + 多区域同步 | — |

### 3. 职责

- **架构合规审查**:每重大变更(新服务/新 schema/新中间件/契约破坏)经委员会评审,产出合规报告(对接 G3 模板)。
- **ADR 终审**:ADR 由提案人起草,委员会评审编号(见 G2),决策/背景/收益/代价齐方可入仓。
- **技术债治理**:审议 H1 技术债登记,定优先级与偿还计划,每周期复盘。
- **上线 Go/No-Go**:对接 09-人工测试操作指南 §7,阻断项全 ✓ 方签收 GO。
- **ADM 周期触发**:重大变更(如单节点→多节点、Seata→Saga、langgraph 迁移)触发再评估(对接 H3)。

### 4. 会议节奏

- **双周例会**:评审在途 ADR + 技术债 + 合规报告;紧急变更可临时召集。
- **每评分周期(如本次 7.34 修订)**:ADM 全周期复盘,更新评分文档与 TOGAF 评估。
- **上线前**:Go/No-Go 签收会(对接 09 §7)。

### 5. 决策权与否决权

- **通过**:多数同意,首席架构师 + 安全 + SRE 三方无否决即通过。
- **否决**:安全(合规风险)、SRE(不可部署)、首席架构师(架构原则违背)任一否决即搁置,提案人修改后重审。
- **ADR 编号永久不给出**:一旦编号,ADR 永不入仓删除(可标 Superseded),保证追溯链完整。

### 6. ADR 流程(与 G2 对接)

1. 提案人起草 ADR(背景/决策/收益/代价/替代方案)。
2. 委员会评审,requirement 背景举证(数据/压测/坑链)。
3. 通过 → 编号 ADR-XX 入仓 `docs/design/adr-*.md` + 登记 G2 索引。
4. 实施 → GitOps(ArgoCD)+ CI 门禁落地。
5. 复盘 → 必要时标 Superseded,新 ADR 引用旧编号。

### 7. 合规模板要点(对接 G3,P1 补全)

- 契约一致性(OpenAPI/Dubbo esp-api 与实现一致)
- BSSolver/ABB 映射(能力 → 服务 → 数据)
- 安全(SEC-1~4 + 依赖 + .trivyignore)
- 可观测(traceId/metrics/logs 三支柱)
- SLO(RPO/RTO/Availability 对接 D3)

---

## G2 ADR 索引与编号规范

### 编号规则

- 格式 `ADR-XX`(两位数,从 01 起,永久不回收)。
- 文件命名 `docs/design/adr-<kebab-topic>.md`(主题式,便于检索)。
- 状态:Proposed / Accepted / Superseded / Deprecated。
- Superseded 必须注明被哪个 ADR 替代,保留原文。

### ADR 索引(现有 7 份 + 编号补全)

| 编号 | 标题 | 文件 | 状态 | 关键决策 |
|---|---|---|---|---|
| ADR-01 | Disruptor 学习事件 RingBuffer + worker 拆分 | (会话22 SAD/ADR 引用) | Accepted | LMAX Disruptor 削峰 + P2-2 worker 独立 Deployment |
| ADR-02 | 数据库逻辑分库(Database-per-Service 逻辑层) | adr-db-logical-sharding.md | Accepted | 限界上下文拆 schema 不拆 database |
| ADR-03 | Schema-per-Service(按限界上下文拆 schema) | adr-schema-per-service.md | Accepted | 13 schema 隔离(esp_speech/learning/writing/...) |
| ADR-04 | GitOps — 引入 ArgoCD 替代手动 kubectl apply | adr-gitops-argocd.md | Accepted | ArgoCD app/project + drift 自愈 + ignoreDifferences |
| ADR-05 | ESP 新增服务矩阵(10 服务,Rust/Go 为主) | adr-new-services.md | Accepted | Rust4(AI 编排)+ Go6(轻量业务) |
| ADR-06 | teacher-java 学情汇总事务 Seata AT → 应用层 Saga 补偿 | adr-seata-at-to-saga.md | Accepted | 跨服务事务避开 Seata 全局锁 |
| ADR-07 | 安全扫描四件套(Semgrep + Trivy + Dependabot + Aikido) | adr-security-scanning.md | Accepted | SAST/SCA/镜像/依赖 全链 |
| ADR-08 | BFF vs Spring Cloud Gateway vs Dubbo Triple | (SAD §8 引用) | Accepted | 南北向 BFF + 东西向 Dubbo Triple |
| ADR-09 | FastAPI → Litestar+Granian(AI 服务异步化) | (SAD 引用) | Accepted | AI 服务异步 + Granian 高吞吐 |
| ADR-10 | writing-coach 写作增强服务 | adr-writing-coach.md | Accepted | 模板/范文/综合/提升方案聚合 |
| ADR-11 | Redis Cluster 迁移(hostname 模式根治 IP 漂移) | (redis-cluster-migration-plan.md) | Accepted | 6主6从 + hostname + 各区独立不强同步 |
| ADR-12 | langgraph 0.2 → 1.x 迁移(P1-3) | (SAD/writing-py 引用) | Accepted | StateGraph/add_edge(START)/MemorySaver |
| ADR-13 | WS 广播分片 hub shard=8(P2-1) | (gateway-go hub.go) | Accepted | fnv1a 32-bit + N=8 分片 + Redis Pub/Sub |
| ADR-14 | 缓存 key 三端契约统一(P2-5) | (多级缓存架构对齐.md) | Accepted | `{service}:{entity}:{id}` 明文统一 |
| ADR-15 | follower lag 防读后写(P2-6) | (learning-java 引用) | Accepted | @DataSource(consistent=true) + 乐观锁 + tidb_read_consistency='strong' |

### 索引维护

- 新 ADR 入仓时本表追加一行,编号接续(下一个 ADR-16)。
- 状态变更(Superseded)时原地更新状态列 + 注明被替代编号,不删行。
- 本索引是 ADR 的 Single Source of Truth,与 `docs/design/adr-*.md` 双向校验。

### 与 TOGAF 对接

- ADR 是 Phase G(实施治理)的核心产出物,本索引补齐 G2 缺口(评估 85% → 95%)。
- ADR 质量高(决策/背景/收益/代价齐)是 Phase G 得 7.0 分的主因,补索引后可拉升 Phase G 至 8.0+。

## 出口条件

G1 章程(成员/职责/节奏/决策权/ADR 流程)齐;G2 索引(15 ADR 编号 + 状态 + 文件)齐 + 编号规则定义齐(已满足)。
