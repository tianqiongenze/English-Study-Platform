# ESP 架构审计报告

> 审计日期: 2026-08-20
> 审计员: Claude 资深架构审计师（对标工业级 10亿用户/100万并发）
> 审计对象: ESP（English Study Platform，AI 英语教育平台）
> 审计性质: 只读审查，不改任何代码
> 对标铁律: 全局 CLAUDE.md 工程铁律 20 条

## 审计范围

对照全局 CLAUDE.md 的 20 条工程铁律逐条审查：
1. 公共能力共享包优先
2. 设计-实现-验证三态追踪
3. 契约测试证明接线
4. 无产物不声称完成
5. 单节点限制显式 DEFERRED
6. CI 门禁
7. 诚实优先
8. 生产域 Mock 零残留 (MPB)
9. 写前核对目标系统 (WTV)
10. 镜像产物内容校验 (BVV)
11. LLM 调用缓存优化 (LCO)
12. API 签名核实 (ASV)
13. 代码级正确性 (CLC)
14. 有状态集群版本漂移 (SVD)
15. 爆炸半径与可逆性 (BRR)
16. 声明式与 GitOps (DGO)
17. 微服务韧性接线 (RWT)
18. 供应链与秘密 (SCS)
19. 只读根文件系统可写路径 (RWP)
20. 安全治理 by design

## 已知背景（不重复审查）

- LCO 三层缓存：tools/lco-proxy（Go）+ backend/services/conversation-go，主线程正在修 lco-proxy forwardUpstream 流式透传
- SCS 5 项已补齐（SBOM/cosign/digest pin/requirements.lock/readOnly），报告 report/scs-implementation-2026-08-20.md
- 宿主中间件（Nacos/Prometheus/Loki/CRDB通道/Redis Cluster）launchd 常驻

---

## 第一部分：结构勘察

### 1.1 服务矩阵（24 服务，4 语言）

| 语言 | 服务 | 数量 |
|---|---|---|
| Go | gateway-go, conversation-go, content-go, community-go, gamification-go, listening-go, reading-go, vocab-srs-go, course-path-go, live-tutor-go, admin-go, asr-adapter-go, tts-adapter-go | 13 |
| Java | bff-java, learning-java, teacher-java（+ integration-tests 契约测试 Maven 工程） | 3 |
| Rust | assessment-rs, llm-orchestrator-rs, media-learn-rs, pronunciation-rs（+ esp-common-rs 共享） | 4 |
| Python | emotion-svc, writing-coach, writing-py | 3 |

### 1.2 共享包（6 个）

| 包 | 子能力 | 状态 |
|---|---|---|
| esp-common-go | middleware, tracing, llm, config, health, redisx, registry, db（8 子包） | 结构良好 |
| esp-common-java | src + test | 有测试 |
| esp-common-rs | Cargo workspace | 有 src/health.rs 但零服务使用（见 shared-code-audit-2026-08-15.md §C） |
| esp-cache-py / esp-db-py / esp-debug-py | Python 共享库 | egg-info 存在=已构建 |

### 1.3 K8s 部署清单（41 个 yaml）

- 基础设施：`06c-redis-cluster.yaml`（6 节点 3主3从）、`07-tidb-tikv.yaml`（+ `.bak` 备份）、`06d-pulsar-geo.yaml`（3 broker + 3 bookie）、`06-sca-middleware.yaml`、`05b-platform-infra-externalnames.yaml`
- 业务服务：per-service yaml（10-gateway-go, 15-bff-java, 20-learning-java, 21-learning-worker, 30/31/32-batch, 33-admin/orchestrator/content/community, 34-asr/content/community, 35-tts, 16-frontend-h5/react/portal）
- 治理：`40-ingress`、`40-networkpolicy`、`40b-traefik-ratelimit`、`41-pdb`（覆盖 24 业务 Deployment）、`45/46-httptest/db-nodeports`、`50-hpa`、`95-otel-collector`、`monitoring/`
- 副本数：redis-cluster=6，pulsar broker/bookie=3；**TiDB 全组件 replicas=1，全部业务服务 replicas=1**（单节点限制）

### 1.4 架构文档（20 份）+ ADR 索引（23 份）

- 架构目录 `docs/design/architecture/`：adr-index.md、adr-lco-proxy.md、engineering-ironrules.md、tech-debt-register.md、slo-error-budget.md、shared-code-audit-2026-08-15.md（既有自审）、togaf-assessment.md、optimization-plan.md、multi-node-migration-checklist.md、multi-node-topology.md、business-capability-map.md、business-event-catalog.md、er-diagrams.md、app-data-matrix.md、phase3-rankings-design.md、java-shared-module-evaluation.md、frontend-beautification-plan.md、architecture-board-charter.md、stakeholder-concerns.md、value-streams.md
- ADR-01~23 已编目（adr-index.md 为 SSoT），覆盖 Disruptor/分库/Schema-per-Service/GitOps ArgoCD/Seata→Saga/安全扫描/BFF+Triple/Redis Cluster/WS 分片/缓存 key 统一/follower lag/content-go/community-go/rankings CQRS/Nacos 统一注册/BFF 动态路由/Go 限流熔断/多语言限流策略/lco-proxy 三层缓存

### 1.5 CI/CD（存在）

- `.github/workflows/ci.yml`（16.8KB，主矩阵）
- `.github/workflows/lco-proxy-ci.yml`（4KB，2026-08-20 新增）
- `.github/workflows/security.yml`（5.2KB，2026-08-16）

### 1.6 PROGRESS.md 当前状态（会话 23-14，2026-08-19）

- Mock/占位符审查完成：生产域 mock 零残留（MPB 符合），仅 listening-go:167 硬编码占位需改 501
- 5 Phase 重构计划已起草（`.claude/plans/robust-baking-wave.md`），自评当前 7.83+6.20=**7.34/10（B+）**，目标 8.0+
- 真实引擎切换（ASR/TTS/LLM）代码+manifest 完成，镜像构建受 registry mirror 403 阻碍（Go 类已用 buildkit 缓存 + esp/ 前缀基础镜像绕过）
- LCO L1 MOVED 重路由 bug 修复（原声称命中率 100% 实为 ~50%，已诚实更正）+ L2 embedding 超时可配（3s→20s）
- 2026-08-19 VM 雪崩事故：LCO embedding MPS 推理 + 3 agent 编译 + 共享基础设施压爆 9 核 VM（单节点限制典型体现）

### 1.7 其他

- 4 个 ESP agent 定义：esp-builder, esp-data, esp-doc, esp-explorer
- 57 个脚本（seed/bench/chaos/ops/selfheal）
- 前端：`frontend-portal` 单目录（React/H5 源码位置待确认，manifest 引用 16-frontend-h5/react/portal）

### 1.8 结构异常（对 DGO/BVV 铁律的信号）

1. `deploy/k8s/07-tidb-tikv.yaml.bak` — 备份 manifest 散落在 gitops 路径（DGO：应移出）
2. `deploy/k8s/00-namespace-secret.yaml.local` — 本地覆盖文件在 deploy 树（DGO：应 formalize 或移出）
3. `deploy/k8s/_schema-fe-pod.yaml` / `_schema-init-pod.yaml` — 下划线前缀 ad-hoc pod 在 gitops 路径（DGO：应正式化或移除）
4. `writing-coach/writing-py/emotion-svc` 各有 `Dockerfile.new` 与 `Dockerfile` 并存 — 双 Dockerfile（BVV/构建可复现性风险：用错则镜像内容不符）
5. `esp-common-go/go.mod.bak` — 源码树内备份文件
6. `redis:8.0-alpine` 出现在 `06c-redis-cluster.yaml`（redis-cluster-new STS）— 与 SVD 铁律记忆中的 Redis 8.0 vs 7.2.13 漂移脑裂事故相关，需核实是否已回写仓库（DGO）及全集群版本是否对齐（SVD）

---

## 第二部分：逐铁律审查

> 证据来源：4 个并行 fork 分支（铁律1-5/6-10/11-15/16-20）+ 主线程补充勘察。每条含现状+证据 file_path:line+缺口+严重度+分数。

### 铁律 11: LLM 调用缓存优化 (LCO)

- **Status**: 符合
- **Severity**: P2
- **Score**: 9/10
- **Dimension**: 架构基础 / 性能
- **Evidence**:
  - `tools/lco-proxy/internal/cache/manager.go:37-43` — Manager 持有 L1Pool/L2Semantic/L3Knowledge 三层结构，New() 构造全连
  - `tools/lco-proxy/internal/cache/manager.go:86-166` — Lookup 顺序 L1(精确 SHA256+协议分键)→ L2(语义 Milvus)→ L3(Neo4j 上下文增强)，三层全接线无空挂
  - `tools/lco-proxy/internal/cache/l2.go:28-60` — L2 真实实装（embedding HTTP 调用 + Milvus cosine 检索，非桩）
  - `tools/lco-proxy/internal/cache/l3.go:22-49` — L3 真实实装（Neo4j 5.x REST Cypher 事务，ensureSchema 建约束+索引）
  - `tools/lco-proxy/internal/cache/manager.go:24-34` — Prometheus 指标 `lco_cache_hits_total{layer}` / `lco_cache_misses_total` / `lco_cache_errors_total` 完整
  - `tools/lco-proxy/internal/server/server.go:100-106,204,225` — 响应头 `X-LCO-Cache-Hit`(含 layer + Source-Protocol + Cross-Protocol + MISS) 暴露完整
  - `tools/lco-proxy/internal/server/server.go:143-156` — forwardUpstream 是合法 miss 回路（SSRF 评估过 G704，host 来自配置信任域），非绕缓存裸调
- **Gap**: L3 的 RAG 真实注入（改上游请求体）标 DEFERRED（manager.go:151-152），当前只暴露为响应头 EnhancedContext；月度成本核算对比"有缓存 vs 无缓存" >40% 未见落盘脚本（铁律要求月度核算）

### 铁律 12: API 签名核实 (ASV)

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 6/10
- **Dimension**: 可测试性 / 正确性
- **Evidence**:
  - `backend/services/bff-java/.../BffApplication.java:38-40` — 历史违规已修复：`@EventListener(ApplicationReadyEvent.class) void initRouteTable(ApplicationReadyEvent event)` 参数类型正确，`event.getApplicationContext().getEnvironment()` 路径正确（非臆造的 `getSpringApplication().getEnvironment()`）
  - `.github/workflows/ci.yml:113-116` — CI 跑 `go vet` + `go test -race`（编译期门禁），`lco-proxy-ci.yml:25-26` 同
  - `.github/workflows/ci.yml:298-303` — Rust 跑 `cargo clippy -D warnings`（静态门禁）
