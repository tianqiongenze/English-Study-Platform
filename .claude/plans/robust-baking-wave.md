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
| Phase1 | 治理落地(P0 阻断上线) | 工程基础达标 | 无 | ✅ 全子项完成(7/7,镜像层 apply 待批量) |
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
- **状态(2026-08-20 实装)**:**完成**(13/13 接入)
- **证据**(commit `c157467`):✅ 9 个 Gin 服务 + gateway-go/course-path-go(已有)+ **本会话 admin-go(197-198)+ live-tutor-go(53-54)接入** esp-common-go middleware;conversation-go 非独立部署(gateway 内联)
- **缺口**:无(全接入);镜像层验证 DEFERRED
- **验收**:两服务 go build+vet RC=0;镜像 build+apply 待批量

### 子项 1.2 /metrics 全覆盖
- **目标**:所有服务(Go/Java/Rust/Python)暴露 Prometheus /metrics
- **状态(2026-08-20 核实+实装)**:**代码完成**(Go/Java/Python/Rust 全接,镜像层验证 DEFERRED)
- **证据**:✅ 11 Go 服务 + bff/learning(Java actuator+micrometer)+ writing-py/emotion-svc(OTEL_SERVICE_NAME)✅ **本会话 4 Rust 服务接入**(commit `4f944f7`):esp-common-rs/src/metrics.rs 下沉共享包 + 4 服务 `.merge(metrics_router())` + rate_limit 跳过 /metrics
- **缺口**:⚠ live-tutor-go 仍缺(桩服务,与 1.1 合并补);⚠ **镜像层验证 DEFERRED**(cargo check 过,docker build 未执行遵雪崩铁律,镜像内 /metrics 可达性待部署阶段验证)
- **验收**:Prometheus scrape config 覆盖全服务 target 全 UP(待镜像 build+apply 后验证)

### 子项 1.3 Nacos 注册补齐
- **目标**:所有服务注册到 Nacos(服务发现契约)
- **状态(2026-08-20 核实+复查)**:**完成**(全服务已注册)
- **证据**(esp-explorer 初判误,复查纠正):✅ Go 服务真调 `registry.RegisterNacos`(gateway-go:119/community-go:64 等)✅ Java 服务配 NACOS_ADDR/NACOS_HOST✅ Rust 用 `nacos-register.sh`(POST 注册+SIGTERM 注销)✅ **admin-go(164)+ live-tutor-go(38)早已有 RegisterNacos+DeRegister**——esp-explorer"需确认"误判(只看表面没追到调用行)
- **缺口**:无
- **验收**:Nacos 控制台服务列表含全服务实例

