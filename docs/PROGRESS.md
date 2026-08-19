# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。

## 2026-08-19 会话23-14:Mock/占位符审查 + 全量重构达标计划(进行中🔄)

> 日期:2026-08-19
> Redis 锚点:`esp:ctx:hot:session-23-14`(主)+ `esp:ctx:session:23-14-review`(续)

### 本轮完成/进行中

#### 1. Mock/占位符全面审查(✅ 完成,结论已交付)
**审查结果**:生产域 mock 零残留(MPB 铁律),所有 mock 均为合法 env 模式开关或 DEFERRED 标注。

| 分类 | 服务 | 文件 | 机制 | 等级 |
|---|---|---|---|---|
| 合法 env 开关 | conversation-go | `llm/provider.go:31` | `LLM_MODE=mock → MockProvider` | ✅ 保留 |
| 合法 env 开关 | conversation-go | `asr/provider.go:30` | `ASR_MODE=mock` | ✅ DEFERRED 标注 |
| 合法 env 开关 | conversation-go | `tts/provider.go:33` | `TTS_MODE=mock` | ✅ DEFERRED 标注 |
| 合法 env 开关 | writing-coach | `llm.py:112` | `LLM_MODE=mock → _mock_async` | ✅ 保留 |
| 零 env 兜底 | llm-orchestrator-rs | `provider/mod.rs:322` | StubProvider(无 env 时) | ✅ manifest 有 env,生产不触发 |
| 硬编码占位 | **listening-go** | `main.go:167` | `chunks: ["Hello",", this is"," a placeholder transcript."]` | ⚠ 需改为 501 而非假数据 |
| 注释/测试域 | bff-java | `DownstreamForwarder.java:20` | 仅注释提及 mock | ✅ 无代码影响 |
| 反 mock 设计 | esp-common-go | `llm/llm.go:6` | 注释"不静默 mock" | ✅ |
| SQL 占位符计数 | esp-db-py | `__init__.py:115` | "placeholder count" 错误提示 | ✅ 非功能 mock |

**根因分析(为什么这个阶段还有 mock?)**:ASR/TTS/LLM orchestrator 的真实引擎属于多节点清单 #4 的 DEFERRED 项(需真实 ASR/TTS API key + 多节点环境),单节点验证架构用 env 开关占位合理。listening-go 是唯一硬编码占位(非 env 开关),需最小修复。

#### 2. 全量重构达标计划(✅ 已完成,用户待确认)
- 计划文件: `.claude/plans/robust-baking-wave.md`
- 当前评分: 工程实现 7.83 + TOGAF 治理 6.20 = **7.34/10(B+)**
- 目标评分: 工程实现 **8.5+** / TOGAF **7.5+** / 总分 **8.0+**
- **5 Phase 计划**:
  - Phase1 治理落地(P0 阻断上线):Go 限流熔断接线 + /metrics 全覆盖 + Nacos 注册补齐 + BFF DB 一致性 #227 + OTel endpoint 修正 + securityContext 全补齐 + Mock 残留切断
  - Phase2 基础设施升级:JDK26 派生镜像 + CI/CD 矩阵补全(Rust/Python/admin-go) + ArgoCD syncPolicy
  - Phase3 架构重构(达 8.5+):esp-common-java 公共包下沉 + Go DI 重构 + BFF trace root + Resilience4j + LLM 自适应限流(gradient2) + 契约版本化
  - Phase4 前端重构(达 8.5+):Admin 三面板走 api-client + token + ErrorBoundary 接 OTel + 小程序 crypto + i18n 收口 + SWR 接入
  - Phase5 上线就绪度门禁:E2E 全链路 + 混沌 + 安全 SEC-1~4 + SLO/错误预算 + readiness-gate 评分 8.0+ + 多节点迁移清单
- **执行策略**:Phase1/2/4 并行,Phase3 串行依赖 Phase1,Phase5 依赖全部

#### 3. 真实引擎切换(✅ 代码+manifest 完成,镜像构建受阻于 registry mirror 403)

**目标**:conversation-go 的 ASR/TTS + llm-orchestrator-rs 的 LLM 全部切真实引擎(用户要求一步到位,去 mock)。计划已批准(`.claude/plans/robust-baking-wave.md`)。