- **Gap**: ① 无"改外部库 API 前查 javadoc"的强制流程守门，靠人工自觉；② Java 侧无 spotbugs/errorprone（见铁律13），反射类 `@EventListener`/`ApplicationListener` 参数类型不匹配编译期不拦、运行期才崩，但 CI 无运行期启动校验（启动一次确认不抛 argument type mismatch）；③ ASV 是过程铁律，代码中无机器可检证据，只能靠"无已知违规+编译/测试门禁"间接佐证，存隐式风险

### 铁律 13: 代码级正确性 (CLC)

- **Status**: 部分符合
- **Severity**: P0阻断
- **Score**: 5/10
- **Dimension**: 可测试性 / 正确性
- **Evidence**:
  - `tools/lco-proxy/internal/cache/manager.go:89-92` — Lookup 历史违规（零值 Manager nil cfg panic）已修：`if m == nil || m.cfg == nil || !m.cfg.CacheEnabled` 首行判空，safeLog/safeWarn 局部判空 logger；Store 同样(line 170-173)
  - `tools/lco-proxy/.golangci.yml` — 唯一的 golangci-lint 配置，启 nilness/nilerr/errcheck/staticcheck/govet/gosec（CLC 铁律点名全启）
  - `.github/workflows/lco-proxy-ci.yml:42-43` — golangci-lint-action@v6 在 lco-proxy CI 实际运行（配而未接已避免）
  - `.github/workflows/ci.yml:160-162` — 仅 4 个 Go 目标跑 go vet（vocab-srs/gamification/course-path/common），其余 9 个 Go 服务（gateway/community/content/conversation/reading/listening/live-tutor/admin/asr/tts）无任何静态检查门禁
  - `.github/workflows/ci.yml:137-138` — Python `ruff check . || true` — `|| true` 致 lint 失败不阻断，门禁形同虚设
  - Java 无 spotbugs/errorprone（grep pom.xml 零命中），仅 jacoco 覆盖率门禁（`ci.yml:42-58`）
- **Gap(P0)**: ① 13 个 Go 服务中 9 个无 golangci-lint/nilness 配置且 CI 不跑，CLC 铁律④"配了 linter 不运行=配而未接"对 lco-proxy 合格但对其余 Go 服务完全缺失；② Python ruff `|| true` 让门禁失效；③ Java 缺 spotbugs/errorprone 静态 bug 扫描；④ CLC 铁律⑤"编译通过≠正确，必跑静态检查+单测+实际执行一次验证运行期不 panic"，CI 无"实际启动一次验运行期"步骤。**这是上线阻断级：生产代码 nil panic/低级错误可逃逸 CI**

### 铁律 14: 有状态集群版本漂移 (SVD)

- **Status**: 部分符合
- **Severity**: P0阻断
- **Score**: 4/10
- **Dimension**: 可运维性 / 安全
- **Evidence**:
  - `deploy/k8s/06c-redis-cluster.yaml:60` — Redis STS spec.image 现声明 `redis:8.0-alpine`（DGO ④回写已补，与历史 7.2.13 不一致来源已清，当前单版本，但缺全集群版本对齐验证）
  - `deploy/k8s/06b-redis-pvc.yaml:36` — 同 `redis:8.0-alpine`（与 06c 一致，无跨大版本混部）
  - `deploy/k8s/07-tidb-tikv.yaml:131,178,225,353,402,451,528,572,616,693` — TiDB v8.5.7/8.5.5 全集群一致（pd/tikv/tidb/tiflash 同 8.5.x）
  - `deploy/k8s/06d-pulsar-geo.yaml:34,133,230,321,347` — Pulsar 3.3.0 全一致
  - `.github/workflows/lco-proxy-ci.yml:104` — 仅 lco-proxy 有 `cosign verify`，非状态集群版本一致性检查
  - 无 `version.*consistency` / `INFO version` / spec==pod imageID 三态对齐 CI 门禁（grep 零命中）
  - `deploy/k8s/07-tidb-tikv.yaml.bak` — 残留 .bak 文件（pd v8.5.5 旧版），.bak 入仓库是隐患
- **Gap(P0)**: ① SVD 铁律①"STS/CR spec.image == 每 pod 实际镜像 imageID == 运行时 INFO version 三态对齐"无 CI 检查门禁（grep 零命中），这是当年脑裂根因（SVD 铁律本身起因），至今无机器防护；② `07-tidb-tikv.yaml.bak` 残留仓库，违反"单一事实源"（DGO），易误用旧版本；③ 镜像用浮动 tag（`redis:8.0-alpine` 非 `@sha256` digest pin），SVD 铁律③精神（派生构建必打标准 tag）达标但缺 digest 锁定，tag 可被劫持重指（当年事故正是 tag 劫持）

### 铁律 15: 爆炸半径与可逆性 (BRR)

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 6/10
- **Dimension**: 可运维性 / 安全
- **Evidence**:
  - `scripts/seed_crdb_full.py:46,536,559` — `--dry-run` 只打印计划不插入（破坏性写前 dry-run，达标）
  - `scripts/rebuild_phase0.sh:5,42` — `--dry-run` 支持重建前预览
  - `report/chaos/crdb-probe-rollback.sh:1-12` — probe 回滚脚本存在（patch 前原值 timeout=1s 记录于注释，可逆）
  - `scripts/chaos/dep_cascade.sh:24-25` — 恢复用 `kubectl rollout undo`（每动作可回滚）
  - `scripts/chaos/` — 8 个混沌脚本（crdb_down/pod_kill/net_partition/latency_inject/redis_down/tidb_down/pulsar_down/dep_cascade）存在
  - `tools/lco-proxy/internal/server/server.go:139-141` — upstream context 解耦注释体现 BRR 意识（不绑 r.Context，独立 600s 回收防泄漏）
- **Gap**: ① 无 L0-L3 操作分级框架（只读/可逆/不可逆外向/灾难性）的显式文档或脚本守门；② 无"k8s patch 前 `kubectl get -o yaml > backup`"的通用 get-before-write 模式（grep 仅见 port-forward 日志重定向，非配置备份）；③ L3 灾难性操作（删 namespace/删 PVC/force push）无强制用户明确授权钩子，靠人工自觉（铁律要求"绝不自动"）；④ 回滚脚本散落，无统一 rollback 注册表

### 主线程补充勘察（RWT 中间件接线修正）

> 修正分支 grep 误判：Go 服务 middleware 接线实际广泛存在（模块名是 `esp/common-go` 非 `esp-common-go`）。
- 12/13 Go 服务 import 了 `esp/common-go/middleware`（仅 conversation-go 未用，内嵌库自写）：gateway-go/content-go/community-go/gamification-go/listening-go/reading-go/vocab-srs-go/course-path-go/live-tutor-go/admin-go/asr-adapter-go/tts-adapter-go
- `backend/services/gateway-go/src/main.go:23,137-138` — 真接线：`middleware.HTTPRateLimit(mux, 100, 200)`（令牌桶 100rps/桶200）+ `middleware.HTTPCircuitBreaker(rateLimited, 5, 30*time.Second, serviceName)`（5次失败触发30s熔断）+ `otelhttp.NewHandler`（tracing）+ `registry.DeRegister`（Nacos 反注册）+ graceful drain

### 铁律 1: 公共能力共享包优先

- **Status**: 部分符合（较 08-15 自审大幅改善）
- **Severity**: P1
- **Score**: 7/10
- **Dimension**: 架构基础
- **Evidence**:
  - 共享包存在且结构完整：`backend/shared/esp-common-go`（middleware/tracing/llm/config/health/redisx/registry/db 8 子包）、`esp-common-java`（jwt/health/sentinel/db/debug）、`esp-common-rs`、`esp-cache-py`/`esp-db-py`/`esp-debug-py`；`platform/`（esp-api/esp-cache/esp-disruptor/esp-rw-java/esp-web-common）
  - `platform/esp-web-common/src/main/java/com/esp/web/common/` — **GlobalExceptionHandler/AppLogClient/TraceIdUtil/ApiError/WebCommonAutoConfiguration 已下沉**（shared-code-audit-2026-08-15.md Top1 建议"esp-web-common 待建"已落地）
  - `backend/shared/esp-common-java/src/main/java/com/esp/common/jwt/BaseJwtValidator.java`+`BaseJwtAuthFilter.java` — JWT 边缘鉴权 starter 已下沉（shared-code-audit Top2 建议已落地）
  - esp-web-common 被 3 Java 服务 import：`grep com.esp.common.jwt|esp-web-common` 命中 bff-java/learning-java/teacher-java
  - esp-api 被 learning-java/teacher-java 消费：`LearningRpcProvider`/`DubboConsumerConfig`/`TeacherController` import `com.esp.api.dubbo.LearningRpcService`/`KpStat`
  - 12/13 Go 服务 import esp-common-go/middleware（见 RWT 补充勘察）
  - **残留缺口**：`backend/shared/esp-common-rs/src/health.rs` — 4 个 Rust 服务 grep `esp_common::health` 零命中，零接线（shared-code-audit §C 记录未修）；跨语言 traceId 键名不一致（Java `traceId`/Go 混用/Rust `trace_id`/Python `trace_id`）未统一
  - **conversation-go 5 处自建 Redis 客户端未下沉**：`cache/cache.go:19`、`llm/cached.go:44`、`llm/provider.go:13`、`hub/hub.go:23`、`session/manager.go:14` 各自直接 `import redis/v9` 自建 ClusterClient，未复用 esp/common-go/redisx（gamification/course-path/gateway 已复用 redisx，conversation 未复用）
  - **admin-go JWT 全量重写**：`admin-go/src/jwt.go:45` 自实现 `verifyToken`（stdlib RSA 验签），与 `gateway-go/src/jwt/validator.go` 用 gojwt 重复（admin 注释称"管理门户 role==admin 故独立实现"，但角色判断可走参数化共享校验，非全量重写）
  - **writing-coach 独立 db.py**：writing-py 复用 esp_cache/esp_db（`service.py:15-16`），但 writing-coach 有独立 `app/db.py`（跨包重复）