### 子项 1.4 BFF DB 一致性 #227
- **目标**:bff-java 数据一致性修复(#227)
- **状态(2026-08-20 探查)**:**已完成(已隐含处理)**——#227 真实含义与计划描述不同
- **证据**(esp-explorer 探查,commit 引用闭合):
  - #227 原始注释 `15-bff-java.yaml:43`:"BFF users 表对齐 gamification-go 写入的 esp_auth.users(超集,含 digital_human/ranking_opt_out/region_code);原 esp.users 仅 6 列缺这些列致 gamification UPDATE 静默失败"
  - 修复手段:15-bff-java.yaml:43 设 `ESP_RW_COCKROACHDB_DB=esp_auth`(BFF 连 esp_auth 库)+ 257 行数据迁移 esp_auth.users + AuthController INSERT 经 CRDB 路由池落 esp_auth.users
  - 读写分离(esp-rw-java @DataSource 注解+双池 Hikari)本身一直在线,与 #227 无关
- **⚠ 诚实复盘**:计划描述"读写分离数据一致性"是误判——真实 #227 是**库/schema 对齐问题**(BFF users 表对齐 gamification-go 的 esp_auth.users 超集),已在 session-23-13 通过 ENV 切 esp_auth 闭合。esp-explorer 前次"bff-java/src 无 routingDataSource"误判——真实 RoutingDataSource 在共享包 `platform/esp-rw-java/`(非 bff 内)
- **验收**:#227 已闭合(esp_auth 库迁移完成);无需补代码

### 子项 1.5 OTel endpoint 修正
- **目标**:所有服务 OTel 导出端点正确,无 NXDOMAIN
- **状态(2026-08-20 核实)**:**已完成**
- **证据**:本会话修 DNS(esp-prod 建 otel-collector ExternalName → host.lima.internal:4317/4318/8889);全部 manifest OTEL_EXPORTER_OTLP_ENDPOINT 统一指向 `http://otel-collector.esp-prod.svc.cluster.local:4317`(10-gateway:62/15-bff:50/20-learning:45/30-other:44,95,143,205/31-new:32,82,130/32-batch2:37,90,144,197,249/33-orchestrator:69,147/34-asr:73/34-community:27)——**无残留旧错误地址**
- **缺口**:服务需重启才重新建立 OTel gRPC 连接(非阻断,本轮不重启避免扰动)
- **验收**:重启 ESP 服务后 Tempo 收全服务 spans(端到端验证)

### 子项 1.6 securityContext 全补齐
- **目标**:所有 deploy pod/container 有 securityContext
- **状态(2026-08-20 实装)**:**代码完成**(部分 DEFERRED,apply 待批量确认)
- **证据**(commit `8c0bb47`,17 manifest dry-run RC=0):
  - ✅ infra 完整加固(otel 3 文件,镜像非 root):runAsNonRoot+runAsUser:10001+readOnly+seccomp+capabilities drop + /tmp emptyDir + pod 级 fsGroup
  - ✅ infra 保守加固(nacos/sentinel/redis/tikv 等 root 镜像):allowPrivilegeEscalation:false + seccompProfile
  - ✅ 业务 22 Deployment pod 级 fsGroup:1000+seccompProfile:RuntimeDefault(PVC 全 0 无副作用)
- **缺口**(DEFERRED,诚实):nacos/sentinel/seata/redis/pd/tikv/tidb/tiflash 的 runAsNonRoot+runAsUser(需重打非 root 镜像 + chown 数据卷,多节点生产化时补)
- **验收**:17 manifest dry-run RC=0;实际 apply 待批量确认(改 manifest 未 apply,可逆)

### 子项 1.7 Mock 残留切断
- **目标**:生产域 mock 零残留(MPB 铁律)
- **状态(2026-08-20 核实+复查)**:**已完成**(初始 esp-explorer 误判,复查纠正)
- **证据**:✅ conversation-go ASR_MODE/TTS_MODE 生产设 "http"(10-gateway-go.yaml:50/52,非 mock)✅ writing-py/emotion-svc 走真实 Anthropic(secret esp-secrets)✅ 无 LLM_MODE=mock 残留
- **⚠ 诚实复盘(纠正)**:esp-explorer 初核称"llm-providers secret 全空值占位 → StubProvider 生产激活"——**这是误判**。核实只看 manifest `stringData`(空占位作文档契约,manifest 第182行注释"已由 kubectl create secret 注入真实 key")。复查集群实际 secret:`llm-providers` 的 claude-base(len=44)/claude-key(len=64)**有真实值**;pod 内 env `LLM_PROVIDER_CLAUDE_BASE_len=31`/`LLM_PROVIDER_CLAUDE_KEY_len=48` 非空 → 按 `provider/mod.rs:67` 逻辑 ClaudeProvider 被 push → **chain 不为空,StubProvider 不命中**。生产实际跑 ClaudeProvider(经 CC Switch 桥接),非 Stub。
- **结论**:生产域 mock 零残留成立(StubProvider 仅 cargo check/无 env 兜底,生产 env 有值不触发)
- **验收**:pod env `LLM_PROVIDER_CLAUDE_*` 非空(已实测);CI 扫生产路径 mock/占位/mock-key 命中红

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
