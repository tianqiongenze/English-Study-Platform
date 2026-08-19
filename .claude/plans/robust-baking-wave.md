# ESP 全量重构达标计划(robust-baking-wave)

> 目标:架构评分 7.34/10(B+) → **8.0+**(工程实现 8.5+ / TOGAF 治理 7.5+)
> 状态:2026-08-19 制定,用户待确认后启动 Phase1
> 关联评分详情:`docs/design/architecture/架构评分与重构方向.md`
> Obsidian 镜像:`人工智能项目/ESP平台/重构达标计划.md`

## 评分基线(2026-08-12)

| 维度 | 分 | 等级 |
|---|---|---|
| 工程实现加权(70%) | 7.83 | A- |
| TOGAF 治理(30%) | 6.20 | B |
| **总分** | **7.34** | B+ |

## 5 Phase 总览

| Phase | 主题 | 目标贡献 | 依赖 | 状态 |
|---|---|---|---|---|
| Phase1 | 治理落地(P0 阻断上线) | 工程基础达标 | 无 | ⏳ 待启动 |
| Phase2 | 基础设施升级 | 镜像/CI/CD | 无(与 P1/P4 并行) | ⏳ 待启动 |
| Phase3 | 架构重构(达 8.5+) | 工程深度 | Phase1 完成 | ⏳ 待启动 |
| Phase4 | 前端重构(达 8.5+) | 前端达标 | 无(与 P1/P2 并行) | ⏳ 待启动 |
| Phase5 | 上线就绪度门禁 | 上线放行 | 全部完成 | ⏳ 待启动 |

**执行策略**:Phase1/2/4 并行,Phase3 串行依赖 Phase1,Phase5 依赖全部。

---

## Phase1:治理落地(P0,阻断上线)

> P0 级——未完成则不具备上线资格。7 个子项。

### 子项 1.1 Go 限流熔断接线
- **目标**:12 个 Go 服务全接限流 + 熔断中间件
- **状态**:**待核实结果填入**(esp-explorer 核实中)
- **已知**:asr-adapter-go / tts-adapter-go 已有(Gin + 限流熔断 + 健康探针)
- **缺口**:待核实其他 Go 服务(gateway-go/listening-go/live-tutor-go/community-go/content-go/course-path-go/gamification-go)
- **方案**:抽公共限流熔断中间件到 esp-common-go(铁律①公共包优先,≥2 服务共用必下沉)
- **验收**:每服务 /metrics 暴露限流计数 + 熔断状态;单测覆盖

### 子项 1.2 /metrics 全覆盖
- **目标**:所有服务(Go/Java/Rust/Python)暴露 Prometheus /metrics
- **状态**:**待核实结果填入**
- **已知**:Java 用 actuator + micrometer;Go 各服务 main.go 注册 /metrics(待核实)
- **缺口**:Rust(orchestrator/assessment/media-learn)、Python(emotion-svc/writing-coach)的 /metrics 路由
- **验收**:Prometheus scrape config 覆盖全服务,target 全 UP

### 子项 1.3 Nacos 注册补齐
- **目标**:所有服务注册到 Nacos(服务发现契约)
- **状态**:**待核实结果填入**
- **已知**:Java 服务 spring.cloud.nacos.discovery / Dubbo registry;Go 服务是否调 Nacos 注册 API(待核实);Rust 看 nacos-register.sh
- **缺口**:待核实哪些服务未注册
- **验收**:Nacos 控制台服务列表含全服务实例