- **Gap**: esp-common-rs/health.rs 零接线；跨语言 traceId 键名不一致；conversation-go 5 处自建 Redis 客户端未下沉到 redisx；admin-go JWT 全量重写而非共享+参数化；writing-coach 独立 db.py。08-15 记录的 Java 三份重复已下沉修复（esp-web-common），铁律1 大幅改善但 Go/Python 侧仍有残留

### 铁律 2: 设计-实现-验证三态追踪

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 6/10
- **Dimension**: 架构基础
- **Evidence**:
  - `backend/services/bff-java/src/main/resources/application.yml:77-83` — 配 `dubbo.protocol.name: tri` + nacos registry，但 `grep @DubboReference bff-java/src/main` 零命中，下游全走 RestClient（`ProxyController.java:43,58`）。**bff Dubbo 全空转**（shared-code-audit §C 记录未修）
  - `backend/services/bff-java/src/main/resources/application.yml:87,116` — **bff 还配 seata + pulsar 全段配置，但代码 0 `@GlobalTransactional`、0 `PulsarClient/PulsarTemplate/@PulsarListener`**（仅 BffApplication.java 注释提及）→ **seata/pulsar 对 bff 是死配置**（双"配而未接"）
  - `backend/services/teacher-java/src/main/resources/application.yml:143` — 声明 seata 全段配置，但 `TeacherService.java:118` 注释"已从 Seata AT(@GlobalTransactional)迁移到应用层 Saga"、`TeacherSummarySaga.java:25` 明示迁离 Seata → **teacher seata 配置残留为死配置**
  - **正常接线（已验证）**：learning-java 三态对齐良好 — yml 声明 pulsar/disruptor/outbox/dubbo/读写分离，代码全有对应：pulsar `LearningEventConsumer.java:13-17`、outbox `LearningEventDisruptor.java:192` `fallbackToOutbox` INSERT learning_event_outbox、读写分离 `LearningService.java:96,127,141` `@DataSource(WRITE/READ,consistent=true)`；Seata 真接 — `application.yml:162-170` Seata 配置 + TC 地址兜底；@DataSource(3 Java 服务全用)、EventPublisher、CacheService 全接
  - **三态追踪文档存在**：`docs/design/architecture/engineering-ironrules.md` 铁律二有证据附录；`shared-code-audit-2026-08-15.md` §C "设计未接线"表
- **Gap**: bff Dubbo 配而未接 + bff seata/pulsar 死配置 + teacher seata 死配置（3 处"配而未接"，shared-code-audit 仅记 bff Dubbo，本次新增 2 处）；需删死配置或真接

### 铁律 3: 契约测试证明接线

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 6/10
- **Dimension**: 可测试性
- **Evidence**:
  - `backend/services/integration-tests/src/test/java/com/esp/it/` — **4 个真实 IT 存在且扎实**：T1 `T1_CrossServiceDubboTripleIT.java`（真 tri://learning-java:50091 跨服务 Dubbo）、T2 `T2_ReadWriteSplitIT.java`（真实 tidb-primary/follower + RoutingHolder ThreadLocal + `@DataSource(READ,consistent=true)` AOP 切面断言）、T3 `T3_MultiLevelCacheIT.java`（真 Redis L2+Caffeine L1+延迟双删+空穿透）、T4 `T4_DisruptorPipelineIT.java`（Disruptor 多级流水线）
  - `backend/services/learning-java/src/test/java/com/esp/learning/contract/LearningRpcPactProviderTest.java` + `teacher-java/.../LearningRpcPactConsumerTest` + `src/test/resources/pacts/teacher-learningrpc-pact.json` — Pact 契约真接（consumer teacher→provider learning）
  - `.github/workflows/ci.yml:214-243` — CI Contract (Pact, esp-api) job：consumer pact test → copy pact → provider verify，真跑
  - `integration-tests` Maven 工程存在（契约/集成测试独立工程）
  - Go 22 个 `_test.go`（`content-go/src/outbox_relay_test.go` 等），含 outbox relay 单测
- **Gap**: ① Pact 仅 teacher-learning 一对，未及全 11 Dubbo 路由（tech-debt TD-03）；② **Pulsar 接线无契约 IT**（integration-tests grep pulsar 仅命中 application-it.yml + T3 注释，T3 注释自承"Pulsar 跨实例广播 @Disabled"，无证明 Pulsar 真实生产/消费闭环的契约测试）；③ **Outbox relay 无端到端 IT**（content-go outbox_relay_test.go 只测退避算法 TestRelayBackoffSec，未证 outbox 表→Pulsar 重投闭环）。读写分离/Dubbo/缓存 IT 扎实但消息链路缺口

### 铁律 4: 无产物不声称完成

- **Status**: 符合
- **Severity**: P2
- **Score**: 8/10
- **Dimension**: 可运维性
- **Evidence**:
  - `docs/PROGRESS.md` 每项完成均附路径+证据：如"L3 Neo4j 从 MVP→真实 REST 实装 `tools/lco-proxy/internal/cache/l3.go`"、`go vet ./... RC=0`、`go build -o /tmp/lco-proxy RC=0(24MB)`、`golangci-lint run RC=0` 均落 RC
  - 诚实标注未完成：L1 MOVED 重路由"运行期命中率实测待集群恢复（本次因 VM 雪崩打断）"；whisper 镜像 404 标 DEFERRED；Rust/Debian 基础镜像恢复标 ⏳DEFERRED
  - Mock/占位审查表（PROGRESS.md 会话23-14 第1节）逐项列文件+机制+等级，listening-go 硬编码占位标 ⚠ 需改 501
  - `listening-go/src/main.go:167` 占位已修为 501 拒绝假数据（`c.JSON(501, ...ASR_NOT_IMPLEMENTED)` + DEFERRED 注释），符合 MPB
  - `docs/PROGRESS.md:167` 声称"listening-go 501" → 实测属实；`:279` 声称"redis 8.0 回写" → 实测属实；`:244` BVV 声称 → 属实（见铁律10）
  - `docs/PROGRESS.md:4-5` 自定"硬上限 ~300 行"但实测 593 行（2x 超限）— 自定纪律未守
  - `.github/workflows/ci.yml:3` + CLAUDE.md §3"项目非 git 仓库"过时假设未纠正（实际是 git repo + GitHub remote）
- **Gap**: PROGRESS 自定 300 行超限；"非 git 仓库"过时假设；极少部分"已打通"表述缺运行期实测（如 TTS 链路"port-forward 测 HTTP 200"但 edge-tts websocket bad handshake 独立 bug 未闭合，仍标部分解除）。整体诚实度高

### 铁律 5: 单节点限制显式 DEFERRED

- **Status**: 符合
- **Severity**: P2
- **Score**: 8/10
- **Dimension**: 可运维性
- **Evidence**:
  - `deploy/k8s/41-pdb.yaml:9,11,12,88,100,...,220` — 每个单副本服务显式注释"单副本，不阻断 drain；多节点扩至 ≥2 后改 1"；多副本服务 minAvailable:1
  - `deploy/k8s/50-hpa.yaml` — gateway min=1/max=4（原 20）、learning min=1/max=6（原 15），注释"单节点 RD cap；多节点生产恢复原值"
  - `deploy/k8s/06c-redis-cluster.yaml` — 注释"单节点 RD 阶段可落地版…跨区分布靠 Pod 反亲和（单节点下同 node，多节点移植时按区 label 分布）"
  - `docs/design/architecture/multi-node-migration-checklist.md` — 完整迁移清单（基础设施/CI-CD/数据层/业务 4 大类，每项标当前状态+多节点目标+依赖+优先级）
  - `docs/design/architecture/multi-node-topology.md` — 3 地域 × 多 AZ 目标拓扑 + D2 容量规划 + D3 SLA/SLO 矩阵
  - `docs/design/architecture/tech-debt-register.md` — TD-01/02/14/17/18 均标"⏳ 待多节点"+ 偿还计划
  - VM 雪崩事故（PROGRESS.md）诚实记录单节点 9 核 VM 承载全栈的瓶颈
  - 代码级 DEFERRED 显式标注：`conversation-go/src/asr/provider.go:15,24,29`、`tts/provider.go:19,28,33` 全 DEFERRED-多节点清单#4 + 拒绝静默降级；`listening-go/src/main.go` transcribe DEFERRED 注释指 asr-adapter；`learning-java application.yml` hikari"多节点迁移靠副本数放大而非加大池"+ Jetty threads"单节点 VM 高负载防线"
- **Gap**: 部分单节点隐含假设未显式 DEFERRED 标注——典型：`conversation-go/src/hub/hub.go` 单实例 WS 会话内存路由迁移多副本时的 sticky-session/Redis pubsub 未标 DEFERRED，需逐项核对。整体仍是全项目最合规铁律之一

### 铁律 6: CI 门禁

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 6/10
- **Dimension**: 可测试性 / 可运维性
- **Evidence**:
  - `.github/workflows/ci.yml:1-364` — 完整 CI 存在：Java commons/services jacoco + Go vet/test + Python ruff/pytest + Rust cargo test/clippy `-D warnings` + Pact 契约 + 前端 pnpm turbo + Go 覆盖率门禁 + 安全扫描
  - `.github/workflows/ci.yml:45-58` — Java 公共包 jacoco 门禁阈值 **75%**（铁律要求 ≥80%），注释自承认"核心可单测逻辑 ≥75%"
  - `.github/workflows/ci.yml:328-344` — Go 覆盖率门禁阈值 **70%**（铁律要求 ≥80%）
  - `.github/workflows/ci.yml:359-364` — golangci-lint **仅对 esp-common-go**；Go extra 服务矩阵（go-extra job）只跑 `go vet`+`go test`，**未跑 golangci-lint(nilness/nilerr/gosec)**，违反 CLC 铁律④
  - `.github/workflows/ci.yml:296-318` — `go-extra` 矩阵含 admin-go/course-path-go 等 9 服务，**遗漏 asr-adapter-go / tts-adapter-go**（PROGRESS 声称已构建上线的新服务），CI 盲区
  - `.github/workflows/ci.yml:3-4` — 注释"本项目当前非 git 仓库"但 `git rev-parse` 确认 IS git repo + GitHub remote（origin https://github.com/tianqiongenze/English-Study-Platform.git）→ **CI 可能实际未在 push/PR 触发**，设计目标未落地
  - `.github/workflows/ci.yml:167-200` — build-images job `load: true` 不推送、**无 deploy 门禁、无 kubectl diff/argocd sync/GitOps 漂移检测**（DGO 铁律③缺）
  - `.github/workflows/ci.yml:252-260` — e2e job 仅 `workflow_dispatch`（手动），非自动门禁
  - `.pre-commit-config.yaml:1-45` — pre-commit 存在（gitleaks/golangci-lint/go-fmt），但 golangci-lint `files: ^tools/lco-proxy/.*\.go$` **仅限 lco-proxy**，其余 Go 服务无 pre-commit lint
  - 无架构约束门禁：CI 无"≥2 服务共用必下沉共享包"检测器、无"服务内复制"检测（铁律①无 CI 守门）
