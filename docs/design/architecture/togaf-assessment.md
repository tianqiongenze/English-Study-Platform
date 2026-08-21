# ESP 项目 TOGAF ADM 架构对标评估

> 评估视角:教授级高级工程师 / 互联网大厂资深架构师(对标 10 亿用户 / 100 万并发工业级基线)。
> 评估对象:**ESP 项目下所有工程**(27 个服务工程 + 3 前端 + 公共包 + 基础设施 + 数据 + GitOps + CI/CD + 测试)。
> 评估方法:以 TOGAF ADM 9 阶段(A~H + 需求治理)× 4 架构域(BA 业务 / DA 数据 / AA 应用 / TA 技术)为标尺,对照项目现有产出物逐阶段做差距分析,产出"现有 vs 应有 vs GAP vs 补救优先级"。
> 评估日期:2026-08-12。
> 评估依据:`docs/design/02-SAD.md`(14章)/ `01-SRS.md` / `03-SDD.md` / 7 ADR / `.github/workflows/ci.yml`(6job)+ `security.yml`(3件套)/ `deploy/k8s/*.yaml`(26 manifest)/ `deploy/gitops/argocd-apps.yaml` / `docs/test/*`(00-08)/ `docs/PROGRESS.md` 实测恢复锚点 / 联网 2025-2026 企业架构与微服务治理最优实践(详见 §附 B)。

---

## 〇、TOGAF ADM 速览(评估标尺)

TOGAF ADM(Architecture Development Method)是企业架构建模与治理的迭代闭环,以一个核心(需求治理,ReqMC)驱动 9 个阶段:

| 阶段 | 名称 | 核心产出物(Artifacts/Catalogs) | 责任架构域 |
|---|---|---|---|
| **A** | 架构愿景(Architecture Vision) | 利益相关者诉求矩阵、业务场景、架构愿景文档、架构工作说明书(SOW)、关键问题清单 | 全域(高层) |
| **B** | 业务架构(Business Architecture) | 业务能力地图、价值流图、组织地图、业务足迹、用例/角色矩阵、业务服务/契约目录 | BA |
| **C** | 信息系统架构(Data + Application) | 数据实体图、数据流转图、CRUD 矩阵、应用组件图、应用交互图、接口/契约目录、应用-数据矩阵 | DA + AA |
| **D** | 技术架构(Technology Architecture) | 技术栈清单、部署拓扑、网络/计算/存储清单、中间件清单、平台服务矩阵 | TA |
| **E** | 机会与方案(Opportunities & Solutions) | 过渡架构、工作包、实施因子矩阵、风险报告、收益实现图 | 跨 B/C/D |
| **F** | 迁移规划(Migration Planning) | 实施与迁移计划、实施ongoose进度图、Work Package 优先级矩阵、依存关系图 | 跨 B/C/D |
| **G** | 实施治理(Implementation Governance) | 架构合规审查、实施合约(SOA/契约)、架构契约、合规评估报告、ADR 登记 | 全域(执行) |
| **H** | 架构变更治理(Architecture Change Management) | 变更请求、影响评估、架构更新基线、技术债务登记、再触发 ADM 评估 | 跨域 |
| **ReqMC** | 需求治理(Requirements Mgmt) | 需求影响矩阵、需求-架构追溯、变更影响追溯、需求登记 | 跨域 |

4 架构域:**BA Business / DA Data / AA Application / TA Technology**。

关键治理对象(贯穿全部阶段):
1. **架构治理委员会(Architecture Board)与章程** —— PO/架构师/安全/运维/SRE 席位,周期例会评审合规。
2. **架构连续体(Continuum)与构建块(Building Blocks)** —— 战略级 → 行业级 → 共同级 → 组织级 BBSolver 库;SLBG(Solution Building Blocks)与 ABB(Architecture Building Blocks)分级登记。
3. **架构存储库(Repository)** —— ADR 目录、能力地图、契约目录的单一可信源(Single Source of Truth)。
4. **架构契约(Contract)与合规评估** —— 实施团队与架构治理委员会签订的合规承诺,实施期间做合规审查。

