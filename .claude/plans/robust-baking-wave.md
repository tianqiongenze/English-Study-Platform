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
| Phase1 | 治理落地(P0 阻断上线) | 工程基础达标 | 无 | 🔄 子项已核实(2 完成/5 部分),待启动 |
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
- **状态(2026-08-20 核实)**:**部分**(11/13 接入,2 缺口)
- **证据**:✅ 9 个 Gin 服务全接 `middleware.GinRateLimit`+`GinCircuitBreaker`(asr-adapter/tts-adapter/community/content/gamification/listening/reading/vocab-srs 各 main.go)+ gateway-go/course-path-go 接 net/http 版 `HTTPRateLimit`+`HTTPCircuitBreaker`;⚠ conversation-go 非独立部署(gateway-go 内联引用,无 main.go)
- **缺口**:❌ admin-go(治理面板,无限流熔断/无 metrics,仅代理 Prom)❌ live-tutor-go(桩服务,main.go 仅 4 条 HandleFunc 无限流无 metrics)
- **方案**:admin-go/live-tutor-go 补接 esp-common-go 限流熔断中间件(铁律①≥2 服务共用必下沉)
- **验收**:每服务 /metrics 暴露限流计数 + 熔断状态;单测覆盖

### 子项 1.2 /metrics 全覆盖
- **目标**:所有服务(Go/Java/Rust/Python)暴露 Prometheus /metrics
- **状态(2026-08-20 核实)**:**部分**(Go/Java/Python 全覆盖,Rust 全缺)
- **证据**:✅ 11 个 Go 服务注册 `/metrics`(promhttp.Handler)✅ bff-java/learning-java 走 actuator+micrometer(OTEL 已配)✅ writing-py/emotion-svc 见 OTEL_SERVICE_NAME
- **缺口**:❌ **4 个 Rust 服务(assessment-rs/llm-orchestrator-rs/media-learn-rs/pronunciation-rs)Cargo.toml 无 metrics/prometheus 依赖,src 无 /metrics 路由——全缺** ❌ live-tutor-go 也缺
- **方案**:Rust 服务加 prometheus crate + /metrics 路由(axum middleware);live-tutor-go 补 promhttp
- **验收**:Prometheus scrape config 覆盖全服务,target 全 UP

### 子项 1.3 Nacos 注册补齐
- **目标**:所有服务注册到 Nacos(服务发现契约)
- **状态(2026-08-20 核实)**:**部分**(核心服务全注册,2 待确认)
- **证据**:✅ Go 服务真调 `registry.RegisterNacos`(gateway-go:119/community-go:64 等)✅ Java 服务配 NACOS_ADDR/NACOS_HOST(15-bff:27/20-learning:41)✅ Rust 用 entrypoint `backend/scripts/nacos-register.sh`(POST 注册+SIGTERM 注销,31-new:131/32-batch2:38,145/33-orchestrator:82 引用)
- **缺口**:⚠ admin-go 引用 nacos(catalog)但 main 无 RegisterNacos 调用,需确认是否真注册 ⚠ live-tutor-go nacos 引用存在同样需确认
- **验收**:Nacos 控制台服务列表含全服务实例;`curl Nacos/ns/instance/list` 全服务有实例

### 子项 1.4 BFF DB 一致性 #227
- **目标**:bff-java 数据一致性修复(#227)
- **状态(2026-08-20 核实)**:**未做/无可核验证据**
- **证据**:git log 无 #227/consistency 提交;bff-java/src 无 routingDataSource/AbstractRoutingDataSource/@Transactional 命中(BffApplication.java 无相关代码)
- **缺口**:未发现 #227 数据一致性修复的代码或提交痕迹
- **⚠ 待用户确认**:此子项来源不明——是计划项还是已在别处处理?PROGRESS 称"routingDataSource 读写分离双数据源"但代码未命中,需澄清 #227 的实际内容(读写分离?事务一致性?多数据源同步?)后再定方案

### 子项 1.5 OTel endpoint 修正
- **目标**:所有服务 OTel 导出端点正确,无 NXDOMAIN
- **状态(2026-08-20 核实)**:**已完成**
- **证据**:本会话修 DNS(esp-prod 建 otel-collector ExternalName → host.lima.internal:4317/4318/8889);全部 manifest OTEL_EXPORTER_OTLP_ENDPOINT 统一指向 `http://otel-collector.esp-prod.svc.cluster.local:4317`(10-gateway:62/15-bff:50/20-learning:45/30-other:44,95,143,205/31-new:32,82,130/32-batch2:37,90,144,197,249/33-orchestrator:69,147/34-asr:73/34-community:27)——**无残留旧错误地址**
- **缺口**:服务需重启才重新建立 OTel gRPC 连接(非阻断,本轮不重启避免扰动)
- **验收**:重启 ESP 服务后 Tempo 收全服务 spans(端到端验证)

### 子项 1.6 securityContext 全补齐
- **目标**:所有 deploy pod/container 有 securityContext
- **状态(2026-08-20 核实)**:**部分**(业务 Deployment 有 container 级,infra 类全缺,pod 级普遍缺)
- **证据**:✅ 业务 Deployment(10/15/20/21/30/31/32/33-orchestrator/34/35)均有 container 级 securityContext(10-gateway:19-21 `runAsNonRoot:true`/`runAsUser:1000`,15-bff:22-25 同)
- **缺口**:❌ **infra 类 Deployment 无 securityContext**——06-sca-middleware/06b-redis-pvc/07-tidb-tikv/95-otel-collector/otel-collector-gateway 全 0 处 ⚠ 业务 Deployment 多为单处(container 级),**pod 级 fsGroup/seccompProfile 普遍缺失**
- **方案**:infra Deployment 补 container 级 securityContext;业务 Deployment 补 pod 级 fsGroup + seccompProfile(RuntimeDefault)
- **验收**:CI 门禁(kube-score/kube-hunter)扫 deploy/k8s/*.yaml 无 securityContext 缺失

### 子项 1.7 Mock 残留切断
- **目标**:生产域 mock 零残留(MPB 铁律)
- **状态(2026-08-20 核实)**:**部分(有真实缺口,推翻之前"零残留"结论)**
- **证据**:✅ conversation-go ASR_MODE/TTS_MODE 生产设 "http"(10-gateway-go.yaml:50/52,非 mock)✅ writing-py/emotion-svc 走真实 Anthropic(secret esp-secrets,30-other:29-31/183-185)✅ 无 LLM_MODE=mock 残留
- **缺口**:**❌ llm-orchestrator-rs StubProvider 生产激活**——`00-namespace-secret.yaml:39-44` secret `llm-providers` 全空值占位(`claude-base:""`/`openai-key:""` 等),代码 `provider/mod.rs:85` `if chain.is_empty() { chain.push(StubProvider) }` 会命中 → **生产实际跑 StubProvider(echo 兜底),违反"生产域 Mock 零残留"**
- **⚠ 诚实复盘**:PROGRESS 第14-29行称"StubProvider manifest 有 env 不触发",核实发现 manifest 的 secret 全空值占位导致 chain 为空 → StubProvider 命中。之前判断"不触发"是错的(只看 env 标签没看 secret 实际值)
- **方案**:`llm-providers` secret 注入真实凭据(当前默认 Zhipu GLM via CC Switch / Claude via esp-secrets);或 orchestrator env `LLM_PROVIDER_*` 明确指向真实 provider 避免 chain 空命中 Stub
- **验收**:llm-orchestrator-rs pod 日志无 "StubProvider" 激活;实测一次 LLM 调用走真实 provider

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