- **Gap**: 覆盖率阈值低于铁律（75%/70% vs 80%）、golangci-lint 仅覆盖子集、2 新 Go 服务无 CI、CI 实际未触发（非 git 假设过时）、无 GitOps 漂移门禁、无架构约束 CI

### 铁律 7: 诚实优先

- **Status**: 部分符合
- **Severity**: P2
- **Score**: 7/10
- **Dimension**: 可运维性
- **Evidence**（逐条核对 PROGRESS 声称 vs 实际）:
  - `docs/PROGRESS.md:167` 声称"listening-go main.go:167 硬编码 chunks 占位 → 接 asr-adapter…501 拒绝假数据" → `backend/services/listening-go/src/main.go:172-173` 实测 `slog.Warn("…hardcoded chunks removed (MPB)…")` + `c.JSON(501, …ASR_NOT_IMPLEMENTED)`。**声称属实**
  - `docs/PROGRESS.md:279` 声称"回写 06c-redis-cluster.yaml:60/128 7.2→8.0 + replicas 4→6" → `deploy/k8s/06c-redis-cluster.yaml:60,128` 实测 `image: redis:8.0-alpine`、`:49 replicas: 6`。**声称属实**
  - `docs/PROGRESS.md:244` 声称"BVV 内容校验：导出二进制 strings grep 到 L2/L3 符号" → 属实（见铁律 10）
  - `docs/PROGRESS.md:71` 声称"34/35 stack yaml 已 apply，asr-adapter + tts-adapter pod Running" → `deploy/k8s/34-asr-stack.yaml`/`35-tts-stack.yaml` 存在（运行态未核，只读审查不连集群）
  - **自我违规**：`docs/PROGRESS.md:4-5` 声明"硬上限 ~300 行"，实测 `wc -l` = **593 行**（2x 超限），自定上限未守
  - **过时声称**：`.github/workflows/ci.yml:3` + `CLAUDE.md` §3"项目非 git 仓库"与 `git rev-parse --is-inside-work-tree = true` + GitHub remote 事实矛盾，未纠正
- **Gap**: PROGRESS 自定 300 行上限实际 593 行；"非 git 仓库"过时假设未纠正。抽查的核心事实（listening 501/Redis 8.0/BVV）均属实，诚实度尚可

### 铁律 8: 生产域 Mock 零残留 (MPB)

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 7/10
- **Dimension**: 安全 / 可测试性
- **Evidence**:
  - `backend/services/conversation-go/src/llm/provider.go:31` — `if os.Getenv("LLM_MODE") == "mock" { return &MockProvider{} }`，env 开关，生产 manifest 未设 → 合法
  - `deploy/k8s/10-gateway-go.yaml:54,56` — `ASR_MODE: "http"` / `TTS_MODE: "http"` 生产实设，mock 不触发 → 合法
  - `backend/services/listening-go/src/main.go:167-173` — 硬编码占位已移除，改 501 拒绝假数据 → 合法
  - `backend/services/bff-java/src/main/java/com/esp/bff/edge/DownstreamForwarder.java:20` / `teacher-java/.../DubboConsumerConfig.java:14` — 仅注释提及 mock（测试域 `@MockBean`），无生产 mock 代码 → 合法
  - `grep -rn "MOCK|mock|stub|fake" deploy/k8s/` → **空**，生产 manifest 无 mock env → 合法
  - **缺口（残留风险）**：`backend/services/llm-orchestrator-rs/src/provider/mod.rs:83-85` — `// 兜底：无 env 配置时放一个 stub…chain.push(Box::new(StubProvider))`，且 `deploy/k8s/33-orchestrator-vocab.yaml:65-70` 所有 `LLM_PROVIDER_*` 用 `secretKeyRef optional: true` → **若 secret 缺失/为空，生产静默回退 StubProvider 返回 `[stub provider] echo: <prompt>` 假数据，无 hard fail、无告警**。StubProvider 非 env 开关而是零配置兜底，违反"mock 仅显式开关"
- **Gap**: orchestrator StubProvider 是零配置静默兜底（非显式 env 开关），secret optional:true 缺失时生产吐假 echo 无阻断

### 铁律 9: 写前核对目标系统 (WTV)

- **Status**: 不符合
- **Severity**: P1
- **Score**: 4/10
- **Dimension**: 安全 / 可运维性
- **Evidence**:
  - `scripts/ccswitch-cluster-bridge.sh:6` — 仅此一脚本注释提及 WTV 铁律（"确认 CC Switch 代理真实可达后再切 env"）
  - `scripts/seed_sessions_parallel.sh:24-30` — 直接 `kubectl -n esp-crdb exec crdb-0 -- cockroach sql --insecure --execute "INSERT INTO esp_speaking.sessions …"`，**无目标核对**（无确认 host/schema/cluster 是否为预期目标，硬编码 `esp_speaking.sessions`），误执行即静默灌错库
  - `scripts/apply_crdb_schema.py:19-25` — 读 `CRDB_HOST/CRDB_PORT` env 连 `root`/`sslmode=disable` 直接 apply schema section，**无"这是目标系统"确认守门**
  - `deploy/k8s/30-other-services.yaml:23-29` — teacher-java manifest 声明 `ESP_RW_STORAGE_TYPE=cockroachdb`/`ESP_RW_COCKROACHDB_HOST=crdb-public`/`ESP_RW_COCKROACHDB_DB=esp_teacher` → **配置作为事实源的模式存在**（WTV 的声明式基础），但脚本层未据此核对就写
  - 无集中 `verifyTarget()`/`confirmTarget()` helper 跨脚本复用（grep 仅 ccswitch 注释命中）
- **Gap**: 种子/schema 脚本直连直写无目标确认守门；无集中 WTV helper；config-as-truth 在 manifest 层存在但脚本层未强制核对

### 铁律 10: 镜像产物内容校验 (BVV)

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 5/10
- **Dimension**: 可运维性 / 安全
- **Evidence**:
  - `docs/PROGRESS.md:244` — llm-orchestrator-rs:1.3.0 派生构建后 BVV：`strings` grep 二进制符号 `L2 milvus store ok`/`neo4j_run`/`L1 exact hit`，对照 1.2.0 计数=0 证新版含 L2/L3 → **Rust 二进制 BVV 实操**
  - `docs/PROGRESS.md:439` — 1.3.1 镜像 BVV：grep `/v1/vector/search`=1（新）、`/v2/vector/search`=0（旧已移除）证 L2 修复在镜像内
  - `backend/services/bff-java/Dockerfile:5-6` / `learning-java/Dockerfile:4` / `teacher-java/Dockerfile:4` — `COPY target/*.jar /app/app.jar` fat-jar 模式，**无镜像内 `javap -p`/`jar xf BOOT-INF/classes/.../XxxApplication.class` 主类签名校验**
  - `.github/workflows/ci.yml:167-200`（build-images job）— `mvn package` + `docker build-push-action load: true`，**无任何镜像内容校验 step**（铁律⑩要求 build 后校验镜像内 app.jar 主类签名/内容指纹）
  - `grep -rn "javap|jar xf|BOOT-INF|Main-Class" scripts/ Makefile` → **空**，全仓无 Java 镜像内容校验脚本
  - `tools/lco-proxy/Dockerfile:21-39` — 多阶段 + digest pin（`@sha256:…`）+ `USER app` + HEALTHCHECK，SCS 强，**但亦无 post-build 内容校验**
  - RWP 联动缺口：`backend/services/bff-java/Dockerfile`/`learning-java`/`teacher-java` **均无 `ENV HOME`**（`grep -c "ENV HOME"` = 0/0/0），违反 RWP 铁律①；但 k8s manifest 已挂 emptyDir（`15-bff-java.yaml:66-72`/`20-learning-java.yaml:64-71`/`30-other-services.yaml:47-54`）缓解运行期，镜像层缺锚定
- **Gap**: Java fat-jar 镜像无 javap/BOOT-INF 主类校验（铁律⑩核心要求未落地）；CI build-images 无内容校验 step；lco-proxy/Rust 有实操但 Java 侧空白；Java Dockerfile 缺 ENV HOME（RWP 联动）

### 铁律 16: 声明式与 GitOps (DGO)

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 6/10
- **Dimension**: 可运维性
- **Evidence**:
  - `deploy/argocd/argocd-app.yaml:1-30` — ArgoCD Application 声明式同步 `deploy/k8s`，automated selfHeal=true，ServerSideApply + RespectManagedFields；GitOps 工具链已落地
  - `deploy/gitops/argocd-apps.yaml:1-40` — App-of-Apps 三子应用（esp-apps/esp-monitoring/esp-infra），但 repoURL/targetRevision 仍是 `<ORG>`/`main` 占位符未替换，**用户 git 迁入前不可 apply**
  - `deploy/k8s/*.yaml` — 23 个服务均有 manifest（逐条核对 backend/services/ 24 项含 integration-tests，23 业务服务全覆盖，含 conversation-go/llm-orchestrator-rs/vocab-srs-go 等此前疑似缺失项确认均在）
  - `.github/workflows/ci.yml` — **无 kubectl diff / argocd app diff / terra plan drift 检测**（grep 空），CI 漂移门禁缺失
  - `docs/PROGRESS.md:265,266,540` — 4 处"未回写/待回写 yaml"标记：13 服务 CPU request live patch（仓库仍 100m 是生产正确值，刻意不回写——合理）；learning-java/teacher-java 挂 /logs+/.dubbo emptyDir 已 live patch 待回写；neo4j.conf 未回写
  - `deploy/argocd/argocd-app.yaml:18-19` — prune=false（单节点 RD 保护 Running 资源，合理妥协但削弱 selfHeal 闭环）