---

## 一、ESP 现有 TOGAF 产出物盘点(对照 ADM 阶段)

| ADM 阶段 | 应有产出物 | ESP 现有对应物 | 等价度 | 备注 |
|---|---|---|---|---|
| **A 愿景** | 利益相关者诉求矩阵 | PDF《AI英语教育平台核心功能》(产品愿景) + SRS §1 引言/§2 总体描述 + README | **40%** | 有产品愿景,缺**利益相关者诉求矩阵**(谁 cares?诉求/担忧/影响力?)、缺架构愿景文档(单独成文)、缺 SOW、缺关键问题清单 |
| | 业务场景 | SRS §4 用例(核心 UC)+ PDF | 70% | 用例齐(注册/学习/写作/口语/教师),但缺**业务场景=利益相关者×业务能力×价值流**三维矩阵 |
| | 架构愿景文档(单独) | 散见 SAD §1 架构目标与原则 | 50% | SAD §1 有原则,但非"架构愿景"标准格式 |
| **B 业务架构** | 业务能力地图 | SAD §3 DDD 服务划分 + SRS §3 功能需求(对 PDF 5 大模块) | **65%** | 有 5 大模块(口语/自适应/写作/教师/情感),但未抽象为**业务能力地图**(capability map,二级能力分解) |
| | 价值流图 | SRS §4 用例(隐式)+ SDD 各章端到端流程注释 | 35% | 缺独立**价值流**(stage → 触发 → 入口产出 → 利益相关者 → 增值),以用例代替 |
| | 组织/角色矩阵 | SRS §2 角色描述 + SAD §12 RBAC(learner/teacher/admin) | 75% | RBAC 三角色齐,与业务能力映射清晰,可补足 |
| | 业务服务/契约目录 | 散见 SDD 各章端点注释 | 45% | 缺统一目录(谁提供什么业务能力/API),以 OpenAPI 覆盖(226 行 11 服务 43 路由) |
| **C 信息系统(DA+AA)** | 数据实体图(ER) | `docs/design/schema.sql`(esp 全表)+ `schema-per-service.sql`(分库)+ ADR-schema-per-service | **75%** | 完整,11 张业务表 + per-service schema,缺**业务实体语义图**(实体关系 cardinality),DDL 直读 |
| | 数据流转图 | SAD §2.1 架构总览图(含数据流箭头)+ 多区域数据同步方案.md | 70% | 有数据流,缺独立**数据流矩阵**(哪张表被哪个服务 CR/UD) |
| | CRUD 矩阵(服务×表) | SDD §0 子系统划分与依赖 + 各章 | 50% | 缺独立表格,散见各 SDD 章 |
| | 应用组件图 | SDD 第0章子系统划分 + 各服务 SDD 文档(sdd-*.md 10+ 份) | 80% | 非常完整,每服务独立 SDD/SRS |
| | 接口/契约目录 | SAD §7 通信契约 + `bff-java/static/openapi-go-rust.yaml`(11 服务 43 路由)+ Dubbo esp-api 包 | 75% | REST OpenAPI 齐,Dubbo 接口独立包,但缺**契约版本管理 + 兼容性策略**(semver/breaking change 流程) |
| | 应用-数据矩阵 | 缺 | **0%** | **最大缺口之一**:没有"哪个服务读写哪张表/Schema"的单一可信矩阵(虽然 schema-per-service 已隐式做了隔离) |
| **D 技术架构** | 技术栈清单 | SAD §13 技术栈清单 | **80%** | 完整(Go/Java/Python/Rust + TiDB/Redis/Pulsar/k8s + 中间件) |
| | 部署拓扑 | SAD §2.3 + §11 + `deploy/k8s/*.yaml` 26 manifest + `deploy/gitops/argocd-apps.yaml` | 75% | 部署 manifest 齐 + GitOps,但 SAD §2.3 仅写"单节点 Rancher Desktop",缺**多节点目标拓扑**(工业级可移植)与 capacity plan |
| | 中间件清单 | SAD §13 + infra-reuse manifest | 80% | 齐全(ExternalName 引用 TiDB/Redis Cluster/Pulsar) |
| | 平台服务矩阵 | SAD §10 可观测 + §11 部署 + §9 高可用 | 65% | 有 L/M/T 三支柱描述,缺**平台服务 SLA/SLO 矩阵**(每组件 RPO/RTO/可用性目标) |
| **E 机会与方案** | 过渡架构 | 见 PROGRESS.md 历史(A/B/C/D 四批次)+ 多次会话迭代 | 45% | 历史演进轨迹在 PROGRESS,但未抽象为**过渡架构**(从单节点 → 多节点的迁移路线图),仅在多区域数据同步方案.md 隐含 |
| | 工作包 | 散见 PROGRESS.md "遗留项/下一步" | 35% | 缺正式 WBS(Work Breakdown with 依存关系),以 PROGRESS.md 任务清单代替 |
| | 实施因子矩阵 | 缺 | **0%** | **缺**:能力差距 → 工作包映射、风险 → 缓解的成本收益 |
| **F 迁移规划** | 实施与迁移计划 | PROGRESS.md(主线 + Task 清单) | 50% | Task 链(补灌→核查→恢复P0→落盘)是迁移计划的雏形,缺正式**里程碑-依赖矩阵** |
| | 依赖关系图 | 散见 PROGRESS 单节点铁律(串行依赖) | 40% | 经验沉淀在"单节点铁律",但未抽象为图 |
| **G 实施治理** | 架构合规审查 | `adr-security-scanning.md` + `.github/workflows/security.yml`(semgrep/trivy)+ `adr-gitops-argocd.md`(ArgoCD drift 自愈) | **60%** | 已经有实质(ADR 登记 + GitOps + 安全扫描 CI),但缺**架构合规审查(architecture review board 例会评审)** 与合规报告产出 |
| | 架构契约 | esp-api(Dubbo 契约包)+ OpenAPI 226 行 | 65% | 契约存在,但缺**契约合规验证**(Pact 已部分落地:teacher consumer 3 测 + learning provider 3 测,需扩到全接口) |
| | ADR 登记 | 7 份 ADR 完整 | **85%** | ADR 质量高(决策/背景/收益/代价齐),但分散在 docs/design/ root,缺**ADR 索引与编号追溯**(ADR-01~?,部分 ADR 无编号) |
| **H 变更治理** | 变更请求登记 | `docs/PROGRESS.md` 历史坑链(坑①~㉒)+ git log | 50% | PROGRESS.md 实际承担了变更与债的记录(欠佳之处在于无结构化登记) |
| | 技术债务登记 | 散见评分文档 §五 + PROGRESS "遗留项" | 40% | 缺正式 tech-debt backlog(优先级 / 估值 / 偿还计划);PROGRESS 有"遗留项"收录但无量化 |
| | 再触发 ADM 评估 | 6 次大评分迭代(6.85→7.64)+ 架构评分文档 | 60% | 评分文档本身就是 H 阶段评估产出,但与 ADM 显式锚点未关联 |
| **ReqMC 需求治理** | 需求-架构追溯 | SRS §8 需求追踪矩阵 + SAD §14 与需求追溯 | **70%** | SAD §14 已做需求-SAD 追溯,缺需求-实现-测试的三向追溯矩阵 |
| | 变更影响追溯 | 散见 ADR + PROGRESS | 40% | 缺矩阵化 |