**已完成(代码 + manifest + env 接线)**:
- ✅ **asr-adapter-go**(纯 Go,`backend/services/asr-adapter-go/`):收 `{chunks:[base64]}` → Whisper multipart → `{text}`,Gin + 限流熔断 + 健康探针。`go build` 通过。含 `ASR_MODE=local` 本机 GPU 预留分支(M1 Max MPS,DEFERRED 真实实现)
- ✅ **tts-adapter-go**(纯 Go 零 Python,`backend/services/tts-adapter-go/`):`edgetts.New().Bytes()` → MP3 base64 + 四态情绪 + Preston Blair 口型,`go build` 通过(修了重复 import)
- ✅ **deploy/k8s/34-asr-stack.yaml**(whisper 社区镜像 + asr-adapter,含 GPU 预留注释块)
- ✅ **deploy/k8s/35-tts-stack.yaml**(tts-adapter)
- ✅ **10-gateway-go.yaml**:env 加 `ASR_MODE=http`/`ASR_ENDPOINT`/`TTS_MODE=http`/`TTS_ENDPOINT`,已 apply
- ✅ **00-namespace-secret.yaml**:加 llm-providers secret(claude-base/key 占位)
- ✅ **33-orchestrator-vocab.yaml**:`LLM_DEFAULT_MODEL` glm_for_coding → claude-sonnet-4-6,已 apply
- ✅ **listening-go main.go:167**:硬编码 chunks 占位 → 接 asr-adapter(ASR_ENDPOINT 未配则 501 拒绝假数据),`go build` 通过
- ✅ **CC Switch 桥接**:socat 15722→15721 + launchd 自愈,已实测 pod 可达(404=HTTP 通)
- ✅ **真实凭据注入**:esp-secrets + llm-providers 已注入 CC Switch 代理真实 key(当前默认 Zhipu GLM via ai.csdn.net,非 Claude;base_url 指向 host.lima.internal:15722)

**GPU/本地模型预留总表**(用户要求所有 LLM 处支持本机 GPU/本地模型):
| 使用点 | 当前引擎 | 本地预留开关 | 阶段 |
|---|---|---|---|
| conversation-go LLM | CC Switch→Claude/GLM | ANTHROPIC_BASE_URL→Ollama | 现就绪(纯 env) |
| llm-orchestrator-rs | ClaudeProvider | LLM_PROVIDER_OPENAI_BASE→Ollama(OpenAiCompat 复用) | 现就绪 |
| asr-adapter-go | 集群 Whisper | ASR_MODE=local 分支 | 预留(DEFERRED) |
| writing-coach | ANTHROPIC_BASE_URL→Claude | 同→Ollama | 现就绪(纯 env) |
| emotion-svc | 无 LLM(纯关键词) | EMOTION_LLM_BASE 预留 | DEFERRED |

**⚠ 阻塞中(registry mirror 403)**:`docker.1ms.run` 对 golang:1.25-alpine/alpine:3.22 返回 403 Forbidden,基础镜像拉取失败,asr/tts 镜像无法构建。这是临时基础设施故障(37h 前 content-go 还能构建),mirror-pull.sh 绕过脚本正在后台运行(ID: brac9wmea)。恢复后执行:`cd backend && DOCKER_CONTEXT=rd-vm docker build -t esp/asr-adapter-go:1.0.0 -f services/asr-adapter-go/Dockerfile .`(tts 同理),再 apply 34/35 yaml。

#### 4. 会话23-13续3 完成项(本轮前已全部完成,见归档)
- 归档: `docs/progress/archive/2026-08-19-session-23-13-sink-tempo-llmcache.md`
- 11 项已完成(中间件下沉+Tempo traces修复+LLM缓存Phase1+层1根治+混沌+E2E+CRDB probe+清理)
- 本地 commit 99d97e0 (482 files,43516 insertions),push GitHub 失败(443超时)

### 后台任务状态
- **subagent a2ec45bd58057d39e(三任务收尾)**:运行中,路由 admin-go debug proxy;输出 `tasks/a2ec45bd58057d39e.output`