- **Gap**: ① CI 无 drift 检测门禁（kubectl diff/argocd app diff）；② 4 处 live patch 待回写 yaml（learning-java/teacher-java emptyDir、neo4j.conf）；③ argocd-apps.yaml 占位符未替换（GitOps 未真正激活同步）；④ prune=false 致 deleted 资源不会被自动回收。核心声明式骨架在但闭环不严

### 铁律 17: 微服务韧性接线 (RWT)

- **Status**: 部分符合
- **Severity**: P0阻断
- **Score**: 5/10
- **Dimension**: 韧性
- **Evidence**:
  - `backend/shared/esp-common-go/middleware/ratelimit.go:10-19` — 共享令牌桶限流器（golang.org/x/time/rate，按 key 独立+自动淘汰）已下沉共享包（铁律1 合规）
  - `backend/shared/esp-common-go/middleware/circuitbreaker.go:13-22` — 共享熔断器含 Prometheus 指标（circuit_breaker_state 0/1/2）
  - `backend/services/gateway-go/src/main.go:137-138` — gateway 接线 HTTPRateLimit（100rps/桶200）+ HTTPCircuitBreaker（5 失败→30s 熔断），主入口韧性齐全
  - `backend/services/learning-java/src/main/resources/application.yml:199-217` — Resilience4j dbSlowCall 熔断（滑窗10/80%慢>5s/半开10探测/wait30s）+ timelimiter 10s，Java 侧 DB 熔断配置完整
  - `backend/services/conversation-go/src/llm/anthropic.go:62` — **裸调 `http.DefaultClient.Do(req)` 无 Timeout**，外部 LLM 调用无超时分级无重试退避（RWT P0 违规；主线程正修 forwardUpstream 但此路径非流式仍裸）
  - `tools/lco-proxy/internal/server/server.go:33-46` — lco-proxy 流式超时设计严谨（ResponseHeaderTimeout 60s + 独立 upCtx，不设 Client.Timeout 避免切断 SSE），LCO 韧性良好
  - `backend/services/conversation-go/src/llm/provider.go:72-75`、`cache/cache.go:124-127`、`pulsar/producer.go:49-58` — conversation-go 对 Redis/Pulsar 均接线超时+DialTimeout+SendTimeout
  - `backend/services/gateway-go/go.mod:45` — backoff/v5 仅 indirect（未在业务路径显式用重试退避）
- **Gap**: ① conversation-go anthropic.go 外部 LLM 裸调无超时无重试无熔断（P0）；② Resilience4j 仅 learning-java 配置，bff-java/teacher-java 等其他 Java 服务未配置；③ 重试退避未系统接线（backoff 依赖 indirect 未用）；④ 舱壁（bulkhead/独立连接池）未见显式配置；⑤ 降级路径未见契约测试覆盖。gateway + lco-proxy 韧性达标，但全链路未系统化

### 铁律 18: 供应链与秘密 (SCS)

- **Status**: 部分符合
- **Severity**: P0阻断
- **Score**: 6/10
- **Dimension**: 安全
- **Evidence**:
  - `.pre-commit-config.yaml:1-40` — gitleaks v8.21.2 秘密扫描 + golangci-lint + pre-commit-hooks（merge-conflict/large-file/换行）；gitleaks 覆盖 sk-/AKIA+/BEGIN PRIVATE KEY
  - `report/scs-implementation-2026-08-20.md:1-35` — SCS 5 项已补齐：SBOM（syft cyclonedx）、cosign 签名（graceful skip 无密钥）、digest pin（FROM @sha256）、requirements.lock（pip-compile 含 hash）、readOnly/cap_drop 说明
  - `.github/workflows/lco-proxy-ci.yml:48-70` — sbom + sign job 已入 CI（syft + cosign）
  - `deploy/k8s/00-namespace-secret.yaml:27` — **JWT_SECRET: "p4kun0TaPfqmQTn0QBZmk2B/xD4H6Xmsl2E/RCApbIE=" 硬编码明文入库**（注释说应走 External Secrets/Vault 但实际值已 commit，违反 SCS 第①条；虽标注是 dev/占位但已固化进 git）
  - `deploy/k8s/00-namespace-secret.yaml:30-31` — DB_USER=root + DB_PASS 空（生产无密码 root，SCS 最小权限违规）
  - `deploy/k8s/00b-rsa-jwt-secrets-split.yaml:1-25` — JWT RS256 私钥收敛（签发私钥仅 BFF 持有，其余仅公钥校验），RCE 扩散收敛到位；私钥经 scripts/gen-rsa-keypair.sh 灌入不入 git
  - `deploy/k8s/06-sca-middleware.yaml:234` — Seata password 用 `${SEATA_CONSOLE_PASSWORD}` env 占位（合规）
  - `deploy/k8s/monitoring/10-observability.yaml:636-655` — Prometheus ClusterRole/ClusterRoleBinding（监控需要，可接受但非最小权限——ClusterRole 粒度粗）
  - 锁文件：`pnpm-lock.yaml`/`tools/lco-proxy/go.sum`/`e2e/package-lock.json`/`apps/{web,h5,miniprogram}/package-lock.json` 齐全，但 backend/services Go 服务缺 go.sum 全覆盖核验（仅 gateway-go/conversation-go 见 go.sum）
  - 测试私钥：`backend/services/bff-java/src/test/resources/rsa/private.pem` 等为测试 RSA 密钥对（单测用，可接受但应标 `.gitignore` 或 rotate）
- **Gap**: ① **JWT_SECRET 明文硬编码入库（P0，立即轮转+走 External Secrets Operator）**；② DB root 无密码无最小权限应用账号；③ backend Go 服务 go.sum 覆盖不全；④ ClusterRole 粒度粗；⑤ 测试私钥虽可接受但未显式标白名单豁免。SCS 5 项 lco-proxy 闭环但生产 Secret 管理未收敛

### 铁律 19: 只读根文件系统可写路径 (RWP)

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 6/10
- **Dimension**: 可运维性
- **Evidence**:
  - `deploy/k8s/10-gateway-go.yaml:26`、`21-learning-worker.yaml:39`、`30-other-services.yaml:25,148,203`、`31-new-services.yaml:30,84,137`、`32-batch2-services.yaml:31,91,146,206,261`、`33-orchestrator-vocab.yaml:49`、`34-*.yaml`、`35-tts-stack.yaml:37` — 大量服务已开 readOnlyRootFilesystem:true（Go/Rust 服务为主）
  - `deploy/k8s/15-bff-java.yaml:65-72` — bff-java 防御性挂载 /.dubbo + /logs emptyDir（注释"未开 readOnly 当前不崩，防将来"）
  - `deploy/k8s/20-learning-java.yaml:64-71` — learning-java 已 DGO 回写挂 /logs+/.dubbo emptyDir
  - `deploy/k8s/21-learning-worker.yaml:78-87` — learning-worker readOnlyRootFilesystem:true 已开 + 挂 /.dubbo+/logs+/tmp emptyDir（RWP 完整修复）
  - `deploy/k8s/30-other-services.yaml:113-120` — teacher-java 同样回写挂载
  - `backend/services/*/Dockerfile` — **23 个服务 Dockerfile 全无 ENV HOME**（仅 `deploy/docker/jdk26-jre/Dockerfile` 和 `whisper-cpu/Dockerfile` 有）；bff-java/learning-java/teacher-java Dockerfile 仅有 OTEL/JAVA_OPTS ENV，**缺 ENV HOME=/home/<user> 锚定**（RWP 第①条违规）
  - `deploy/k8s/15-bff-java.yaml` — **未开 readOnlyRootFilesystem**（当前靠 emptyDir 兜底，但铁律要求 Java 服务 readOnly+emptyDir 全开）
- **Gap**: ① **23 个 Dockerfile 全缺 ENV HOME**（RWP 核心第①条，Java Dubbo 3.3.6 写 $HOME/.dubbo 会落 / 失败）；② bff-java/teacher-java/learning-java（部分）未开 readOnlyRootFilesystem；③ RWP 仅对已爆雷服务修复（learning-worker/learning-java/teacher-java），bff-java/其他 Go 服务 emptyDir 挂载未全局同步；④ CI 无 readOnly 服务必含 emptyDir 挂载的拦截门禁

### 铁律 20: 安全治理 by design

- **Status**: 部分符合
- **Severity**: P1
- **Score**: 6/10
- **Dimension**: 安全
- **Evidence**:
  - `docs/design/architecture/` — 21 份 ADR/治理文档（adr-index、engineering-ironrules、slo-error-budget、togaf-assessment、tech-debt-register、multi-node-migration-checklist 等）
  - `docs/design/sdd-*.md` + `srs-*.md` — 17 份 SDD/SRS 覆盖 9 服务（assessment/community/content/gamification/listening/llm-orchestrator/pronunciation/reading/vocab-srs/writing-coach），**24 服务仅覆盖 ~10**（bff-java/conversation-go/gateway-go/emotion-svc/admin-go/asr-adapter/live-tutor/media-learn/teacher-java/writing-py/course-path 缺独立 SDD）
  - `docs/design/sdd-content-go.md:113,121,155,384` — SDD 含认证列（认证鉴权 BFF 注入），但安全/风控/数据治理为**分散提及非独立章节**（RWP 要求 SDD 必含安全/治理/数据治理章节）
  - `deploy/k8s/10-gateway-go.yaml:73-74`、`15-bff-java.yaml:60-61`、`20-learning-java.yaml:54-55` — 健康检查 readinessProbe/livenessProbe 齐全（Go /readyz//livez，Java /actuator/health/readiness//liveness）
  - `backend/services/{bff,learning,teacher}-java/src/main/resources/application.yml` — 3 Java 服务 Nacos 注册（spring.cloud.nacos）；**其他 Java 服务未见，Go 服务无 Nacos 注册**（Go 走 gateway 聚合，可接受但服务发现非全 Nacos）
  - `docs/design/schema.sql` — 31 张表仅 **6 个 UNIQUE KEY**（uk_email/uk_code×2/uk_word/uk_user_word/uk_user_ach）；反刷/幂等键覆盖不足（如 session/turn/order 表缺 UK 防重）
  - `backend/shared/esp-common-go/middleware/` — 限流+熔断已下沉共享包（铁律1+20 合规，gateway 已接）
  - `deploy/k8s/40-networkpolicy.yaml` + `40b-traefik-ratelimit.yaml` — NetworkPolicy + traefik 限流层存在