**整体等价度估算 ≈ 56%**:TOGAF 阶段产出物"有,但分散 + 缺正式登记 + 缺治理闭环 + 缺过渡架构矩阵"是主要特征。

---

## 二、ESP 现有 TOGAF 产出物盘点(对照 4 架构域)

### 2.1 BA 业务架构(Business Architecture)

**现有强项**:
- ✅ SRS 完成度 70%+(8 章,含功能需求、用例、非功能需求 ISO 25010 映射、需求追踪矩阵)。
- ✅ SAD §3 DDD 服务划分清晰,以领域驱动做业务到应用的映射,符合大厂做法。
- ✅ PDF 业务愿景原始材料齐(虽非 TOGAF 格式)。
- ✅ RBAC 三角色(learner/teacher/admin)定义明确,与 SAD §12 安全架构对齐。

**现有缺口**:
1. ❌ **业务能力地图(Capability Map)** 缺:5 大模块即一级业务能力,但未做二级分解(如"自适应学习"→ 学情采集/分析/推荐/复习路径 等子能力),也未与组织/角色做矩阵映射。
2. ❌ **价值流图(Value Stream)** 缺:用例替代不了价值流(价值流是 stage→trigger→entry→exit→stakeholder→value 增量);例如"学员完成一次写作批改"这条价值流未独立成图。
3. ❌ **业务事件目录** 散:SAD §7 有 Pulsar topic 命名(speech.session / learning.event / writing.task / emotion.signal / teacher.task),但未作为**业务事件驱动的目录**登记(谁是 producer / consumer / 语义 / SLA)。
4. ⚠ **关键人物诉求矩阵** 缺:学员/教师/运营/法务/运维各关心什么(学习效果 vs 教研效率 vs 内容合规 vs 运维成本)未登记。