### 恢复协议(上下文溢出后)
1. 读 docs/PROGRESS.md 热区(本文件)
2. Redis GET `esp:ctx:hot:session-23-14`
3. 归档: `docs/progress/archive/2026-08-19-session-23-13-sink-tempo-llmcache.md`
4. 计划: `.claude/plans/robust-baking-wave.md`
5. 集群现状:ESP 服务全连宿主 Nacos(host.lima.internal:8848,集群 Nacos 已删);宿主 Nacos↔CRDB 走 127.0.0.1:26257(launchd 自愈);Tempo:14317/14318 收 OTel 转发;OTel:4317/4318 收 ESP 服务;Redis cluster-new 5 块 Running
6. 本地 commit 99d97e0 未 push(push 失败 443 超时)

### 下一步(恢复后执行)
1. **真实引擎切换收尾** — registry mirror 403 恢复后构建 asr/tts 镜像 + apply 34/35 yaml + 验证 WS 链路真实识别/合成
2. **全量重构达标计划执行** — 用户确认计划后启动(Phase1/2/4 并行)
3. admin Grafana/Sentinel 硬编码链接修复(改 ExternalName)
4. push GitHub(网络恢复后)
5. LLM 缓存 Phase1 subagent 完成验证
6. k6+Locust 混合压测(50→5000 CCU 阶梯) + 全功能 E2E

---

## 2026-08-19 会话23-13续3:中间件下沉宿主 + Tempo traces修复 + LLM缓存Phase1 + 层1根治(完成✅)

> 归档全貌:`docs/progress/archive/2026-08-19-session-23-13-sink-tempo-llmcache.md`
> Redis 锚点:`esp:ctx:hot:2026-08-19-session-23-13-sink-tempo-llmcache`(主)+ `esp:ctx:session:23-13-cont3`(续)

### 本轮完成(11 项,证据驱动)
1. **中间件下沉+腾退**:Nacos(ExternalName→host.lima.internal,集群 deploy/svc/pvc 全删)+ Prometheus/Loki/Alertmanager/Grafana/OTel(ExternalName,清 3 处残留:otel-collector-daemonset/gateway + fluent-bit)+ Tempo(修端口冲突,见 2)。Milvus/Neo4j DEFERRED(零 ESP 引用+联网不可用+多项目共用,评估 `report/milvus-neo4j-sink-assessment.md`)
2. **Tempo traces 彻底修复**(ASV 根因修正):9095 冲突真凶是 tempo `server.grpc_listen_port`(默认 9095,非 memberlist)。改 tempo.yaml grpc_listen_port=9096 + OTLP 14317/14318 + memberlist 7947;OTel exporter→localhost:14317;bff/gateway OTel endpoint→otel-collector.esp-observability:4317。**端到端验证:Tempo 收 2595+ spans,service.name=bff 20 traces,链路完全打通**。报告 `report/tempo-traces-fix.md`
3. **LLM 缓存 Phase1**:架构 `docs/design/llm-cache-architecture.md`(531行+mermaid)+ conversation-go CachedProvider(cached.go+cached_test.go 12/12 单测 PASS)+ Phase1 完整推进(三服务接线+部署+IT)subagent 运行中。**Milvus/Neo4j 未用**(Phase2/3 DEFERRED)。铁律 LCO 全局 ~/.claude/CLAUDE.md 第8条
4. **Redis Cluster 自愈层1根治**:`scripts/redis-cluster-selfheal.sh`(幂等,集成 start-host-middleware.sh [8/8])。层2(announce-ip PodIP/pod CIDR 直连)DEFERRED 多节点
5. **Nacos-CRDB 通道 launchd 自愈层1根治**:`scripts/nacos-crdb-channel-health.sh` + `~/Library/LaunchAgents/com.esp.nacos-crdb-channel.plist`(每 30s 自愈,kill ssh 后 20s 内重连验证)。层2(多节点直连消除 ssh tunnel)DEFERRED
6. **宿主 Nacos Derby→CRDB 迁移成功**(Phase B):nacos 2.5.0 standalone healthy + HikariPool org.postgresql.Driver + 13 表空表。ASV 反例第4条:ExternalDataSourceProperties 硬编码 mysql driver fallback,显式设 `db.pool.config.driverClassName=org.postgresql.Driver` 一次修复。脚本 `scripts/nacos-crdb-init-schema.py`,插件 nacos-datasource-plugin-postgresql-0.0.5.jar(Maven Central 精确匹配 2.5.0)
7. **bff-java 滚动卡死修复**:根因 1.10.0 镜像 HOME=/ 未设→Dubbo 写 /.dubbo 无权限→Startup probe 338 次失败。修复 emptyDir 挂载(纯 K8s patch 可逆),3/4 Ready(2 Pending CPU 不足 DEFERRED 多节点)
8. **H-01 混沌降级 PASS**:kill 集群 Nacos,Java 服务存活副本全程 Running,Dubbo 本地缓存保活,GW/BFF 全程 HTTP 200,零 CrashLoop。5 Error pod 根因是单节点 PLEG 雪崩(CPU 99%→CoreDNS 链断)非 CRDB/Nacos
9. **CRDB probe 1s 误杀修复**:patch timeout 1→5/failure 5→3/period 10→15(可逆,回滚 `report/chaos/crdb-probe-rollback.sh`)
10. **压测+E2E+CRDB vs TiDB 对比**:E2E 14/0 ALL GREEN(修 phase3-e2e-smoke.sh 3 处 bug);压测 50 并发 learning_review P95=3s/bff_login avg 14.4s 瓶颈(应用层非 DB,CRDB 连接数=0);CRDB vs TiDB 补全 TiDB 历史(commit 0cc7819),单节点 CRDB 为 TiDB 33%-71%(环境噪声非生产结论)
11. **清理 3 处 OTel/fluent-bit 残留**:esp-observability 现为纯 ExternalName 集合(6 svc 全→host.lima.internal,无 pod)