- **Gap**: ① **14 服务缺独立 SDD**（24 服务仅 ~10 有）；② SDD 无统一安全/治理/数据治理独立章节（散落）；③ schema UK 覆盖率低（31 表 6 UK，session/order 等缺幂等键）；④ Nacos 注册仅 3 Java 服务，Go 服务发现非标准；⑤ 限流/熔断仅 gateway 接线，其他服务未全接（铁律20+17 重叠缺口）

---

## 第三部分：量化打分

### 3.1 逐铁律打分表

| 铁律 | 名称 | 分数(0-10) | 状态 | 严重度 | 维度 |
|---|---|---|---|---|---|
| 1 | 公共能力共享包优先 | 7 | 部分符合 | P1 | 架构基础 |
| 2 | 设计-实现-验证三态追踪 | 6 | 部分符合 | P1 | 架构基础 |
| 3 | 契约测试证明接线 | 6 | 部分符合 | P1 | 可测试性 |
| 4 | 无产物不声称完成 | 8 | 符合 | P2 | 可运维性 |
| 5 | 单节点限制显式 DEFERRED | 8 | 符合 | P2 | 可运维性 |
| 6 | CI 门禁 | 6 | 部分符合 | P1 | 可测试性 |
| 7 | 诚实优先 | 7 | 部分符合 | P2 | 可运维性 |
| 8 | 生产域 Mock 零残留 (MPB) | 7 | 部分符合 | P1 | 安全 |
| 9 | 写前核对目标系统 (WTV) | 4 | 不符合 | P1 | 安全 |
| 10 | 镜像产物内容校验 (BVV) | 5 | 部分符合 | P1 | 可运维性 |
| 11 | LLM 调用缓存优化 (LCO) | 9 | 符合 | P2 | 架构基础 |
| 12 | API 签名核实 (ASV) | 6 | 部分符合 | P1 | 可测试性 |
| 13 | 代码级正确性 (CLC) | 5 | 部分符合 | P0阻断 | 可测试性 |
| 14 | 有状态集群版本漂移 (SVD) | 4 | 部分符合 | P0阻断 | 可运维性 |
| 15 | 爆炸半径与可逆性 (BRR) | 6 | 部分符合 | P1 | 可运维性 |
| 16 | 声明式与 GitOps (DGO) | 6 | 部分符合 | P1 | 可运维性 |
| 17 | 微服务韧性接线 (RWT) | 5 | 部分符合 | P0阻断 | 韧性 |
| 18 | 供应链与秘密 (SCS) | 6 | 部分符合 | P0阻断 | 安全 |
| 19 | 只读根文件系统可写路径 (RWP) | 6 | 部分符合 | P1 | 可运维性 |
| 20 | 安全治理 by design | 6 | 部分符合 | P1 | 安全 |

### 3.2 五维度加权汇总

| 维度 | 权重 | 含铁律 | 加权得分 | 说明 |
|---|---|---|---|---|
| 架构基础 | 25% | 1(7)+2(6)+11(9) | (7+6+9)/3=7.33×25%=1.83 | 共享包下沉已推进/缓存设计工业级/Dubbo 配未接 |
| 韧性 | 20% | 17(5)+15(6)+7(7诚实属韧性旁证) | (5+6)/2=5.5×20%=1.10 | RWT 全链路未系统化/gateway+lco-proxy 达标 |
| 安全 | 25% | 8(7)+9(4)+18(6)+19(6)+20(6) | (7+4+6+6+6)/5=5.8×25%=1.45 | JWT_SECRET 明文(P0)/WTV 不符合/RWP 部分 |
| 可测试性 | 15% | 3(6)+6(6)+12(6)+13(5) | (6+6+6+5)/4=5.75×15%=0.86 | 契约仅 1 对/CI 阈值低/Go lint 缺失 |
| 可运维性 | 15% | 4(7)+5(9)+10(5)+14(4)+16(6) | (7+9+5+4+6)/5=6.2×15%=0.93 | 单节点 DEFERRED 强/BVV-SVD-DGO 弱 |
| **总分** | 100% | 20 条 | **6.18/10** | 加权 6.18；算术均分 6.15/10（123/200） |

> 总分约 **6.2/10（B-，距工业级 8.0+ 目标差 1.8 分）**。项目自评 7.34/10 偏乐观约 1.1 分（主要高估 RWT/SCS/RWP 的系统化程度与 CI 阈值）。

### 3.3 与工业级（10亿用户/100万并发）对标差距

- 单节点实压 533 rps / P95 25s / 7.3% 失败（vs 目标 5000 CCU）——容量差 ~10x，距 100万并发工业级基线差 ~1800x
- 所有业务服务 + TiDB 全组件 replicas=1（单节点限制，文档诚实标注 + 多节点迁移规划存在）
- Pulsar geo-replication / TiDB 跨 AZ Raft 均为规划态（多节点才落地）
- LCO 三层缓存是全项目最接近工业级的设计（L1 Redis Cluster + L2 Milvus 向量 + L3 Neo4j 知识图谱，全实装+监控指标）
- 读写分离 + Outbox 兜底 + Seata Saga 是工业级可靠性设计（真接线，非配而未接）
- 韧性（RWT）是离工业级最远的维度：全链路限流/熔断/超时/重试/舱壁/降级未系统化，仅 gateway+lco-proxy+learning-java(Resilience4j) 三处达标

---

## 第四部分：风控审查

### 4.1 单点故障 (SPOF)

| 风险 | 证据 | 严重度 | 现状缓解 |
|---|---|---|---|
| 所有 24 个业务服务 replicas=1 | `deploy/k8s/*.yaml` 全部 `replicas: 1` | P1（单节点限制） | PDB 覆盖（41-pdb.yaml 24 服务）+ HPA 存在（min=1）+ 多节点迁移规划（multi-node-topology.md） |
| TiDB 全组件 replicas=1（PD/TiKV/TiFlash/TiDB） | `07-tidb-tikv.yaml:119-683` 全 1 | P1 | 单节点限制，多节点跨 AZ Raft 是规划态 |
| Redis Cluster 3主3从全在同 node | `06c-redis-cluster.yaml:49 replicas:6` | P1 | hostname 模式根治 IP 漂移（ADR-11），跨区靠反亲和（单节点失效） |
| Nacos 单实例（宿主中间件） | launchd 常驻宿主非集群 | P1 | Nacos 是服务发现 SPOF，宕则注册/配置全断；多节点应 Nacos Cluster |
| 宿主中间件（Prometheus/Loki/CRDB通道/Redis）launchd 常驻 | 非集群化，单 Mac 宕则全断 | P1 | 单节点验证拓扑固有，多节点需下沉集群 |

### 4.2 数据一致性风险

| 风险 | 证据 | 严重度 | 现状缓解 |
|---|---|---|---|
| 读写分离 follower lag 致读后写不一致 | `learning-java application.yml:74,82-83` follower 读 | P2 | @DataSource(consistent=true) 强制读主 + 乐观锁（ADR-15）+ follower session_tracks_gtids 兜底 |
| Outbox Relay 单点（仅 learning-java） | shared-code-audit §C："Outbox 仅 learning 独享" | P2 | learning 有完整 OutboxRelay + 重试；但其他服务无 outbox 兜底，批量失败可能丢数据 |
| achievements/user_achievements 双 Owner 跨域写 | tech-debt TD-08 | P1 | 已登记待事件驱动重构 |
| Seata→Saga 迁移未完成 | ADR-06 Accepted 但 tech-debt 未标完成 | P2 | Saga 代码存在（TeacherSummarySaga），全局事务一致性依赖 Saga 补偿正确性 |
| Redis 各区独立不强同步 | multi-node-topology.md §1 | P2（设计权衡） | 灾备区靠业务重放/缓存重建，接受最终一致 |

### 4.3 安全风控

| 风险 | 证据 | 严重度 | 现状缓解 |
|---|---|---|---|
| **JWT_SECRET 明文硬编码入库** | `00-namespace-secret.yaml:24` `"p4kun0TaPfqmQTn0QBZmk2B/xD4H6Xmsl2E/RCApbIE="` | **P0** | 注释说"生产必须 External Secrets Operator/Vault"，但当前明文密钥已入库，违反 SCS 铁律①"秘密永不入库" |
| DB_USER=root 无密码 | `00-namespace-secret.yaml:25-26` + TiDB root 无密码 | P1 | 单节点验证拓扑，多节点必须最小权限应用账号（只 CRUD 不 DDL）+ 独立密码 |
| llm-orchestrator-rs StubProvider 零配置静默兜底 | `provider/mod.rs:83-85` + `33-orchestrator-vocab.yaml:65-70` secretKeyRef optional:true | P1 | secret 缺失时生产吐假 echo 无 hard fail 无告警，违反 MPB（见铁律8） |
| 镜像浮动 tag 无 digest pin（Redis/TiDB） | `06c:60 redis:8.0-alpine` 无 `@sha256` | P1 | tag 可被劫持重指（当年 SVD 事故根因），需 digest pin |
| NetworkPolicy 单节点 CNI 支持有限 | `40-networkpolicy.yaml` 注释"k3s flannel 支持有限" | P2 | 策略声明意图，多节点用 Calico/Cilium 强制 |
| securityContext 部分服务未开 readOnlyRootFilesystem | `15-bff-java.yaml:28-29` 有 runAsNonRoot/runAsUser 但第65行注释"未开 readOnly" | P2 | Java 服务挂了 emptyDir 防御但没开 readOnly（RWP 部分符合） |

### 4.4 灾备/可恢复性

| 风险 | 证据 | 严重度 | 现状缓解 |
|---|---|---|---|
| 多区域 DR Runbook 未演练 | tech-debt TD-14 P2 | P2 | multi-node-topology.md §4 有 Runbook 要点，未实演 |
| Pulsar Geo Replication 规划态 | `06d-pulsar-geo.yaml:368` 注释"多节点移植时加 azure,gcp" | P2 | 单集群，跨区消息最终一致是规划态 |
| TiDB 跨 AZ Raft 规划态 | multi-node-topology.md §1 | P2 | 单节点 replicas=1，RPO 0 强一致是多节点目标 |
| Redis 灾备区独立不复制 | multi-node-topology.md §1 | P2（设计权衡） | 灾备区靠业务重放，缓存可重建 |
| 无统一 backup/restore 自动化 | scripts 有 rollback 但无统一 backup 注册表 | P2 | crdb-probe-rollback.sh 等单点回滚存在 |

### 4.5 成本/性能瓶颈