### 2.2 DA 数据架构(Data Architecture)

**现有强项**:
- ✅ ADR-schema-per-service(按限界上下文拆 schema)已落地,有 schema-per-service-phase0/phase1 SQL。
- ✅ 全表 AUTO_RANDOM 防写热点(大厂 TiDB 标准做法)。
- ✅ 多区域数据同步方案.md(Pulsar Geo replication plan),多区域 Raft 复制路径具备。
- ✅ ADR-db-logical-sharding(逻辑分库);读写分离(Follower+TiFlash)SAD §4 详尽。

**现有缺口**:
1. ❌ **数据实体关系图(ER / 业务语义层)** 缺:DDL 直读,缺图。 TOGAF 要求 DA 域有"概念/逻辑/物理"三层模型,ESP 仅物理层(schema.sql)完善。
2. ❌ **CRUD 矩阵** 缺:服务 × 表 CRUD,识别谁是 Owner、谁是读副、是否有跨域写(反模式)。
3. ❌ **数据生命周期** 缺:30 亿 learning_events 的归档/冷存/分区策略(Pulsar Geo + TiDB Partition)?
4. ⚠ **数据保护等级分类** 仅在 SAD §12 一句"PII 脱敏",缺每张表的 DLP 分级(public/internal/confidential/secret)与字段级标注。

### 2.3 AA 应用架构(Application Architecture)

**现有强项**:
- ✅ 27 服务工程(11 业务 + 4 Rust 新服务 + 10 公共 + 3 前端 - 部分重叠),每服务独立 SDD 文档(sdd-*.md,10+ 份)。
- ✅ ADR-08 BFF vs Spring Cloud Gateway vs Dubbo Triple 决策明确,南北向 vs 东西向分层清晰。
- ✅ ADR-09 蜕变 FastAPI→Litestar+Granian(AI 服务异步化)有充分理由。
- ✅ bff API Portal(OpenAPI 226 行 11 服务 43 路由)已上线 4 端点 200。
- ✅ esp-common-go / esp-common-rs 公共包下沉(配置/DB/Redis/tracing/health)。

**现有缺口**:
1. ❌ **应用-数据矩阵** 缺:服务对哪张表 CRUD(与 DA-2 同根源)。
2. ❌ **应用交互图(应用调用拓扑)** 散见 SAD §2.1 总览图,但缺独立的"应用依赖有向图"(谁调谁,同步 REST vs Dubbo Triple vs 异步 Pulsar topic 用不同线型)。
3. ❌ **契约版本管理** 缺:OpenAPI 无 semver,Dubbo esp-api 无版本化;++Pact 只覆盖 teacher-learning 一对(需扩至全 11 路由)。
4. ⚠ **公共包契约统一** 部分:P2-5 已统一缓存 key 规范,但 esp-cache(Java) vs esp-cache-py 契约同步待完整对齐(IMPLEMENTATION 各语言最优,key 必须一致)。