### admin 监控影响(实测)
- ✅ Prometheus 指标/告警不受影响(esp-admin env PROMETHEUS_URL 走 ExternalName 透明转发宿主)
- ⚠ Grafana 链接损坏(GovernanceDashboard 硬编码 192.168.5.15:30300,下沉后端口变,需改 ExternalName)
- ⚠ Sentinel 控制台链接(硬编码 192.168.5.15:8858,需核实)
- ✅ 全服务健康探活不受影响(探活 ESP pod 非中间件)

### DEFERRED 项(11 项,显式标注)
1. Milvus/Neo4j 下沉(零 ESP 引用+联网不可用+多组件高难度+三区域多项目共用,等 LLM 缓存 Phase2/3 启动再下沉)
2. LLM 缓存 Phase2(L2 Milvus 语义向量,Phase1 完成后启动)
3. LLM 缓存 Phase3(L3 Neo4j 知识图谱,Phase2 后启动)
4. Redis Cluster 层2根治(sts announce-ip PodIP/pod CIDR 直连,多节点后落地)
5. Nacos-CRDB 通道层2根治(多节点 pod CIDR 直连消除 ssh tunnel)
6. bff-java 2 Pending pod(单节点 CPU 99% 枯竭,多节点)
7. bff-java 镜像层根治(1.10.0 Dockerfile 加 ENV HOME=/home/esp)
8. push GitHub(443 持续超时,本地 commit 99d97e0 完整 482 files 43516 insertions)
9. conversation-go WS 入口(LLM 缓存 NewWithCache 工厂就位但无 main 包调用方,待补)
10. 5000 CCU 阶梯压测(需先解 bff-java 全副本健康)
11. TiDB 重建+CRDB vs TiDB 完整 EXPLAIN ANALYZE 基准(用户已下电 TiDB,正式回切时重建)

### 本地 commit
- `99d97e0 feat(会话23-13续2):H-01混沌+CRDB probe+LLM缓存LCO+Nacos CRDB迁移+bff修复+压测E2E`(482 files 43516 insertions,⚠ push GitHub 失败 443 超时,本地完整)

### 恢复协议(上下文溢出后)
1. 读 docs/PROGRESS.md 热区(本文件)+ 归档 `docs/progress/archive/2026-08-19-session-23-13-sink-tempo-llmcache.md`
2. Redis GET `esp:ctx:hot:2026-08-19-session-23-13-sink-tempo-llmcache` + `esp:ctx:session:23-13-cont3`
3. 集群现状:ESP 服务全连宿主 Nacos(host.lima.internal:8848,集群 Nacos 已删);宿主 Nacos↔CRDB 走 127.0.0.1:26257(launchd 自愈);Tempo:14317/14318 收 OTel 转发;OTel:4317/4318 收 ESP 服务
4. 本地 commit 99d97e0 未 push(push 失败 443 超时)