| 风险 | 证据 | 严重度 | 现状缓解 |
|---|---|---|---|
| 单节点 533 rps 远低 5000 CCU 目标 | multi-node-topology.md §2 + tech-debt TD-02 | P1 | 多节点 + 分布式 Locust 是规划态 |
| LLM 调用长尾（writing P95 73s） | multi-node-topology.md §3 业务 SLO | P1 | LLM surge 容量 + 限流降级（RWT）；LCO 缓存省调用 |
| LCO embedding MPS 推理 8-34s 占满 VM CPU | PROGRESS.md VM 雪崩事故 | P1 | 限流/离峰/不与编译并行（已记 memory） |
| 9 核单节点 VM 承载全栈（业务+中间件+编译） | PROGRESS.md VM 雪崩 load 543 | P1 | 单节点固有，多节点分离业务/中间件/编译 |
| HPA maxReplicas 被降（gateway 4 原应 20） | `50-hpa.yaml` 注释"单节点 RD cap" | P2 | 多节点恢复原值 |

---

## 第五部分：重构方案

> 按优先级分级。每项含问题+根因+修复方案+影响范围+预估工作量。P0=上线阻断必须修，P1=上线前应修，P2=长期优化。

### P0 — 上线阻断（必须修）

#### P0-1: JWT_SECRET 明文硬编码入库（SCS 铁律①）
- **问题**：`deploy/k8s/00-namespace-secret.yaml:27` JWT_SECRET 明文 `"p4kun0TaPfqmQTn0QBZmk2B/xD4H6Xmsl2E/RCApbIE="` 已 commit 进 git
- **根因**：SCS 铁律①"秘密永不入库"未落实；虽注释说生产走 External Secrets Operator/Vault，但 dev 值已固化进仓库历史
- **修复方案**：① `git filter-repo` 清除 JWT_SECRET 历史（--replace-text）；② 立即轮转该密钥（已泄露视为泄露）；③ 改为 `JWT_SECRET: ""` + External Secrets Operator / Vault / `kubectl create secret --from-literal` 运行时注入；④ pre-commit gitleaks 加 JWT_SECRET 模式扫描防再犯
- **影响范围**：所有用 JWT 鉴权的服务（bff-java 签发 + learning/teacher 校验）
- **预估工作量**：1 人日（filter-repo + 轮转 + ES Operator 接入）

#### P0-2: conversation-go 外部 LLM 裸调无超时无熔断（RWT 铁律）
- **问题**：`backend/services/conversation-go/src/llm/anthropic.go:62` `http.DefaultClient.Do(req)` 无 Timeout，外部 LLM 调用无超时分级无重试退避无熔断
- **根因**：RWT 铁律③"超时分级（外部 LLM 30-60s）每层独立，禁裸 HTTP 调无超时"未接线；主线程正修 forwardUpstream 但此非流式路径仍裸
- **修复方案**：① 用 `&http.Client{Timeout: 60s}` 替换 DefaultClient（LLM 超时分级 60s）；② 加 context.WithTimeout + 重试退避（backoff/v5，3 次指数+jitter）；③ 接 esp-common-go/middleware CircuitBreaker（LLM 失败率阈值触发降级）；④ 降级返回缓存/默认/错误响应不阻塞主链；⑤ 契约测试覆盖降级路径
- **影响范围**：conversation-go LLM 调用链
- **预估工作量**：1.5 人日

#### P0-3: Go 服务静态检查门禁大面积缺失（CLC 铁律④）
- **问题**：13 个 Go 服务中 9 个（gateway/community/content/conversation/reading/listening/live-tutor/admin/asr/tts）无 golangci-lint 配置且 CI 不跑；Python `ruff check . || true` 门禁失效；Java 无 spotbugs/errorprone
- **根因**：CLC 铁律④"配了 linter 不运行=配而未接"；lco-proxy 有完整 .golangci.yml 但未推广到全 Go 服务
- **修复方案**：① 将 lco-proxy 的 `.golangci.yml`（nilness/nilerr/errcheck/staticcheck/govet/gosec）提升为仓库根级共享配置；② CI ci.yml 的 go-extra matrix 加 `golangci-lint run`（对全部 13 Go 服务）；③ Python `ruff check .` 去掉 `|| true`；④ Java pom.xml 加 spotbugs-maven-plugin + errorprone
- **影响范围**：全 24 服务 CI 门禁
- **预估工作量**：2 人日

#### P0-4: 有状态集群版本一致性无 CI 三态对齐门禁（SVD 铁律①）
- **问题**：无 spec.image == pod imageID == runtime INFO version 三态对齐 CI 检查（当年脑裂根因至今无机器防护）；`deploy/k8s/07-tidb-tikv.yaml.bak` 残留仓库；镜像浮动 tag 无 digest pin
- **根因**：SVD 铁律①"有状态集群操作前先核三态对齐"无 CI 守门；DGO 铁律③ drift 检测缺失
- **修复方案**：① 写 CI job：`kubectl get sts -o yaml` spec.image vs pod status.containerStatuses.imageID vs `redis-cli INFO version`/`cockroach version` 三态比对，不一致红；② 删除 `07-tidb-tikv.yaml.bak`（移出 gitops 路径）；③ 状态集群镜像改 `@sha256` digest pin（redis/tidb/pulsar）；④ ArgoCD ignoreDifferences 精确管理 image 字段
- **影响范围**：Redis/TiDB/Pulsar/Neo4j 等所有有状态组件
- **预估工作量**：2 人日

### P1 — 上线前应修

#### P1-1: bff Dubbo + seata + pulsar / teacher seata 死配置（铁律2 三态追踪）
- **问题**：`bff-java application.yml:77-83` 配 dubbo tri+nacos+esp-api 但 0 @DubboReference（下游全走 RestClient）；`:87,116` 配 seata+pulsar 全段但 0 `@GlobalTransactional`/0 `PulsarClient`（死配置）；`teacher-java application.yml:143` 配 seata 全段但已迁 Saga（`TeacherService:118`/`TeacherSummarySaga:25` 注释明示迁离 Seata）。**3 处"配而未接"**
- **根因**：shared-code-audit §C 仅记 bff Dubbo，本次新增 bff seata/pulsar + teacher seata；迁移后死配置未清，违反三态对齐
- **修复方案**：① bff：要么真接 Dubbo（@DubboReference 替 RestClient）要么删 dubbo+seata+pulsar 三段配置；② teacher：删 seata 配置段（已迁 Saga）；③ CI 加"配置声明 vs 代码 import"三态对齐检查器（grep yml 声明的 feature vs 代码引用，不匹配红）
- **影响范围**：bff-java 路由 + teacher-java 事务
- **预估工作量**：2 人日（接/删）+ 0.5 人日（CI 三态检查器）

#### P1-2: WTV 脚本层无目标确认守门（WTV 铁律）
- **问题**：`scripts/seed_sessions_parallel.sh:24-30` 直连直写 `esp_speaking.sessions` 无目标核对；`apply_crdb_schema.py` 直 apply 无确认
- **修复方案**：① 写 `scripts/verify_target.sh`（按 ENV ESP_RW_STORAGE_TYPE/CRDB_HOST/DB 核对目标库/schema/cluster，不对则 exit 1）；② 所有 seed/migration 脚本调用前先 verify_target
- **影响范围**：所有 DB 写脚本
- **预估工作量**：1 人日

#### P1-3: BVV Java 镜像无主类校验 + ENV HOME 缺失（BVV+RWP 联动）
- **问题**：Java fat-jar 镜像无 `javap -p`/`jar xf BOOT-INF` 主类签名校验；23 Dockerfile 全缺 `ENV HOME`
- **修复方案**：① CI build-images job 加 post-build 校验 step：`docker run --entrypoint sh <img> -c "jar xf /app/app.jar BOOT-INF/classes/.../XxxApplication.class && javap -p ... | grep <mainMethod>"`；② 23 Dockerfile 加 `ENV HOME=/home/app`（useradd -d）
- **影响范围**：所有 Java 服务镜像
- **预估工作量**：1.5 人日

#### P1-4: DGO GitOps 未激活 + 4 处 live patch 待回写
- **问题**：argocd-apps.yaml repoURL 占位符未替换（GitOps 未真激活同步）；CI 无 kubectl diff drift 门禁；4 处 live patch 待回写 yaml
- **修复方案**：① 替换 repoURL/targetRevision 为真实仓库；② CI 加 `argocd app diff` drift 检测；③ 回写 4 处 live patch 到 yaml
- **影响范围**：全部部署
- **预估工作量**：1.5 人日

#### P1-5: RWP bff/teacher Java 未开 readOnly + 全局同步
- **问题**：bff-java/teacher-java 未开 readOnlyRootFilesystem；emptyDir 挂载未全局同步所有服务
- **修复方案**：① Java 服务全开 readOnlyRootFilesystem:true + 挂 /.dubbo+/logs/nacos+/tmp emptyDir；② CI 扫 yaml readOnly 服务必含 emptyDir 挂载否则红
- **影响范围**：bff-java/teacher-java/其他未开 readOnly 服务
- **预估工作量**：1 人日

#### P1-6: MPB llm-orchestrator-rs StubProvider 零配置静默兜底
- **问题**：`provider/mod.rs:83-85` 无 env 时静默回退 StubProvider 返回假 echo，secret optional:true 缺失时无 hard fail 无告警
- **修复方案**：改为显式 env 开关（`LLM_STUB=1` 才启用），缺失 secret 时 hard fail + 告警而非静默兜底
- **影响范围**：llm-orchestrator-rs
- **预估工作量**：0.5 人日

#### P1-7: RWT 全链路韧性系统化
- **问题**：Resilience4j 仅 learning-java 配置；重试退避/舱壁未系统接线；降级路径无契约测试
- **修复方案**：① 全 Java 服务接 Resilience4j（DB/RPC/LLM 分级熔断+超时）；② 全 Go 服务 wrap esp-common-go middleware（限流+熔断）；③ 接 backoff/v5 重试退避；④ 降级路径契约测试
- **影响范围**：全 24 服务
- **预估工作量**：4 人日

#### P1-8: 安全治理 14 服务缺 SDD + schema UK 覆盖低
- **问题**：24 服务仅 ~10 有独立 SDD；SDD 无统一安全/治理/数据治理章节；31 表仅 6 UK（session/order 缺幂等键）
- **修复方案**：① 补 14 服务 SDD（含安全/治理/数据治理独立章节）；② schema 加 session/order 等表 UK 幂等键防重
- **影响范围**：缺 SDD 服务 + schema
- **预估工作量**：5 人日