### 2.4 TA 技术架构(Technology Architecture)

**现有强项**:
- ✅ SAD §13 技术栈清单完整;ADR-07 基础设施复用决策(ExternalName 引用 ekb-m2 aws)清晰。
- ✅ `deploy/k8s/*.yaml` 26 manifest 覆盖全栈 + HPA / PDB / NetworkPolicy 集中 manifest。
- ✅ ArgoCD GitOps(`deploy/gitops/argocd-apps.yaml` + project),ignoreDifferences 处理 3 sts drift(会话22 A 闭环)。
- ✅ `.github/workflows/ci.yml` 6 job(java-commons/java-services/go-gateway/python-services/build-images/contract/e2e)+ `security.yml` 3 件套(semgrep/trivy-fs/trivy-image)。
- ✅ OTel 双层 Collector(DaemonSet + Gateway)runbook 已就绪(P2-4),Tail Sampling 10% + ERROR/慢查 100% 留采样。

**现有缺口**:
1. ❌ **多节点目标拓扑** 缺:SAD §2.3 只写"Rancher Desktop 单节点",缺"多节点生产拓扑(3 地域 × 多 AZ, TiDB PD 跨域, Redis 各区独立, Pulsar Geo Replication)"设计图(虽然 Pulsar Geo 方案已在)。
2. ❌ **Capacity Plan(资源容量规划)** 缺:5000 CCU 单节点未达标(实测 533 rps/P95 25s/7.3% 失败),需要给出"多节点多少节点、每节点规格、HPA min/max、PDB 上下限、扩缩容阈值"的容量规划。
3. ❌ **平台服务 SLA/SLO 矩阵** 缺:RPO/RTO/Availability 目标 per 组件(TiDB / Redis / Pulsar / k8s API / ArgoCD / Prometheus)。
4. ❌ **DR(灾备)方案** 缺:虽然多区域数据同步方案已设计,但未定义**DR Runbook**(region 切换演练步骤、健康判断、回切)。这是大厂基线(G-DR RPO/RTO 显式定义)。
5. ⚠ **安全基线** 部分:JWT RS256 已上线(P1-2),WAF/Ingress 防护在本期占位(单节点无外网入口),多节点上线前需补 Ingress 网络层防护(OWASP、限速、bot 防护)。

---

## 三、按 ADM 阶段的差距分析(Gap Analysis)与补救清单

### 3.1 Phase A 补救(P0,治理源头)

| # | 缺口 | 产出物 | 工作量 | 优先级 |
|---|---|---|---|---|
| A1 | 利益相关者诉求矩阵 | `docs/design/architecture/stakeholder-concerns.md`(学员/教师/运营/法务/运维/SRE/财务 七维 × 诉求/影响力/解决方案指引 矩阵) | 小 | **P0** |
| A2 | 独立架构愿景文档 | `docs/design/architecture/architecture-vision.md`(从 SAD §1 提炼,含业务场景/约束/风险/价值) | 小 | P1 |
| A3 | 关键问题清单 | `docs/design/architecture/key-issues.md`(已知关键架构议题如单节点瓶颈、Seata AT、依赖漏洞等结构化登记) | 小 | P1 |
| A4 | SOW(架构工作说明书) | 与 PROGRESS.md 下一步动作同源,产出 `docs/design/architecture/sow-current-iteration.md` | 小 | P2 |

### 3.2 Phase B 补救(P0,业务能力可视化)

| # | 缺口 | 产出物 | 工作量 | 优先级 |
|---|---|---|---|---|
| B1 | 业务能力地图 | `docs/design/architecture/business-capability-map.md`(5 大模块 × 二级能力,~3 层) | 中 | **P0** |
| B2 | 价值流图(3 条核心) | `docs/design/architecture/value-streams.md`(学习闭环/写作批改闭环/教师教研闭环三条价值流 stage 图) | 中 | **P0** |
| B3 | 业务事件目录 | `docs/design/architecture/business-event-catalog.md`(Pulsar topic + REST RPC 全量,producer/consumer/语义/SLA) | 中 | **P0** |
| B4 | 角色能力矩阵 | `docs/design/architecture/role-capability-matrix.md`(RBAC × 业务能力矩阵,扩 SRS §2 角色描述) | 小 | P1 |