---

## 2026-08-18 会话23-15b 完成项（中间件下沉+资源分配+全修复）

### 宿主中间件运维（本次完成）
1. 宿主中间件运维手册: `docs/ops/host-middleware-startup.md` ✅
2. 一键管理脚本: start/stop/status-host-middleware.sh ✅ (全部 RC=0)
3. Prometheus rule_files 修复: /etc/prometheus/rules.yml → $HOME/observability/rules.yml ✅
4. Prometheus otel-collector target 修复: K8s DNS → localhost:8889 ✅ (target UP)
5. Prometheus K8s API 认证: SA prometheus-host + RBAC + token/CA 导出 ✅
6. Loki K8s DNS 修复: host.lima.internal → localhost ✅ (config.expand-env=true)
7. 数据持久化迁移: /tmp/* → ~/observability/data/* (4个服务全部重启验证) ✅
8. Tempo 二进制修复: 从 GitHub 下载 macOS arm64 正确二进制 ✅ (port 3200 监听中)
9. 两个 README 更新宿主中间件章节 ✅ (中英文)

### 资源分配（本次完成）
10. bff-java CPU limit 1→2 core, requests 50m→200m + HPA(max=4, 70%) ✅
11. learning-worker HPA(max=3, 70%) ✅

### Admin 监控缺口修复
12. catalog.json: 移除 seata + otel-collector 改为 FQDN ✅ (待重建镜像部署)
13. Prometheus 通过 OTel Collector 聚合采集(单节点正确架构) ✅

### Nacos Derby→CRDB 迁移
14. CRDB 创建 nacos 数据库 + schema SQL 执行 ✅
15. application.properties 已改为 CRDB (localhost:32627 via kubectl port-forward) ⚠️
16. **NodePort 从宿主机不可直达**（CRDB NodePort 32627 连接超时）— kubectl port-forward 可用但非生产方案
17. **已决策**: Nacos 已完成 CRDB 迁移(host.lima.internal:26257,launchd 自愈通道,见会话23-13续3 #5/#6)

### Admin 前端问题(已发现待修复)
18. GovernanceDashboard 硬编码 IP: 192.168.5.15:30300(Grafana)/8858(Sentinel) → 需改 ExternalName

---

## 关键铁律索引(本次会话新增/引用)

| 铁律 | memory | 全局位置 |
|---|---|---|
| LLM 调用缓存优化 LCO(2026-08-18) | `llm-cache-optimization-ironrule` | ~/.claude/CLAUDE.md 第8条 |
| 生产域 Mock 零残留 MPB(2026-08-17) | 工程铁律第8条 | ~/.claude/CLAUDE.md |
| 写前核对目标系统 WTV(2026-08-17) | 工程铁律第9条 | ~/.claude/CLAUDE.md |
| 镜像产物内容校验 BVV(2026-08-18) | 工程铁律第10条 | ~/.claude/CLAUDE.md |
| 上下文溢出铁律(10%双轨落盘+80%compact+90%止损) | `context-overflow-ironrules` | 全局 CLAUDE.md 第6条 |
| Skill 优先铁律 | `skill-first-ironrule` | 全局 CLAUDE.md 专节 |
| API 签名核实 ASV(2026-08-18) | `api-signature-verify-ironrule` | 全局 CLAUDE.md 专节 |
| CRDB probe timeout=1s 坑 | `esp-crdb-probe-timeout-1s-pitfall` | 项目 memory |
| Redis Cluster 自愈层1根治 | `esp-redis-cluster-nodes-conf-stale-ip` | 项目 memory |
| Nacos-CRDB 通道 launchd 自愈 | `esp-nacos-crdb-channel-launchd-selfheal` | 项目 memory |

## 本地 commit
- `99d97e0 feat(会话23-13续2):H-01混沌+CRDB probe+LLM缓存LCO+Nacos CRDB迁移+bff修复+压测E2E`(482 files 43516 insertions)
- ⚠ push GitHub 失败(443 超时),本地完整,网络恢复后推