#### P1-9: 契约测试覆盖全 11 路由 + Pulsar/Outbox E2E
- **问题**：Pact 仅 teacher-learning 一对，未及全 11 路由（TD-03）；**Pulsar 接线无契约 IT**（T3 注释自承"Pulsar 跨实例广播 @Disabled"，无生产/消费闭环验证）；**Outbox relay 无端到端 IT**（content-go outbox_relay_test.go 只测退避算法，未证 outbox 表→Pulsar 重投闭环）
- **修复方案**：① 补全 11 Dubbo 路由 Pact consumer/provider；② 写 Pulsar 生产+消费 E2E IT（跨实例广播启用+验证）；③ 写 Outbox relay E2E IT（DB outbox 表→relay→Pulsar 重投→消费落库 全闭环）
- **影响范围**：Dubbo esp-api 全路由 + Pulsar 消息链路 + Outbox 兜底链路
- **预估工作量**：4 人日（TD-03 估值 3 人日 + Pulsar/Outbox E2E 1 人日）

### P2 — 长期优化

#### P2-1: 共享包下沉残留（esp-common-rs/health + conversation-go Redis 客户端 + admin-go JWT + 跨语言 traceId）
- **问题**：`esp-common-rs/src/health.rs` 4 Rust 服务零接线；conversation-go 5 处自建 Redis Cluster 客户端（`cache/cache.go:19`/`llm/cached.go:44`/`llm/provider.go:13`/`hub/hub.go:23`/`session/manager.go:14`）未复用 esp/common-go/redisx；admin-go `src/jwt.go:45` 自实现 RSA 验签与 gateway-go gojwt 重复；跨语言 traceId 键名不一致（Java `traceId`/Go 混用/Rust `trace_id`/Python `trace_id`）
- **修复方案**：① conversation-go 5 处 Redis 客户端改用 esp/common-go/redisx（gamification/course-path/gateway 已示范）；② admin-go JWT 改用共享 BaseJwtValidator + role 参数化；③ Rust 服务接 esp_common::health；④ 统一 trace_id 键名跨语言
- **影响范围**：conversation-go/admin-go/4 Rust 服务
- **预估工作量**：3 人日

#### P2-2: 多节点迁移落地（消除单节点 SPOF）
- **问题**：所有服务 replicas=1 + TiDB 全 1 + Pulsar geo 规划态 + 单节点 533 rps 远低 5000 CCU
- **修复方案**：按 multi-node-migration-checklist.md + multi-node-topology.md 落地多节点（跨 AZ 反亲和 + PDB minAvail=1 + HPA 恢复原值 + TiDB 跨 AZ Raft + Pulsar Geo + Redis 各区独立）
- **预估工作量**：16 人日（TD-01/02/14/17/18）

#### P2-3: LCO L3 RAG 真实注入 + 月度成本核算
- **问题**：L3 Neo4j 仅暴露响应头 EnhancedContext，未真实注入改上游请求体（DEFERRED）；月度成本核算脚本未见
- **修复方案**：① L3 RAG 注入上游请求体；② 写月度成本对比脚本（有缓存 vs 无缓存，目标省 >40%）
- **预估工作量**：2 人日

#### P2-4: DR Runbook 实演
- **问题**：多区域 DR Runbook 未演练（TD-14）
- **修复方案**：region 切换演练 + 数据一致性校验
- **预估工作量**：2 人日

#### P2-5: 数据生命周期与字段级 DLP 分级
- **问题**：learning_events 30 亿归档/分区/冷存未定义（TD-11）；字段级 DLP 分级未标（TD-12）
- **修复方案**：定义数据生命周期 + 字段 DLP 标注
- **预估工作量**：3 人日

### 重构方案汇总

| 优先级 | 条数 | 总工作量(人日) | 关键阻断项 |
|---|---|---|---|
| P0 | 4 | 6.5 | JWT_SECRET 明文 / LLM 裸调 / Go lint 缺失 / SVD 无门禁 |
| P1 | 9 | 20.0 | Dubbo+seata/pulsar 死配置 / WTV / BVV / DGO / RWP / MPB / RWT 全链路 / 安全治理 / 契约+Pulsar/Outbox E2E |
| P2 | 5 | 26.0 | 共享包残留 / 多节点 / LCO RAG / DR 演练 / 数据治理 |
| **合计** | **18** | **52.5** | — |

> 注：与项目自有 tech-debt-register.md（20 条/44 人日）高度重合，本审计的增量缺口主要为 SCS JWT_SECRET 明文（P0）、RWT conversation-go 裸调（P0）、CLC Go lint 缺失（P0）、SVD 无门禁（P0）四项上线阻断级，其余多与 tech-debt 登记项一致。

---

## 总结

### 审计结论

- **总分**：6.18/10（B-，加权五维度；算术均分 6.15/10，123/200）
- **P0 缺口数量**：4 项（上线阻断）
  1. JWT_SECRET 明文硬编码入库（SCS）
  2. conversation-go 外部 LLM 裸调无超时无熔断（RWT）
  3. Go 服务静态检查门禁大面积缺失（CLC，9/13 服务无 golangci-lint）
  4. 有状态集群版本一致性无 CI 三态对齐门禁（SVD）
- **P1 缺口数量**：9 项（上线前应修，含 bff Dubbo 配未接/WTV 不符合/BVV Java 无主类校验/DGO GitOps 未激活/RWP ENV HOME 缺失/MPB StubProvider 静默兜底/RWT 全链路未系统化/安全治理 14 服务缺 SDD/契约仅 1 对）
- **P2 缺口数量**：5 项（长期优化，含共享包残留/多节点迁移/LCO L3 RAG 注入/DR 演练/数据治理）

### 一句话结论

ESP 架构在"设计文档完整度 + 诚实记录单节点限制 + LCO 三层缓存工业级 + 读写分离/Outbox/Seata 真接线"四方面达标，但在"生产安全（JWT_SECRET 明文）、全链路韧性（LLM 裸调）、静态检查门禁（Go lint 缺失）、状态集群版本防护（无三态对齐 CI）"四项 P0 上线阻断级缺口未闭合，距工业级 8.0+ 目标差 1.8 分，需优先修 4 项 P0（约 6.5 人日）方可谈上线。

### 与项目自评的偏差

- 项目自评 7.34/10（B+），本审计 6.2/10（B-），差 1.1 分
- 偏差主因：项目自评高估了 RWT（韧性全链路）/SCS（Secret 管理）/RWP（ENV HOME 全缺）/CI（阈值 75%/70% 低于铁律 80%）的系统化程度
- 本审计与项目 tech-debt-register.md（20 条/44 人日）高度一致，增量缺口为 4 项 P0 + WTV 不符合

### 最合规（≥7分）与最薄弱（≤5分）

- **最合规**：铁律11 LCO 三层缓存（9分）、铁律4 无产物不声称完成（8分）、铁律5 单节点 DEFERRED（8分）、铁律7 诚实优先（7分）、铁律1 共享包优先（7分，较 08-15 大幅改善）、铁律8 MPB（7分）
- **最薄弱**：铁律9 WTV（4分，不符合）、铁律14 SVD（4分）、铁律13 CLC（5分）、铁律17 RWT（5分）、铁律10 BVV（5分）

### 关键正向发现

1. **LCO 三层缓存全实装**（L1 Redis + L2 Milvus + L3 Neo4j + Prometheus 指标 + 响应头元数据）——全项目工程化最扎实的部分，9/10
2. **Java 共享包下沉已推进**——esp-web-common（GlobalExceptionHandler/AppLogClient/TraceIdUtil）+ esp-common-java JWT starter（BaseJwtValidator/BaseJwtAuthFilter）已建并被 3 Java 服务（bff/learning/teacher）import，较 08-15 自审的"待建"状态大幅改善；esp-common-go/redisx 已被 gamification/course-path/gateway 3 服务复用（消除 NewClusterClient 重复）。残留：conversation-go 5 处自建 Redis 客户端、admin-go JWT 重写、esp-common-rs/health 零接线
3. **可靠性设计真接线**——读写分离（follower + @DataSource + 乐观锁）、Outbox（关键事件 DROP 兜底 + 批量失败改写 + Relay 重试）、Seata Saga 均非配而未接
4. **单节点限制诚实标注**——PDB/HPA/redis/tech-debt/multi-node 清单全显式 DEFERRED + 多节点迁移规划完整，9/10
5. **GitOps 骨架在**——ArgoCD selfHeal=true + ServerSideApply，虽占位符未替换未真激活

### 关键负向发现

1. **JWT_SECRET 明文入库**（P0）——`00-namespace-secret.yaml:27` 硬编码，已 commit 进 git 历史，违反 SCS 铁律①，需 filter-repo 清+轮转
2. **conversation-go LLM 裸调**（P0）——`anthropic.go:62` `http.DefaultClient.Do(req)` 无超时无熔断无重试，外部 LLM 调用全链路无韧性
3. **9/13 Go 服务无 golangci-lint**（P0）——CLC 铁律④"配了不运行=配而未接"，生产 nil panic 可逃逸 CI
4. **状态集群无版本一致性 CI 门禁**（P0）——SVD 当年脑裂根因至今无机器防护，浮动 tag 可被劫持
5. **3 处"配而未接"死配置**（P1）——bff Dubbo（yml 配 tri+nacos+esp-api 但 0 @DubboReference）+ bff seata/pulsar（yml 配全段但 0 @GlobalTransactional/0 PulsarClient）+ teacher seata（yml 配但已迁 Saga），shared-code-audit 仅记 bff Dubbo，本次新增 2 处
6. **GitOps 未真激活**（P1）——argocd-apps.yaml repoURL 是 `<ORG>` 占位符，CI 无 drift 检测，4 处 live patch 待回写

---

> 审计完成。报告路径：`/Users/joelgong/claude-Projects/English-Study-Platform/report/audit/architecture-audit-2026-08-20.md`
> 证据来源：5 个并行 fork 分支（结构勘察 + 铁律 6-10/11-15/16-20）+ 主线程补充勘察（铁律 1-5 + RWT 修正 + 风控审查）。
> 审查性质：只读，未改任何代码。对照全局 CLAUDE.md 20 条工程铁律逐条审查，证据优先，诚实记录缺口。