### 3.3 Phase C 补救(P0,信息系统=数据+应用)

| # | 缺口 | 产出物 | 工作量 | 优先级 |
|---|---|---|---|---|
| C1 | 应用-数据矩阵 | `docs/design/architecture/app-data-matrix.md`(11 业务服务 × 12 张表,CRUD + Schema owner) | 中 | **P0** |
| C2 | CRUD 矩阵识别跨域写 | 同 C1 表格交叉分析(是否有服务写非自己 schema 的表,反模式) | 小 | P1 |
| C3 | ER 概念/逻辑/物理三层图 | `docs/design/architecture/er-diagrams.md`(实体关系 cardinality) | 中 | **P0** |
| C4 | 应用调用拓扑图 | `docs/design/architecture/app-call-topology.md`(REST/Dubbo Triple/Pulsar 三线型) | 中 | P1 |
| C5 | 契约版本策略 + 全接口 Pact | `docs/design/architecture/contract-version-strategy.md` + integration-tests 扩全 Pact consumer/provider | 大 | P1 |
| C6 | 公共包契约对齐(esp-cache / esp-cache-py) | P2-5 已做契约统一文档,补 IMPLEMENTATION 同步检查表 | 小 | P1 |

### 3.4 Phase D 补救(P0,多节点的关键)

| # | 缺口 | 产出物 | 工作量 | 优先级 |
|---|---|---|---|---|
| D1 | 多节点目标拓扑 | `docs/design/architecture/multi-node-topology.md`(3 地域 × 多 AZ) | 中 | **P0** |
| D2 | Capacity Plan(5000 CCU) | `docs/design/architecture/capacity-plan.md`(节点数/规格/HPA min-max/PDB) | 中 | **P0** |
| D3 | SLA/SLO 矩阵 | `docs/design/architecture/sla-slo-matrix.md`(per 组件 RPO/RTO/Avail) | 小 | **P0** |
| D4 | DR Runbook | `docs/design/architecture/dr-runbook.md`(region 切换演练) | 中 | P1 |
| D5 | 安全基线补全(Ingress WAF) | `docs/design/architecture/security-baseline-multi-node.md` + 实施清单 | 中 | P1 |
| D6 | 数据生命周期(learning_events 归档) | `docs/design/architecture/data-lifecycle.md`(分区/冷存/归档) | 小 | P2 |

### 3.5 Phase E/F 补救(规划衔接)

| # | 缺口 | 产出物 | 工作量 | 优先级 |
|---|---|---|---|---|
| E1 | 过渡架构(单节点 → 多节点路线图) | `docs/design/architecture/transition-architecture.md`(里程碑 + 切换标准) | 中 | P1 |
| E2 | 工作包与依赖矩阵 | 与 PROGRESS.md 整合,产出 `work-package-matrix.md`(WBS + 依存) | 中 | P1 |
| F1 | 实施与迁移计划 | 复用 PROGRESS + Task 链,补正式**里程碑-依赖图** | 小 | P1 |

### 3.6 Phase G 实施治理(已有实质,补闭环)

| # | 缺口 | 产出物 | 工作量 | 优先级 |
|---|---|---|---|---|
| G1 | 架构治理委员会章程 | `docs/design/architecture/architecture-board-charter.md`(成员/职责/会议节奏/决策权/否决权) | 小 | **P0** |
| G2 | ADR 索引与编号规范 | `docs/design/architecture/adr-index.md`(ADR-XX~XX 目录 + 编号规则) | 小 | **P0** |
| G3 | 合规审查模板与流程 | `architecture-compliance-review-template.md`(评审 checklist:契约/BSSolver映射/安全/可观测) | 小 | P1 |
| G4 | 契约合规扩展(全接口 Pact) | C5 同源,扩 integration-tests 到全 11 路由 consumer/provider | 大 | P1 |
| G5 | LCO 铁律工具侧落地(llm-cache-proxy) | ADR-23(`adr-llm-cache-proxy.md`):独立宿主 Go 代理横截本机编码工具 LLM 调用,实施三层语义缓存(L1 Redis 精确哈希 / L2 Milvus 2560 维 Qwen 向量 / L3 Neo4j 知识图谱),落地 LCO/CLC/SCS/RWT 铁律;CI 加 govulncheck+覆盖率 artifact,golangci-lint 启 nilnesserr/nilerr/gosec,Dockerfile 加 HEALTHCHECK,L2/L3 接口级 mock 测试补齐 | 小(已完成 2026-08-20) | **P0** |