### 子项 1.4 BFF DB 一致性 #227
- **目标**:bff-java 数据一致性修复(#227)
- **状态**:**待核实结果填入**
- **已知**:bff-java routingDataSource 读写分离(CRDB 读写双数据源)
- **缺口**:待核实 #227 具体内容与修复状态
- **验收**:契约测试证明读写分离一致性

### 子项 1.5 OTel endpoint 修正
- **目标**:所有服务 OTel 导出端点正确,无 NXDOMAIN
- **状态**:**本会话已修 DNS**(esp-prod 建 otel-collector ExternalName → host.lima.internal:4317/4318/8889)
- **缺口**:服务需重启才重新建立 OTel gRPC 连接(非阻断);env 残留旧错误地址待核实清理
- **验收**:Tempo 收全服务 spans

### 子项 1.6 securityContext 全补齐
- **目标**:所有 deploy pod/container 有 securityContext
- **状态**:**待核实结果填入**
- **缺口**:待核实哪些 deploy 缺 runAsNonRoot/runAsUser/fsGroup/seccompProfile
- **验收**:CI 门禁扫 deploy/k8s/*.yaml 无 securityContext 缺失

### 子项 1.7 Mock 残留切断
- **目标**:生产域 mock 零残留(MPB 铁律)
- **状态**:**本会话已审零残留**(PROGRESS 第 14-29 行,全代码库 grep 证据驱动)
- **结论**:conversation-go LLM_MODE/ASR_MODE/TTS_MODE、writing-coach LLM_MODE 均为合法 env 开关(生产 manifest 不设即不触发);listening-go 硬编码已改 501;llm-orchestrator-rs StubProvider manifest 有 env 不触发
- **验收**:CI 扫生产路径(deploy/k8s/ + 生产 main)的 mock/占位/mock-key 命中红

---

## Phase2:基础设施升级(与 P1/P4 并行)

> 目标:镜像基线统一 + CI/CD 矩阵补全 + GitOps。

- **JDK26 派生镜像**:esp/jdk26-jre 已就绪(派生构建,Dockerfile.derive 模式)
- **CI/CD 矩阵补全**:Rust / Python / admin-go 流水线补齐(现有 Java/Go 全)
- **ArgoCD syncPolicy**:所有 Application 加 syncPolicy(automated prune + selfHeal)
- **关联**:`docs/design/architecture/multi-node-migration-checklist.md`(Obsidian `多节点迁移清单.md`)

## Phase3:架构重构(达 8.5+,串行依赖 P1)

> 目标:工程深度提升至 8.5+。

- **esp-common-java 公共包下沉**:评估结论**暂不建**(重复有限 + esp-rw-java/esp-web-common/esp-cache 三套已覆盖 + 3 服务收益不足),待第 4+ 服务再评(Obsidian `Java公共包下沉评估.md`)
- **Go DI 重构**:12 Go 服务依赖注入统一(当前各服务手写初始化)
- **BFF trace root**:bff-java 作为 trace 根(当前 trace 从前端/bff 断裂)
- **Resilience4j**:Java 服务接 Resilience4j 限流熔断(替代手写)
- **LLM 自适应限流(gradient2)**:LLM 长尾响应用 gradient2 算法自适应限流
- **契约版本化**:API 契约加版本号,消费者兼容策略
- **关联**:Obsidian `共享代码审计.md`(Top5 抽共享包清单)

## Phase4:前端重构(达 8.5+,与 P1/P2 并行)

> 目标:前端达标 8.5+。详见 `docs/design/architecture/frontend-beautification-plan.md`(Obsidian `前端重构与美化.md`)。

- **Admin 三面板**:走 api-client + token(ErrorBoundary 接 OTel)
- **小程序 crypto**:加解密(当前明文传输)
- **i18n 收口**:三端(Web/H5/小程序)i18next 同源
- **SWR 接入**:数据请求缓存(替代手写 fetch 状态)
- **UI tokens 激活**:ui-tokens 已建未用资产激活(主题 CSS 变量 + 无 key 图)

## Phase5:上线就绪度门禁(依赖全部)

> 目标:上线放行。详见 `docs/design/architecture/slo-error-budget.md` + `multi-node-migration-checklist.md`(Obsidian `SLO与错误预算.md` + `多节点迁移清单.md`)。

- **E2E 全链路**:三端 E2E 覆盖
- **混沌测试**:故障注入(已有 `report/chaos/` + `scripts/chaos/host-middleware-chaos.sh`)
- **安全 SEC-1~4**:安全测试执行(`docs/test/06-security` 已写待实跑)
- **SLO/错误预算**:99.9% 可用 / P95<500ms / 错误率<0.1% / 月度错误预算 259.2s
- **readiness-gate**:评分 8.0+ 才放行
- **多节点迁移清单**:8 大类待办(CNI 升级 Cilium / Harbor / CRDB 重建 / 分布式 Locust)

---

## 验收门禁(每 Phase)

- **契约测试**:Dubbo/Pulsar/outbox/读写分离必有 IT/E2E(铁律③)
- **CI 门禁**:架构约束 + 覆盖率≥80%(铁律⑥)
- **无产物不声称完成**:声称完成必附文件路径 + 测试证据(铁律④)
- **诚实优先**:落盘以可核验事实为准(铁律⑦)

## 进度追踪

- 本文件 + `docs/PROGRESS.md` 热区双轨落盘
- 每 Phase 完成更新本表状态列 + PROGRESS.md

## 关联
- Obsidian 镜像:`人工智能项目/ESP平台/重构达标计划.md`
- 评分基线:`docs/design/architecture/架构评分与重构方向.md`(Obsidian `架构评分与重构方向.md`)
- 工程铁律:`engineering-ironrules` memory / `docs/design/architecture/engineering-ironrules.md`
- Phase4 输入:`frontend-beautification-plan.md`
- Phase5 输入:`slo-error-budget.md` / `multi-node-migration-checklist.md`
- Phase3 输入:`java-shared-module-evaluation.md` / `shared-code-audit-2026-08-15.md`