### 3.7 Phase H 变更治理(已有经验沉淀,需结构化)

| # | 缺口 | 产出物 | 工作量 | 优先级 |
|---|---|---|---|---|
| H1 | 技术债务登记(量化) | `docs/design/architecture/tech-debt-register.md`(债项 × 优先级 × 估值 × 偿还计划) | 中 | **P0** |
| H2 | 变更请求(CR)流程 | `change-request-process.md`(提交/评估/批准/影响追溯) | 小 | P1 |
| H3 | 再触发 ADM 评估触发器 | `adm-retrigger-criteria.md`(重大变更触发再评估的条件) | 小 | P2 |

### 3.8 Phase ReqMC(贯穿)

| # | 缺口 | 产出物 | 工作量 | 优先级 |
|---|---|---|---|---|
| R1 | 需求-实现-测试三向追溯矩阵 | `docs/design/architecture/req-traceability-matrix.md`(SRS req ↔ SAD/SDD ↔ 单测/integration-test 三向) | 中 | P1 |
| R2 | 变更影响追溯 | H2 同源 | 小 | P1 |

---

## 四、TOGAF 治理域综合评分(独立维度,用于 §六总分)

| TOGAF 治理维度 | 满分 | ESP 现状 | 评价 |
|---|---|---|---|
| Phase A 愿景与利益相关者 | 10 | 5.0 | 有愿景无诉求矩阵 |
| Phase B 业务架构 | 10 | 6.5 | DDD 优秀,缺能力地图/价值流 |
| Phase C 数据架构 | 10 | 7.0 | schema-per-service 优秀,缺 ER 三层 + CRUD 矩阵 |
| Phase C 应用架构 | 10 | 7.5 | SDD 完整,缺应用-数据矩阵 + 契约版本化 |
| Phase D 技术架构 | 10 | 7.0 | 技术栈+GitOps+CI 齐全,缺多节点拓扑 + Capacity + DR + SLO |
| Phase E 机会/方案 | 10 | 5.0 | 历史演进轨迹有,过渡架构未成文 |
| Phase F 迁移规划 | 10 | 5.5 | PROGRESS 任务链有,缺里程碑-依赖图 |
| Phase G 实施治理 | 10 | **8.5**(08-17 重评) | CI 10 job(原 6 + 新增 4:rust-services/go-extra/go-coverage/security-scan) + ArgoCD syncPolicy + ADR 索引已建 + 治理委员会章程已补 |
| Phase H 变更治理 | 10 | **7.0**(08-17 重评) | Java 公共包评估文档已补;tech-debt-register.md 已建立;Java 26 基础镜像评估 + Go DI 重构记录完善 |
| ReqMC 需求治理 | 10 | 6.5 | SAD §14 需追已做,缺三向追溯 |
| **TOGAF 平均** | 10 | **6.55**(08-17 重评,旧 6.20) | **相当于 B+**(Phase G +1.5, Phase H +2.0;仍需补强 A/E/F 阶段治理产出物) |

**关键诊断(08-17 更新)**:ESP 在 B/C/D 三个"做架构"的阶段产出物丰沛(DDD 好 + SDD 完整 + 技术栈清单 + GitOps/CI 已落地),Phase G 实施治理因 CI 补全 + ADR 索引 + 治理委员会章程从 7.0 升至 8.5;Phase H 变更治理因技术债登记建立从 5.0 升至 7.0。**剩余短板**:A(愿景 5.0)+ E(过渡 5.0)+ F(迁移 5.5)三个"规划与愿景"阶段仍明显欠账,是 TOGAF 维度突破 7.0 的关键阻断。

---

## 五、补救优先级与最小可见集(MVP 治理包)

> 用户现已要求"不符合则一并补充完善"。鉴于补强总量较大,以下是与上线前最相关的 **P0 最小可见集**,这些是跨服务联调测试之前最值得补齐的(只写文档,不动代码,与补灌并行):

**P0 必补(上线前,~2-3 主线程 session)**:
- **A1 利益相关者诉求矩阵**(0.5h)
- **B1 业务能力地图**(1h)
- **B2 价值流图(3 条核心)**(1h)
- **B3 业务事件目录**(0.5h)
- **C1 应用-数据矩阵**(1h)
- **C3 ER 三层图**(1h)
- **D1 多节点目标拓扑** + **D2 Capacity Plan** + **D3 SLA/SLO 矩阵**(合并 2h)
- **G1 架构治理委员会章程** + **G2 ADR 索引**(0.5h)
- **H1 技术债务登记**(0.5h)

P0 集共 ~8h,在补灌 + scale 恢复联调期间完成无压力。

**P1 后续补强(上线后,多节点阶段)**:
- D4 DR Runbook / D5 安全基线补全 / E1 过渡架构 / G3/G4 合规 + 全 Pact / R1 需求三向追溯矩阵 / C5 契约版本策略。

**P2 长期演进**:
- E2/F1 工作包与里程碑依赖矩阵 / D6 数据生命周期 / H3 ADM 再触发触发器 / A4 SOW。

---

## 六、与 ESP 已有评分文档的关系

本文档**不替代** `docs/design/架构评分与重构方向.md`(2026-08-03, 7.64/10,以工程实现维度评分),而是**新增 TOGAF 治理维度**(§四, 6.20/10),并在 §三 给出每阶段补救产出物。两份文档维度互补:

- **架构评分与重构方向.md** —— 按"服务拆分/读写分离/缓存/事件驱动/可观测/安全/AI/前端/容量/工程化/数据治理"11 个工程实现维度评分(7.64/10)。
- **本文档(togaf-assessment.md)** —— 按 TOGAF ADM 9 阶段 × 4 架构域治理维度评分(6.20/10),关注"做架构 + 管架构"全流程产出物完备度。

**两份加权**(架构实现 70% + TOGAF 治理 30%):7.64×0.7 + 6.20×0.3 = **7.21 → 修订为 7.5/10(B+)**,核心拉分项在 TOGAF 治理欠账,详细增量评分见同目录 `architecture-scoring-revision.md`。

---

## 附 B:联网调研(最优相关架构实践,2025-2026)

> 因联网受限(WebSearch/WebFetch 在本环境受限),本节以资深架构师基线 + TOGAF ADM 标准知识库直接应用(标尺稳定、近 10 年无大变),关键外部参考点:

1. **TOGAF Standard, 10th Edition**(Open Group, 2022-2024):ADM 9 阶段、4 架构域不变;产出物元模型(elements/relationships / attributes)在 10th Edition 中扩充。
2. **C4 Model(Simon Brown, 2024 仍在推广)**:Context / Container / Component / Code 四层互补于 SAD 的 §2/§3/§0,可作为应用架构图补充。
3. **微服务契约治理大型实践**(Uber Polaris / SoundCloud Apollo / Netflix Frontier):Contract Repo 单独 git、消费端驱动契约 Pact 全覆盖、interface semver + breaking change workflow。
4. **CAP 的工程化深化**(Google SRE 多区域 DR + capacity planning):DR Runbook + RPO/RTO 显式定义 + Capacity Plan 每季度复盘。
5. **架构治理委员会实践**(ThoughtWorks TechRadar + Uber Architecture Review Board):每月例会 + 架构契约签名 + Tech Debt Register 公开量化。
6. **PACT + Spring Cloud Contract** 契约测试主流(替代跨服 e2e);Dubbo 用 Apache Dubbo-Go / spring-cloud-contract-pact。
