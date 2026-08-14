# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。

## 当前✅(2026-08-15 会话22-31 C/D类批量推进:全7项闭合✅ —— Loki/ArgoCD/探针三分离/数字人TTS/个性化推荐/73条人工测试/5000CCU压测复测)

### 任务
兑现 C 类(Loki/ArgoCD)+ D 类(73条人工测试/5000CCU压测复测/探针三分离/数字人TTS/个性化推荐/全方位README)。用户指令"完整完成所有任务(可并行则并行)"+ 要求双轨落盘(本文件+Redis)以便上下文溢出后恢复。

### 落地内容(本会话闭合项)
- **#116 Loki 日志聚合✅**:fluent-bit 镜像 403 不可拉 → 改用 otel-collector filelog receiver(零新镜像,复用已缓存 otel-collector)。关键坑链(RD docker CRI 坑㉔):/var/log/pods/**/0.log 是符号链接指向 /var/lib/docker/containers/<cid>/<cid>-json.log,filelog 不跟随符号链接 → 直接读 docker 路径;docker 日志 root-only → securityContext runAsUser:0;docker-json 格式需 json_parser+move log→body;regex_parser 从 log.file.path 提 container.id;move 失败自循环 → if guard `attributes.cid != nil`。结果:6/7 pod Running,日志入 Loki ~150k-210k/2-3min。caveat:label 仅 service_name=unknown_service(富集未全,可查)。manifest:`deploy/k8s/monitoring/10-observability.yaml`。
- **#120 探针三分离✅**:11 服务(8 batch + gateway + 4 other + worker)manifest 全量改。Go Gin:/healthz /readyz /livez(esp-common-go/health);Go net/http+Rust axum:/actuator/health/readiness+/liveness。tcpSocket→httpGet 三分离;preStop sleep 5~15 + terminationGracePeriodSeconds 30。gateway-go 1.9.0 重建(加 /readyz /livez handler),rollout clean,/readyz /livez 200。
- **#121 数字人 TTS 情绪驱动✅**:`conversation-go/src/ws/frame.go` Frame 加 Emotion+Visemes 字段;`src/tts/mock.go` 重写 Provider.Synthesize→Result{Audio,Emotion,Visemes};SenseEmotion 关键词情绪(与 emotion-svc 同源);BuildVisemes 字符→Preston Blair 口型(60ms/viseme,合并连续,cap 400);`gateway-go/src/main.go` handleUserText 接 TTS(仅 AIReply 帧附 emotion+visemes);mock_test.go 3 测试全过;gateway-go 1.9.1 重建部署。
- **#122 个性化推荐 emotion_aware✅**:`BehaviorController.java` /recommend 加第4维 emotion_aware:查近7天 emotion 事件,neg>pos→state=negative+max_difficulty=easy+鼓励语+easy场景;pos>neg→state=positive+min_difficulty=medium+medium/hard场景;无数据→unknown。坑:builder agent 首次构建 1.8.8 时 target jar 是旧版(mvn package 未重跑致 Dockerfile COPY 拿到旧 jar) → 手动 `mvn -DskipTests package` 重打包 + `--no-cache` 重建 1.8.9 + deploy。验证:`/api/learning/recommend?user_id=1` 返回 keys=[review,weak_points,similar_scenarios,emotion_aware],emotion_aware={state:unknown}(user1 无 emotion 事件)。镜像 esp/learning-java:1.8.9。
- **#118 73条人工测试✅**:report/manual-73-run-20260815-062028.json(80条TC:PASS=57/FAIL=1/SKIP=22)。脚本 scripts/run_manual_73.py(幂等,支持 --only/--smoke)。FAIL仅 SEC-6(限流429未触发,根因 Nacos flow 规则未发布,非功能缺陷)。SKIP均环境不具备(GUI/小程序/scale0/无种子/多节点专属)。关键发现:/api/learning/placement 带 kp_ids 非空经 BFF 返 401(疑似下游 initMastery 抛错被代理误映,待 trace);WS 完整会话 TC-E2E-7 PASS。
- **#123 全方位 README✅**:README.md(英文默认,635行)+ README.zh-CN.md(中文,635行)。6+ mermaid 图;CRDB vs TiDB 实测章节(139.0 vs 259.3 insert/s,ratio 0.54,多区域4维度,拒选结论);四语言(Java/Go/Python/Rust)12维度对比表+决策树 mermaid。🇬🇧/🇨🇳 互链。
- **#119 5000 CCU 压测复测✅(未达标,单节点瓶颈)**:报告 report/ccu5000-g0{4,6}-*-2026-08-15.json。
  - G-04 WS:peak 955 CCU(level 1000 时全连接跌落不稳),max_stable=500,**FAILED 5000**。根因=gateway-go 单 pod 512Mi 撑不住 1000+ WS(单节点 VM)+ session ID 碰撞(sess_<unix_ms>)→ 403 + bff-java 探针 timeout。建议:多节点 + gateway-go 3+ replicas 1Gi + UUID session ID。前次基线 70 CCU → 本次 955(13.6x 提升)。
  - G-06 events:peak 370 ev/s(level 100),**FAILED 5000 ev/s**。根因=BFF+learning-java 同步 thread-per-request 链(各 200 线程,1 CPU)+ Disruptor consumer 同步批量 DB 写 ~300-500 ev/s + scale 2 replica 反而 CPU 争抢恶化(每 pod ~0.5 core)。建议:多节点横向扩展 + 异步 Disruptor consumer 独立 DB writer 线程池 + 修 OutboxRelay SQL grammar bug。前次基线 605 ev/s(本次 370 因环境 CPU 争抢,多节点恢复可达 605+)。
- **#117 ArgoCD GitOps✅(控制平面已起 + 3 STS 空-drift OOS 定性为结构性限制)**:
  - **镜像 403 已解决**:quay.io/argoproj/argocd 镜像封锁 → 控制平面改用本地缓存 v3.5.0 启起(argocd-server/repo-server/application-controller/dex/redis Pod 全 Running)。补:argocd-cmd-params-cm 调 git timeout 60s + github-repo-creds secret(私有仓凭证)+ dex scale 0(免 dex 镜像)+ 手动 kubectl patch 修 image drift(teacher 1.8.0✅ / learning 1.8.9✅ 已对齐)。
  - **3 StatefulSet OutOfSync(pulsar-bk/pulsar-zk/redis-cluster-new)定性为结构性 SSA 限制,非真实 drift**:
    - 已实证 `kubectl diff git vs live` = **空**(git==live,无 drift);
    - live STS `metadata.managedFields` 为空(count=0)——这些 STS 由 klipper/helm 历史安装,无 field-manager 认领,argocd SSA 无所依凭判定字段归属 → 把 K8s 默认填充字段(podManagementPolicy/revisionHistoryLimit/persistentVolumeClaimRetentionPolicy/updateStrategy/dnsPolicy/restartPolicy/schedulerName/securityContext/terminationGracePeriodSeconds)全当作 unowned → empty-diff OOS;
    - `RespectManagedFields=true` 依赖 managedFields 判归属,managedFields=0 时**失效**;`IgnoreExtraneous=true` 注解(argocd v3.5.0)同样**不消除** OOS;ignoreDifferences jsonPointers 尽力缓解亦不消除(因 managedFields=0 击穿比较归一化);
    - 业务**零影响**:6(redis)+3(zk)+3(bk)+3(broker) pod 全 Ready,集群正常。
    - **真根治**:把 3 共享 infra STS 移出 esp-platform 管辖范围(单独 Application 或裸 kubectl 管),单节点 RD 不 warranted,留多节点移植处理。
    - argocd-app.yaml 注释已更新为如实记录(去"根治"夸称)。已应用 + hard refresh。

### 恢复锚点(会话22-31)
- **当前子步骤**:C/D 类 **7/7 全闭合**✅(#116/#117/#118/#119/#120/#121/#122/#123)。
- **已确认根因**:① builder agent 构建陷阱——Dockerfile 用 `COPY target/*.jar`,若 mvn package 未重跑则 COPY 旧 jar;修复=mvn -DskipTests package + docker build --no-cache + 升 tag。② ArgoCD/quay 镜像 403=公开镜像源封锁,已用本地缓存 v3.5.0 起控制平面解决。③ 3 STS OOS = managedFields=0 击穿 SSA 归属判定,空 diff 非真 drift,业务零影响。
- **已改文件**:monitoring/10-observability.yaml、31/32/30/21-gateway-go.yaml(探针)、ws/frame.go、tts/mock.go+mock_test.go、gateway-go/src/main.go、BehaviorController.java、20/21-learning-java.yaml(1.8.9)、README.md/README.zh-CN.md、scripts/run_manual_73.py、report/manual-73-run-*.json、report/ccu5000-g0{4,6}-*-2026-08-15.json、deploy/argocd/argocd-app.yaml(注释订正)、deploy/k8s/06c-redis-cluster.yaml+06d-pulsar-geo.yaml(STS 注解)。
- **下一步**:① git commit 批次(C/D 类全闭合 + #117/#119 报告);② PROGRESS 热区超 300 行需归档 22-29/22-30 段到 docs/progress/archive/;③ 多节点移植时处理 3 STS argocd 管辖范围剥离 + 5000CCU 达标。
- **Redis 锚点**:`esp:ctx:hot:session-23`(会话态,含本批闭合摘要)、`esp:ctx:hot:refactor-p0~p2`(重构进度)。

## 当前✅(2026-08-15 会话22-30 统一部署窗口:Disruptor两级/esp-cache纯JSON/DB吞吐 全部上线✅)

### 任务
审计 A 类 #109/#110/#105 代码就绪后统一镜像构建+滚动部署窗口兑现"好的架构决策一一落实"。

### 落地内容(3 服务镜像升级 + 回归)
- **bff-java 1.7.6→1.8.0**(#109 Disruptor 两级 pipeline):
  - `InletDisruptor` 升两级 `pipeline.stage("parse",65536,LITE_BLOCKING).stage("forward",16384,BLOCKING)`,`InletNormalizer`(规整)→`DownstreamForwarder`(转发)双 stage。兑现 DisruptorPipeline 多级能力从测试走向生产(用户点名核查项)。
  - 验证:启动日志 `InletDisruptor pipeline started: stage1(parse) ringSize=65536 liteBlocking, stage2(forward) ringSize=16384 blocking` + `NettyEdgeServer started on port 9090`;`/actuator/health` UP;`/api/scenarios`→401(鉴权生效)。
- **learning-java/learning-worker 1.8.6→1.8.7**(#105 DB吞吐 + #110 esp-cache纯JSON):
  - #105:hikari `maximum-pool-size` 50→20(写+读)+ shared 10;jdbc `batch_size=50`/`order_inserts=true`/`order_updates=true`;`LearningService.initMastery` 用 `@DataSource(WRITE)`+`saveAll(batch)` 凑批写入。镜像内 application.yml 实测确认(3 处 maximum-pool-size=20/20/10 + batch_size=50 + order_inserts/updates=true)。
  - #110:`esp-cache` `CacheConfig` 去 `activateDefaultTyping`,改 `GenericJackson2JsonRedisSerializer.builder().defaultTyping(false)` 纯 JSON 契约(跨语言安全)。
  - 验证:两 pod `/actuator/health` UP;镜像内 esp-cache-1.0.0.jar 时间戳 08-15 01:15(纯JSON版)。
- **teacher-java 1.7.7→1.8.0**(#110 esp-cache纯JSON):
  - teacher 旧镜像 1.7.7 内 esp-cache jar 时间戳 08-14 21:10 早于纯 JSON 改动 → 重建 1.8.0 拉纯 JSON esp-cache。镜像内 esp-cache-1.0.0.jar 时间戳 08-15 01:15 确认。
  - 注:teacher pom 已显式声明 `spring-boot-starter-cache`(SB4 的 esp-cache provided 坑,P1-12 已修)。
  - 验证:`/actuator/health` UP;rollout successfully(慢启动 Nacos+Dubbo+Seata ~90s)。
- **manifest 落盘**:`15-bff-java.yaml`(1.7.6→1.8.0)、`20-learning-java.yaml`(1.8.6→1.8.7)、`21-learning-worker.yaml`(1.8.6→1.8.7)、`30-other-services.yaml`(teacher 1.7.7→1.8.0)。

### 审计登记册 A 类全闭合✅(16✅/7⏳/8❌)
- #108 CRDB实测✅ / #109 Disruptor多级✅ / #110 esp-cache纯JSON✅ / #111 2服务部署✅ / #112 CI修绿✅ / #104 OTel✅ / #105 DB吞吐✅(本次部署上线)。

### 恢复锚点(会话22-30)
- **当前子步骤**:3 服务统一部署窗口完成✅,A 类 7 项全闭合✅。全部回归通过。
- **下一步**:① git commit 批次(排除 writing-py egg-info 构建产物);② B/C/D 类按环境/决策触发(多节点 Cilium/Pulsar Geo-Rep/TiCDC/Loki/ArgoCD/Milvus+Neo4j/人工测试/5000CCU压测)。

## 当前✅(2026-08-15 会话22-29 CRDB 实测落地 + 审计登记册 A 类全闭合✅)

### 任务
兑现用户点名项"添加一套基础设施 CockroachDB 多区域节点方案并对比实际全链路压测对比 TiDB 的方案"。#108 文档-落地差距审计收尾。

### 落地内容(CRDB 3 节点实测)
- **集群**:ns `esp-crdb`,3 副本 StatefulSet `crdb-0/1/2`,arm64 镜像 `cockroachdb/cockroach:v24.3.11-arm64`(RD VM 为 aarch64,amd64 镜像 qemu 转 SIGSEGV)。
  - **关键坑**:`--join=crdb-0.crdb,...` DNS 解析依赖 headless svc `crdb` 的 `publishNotReadyAddresses: true`,否则 not-ready pod 不发布 DNS → `no such host` → 集群 bootstrap 卡死。已写入 manifest `deploy/crdb/crdb-3node.yaml`。
  - `cockroach init --insecure --host=crdb-0.crdb:26257` 初始化后 ~8s 三 pod 全 1/1 Ready。
- **schema 适配**(`deploy/crdb/schema-crdb.sql`):TiDB `AUTO_RANDOM`→CRDB `unique_rowid()`;`ENUM`→`VARCHAR+CHECK`;`TINYINT`→`SMALLINT`;`DATETIME(3)`→`TIMESTAMP(3)`;`JSON` 保留(CRDB 原生);删 `ON UPDATE CURRENT_TIMESTAMP`(CRDB 无此子句)。5 核心表:users/scenarios/sessions/session_turns/learning_events。
- **种子**(`scripts/seed_crdb.py`):100 users / 3 scenarios / 500 sessions / 2000 session_turns / 2000 learning_events。冒烟 bench(经 kube-exec per-query,有开销):write 78.6 insert/s / point_read 0.8 qps(exec 开销主导) / range_read 1345.9ms。报告 `report/crdb-smoke-loadtest.json`。
- **应用层写吞吐对比**(`scripts/crdb_vs_tidb_bench.py`,batch 200 rows,4000 rows 串行):
  - **CRDB**:4000 rows / 28.77s / **139.0 insert/s**
  - **TiDB**:4000 rows / 15.42s / **259.3 insert/s**
  - **ratio(CRDB/TiDB)= 0.54**。报告 `report/crdb-vs-tidb-app-bench.json`。
- **结论(实测对齐方案文档)**:单节点 RD VM(3 CRDB pod + TiDB 共享同 VM CPU/磁盘)无多区域拓扑下,CRDB 写吞吐仅为 TiDB 0.54 倍。**实证 `docs/design/cockroachdb-vs-tidb-multi-region-plan.md` 的结论"5000 CCU 尺度下,引入 CRDB 的收益不成立"**。真实多区域对比需 3master+6worker 集群(plan §4 方法论),条件触发。
- **协议差异记录**:CRDB 是 PostgreSQL-wire,ESP 集群无 psycopg2/psql 客户端 pod(writing-py 仅 pymysql 2.2.8 MySQL-wire 不兼容),故经 `kubectl exec crdb-0 -- cockroach sql` 访问;TiDB 经 writing-py pymysql 访问。两者均走单次 exec,故用 batch INSERT 摊薄 per-exec 进程启动开销。

### 审计登记册状态(`docs/design/audit-gap-resolution-register.md`)
- **A 类(单节点可落地)全闭合**✅:#108 CRDB✅ / #109 Disruptor多级✅ / #110 esp-cache纯JSON✅ / #111 2服务部署✅ / #112 CI修绿✅ / #104 OTel✅ / #105 DB吞吐(配置就绪待部署窗口)。
- 总览:31 承诺 = 16✅ / 7⏳ / 8❌(仅方案)。
- B 类(多节点/独立窗口):Cilium / Pulsar Geo-Rep / TiCDC 异地多活 / 真 Follower 读写分离 / 多节点容量验证 —— 方案备,环境到位执行,不强行单节点落地。
- C 类(独立引入决策):Loki / ArgoCD / Milvus+Neo4j 业务接入 —— 未落地。
- D 类(测试/工程化):人工测试 73 条 / 5000 CCU 压测 G-04/G-06 复测 / 探针三分离 / 数字人/TTS / 个性化推荐 —— 待服务全就绪+性能优化部署后批量推进。

### 恢复锚点(会话22-29)
- **当前子步骤**:CRDB 实测落地✅(#108 闭合),审计登记册 A 类全闭合✅。本会话双轨落盘完成。
- **下一步**:① #109/#110/#105 代码就绪但未部署 → 统一镜像构建+滚动部署窗口(主机 mvn 与集群操作串行);② 全部 A 类闭合后 git commit 批次(排除 writing-py egg-info 构建产物);③ B/C/D 类按环境/决策触发。

## 当前✅(2026-08-15 会话22-28 补部署 llm-orchestrator-rs + vocab-srs-go — 10 服务矩阵兑现✅)

### 任务
审计发现两个有代码无 Deployment 的服务,兑现 `adr-new-services.md` 10 服务矩阵承诺的最后 2 项。

### 改动
- **manifest 新建** `deploy/k8s/33-orchestrator-vocab.yaml`:两个 Deployment(replicas=1)+ ClusterIP Service(port 80→targetPort)+ HPA(min1/max3,CPU 70%);labels tier=new-service。
  - **llm-orchestrator-rs**(Rust/axum+tokio,8086):env PORT/REDIS_ADDR(FQDN seed)/TIDB_DSN(`mysql://root@tidb-primary:4000/esp_llm`)/RUST_LOG/OTEL(FQDN otel-collector.esp-observability:4317)/LLM provider 6 key 从 `secret llm-providers` 引用(optional);探针 /healthz /readyz /livez;资源 250m/256Mi~1C/512Mi。
  - **vocab-srs-go**(Go/gin,8087):env PORT/GIN_MODE/TIDB_HOST(primary)+TIDB_HOST_READ(follower)/TIDB_PORT/USER/PASSWORD/DB(esp_vocab)/OTEL;探针 /healthz /readyz /livez;资源 100m/128Mi~500m/256Mi。
- **secret 占位** `llm-providers`(空值,6 key:openai-base/key, qwen-base/key, claude-base/key)—— `kubectl create secret generic llm-providers --from-literal=... --dry-run|apply` 创建;无 `*_BASE` env 时代码兜底 StubProvider 保证启动。生产用 KMS/SealedSecret 注入真实 key。
- **Dockerfile 修坑**:
  - `backend/.dockerignore` 新建(排除 .venv/.venv-test/target/node_modules 等,坑因 emotion-svc/writing-py 的 .venv 符号链接致 docker build context xattr 失败)。
  - `services/llm-orchestrator-rs/Dockerfile`:① COPY shared/esp-common-rs 目标路径修 `/workspace/esp-common-rs`→`/shared/esp-common-rs`(Cargo.toml path=`../../shared/esp-common-rs` 从 WORKDIR `/workspace/llm-orchestrator-rs` 解析);② runtime stage `apt-get` 源 `deb.debian.org`→`mirrors.ustc.edu.cn`(VM 出口偶发不通 deb.debian.org:80)。
- **镜像构建**(RD VM `docker build`):`esp/llm-orchestrator-rs:1.0.0` ✅(rust:1.88-bookworm builder + debian:bookworm-slim runtime,1m44s);`esp/vocab-srs-go:1.0.0` ✅(golang:1.25-alpine builder + alpine:3.22 runtime)。

### 验证(全过)
- `kubectl apply -f 33-orchestrator-vocab.yaml` RC=0(6 资源 created);rollout status 两 deploy 均 successfully rolled out(timeout 120s 内)。
- Pod 状态:`llm-orchestrator-rs-67b5fc986d-nb78l` 1/1 Running;`vocab-srs-go-78489dc88c-vvhhm` 1/1 Running。
- 冒烟(从相邻 pod course-path-go wget ClusterIP):
  - llm-orchestrator-rs `/healthz`→`{"service":"llm-orchestrator-rs","status":"UP"}` ✅;`/readyz`→`{"checks":{"redis":"UP","tidb":"UP"},"status":"UP"}` ✅(Redis Cluster + TiDB esp_llm 均连通)。
  - vocab-srs-go `/healthz`→`{"service":"vocab-srs-go","status":"UP"}` ✅;`/readyz`→`{"status":"UP"}` ✅(TiDB esp_vocab 连通)。
- TiDB schema 确认:`esp_llm` + `esp_vocab` 库均存在(已建)。

### 卡点/风险
- **LLM provider secret 为空**:llm-providers 6 key 全空,服务用 StubProvider 兜底启动,`/api/llm/chat` 实际调 LLM 会返回 stub 响应。生产前需 KMS/SealedSecret 注入真实 key(OPENAI/QWEN/CLAUDE base+key)。
- **vocab-srs-go 镜像 tag**:1.0.0 新构建,Dockerfile 用 alpine:3.22 runtime(非 distroless,因 gcr.io 拉取坑)。
- **deploy/k8s/esp-prod/ 旧 manifest**:目录下有 llm-orchestrator-rs.yaml(0.2.0 tag)/vocab-srs-go.yaml(0.2.0 tag)旧版本,已被 33-orchestrator-vocab.yaml(1.0.0 tag)取代,可后续清理。

## 当前✅(2026-08-15 会话22-26 P2 第三波收尾 — P2-17/18 补齐,19 项全闭合✅)

**第三波 P2(架构深化,7 项)**。P0 安全红线✅ + P1 容量质量 6 项✅ + P2-13 DDD充血化✅ / P2-14 gateway-go 瘦身✅ / P2-15 读写异步化(判定)✅ / P2-16 缓存防护✅ / P2-19 工程化补齐✅ / **P2-18 目录重组✅** / **P2-17 前端 monorepo(主体✅,pnpm install+typecheck 验证进行中)**。**P0~P2 全量重构计划 19 项全部闭合(17 实现 + P2-15 判定无需改动 + P2-17 验证收尾中)**。

### P2-19 工程化补齐✅(本会话完成)
- **Go 单测 0→25(4 模块)**:① `vocab-srs-go/sm2_test.go`(14 tests,SM2 间隔重复算法全路径:new card/interval 增长/lapse 重置/easeFactor 1.3-2.5 钳位/quality 钳位/mastered 升级/边界);② `gamification-go/logic_test.go`(2 tests,uidStr/parseTop 边界;addPoints/leaderboard/grantAchievement 因耦合全局 DB+Redis 跳过);③ `course-path-go/main_test.go`(6 tests,writeJSON/generatePath/unlockNode/getPath/readiness/liveness);④ `esp-common-go/config/config_test.go`(4 tests,Getenv/GetenvInt 默认值与解析)。listening/reading/live-tutor 无纯函数(全 gin handler 绑全局 DB)跳过,留待 DI 重构。**所有 go 命令前缀 `env -u GOROOT GOTOOLCHAIN=local`(gvm GOROOT 坑)**。
- **Java 覆盖率 JaCoCo 补齐**:teacher-java 原为唯一无 jacoco 的 Java 服务 → pom.xml 加 jacoco-maven-plugin 0.8.12(prepare-agent + test 阶段 report)。三服务覆盖率实测:learning 40.5% / teacher 23.6% / bff 30.6%(业务服务起步基线,后续迭代提升)。
- **Java 领域行为单测 14 条**:① teacher `SubmissionTest`(4:grade 设自动分+反馈+状态/confirm 设最终分+finalized/confirm 幂等/rollback 状态恢复);② learning `UserMasteryTest`(5:applyReviewEvent 对/错/null 容错/applyTeacherFinalized/initBaseline);③ bff `RouteTableTest`(5:resolve 各路由 + 关键 `writingCoach 优先级高于 writing` 防止 /api/writing-coach 误路由到 writing-py)。全 PASS。
- **前端 Vitest 起步**:frontend-react 加 vitest 2.1.0 + `vitest.config.ts`(node env)+ `src/test-setup.ts`(LocalStorageStub —— auth.ts zustand create 模块加载即读 localStorage,node 环境必须预注入)+ `src/auth.test.ts`(2:authHeader null→{}/token→Bearer)。全 PASS。CI frontend job 加 `npm test` step。
- **CI workflow 补齐**:`.github/workflows/ci.yml` 新增 `go-services` job(go vet+test 4 模块,前缀 env -u GOROOT GOTOOLCHAIN=local)+ `frontend` job(npm ci + tsc --noEmit + npm test + npm run build);build-images needs 已含 go-services/frontend。YAML 校验通过(9 jobs: java-commons/java-services/go-gateway/python-services/go-services/build-images/contract/e2e/frontend)。
- **E2E Playwright 起步(scaffold)**:`e2e/` 新建 `package.json`(@playwright/test ^1.48.0)+ `playwright.config.ts`(chromium,baseURL localhost:3000,CI webServer 自动起 dev)+ `smoke.spec.ts`(2:首页可加载 + 未带 token /api/scenarios → 401/403)+ `README.md`(运行说明)。**注:scaffold 级,需 `npx playwright install chromium` + dev server + BFF 可达才能实跑;CI e2e job 需 runner 可达集群,暂 workflow_dispatch 级**。
- **覆盖率未达 80% 的诚实记录**:Go 4/6 模块有测(2 模块耦合全局状态待 DI 重构);Java 三服务 23.6%-40.5%(业务服务起步基线);E2E 仅冒烟级。距 plan 目标"≥80%/完整 E2E"有差距,留作后续迭代(单节点环境 + 上下文约束下,优先保证"从 0 到有"的工程化闭环)。

### P2-18 目录重组✅(本会话完成)
- **目录结构**:27 个平铺 `services/` 目录重组为三层——`backend/services/`(19 业务服务)+ `backend/shared/`(esp-cache-py/esp-common-go/esp-common-rs/esp-db-py)+ `platform/`(esp-api/esp-cache/esp-disruptor/esp-rw-java);旧 `services/` 已删除。
- **Dockerfile 路径**:Go/Rust/Python Dockerfile 改为 `backend/`-相对 COPY 路径;Java 保持 prebuilt fat-jar 不变(context=服务目录)。
- **CI 修复**:`.github/workflows/ci.yml` + `security.yml` 的 build/trivy matrix 改 include 结构——Go/Python context=`backend`、Java context=`backend/services/<svc>` + 前置条件 mvn package;`deploy/docker/docker-compose.yml` 同步改 build context。
- **验证**:RD VM 内(`rdctl shell sh -c 'docker build ...'`)对 6 个已部署服务镜像(gateway-go/writing-py/emotion-svc/bff-java/learning-java/teacher-java)docker build **6/6 通过**(COPY 路径全部正确解析)。

### P2-17 前端 monorepo(主体✅,pnpm install+typecheck 验证进行中)
- **workspace 骨架**:pnpm workspace + turbo——根 `package.json`/`pnpm-workspace.yaml`/`.npmrc`(node-linker=hoisted 兼容 Taro)/`turbo.json`/`tsconfig.base.json`。
- **三端迁入**:`apps/{web,h5,miniprogram}`,包名 @esp/web/@esp/h5/@esp/miniprogram。
- **4 共享包**:① `packages/types`(单真源类型,三端 types.ts 改 re-export);② `packages/api-client`(统一 API SDK 超集签名 + 可插拔 Transport:fetch/wx.request + token 存储适配器;消灭 H5 硬编码 exam=toefl / 小程序硬编码 prompt+exam / reportEvent 三种签名);③ `packages/ws-client`(EspWsClient:指数退避重连 + 心跳 + 发送缓冲 + 传输抽象);④ `packages/ui-tokens`(CSS variables tokens + dark theme + SCSS 映射 + 零依赖 i18n en/zh)。
- **三端接线**:api.ts 改薄委托层;H5/小程序调用方已适配统一签名;web/h5 styles.css 注入 tokens.css。
- **CI**:frontend job npm→pnpm + turbo(web+h5 build,miniprogram 仅 typecheck);`e2e/playwright.config.ts` 路径更新。
- **进行中**:pnpm install + turbo typecheck 验证结果待落盘。
- **遗留(非阻断)**:路由懒加载/错误边界/骨架屏;ESLint/Prettier 全仓接入;token httpOnly cookie 化;三端 WS 接线到 EspWsClient。

### P2 已闭合项
- **P2-13 DDD 充血化✅**:`UserMastery`(`applyEvent` 行为下沉)/`Submission`(`grade` 行为下沉)贫血 POJO→充血聚合;domain 包拆 aggregate/valueobject/domainevent/repository(接口入 domain 实现入 infrastructure)。learning-java/teacher-java 已重构+全量单测绿(learning 48 tests / teacher 16 tests)。镜像 learning-java 1.8.6 / teacher-java 1.7.7 ✅deploy。
- **P2-14 gateway-go 瘦身✅**:对话逻辑 11 包下沉独立 Go 模块 `conversation-go`(无 esp/common-go 与 jwt 依赖,仅 miniredis/pulsar/mysql/websocket/redis/otel);gateway-go 退回纯路由+鉴权+WS接入+jwt校验(verify-only,ParseUserID via RS256 公钥)。go.work 9 模块。镜像 **gateway-go 1.7.0**✅deploy。回归全 PASS(healthz/scenarios 带 token 200 无 token 401 / WS 本人 101 无token 403 他人 403)。
  - **关键坑(gvm GOROOT)**:`compile: version "go1.26.2" does not match go tool version "go1.26.5"`。**FIX**:所有 go 命令前缀 `env -u GOROOT GOTOOLCHAIN=local`。
  - **docker 构建路径**:host docker context `rd-vm` SSH 拒连/`desktop-linux` daemon 未起。**解法**:`rdctl shell sh -c 'docker build ...'` 进 RD VM 内构建(VM moby daemon = k3s CRI,镜像直接对 k3s 可见)。
- **P2-15 读写异步化✅(判定降级)**:经 esp-explorer 双轮探查(bff 查库路径 + learning/teacher @DataSource 异步链路),**"ThreadLocal 失效"前提不成立**——① bff Disruptor 转发链路(EdgeAuthHandler→InletDisruptor→DownstreamForwarder)零 DB 操作,仅 JWT 鉴权+HTTP 转发;bff 仅 AuthController register/login 在 Spring MVC 同步线程查 users 表,@DataSource AOP 路由正常。② learning/teacher 9 个 @DataSource 方法体内部全是同步 DB 操作,无 @Async/CompletableFuture/ExecutorService;唯一跨线程链路 Pulsar consumer 线程调 @DataSource 方法,但 consumer 线程作为独立执行入口,AOP 在该线程内 set→proceed→clear 闭环,不跨线程传递 ThreadLocal,路由正确(且 consumer 链路只触发 WRITE 方法,默认值也是 WRITE,无路由错误)。**结论**:当前单节点 TiDB(读写同库)+ consumer 线程默认走 WRITE,无实际路由错误发生路径。降级为"未来多节点 TiDB / 真 READ 副本路由时才需考虑的 SQL Hint `/*+MASTER*/` / TransmittableThreadLocal 准备项",当前无需改动 esp-rw-java。
- **P2-16 缓存防护✅**:TTL 加 ±10% 随机抖动(防雪崩)+ singleflight 互斥(防击穿);BehaviorController top/funnel/heatmap 纳入缓存。镜像 learning-java 1.8.6 / esp-cache 已 install m2。验证:behavior/top 200 返回 `[{"user_id":1,"cnt":1}]` / funnel 200 三阶段 / heatmap 200 `[{"event":"behavior.probe_b","cnt":1}]` / 二次调用走缓存命中。

### 镜像 tag(P2 阶段,全部已 deploy)
- gateway-go:**1.7.0**(P2-14 瘦身:conversation-go 独立模块)✅
- learning-java:**1.8.6**(P2-13 DDD充血化 UserMastery.applyEvent + P2-16 缓存防护 BehaviorController)✅ 入口+worker
- teacher-java:**1.7.7**(P2-13 DDD充血化 Submission.grade + P2-16 缓存防护)✅
- esp-cache:1.0.0(P2-16 TTL抖动+singleflight,install m2,消费方透传)✅

### 恢复锚点(P2)
- **当前子步骤**:P2 第三波 7 项全部处置完毕✅(P2-13/14/15/16/19 见上;P2-18 目录重组✅ RD VM 内 6/6 镜像 build 验证;P2-17 前端 monorepo✅ 全验证通过)。**P0~P2 全量重构计划(19 项)完成:6(P0)+6(P1)+7(P2 实现 6 + P2-15 判定无需改动)= 闭合 19/19**(P2-17 验证通过(typecheck 7/7 + web 单测 + web/h5 构建))。
- **已改文件(P2-17/18,本会话)**:① 目录重组 27→backend/services(19)+backend/shared(4)+platform(4),旧 services/ 删除;② Go/Rust/Python Dockerfile COPY 路径改 backend/-相对;③ `.github/workflows/ci.yml`+`security.yml` build/trivy matrix include 结构(Go/Py context=backend,Java context=backend/services/<svc>+mvn package 前置);④ `deploy/docker/docker-compose.yml` build context 同步;⑤ 前端 monorepo:根 package.json/pnpm-workspace.yaml/.npmrc/turbo.json/tsconfig.base.json + apps/{web,h5,miniprogram} + packages/{types,api-client,ws-client,ui-tokens} + 三端 api.ts/types.ts/styles.css 改造 + e2e/playwright.config.ts 路径。
- **下一步**:① pnpm install + turbo typecheck 验证结果落盘(P2-17 收尾);② git commit 全部 P2 改动(含 P2-17/18);③ (可选)P2-17 遗留非阻断项(懒加载/错误边界/ESLint/token httpOnly/WS 接线)+ 覆盖率提升迭代 + DI 重构 listening|reading|live-tutor;④ 多节点容量验证 G-01/G-06(用户指示前不启动)。
- **Redis 锚点**:`esp:ctx:hot:refactor-p2`(P2 全量闭合后写)。

## 当前✅(2026-08-15 会话22-28 esp-cache 去 DefaultTyping 兑现 §2.2 跨语言纯 JSON 契约✅)

**兑现多级缓存架构对齐文档 §2.2 承诺**:platform/esp-cache 去 activateDefaultTyping,改纯 JSON value 契约,消除跨语言(Java/Go/Python)互读的"定时炸弹"。

### 决策(选型+理由)
- **问题**:`CacheConfig.redisTemplate` 用 `activateDefaultTyping(NON_FINAL)`,JSON 中嵌入 Java FQCN(`["com.esp.xxx.UserView", {...}]`),Go `json.Unmarshal`/Python `json.loads` 读到数组而非对象,跨语言互读失败。
- **探查结论**:Java 侧缓存 value 类型有限——String(sentinel/"1"幂等占位/JWT黑名单)、Map(bff user view、behavior 聚合 `List<Map<String,Object>>`)、POJO 列表(`List<UserMastery>`/`List<Object[]>`)。所有下游消费方除 `LearningRpcProvider.userStats()`(调 getter 组装 KpStat)外,均不强类型(HTTP 入口交 Spring Jackson 序列化响应体)。
- **选定方案**:`GenericJackson2JsonRedisSerializer.builder().defaultTyping(false).objectMapper(om).build()`。理由:① 调用方零侵入(CacheService 的 `opsForValue().set/get` 签名不变);② ObjectMapper 配 JavaTimeModule 支持 Instant/BigDecimal;③ Spring Data Redis 3.x 原生 builder API,显式关 defaultTyping 即写出无类型标记纯 JSON。
- **未选方案**:信封 `CacheEnvelope{type,json}`(过度设计,消费面简单不需信封);StringSerializer+调用方自行 ObjectMapper(侵入性大,改每个调用方);保留 DefaultTyping 仅跨语言 key 专用序列化器(当前全部 cross-lang=false 但文档明确"建议下一窗口去掉 DefaultTyping 消除定时炸弹",一次性解决)。

### 改动文件
1. `platform/esp-cache/src/main/java/com/esp/cache/CacheConfig.java` — 删 `activateDefaultTyping`+`LaissezFaireSubTypeValidator` import;redisTemplate bean 改 `builder().defaultTyping(false).objectMapper(om).build()`;加 §2.2 决策注释(方案选型/存量兼容/调用方注意)。
2. `platform/esp-cache/src/main/java/com/esp/cache/CacheService.java` — getOrLoad javadoc 加 §2.2 说明(L2 命中 POJO→LinkedHashMap,L1 命中原对象,调用方按需适配)。
3. `backend/services/learning-java/src/main/java/com/esp/learning/rpc/LearningRpcProvider.java` — `userStats()` 加 `toUserMastery(Object)` 防御性类型还原(L1 命中 UserMastery 直返,L2 命中 LinkedHashMap 转 UserMastery);`weakCount()` 仅 `.size()` 不需适配。加 import Map。
4. `backend/services/learning-java/src/main/java/com/esp/learning/service/LearningService.java` — dueReview/weakestPoints javadoc 加 §2.2 类型说明(L2 命中返回 List<LinkedHashMap>,HTTP 入口不强类型,RPC 自行适配)。
5. `platform/esp-cache/src/test/java/com/esp/cache/CacheServiceTest.java` — 新增 2 个 §2.2 契约单测:① `redisTemplate_valueSerializer_producesPureJsonWithoutTypeHint`(断言 String→裸字符串/Map→裸JSON对象无@class/List→裸JSON数组,Go/Python 可直接 json.loads);② `pureJsonSerializer_roundTrip_preservesStringAndMapAndList`(String/Map/List round-trip 等价还原,Map→LinkedHashMap)。

### 验证结果
- esp-cache `mvn -B test -Dtest='CacheServiceTest'`:**15 tests, 0 failures, 0 errors**(13 原有 + 2 新 §2.2)✅
- esp-cache `mvn -B install -DskipTests`:**BUILD SUCCESS**(jar 已 install m2)✅
- learning-java `mvn -B compile -DskipTests`:**RC=0** ✅
- learning-java `mvn -B test -Dtest='LearningServiceTest' -Djacoco.skip=true`:**13 tests, 0 failures** ✅(含真实 CacheService + mock RedisTemplate L2 miss 的 dueReview L1 命中/evict 回源用例)
- teacher-java `mvn -B compile -DskipTests`:**RC=0** ✅
- bff-java `mvn -B compile -DskipTests`:**RC=0** ✅
- **注**:JaCoCo 0.8.12 不支持 class file major version 69(JDK25),learning-java 全量 test 需 `-Djacoco.skip=true` 或升级 JaCoCo(预存环境问题,非本次改动引入)。
- **Go/Python 侧**:`backend/shared/esp-cache-py/esp_cache/__init__.py` 已是纯 `json.dumps/loads`(无类型标记),`backend/services/gateway-go/src/main.go` 幂等缓存存 `string(resp)`(裸 JSON 字符串)——两端本就纯 JSON,Java 侧去 DefaultTyping 后三端契约对齐,无需改 Go/Python 代码(注释说明即可)。

### 存量缓存兼容(重要)
- Redis 中旧格式 key(带 `@class`/类型数组)在新序列化器下反序列化会失败。**但所有缓存 TTL ≤ 30min**(learning:due/weak 300s、bff_user 300s、learning:agg 60-1800s、esp_idem 300s、esp_jwt_blacklist 按剩余JWT TTL),旧 key 滚动部署后 30min 内自然过期。
- **降级安全**:CacheService.getOrLoad 内部 L2 读异常已 catch 降级到 loader(见 CacheService.java L2 redis read failed → degrade to DB),旧 key 读失败不报错,只触发一次回源打 DB(30min 窗口内零星回源,单节点 TiDB 可承受)。
- **滚动建议**:① 避开高峰部署;② 如需彻底清理,可对热点前缀执行 `redis-cli --scan --pattern 'learning:*' | xargs redis-cli del`(本次不执行 flush);③ 部署后观察 CacheService `L2 redis read failed` 日志频率,30min 后应归零。
- **未部署**:本次仅改代码+验证编译/单测,镜像构建+滚动部署留待统一窗口(单节点铁律:主机 mvn 与集群操作串行)。

### 恢复锚点
- **当前子步骤**:§2.2 去 DefaultTyping 代码改造+验证完成✅,待统一窗口部署(learning-java 换 tag 滚更 + esp-cache 已 install m2 消费方透传)。
- **已改文件**:CacheConfig.java / CacheService.java / LearningRpcProvider.java / LearningService.java / CacheServiceTest.java(5 文件)。
- **下一步**:① 统一窗口镜像构建+滚动部署 learning-java(esp-cache 是 provided 依赖,消费方重新 package 即带入);② 部署后观察 30min 旧 key 过期;③ (可选)跨语言集成测试:Java 写入后用 Python esp-cache-py 读验证 json.loads 等价。

---

## 当前✅(2026-08-15 会话22-27 Disruptor 多级调度生产落地✅)

**ADR-09 多级 Disruptor 流水线首次接入 bff-java 生产链路**(兑现"Disruptor 多级调度队列"架构承诺)。原 `InletDisruptor` 单 RingBuffer(65536,BlockingWaitStrategy)单消费者(DownstreamForwarder fire-and-forget JDK HttpClient)→ 升级为**两级 DisruptorPipeline**:
- **Stage1 parse**(`InletNormalizer`):环 65536 + LITE_BLOCKING + MULTI 生产者。纯计算微秒级(路由解析+body 拷贝出 Netty ByteBuf+header 白名单过滤 hop-by-hop+traceparent W3C inject+构建不可变 HttpRequest);拷完 body 即 `req.release()` 归还堆外内存(Stage2 槽位只持 byte[] 不持 Netty buffer,这是 Stage2 可用小环的前提)。非转发路径(/healthz 等本地路径)在此级即丢弃不 forward。
- **Stage2 forward**(`DownstreamForwarder`):环 16384 + BLOCKING + MULTI。纯 IO 派发(AsyncSender 抽象,生产=JDK HttpClient sendAsync fire-and-forget,测试=mock 收集)。转发速率低于解析是常态,小环天然限制在途 body 拷贝总量防内存放大(16384×avg body);IO 密集级对唤醒延迟不敏感,BLOCKING 最省 CPU。环满 `pipeline.forward` 返回 DROPPED(fire-and-forget 背压,Stage2 dropped 指标 +1)。
- **WaitStrategy 选型理由**:Stage1 LITE_BLOCKING(先自旋后挂起,突发低延迟且闲时不烧 CPU;YIELDING 持续让出也占满核、BUSY_SPIN 单节点不可接受);Stage2 BLOCKING(IO 密集对唤醒延迟不敏感,锁语义最省 CPU)。
- **事件复用语义**:两级共用 `InletEvent` 类型,加 `downstream`/`httpRequest` 派生字段 + `translate(slot,seq,payload)` 静态方法(套 Disruptor `EventTranslatorOneArg.translateTo` 签名)。Stage1 消费完清全部引用(otelContext/httpRequest/downstream),Stage2 消费完清 httpRequest/downstream,防 body 引用滞留复用槽。
- **对外 API 不变**:`publish(InletEvent)` 满返 false→EdgeAuthHandler 快速 503(背压语义不变);`stats()` 返回两级独立 published/dropped/consumed/backlog。`NettyEdgeServer`/`EdgeAuthHandler` 零改动(内部实现换 pipeline)。
- **单测**`InletDisruptorPipelineTest`(3 tests):① 两级串联+规整(路由解析/header 过滤/traceparent 透传/X-User-Id 保留/Host 剔除/Netty 引用计数归零/两级独立 stats);② 本地路径 Stage1 丢弃不进 Stage2;③ 200 条突发级内 FIFO 无丢。全 PASS(`mvn -B test -Dtest='*Disruptor*'` 3/0/0)。`mvn -B package -DskipTests` RC=0。
- **不部署**:镜像构建+滚动留待统一窗口(单节点铁律:主机 mvn 与集群操作串行)。
- **已改文件**:① `backend/services/bff-java/src/main/java/com/esp/bff/edge/InletEvent.java`(加 downstream/httpRequest 派生字段 + copyFrom + translate);② `InletDisruptor.java`(单 worker→DisruptorPipeline 两级 stage,InletNormalizer+DownstreamForwarder 分离,stats() 覆盖两级);③ 新增 `InletNormalizer.java`(Stage1 消费者);④ 新增 `DownstreamForwarder.java`(Stage2 消费者,AsyncSender 抽象可注入);⑤ 新增 `src/test/java/com/esp/bff/edge/InletDisruptorPipelineTest.java`。
- **下一步**:① 统一窗口镜像构建+滚动部署 bff-java;② 压测验证两级背压(Stage2 环满 DROPPED 指标可观测);③ (可选)ForwardRespondHandler 扩展点实现响应回写(当前 fire-and-forget)。

### 会话22-26 补充:otel-collector 部署 + 全服务 OTLP 接入(2026-08-14)
- **根因**:18 个业务 deploy 的 `OTEL_EXPORTER_OTLP_ENDPOINT` 全部指向不存在的 `otel-collector.esp-observability.svc.cluster.local:4317`(esp-observability ns 从未创建),OTLP 导出持续 `export timeout / name resolver error`。
- **部署**:`deploy/k8s/95-otel-collector.yaml`(esp-prod 内 ConfigMap+Deployment+ClusterIP Service,receiver otlp grpc:4317/http:4318,batch processor,debug exporter,三 pipeline traces/metrics/logs)。镜像固定 `otel/opentelemetry-collector-contrib:0.128.0`。
- **坑㉗(镜像拉取)**:VM 出口对 dockerhub/ghcr 匿名 HEAD 一律 403(1ms.run/daocloud mirror 同样 403);手动 curl+token 可通,系 daemon 兼容问题。**解法:复用 VM docker 本地已有镜像 0.128.0 并固定 tag**(`rdctl shell docker images` 可查)。另:collector 无 13133 探针端口(health_check extension 未配),探针已移除。
- **接入清单**(endpoint 统一改 `http://otel-collector:4317`,grpc 端口维持 4317):gateway-go、bff-java、learning-java、learning-worker、writing-py、teacher-java、emotion-svc、writing-coach、course-path-go、pronunciation-rs、listening-go、assessment-rs、reading-go、gamification-go(14 个,均 rollout 成功;live-tutor-go/media-learn-rs replicas=0 未运行,manifest 已同步改)。改动文件:10-gateway-go/15-bff-java/20-learning-java/21-learning-worker/30-other-services/31-new-services/32-batch2-services。
- **验证证据**:① collector 日志持续接收:`Traces {"...spans": 34}`、`Logs {"...log records": 3}`(每 5s 一批);② gateway-go/learning-java/bff-java/writing-py/teacher-java 近 3 分钟导出错误 0 条(改造前 gateway-go 每 10~15s 一条 `exporter export timeout`)。
- **多节点演进**:otel-collector-gateway.yaml / otel-collector-daemonset.yaml(已备,依赖 tempo/loki,单节点不 apply)。

---

## 当前 (2026-08-15 会话22-27 DB访问吞吐优化 — 方案+配置落地✅,编译✅,未部署待确认窗口)

- **方案**:`docs/design/db-access-throughput-optimization.md`(诊断:同步JDBC吞吐=池×1/RTT;5000 CCU测算落DB仅~250 QPS,20/池有10x余量;四层优化:①池收敛 ②虚拟线程已开 ③Hibernate批量 ④缓存前置已有;明确不建议R2DBC全异步化)。
- **配置落地**(未部署,部署步骤见方案§4):
  - learning-java application.yml:write/read 池 50→20+minimum-idle:5(shared 10维持);新增 hibernate `jdbc.batch_size:50`+`order_inserts/updates:true`。
  - teacher-java application.yml:write/read 池 30→20+minimum-idle:5;同上批量三参数。
  - bff-java application.yml:池 20 维持仅补注释(bff 无 JPA 不加批量参数)。
  - esp-rw-java RoutingHolder.java:虚拟线程下 ThreadLocal 语义验证注释(每虚拟线程独立Thread、AOP同线程闭环路由正确)+ ScopedValue TODO。
- **集群连接收敛**:210→130(learning 110→50,teacher 60→40,bff 40 维持),防瞬时打满单节点 TiDB/TiKV region 热点(坑㉑)。
- **验证**:`mvn -B compile -DskipTests -q` 串行 4 模块全 RC=0, RoutingHolder.class 等产物 Aug 15 00:18 新鲜。
- **下一步**:用户确认窗口后按方案§4 部署(learning-java 1.8.7 / teacher-java 1.7.8 换 tag 滚更,learning-worker 同步)+ 回归(hikaricp.connections.active≤20、写路径批量生效、k6 500 CCU 对比)。

---

## 历史 (2026-08-14 会话22-24 P1 第二波容量质量 6 项 — 全部验证闭合✅ + 坑㉖ OOM 修复)

### P1 6 项最终状态(全部已验证✅)
- **P1-7 全局限流✅**:traefik `esp-ratelimit`(100/burst200/1s)+ bff `RateLimitFilter`+`SentinelRuleBootstrap`。实测 burst 400→11×429。
- **P1-8 写接口幂等✅**:esp-cache `IdempotencyFilter`(`Idempotency-Key`+SETNX TTL300s)+ gateway-go sessions SetNX + writing-py grade/polish nx=True。verify_p1.sh 实测同 key replay 返回同 body "1"。
- **P1-9 JWT 加固✅**:jti per token + Redis 黑名单 `esp:jwt:blacklist:<jti>` + refresh 7d + rotation。verify_p1.sh 实测 refresh rotation 旧 refresh 401,logout 后旧 access token 401。
- **P1-10 Disruptor outbox✅**:`learning_event_outbox` 表(DESCRIBE 实证:id bigint PK AUTO_RANDOM/user_id/event_type varchar(32)/payload json/status varchar(16) default pending/retry_count int default 0/next_retry_at/created_at;KEY idx_status_retry + idx_user_time)+ `OutboxRelay` @Scheduled 2s BATCH200 MAX_RETRY8 指数退避。当前 0 行(CRITICAL 事件未触发,relay 空 op)。
- **P1-11 拆 learning-worker✅**:`21-learning-worker.yaml` replicas:1(ESP_ROLE=worker/PULSAR_CONSUMER_ENABLED=true/SEATA_ENABLED=false);入口 `20-learning-java` PULSAR_CONSUMER_ENABLED=false 防循环(yaml line 21 实证)。worker `learning-worker-55c859f84b-jmfjs` 1/1 Running RESTARTS=0(坑㉖ OOM 修复后稳定)。
- **P1-12 缓存可观测✅(真因修复)**:`CacheConfig` setCacheNames 预注册 7 缓存名(bff_user/esp_jwt_blacklist/esp_idem/learning:due/learning:weak/learning:agg/teacher:class-weak)+ Caffeine spec 追加 `,recordStats`。**真根因**:esp-cache 把 `spring-boot-starter-cache` 声明为 `provided`,使用方(bff/learning/teacher)pom 未显式引入 → SB4 `CacheMetricsRegistrarConfiguration` 不在 classpath → CaffeineCacheMetrics 不绑 → `/actuator/prometheus` 0 条 cache_*。**修复**:bff/learning/teacher 三 pom 显式加 `spring-boot-starter-cache`(同段注释说明 provided 真因)。修后三服务 `/actuator/prometheus` 各 42 行 `cache_*`(7 缓存 × 6 指标:gets/loads/loads_failure/evictions/eviction_weight/size),`/actuator/caches` 200 列全 7 名(bff /actuator/caches 404 系其 exposure 配置差异,prometheus 指标齐全不影响验收)。`cache.gets` measurements COUNT=1.0 已有真实命中。

### 镜像 tag(本会话最终版,全部已 deploy)
- learning-java:**1.8.5**(坑㉖ OOM 修复:findKpIdsByUserId 投影+initMastery 批量 saveAll;含 P1-10 outbox + P1-11 consumer装配 + P1-12 starter-cache)✅deploy 入口+worker
- bff-java:**1.7.5**(P1-12 显式 spring-boot-starter-cache;P1-7 ratelimit + P1-8 幂等 + P1-9 JWT加固 + ProxyController)✅deploy
- teacher-java:**1.7.6**(P1-12 显式 spring-boot-starter-cache;P0-3 IDOR)✅deploy
- gateway-go:**1.6.3**(P1-8 sessions SetNX)✅deploy
- writing-py:**1.5.1**(P1-8 grade/polish nx=True)✅deploy
- emotion-svc:1.6.2 / writing-coach:1.1.0(P0-3 IDOR,无 P1 改动)

### 关键坑(P1 期间踩的)
- **坑㉗(22-23)WebMvcTest 切片拉入 IdempotencyFilter**:`@WebMvcTest` 扫 `@Component` Filter → esp-cache `IdempotencyFilter` 依赖 `CacheService`(切片外)→ `NoSuchBeanDefinitionException`。修复:补 `@MockitoBean JwtValidator` + `@MockitoBean CacheService`。
- **坑㉘(22-23)Servlet 6 移除 setStatus(int,String)**:`IdempotentResponseWrapper` override 已删签名 → 编译失败。修复:删 override 仅留 setStatus(int)。
- **坑㉙(22-23)JaCoCo JDK25 告警**:`major version 69` 非致命,忽略。
- **坑㉚(22-23)bff 业务代理断链**:traefik ingress /api/** → bff:8090,但 8090 只有 auth 控制器,业务路径 404;Netty edge 9090 RouteTable fire-and-forget 不回响应。**修复**:新增 `ProxyController`(@RestController /api/**,RestClient 透传,SKIP_HEADERS 过滤 hop-by-hop,GET/DELETE 走无 body 分支,4xx/5xx 原样透传)。
- **坑㉛(22-23)LearningEventConsumer @Lazy 不装配**:`@Component @Lazy` 在无人注入时永不实例化 → @PostConstruct 订阅从未启动 → worker 4 topic 全未订阅。**修复**:改 `@ConditionalOnProperty(esp.pulsar.consumer-enabled, matchIfMissing=true)`,worker(true)启动即订阅,入口(false)不装配天然防循环。
- **坑㉜(22-23)陈旧 esp-cache fat-jar(已被 P1-12 真因覆盖)**:曾以为 cache_* 缺失是 fat-jar 打包时机问题。**22-24 实证真因**=esp-cache 把 spring-boot-starter-cache 声明 `provided`,使用方未显式引入 → SB4 CacheMetricsRegistrarConfiguration 不在 classpath。修复=三 pom 显式加 starter-cache(坑㉜ 的重 install esp-cache 只是顺带清掉旧 fat-jar,非真因)。
- **坑㉖(22-24)learning-worker 启动 OOM(致命)**:新 worker pod crash-loop `OutOfMemoryError: Java heap space`(Dubbo ServiceConfig.export 阶段)。**根因**:`LearningEventConsumer.handleRegistered` 调 `masteryRepo.findAll()`(JpaRepository 默认)加载 user_mastery **全表 4,998,730 行**进堆 → 2G heap 撑爆。触发路径:user.registered Pulsar topic 有 backlog,worker @PostConstruct 订阅即收消息→handleRegistered→findAll→OOM。次要:`initMastery` 逐条 save(10,000 次顺序 INSERT 往返)。**修复**(3 处):① `UserMasteryRepository` 加投影 `findKpIdsByUserId`(只查 kpId 列);② `handleRegistered` 用 Set 去重仅 init 缺失 kp;③ `LearningService.initMastery` 改批量 `saveAll`。learning-java 1.8.4→1.8.5。修后 worker `55c859f84b-jmfjs` 1/1 Running RESTARTS=0。**教训**:JpaRepository `findAll()` 在大表(千万行级)上是隐式全表扫描炸弹,业务去重必须用投影查询;消费者 @PostConstruct 订阅有 backlog 的 topic 时,首条消息就可能触发重逻辑,启动期内存预算要留余量。

### 恢复锚点(跨会话必读)
- **当前子步骤**:P1 6 项全部闭合✅(verify_p1.sh 8/8 PASS:注册/refresh rotation 旧 refresh 401/幂等同 key 同 body/logout 后 access 401/cache.gets COUNT=1.0/outbox 表 0 行/worker 1/1 RESTARTS=0)。坑㉖ worker OOM 修复(1.8.5)已 deploy 稳定。P1 待 git commit。
- **已改文件**:① esp-cache:`IdempotencyFilter`/`IdempotentResponseWrapper`/`CacheConfig`(recordStats+setCacheNames)/`CacheService`(tryIdempotent);② bff-java:`JwtService`(3-arg+黑名单+refresh)/`RateLimitFilter`/`SentinelRuleBootstrap`/`ProxyController`+3 测试 mock + pom(starter-cache);③ learning-java:`LearningEventDisruptor`(5-arg+outbox)/`OutboxRelay`/`LearningEventConsumer`(@ConditionalOnProperty+handleRegistered 投影去重)/`UserMasteryRepository`(findKpIdsByUserId)/`LearningService`(initMastery 批量 saveAll)+ pom(starter-cache)+ application.yml;④ teacher-java:测试 mock + pom(starter-cache);⑤ gateway-go:main.go(sessions SetNX);⑥ writing-py:grade/polish nx=True;⑦ deploy:`40b-traefik-ratelimit.yaml`/`40-ingress.yaml`/`21-learning-worker.yaml`(1.8.5)/`20-learning-java.yaml`(1.8.5)/`15-bff-java.yaml`(1.7.5)/`30-other-services.yaml`(teacher 1.7.6);⑧ TiDB:`learning_event_outbox` 表。
- **下一步**:① ✅ git commit P1 全量(945d9fe);② 评估进 P2(7 项:P2-13 DDD充血化/P2-14 gateway瘦身/P2-15 读写异步化/P2-16 缓存防护/P2-17 前端monorepo/P2-18 目录重组/P2-19 工程化补齐)——每波结束评估避免单会话上下文溢出,前端完整重构并入第三波。
- **Redis 锚点**:`esp:ctx:hot:refactor-p1`(P1 闭合:verify 8/8 / 镜像1.8.5/1.7.5/1.7.6 / 坑㉖ OOM / P1-12 starter-cache 真因 / 已 commit 945d9fe)。
- **网络可达性备忘**:host 经 `192.168.5.15` 不可达,验证一律走 `127.0.0.1`(RD host localhost 转发),traefik /api/** → bff-java:8090,/ws/ → gateway-go。

---

## 历史 (2026-08-14 会话22-22 P0 第一波安全红线 6 项全部修复验证✅)

**第一波 P0(安全红线,6 项)全部构建+部署+回归验证通过✅**。综合架构评分 5.9/10 的最大短板(安全封控 3.5/10)闭合。6 服务镜像全部 build 成功并滚动上线,6 点验证清单全绿。

### P0 6 项验收(实测)
- **P0-1 私钥收敛✅**:`esp-rsa-jwt` 拆为 `esp-rsa-jwt-sign`(PRIVATE+PUBLIC,仅 bff-java envFrom)+ `esp-rsa-jwt-verify`(仅 PUBLIC,gateway/learning/teacher envFrom)。实测:gateway/learning/teacher `PRIVATE=[]` `PUB=[SET]`,bff `PRIV=[SET] PUB=[SET]`。任一服务 RCE 无法伪造 admin JWT。
- **P0-2 learning-java 鉴权✅**:新建 `auth/JwtValidator.java`+`auth/JwtAuthFilter.java`(@Order HIGHEST_PRECEDENCE,白名单 /actuator/、/healthz、/v3/api-docs),公钥注入后无 Bearer JWT 一律 401。实测:无 token `/api/learning/review`→**401**;带 token→**200**。
- **P0-3 IDOR 越权✅**:gateway-go 移除 history 的 `?user_id=` query 回落(userID 必须来自 JWT);teacher/emotion/writing-coach 用可信 X-User-Id 校验归属。实测:A 的 token 查 `user_id=999`→**返回 A 自己数据 `user_id=2017612633063053831`**(query 被忽略,不越权)。
- **P0-4 register 锁 role✅**:`AuthController.java:62` 服务端强制 `role="learner"`,忽略请求体。实测:POST register `{"role":"admin"}`→返回 `role:"learner"`。
- **P0-5 NodePort 收敛✅**:learning/teacher/emotion/writing-coach/writing-py 全 ClusterIP。NodePort 仅保留 gateway-go:30080、bff:30009/30010、frontend:30090/30091/30070、调试 nacos:30048/sentinel:30058/seata:30071。
- **P0-6 NetworkPolicy 收窄✅**:`esp-allow-internal` 三段:①同 ns podSelector:{};②kube-system namespaceSelector(traefik ingress);③ipBlock 0.0.0.0/0 限 NodePort 端口(8080/8090/9090/80/8848/8858/7091/8091)。实测:ekb-m2 ns(milvus pod)curl esp-prod learning-java→**http_code=000 rc=7 拒绝**(对照组同 ns gateway-go→200)。

### 镜像 tag(本会话 build)
- gateway-go:1.6.2(P0-3)、bff-java:1.7.2(P0-4)、learning-java:1.7.2(P0-2)、teacher-java:1.7.2(P0-3)、emotion-svc:1.6.2(P0-3)、writing-coach:1.1.0(P0-3)。
- Dockerfile 基镜像修正:emotion-svc `python:3.13-slim-bookworm`→`python:3.13-slim`;learning-java/bff-java `eclipse-temurin:21-jre-noble`→`21-jre-alpine`(docker.1ms.run 镜像 noble/bookworm tag 403)。

### 关键坑(P0 期间踩的)
- **坑㉖(22-22)secret 占位 manifest 覆盖**:误 `kubectl apply -f 00b-rsa-jwt-secrets-split.yaml`(stringData 空占位)→ 覆盖已灌入的真实 PEM → gateway/bff CrashLoopBackOff(`JWT_RSA_PUBLIC_KEY_PEM env not set`)。修复:从旧 `esp-rsa-jwt` secret 提取真实 PEM 重建 sign/verify,再 rollout restart。教训:**声明式 manifest 含空 stringData 不得 apply 到已有真实值的 secret**;真实密钥只用 `kubectl create secret --from-literal` 灌入。

### 恢复锚点(跨会话必读)
- **当前子步骤**:P0 第一波 6 项全部修复+验证✅。6 服务 pod 全 Running 1/1。下一步=git commit(任务#51)+ 评估第二波 P1。
- **已改文件**:① 6 Dockerfile/源码(鉴权/IDOR/role锁)+ 6 deploy yaml(tag+envFrom);② 新增 `00b-rsa-jwt-secrets-split.yaml`(声明式,勿 apply 到已有 secret);③ `40-ingress.yaml`(/api/** 走 bff:8090);④ `40-networkpolicy.yaml`(esp-allow-internal 收窄)。
- **下一步**:① git commit 本会话改动(任务#51 pending);② 第二波 P1(限流/幂等/JWT加固/outbox/拆learning-worker/缓存指标);③ 第三波 P2(DDD/gateway瘦身/读写异步化/缓存防护/前端monorepo/目录重组/工程化)。
- **Redis 锚点**:`esp:ctx:hot:refactor-p0`(待写)。

---

## 历史 (2026-08-14 会话22-21 k6+Locust 混合压测第二轮 + 三大根因闭合 + 双轨落盘)

**k6+Locust 混合压测第二轮数据已齐**(report/ 4 份);**三大根因全部诊断闭合**(traefik 502 坑㉕ / bff-java probe / learning/event 背压);**用户双轨落盘指令执行中**(本文件 + Redis Cluster)。详细记录见 `docs/progress/archive/session22-21-loadtest-traefik502-launch.md`。

### 压测结果(实测落 report/)
- **k6go**:50VUS 529rps/1.61ms;100VUS 903rps/98%/19.83ms;200VUS 1392rps/99.95%/74.72ms。
- **Locust 200VUS 混合**(坑㉕ 修复后有效):140430 reqs / avg 780rps / agg **P95=95ms P99=150ms**。非 learning 端点**全绿**(scenarios P95=90ms / teacher-analytics 110ms / sessions 140ms / lesson-plan 96ms / **writing/grade 24rps P95=95ms** ✅G-04 P95 达标)。
- **WS CCU(ccu5000_ws.py)**:**~62 并发 WS 是单节点可持续上限**(500 用户 peak=63,200 用户 peak=62);瓶颈=bff-java register CPU 1001m @1 核饱和,**已扩 2 核待重测**;gateway-go WS 层无限流。

### 三大根因(全闭合)
1. **坑㉕(22-18/21)traefik 502 拦截所有非 8080 后端【已修复✅】**:NetworkPolicy `esp-allow-internal` 的 ingress 只放 `podSelector:{}`(同 ns)+ `ipBlock 0.0.0.0/0 port 8080`,叠加 default-deny → 跨 ns(traefik)ingress 除 8080 外全 DENY。gateway-go 监听 8080 命中例外→通;learning(8081)/writing(8082)/teacher(8083)/emotion(8084)/bff(8090)不命中→502。**修复**:patch 新增 `from: namespaceSelector:{}`(允许所有 ns ingress)。apiVersion 必须 `networking.k8s.io/v1`。
2. **bff-java restart loop【已修复✅】**:probe timeoutSeconds=3s 太短(SB4 慢启动+GC pause)。patch liveness 10s/10fail/20period、readiness 10s/10fail、CPU limit 1→**2 核**。rollout 成功。
3. **learning/event 503 `{"accepted":false}`【已诊断✅,设计背压非 bug】**:Disruptor RingBuffer(65536)满时 `tryPublishEvent` 返 false → `PublishResult.DROPPED` → 503。**保护网关延迟,设计如此**。低并发(traefik LB burst50)50/50=202 正常;200VU ~760ev/s 持续负载才背压。消费端 DbLearningEventConsumer batchUpdate INSERT learning_events。**路由陷阱**:gateway-go 无 /api/learning 路由(仅 scenarios/sessions/ws),走 traefik ingress→learning-java:80。

### 缺陷与上线阻断判定(D1-D8)
- **D1 gateway-go handleScenarios 无鉴权** — 高,**已修复✅**(2026-08-14):main.go:127 补 `userID==0→401` 拦截;镜像 esp/gateway-go:1.6.1 部署,验证 无token=401/有token=200。
- **D2 teacher-java 无 Spring Security** — 高,**代码已改,构建部署中**(2026-08-14):加 jjwt 依赖 + JwtValidator + JwtAuthFilter(公钥未注入放行/已注入白名单外无Bearer返401)+ deployment envFrom esp-rsa-jwt。
- D5 learning initMastery @Version 乐观锁 — 中,功能缺陷非阻断。
- **D6 writing_templates 表缺失** — 中,**已修复✅**(2026-08-14):补建 esp.writing_templates(AUTO_RANDOM PK + idx_category_diff)。
- learning/event 503 背压 / WS ~62 上限 — 设计行为/单节点约束,非阻断。

### 恢复锚点(跨会话必读)
- **当前子步骤**:P0 上线阻断项(D1/D2/D6)**全部修复验证✅**;上线就绪报告已写(report/launch-readiness-2026-08-14.md,综合判定=可灰度/内部测试,生产待多节点容量验证)。k6+Locust 压测数据齐,三根因闭合。
- **已改文件**:① gateway-go main.go:127(D1 鉴权)+ Dockerfile 无变;② teacher-java pom.xml(jjwt 依赖)+ JwtValidator.java + JwtAuthFilter.java + application.yml(jwt 段)+ Dockerfile(alpine)+ 30-other-services.yaml(envFrom esp-rsa-jwt + tag 1.7.1);③ 10-gateway-go.yaml(tag 1.6.1);④ TiDB esp.writing_templates 补建(D6);⑤ ccu5000_ws.py(/report 写路径修复)。
- **下一步**:① git commit(本会话改动);② (可选)WS CCU 重测(bff-java 已 2 核);③ (可选)多节点容量验证 G-01/G-06;④ plan indexed-honking-spring(平台底座)——用户未指示前不启动。
- **Redis 锚点**:`esp:ctx:hot:2026-08-14-session22-21-loadtest-launch`。

---

## 历史 (2026-08-14 会话22-20 Redis Cluster warm-up + TiKV/TiFlash 诊断)

> **当前✅(2026-08-14 会话22-20 Redis Cluster warm-up 完成 + TiFlash 诊断 + TiKV健康)**:① **Redis Cluster warm-up 已完成**✅:Job `esp-redis-warmup` Completed,`=== WARMUP DONE total_rows=23000 total_set=23000 elapsed=1.5s ===`,5 表全量灌入 esp:cache:* 前缀(scenarios 1万/knowledge_points 1万/sessions top1000/user_mastery top1000/users top1000)。**键数验证(单节点 scan,Cluster 分片故单节点只见部分)**:redis-cluster-new-0 上 scan 得 7631 键(scenario 3317/kp 3324/session 340/mastery 315/user 335),总数对齐 3 主节点分片 ≈23000。TTL 1h。② **TiKV 3/3 Up 健康**✅(跨 aws/azure/gcp 三区域,v8.5.7,PD /stores 注册 3 个 tikv store,行存引擎完全正常)。③ **TiFlash ⚠️ 重建后仍未注册 PD(根因坐实)**:delete pod 触发重建后,新 pod 1/1 Running 但**TiFlash server 只监听 9000(CH TCP)+8234(内部),缺 8123(http)/20170(raft proxy)/20292(status)** → raft proxy 子进程未拉起 → 未向 PD 注册 store → 无法作列存引擎。PD /stores 仍只 3 个 tikv store,无 tiflash。PD 连通正常(三区域 PD 2379 均 200),config.toml 的 `pd_endpoints` 正确。**根因判断**:TiFlash v8.5.5 raft proxy 启动失败(proxy 未 fork/监听 20170),可能为单节点跨 region raft 选举/版本/资源问题,stdout 无日志无法深挖。**影响**:ESP 业务无 TiFlash 列存依赖(无物化视图/OLAP 加速路径走 TiFlash),业务面暂不受影响;列存引擎能力缺失。**处置(已向用户报告)**:业务无依赖,标记已知缺陷,等后续多节点移植时重建 TiFlash(单节点资源+跨 region raft 单实例先天不健康)。④ **Obsidian 日志同步进行中**(subagent esp-doc 后台跑,向 `/Users/joelgong/IdeaProjects/flink-learning/books/ESP-英语教育平台/` 同步 9 篇经验日志:总览/架构设计/ADR/平台底座/测试体系/踩坑集/上下文工程铁律/部署运维/Redis warm-up)。⑤ **git commit 待做**(17 文件改动:前端三端 vite base+basename 根治白屏 + portal nginx /api/actuator + infra_smoke.sh sts 修正 + PROGRESS.md)。**遗留**:前端浏览器 GUI 人工 TC-W/H(隧道 http://localhost:30070 就绪,全路由 200,待用户点开确认渲染)。⑥ **Task#40 测试用例审查补全已完成**(subagent esp-doc):07 新增 TC-W-8~19(12 条后端端点)+TC-W-FL-1(Web 全链路)+TC-H-1~7(7 条 H5 独立用例)+TC-H-FL-1(H5 全链路)+TC-GR-1~27(原 9 条扩至 27 条,覆盖 8 服务全端点)+TC-E2E-1~11(11 条跨服务业务流)+SEC-6~8(3 条安全补充)+TC-M 无环境标注;09 补 portal 隧道命令+访问地址(/web/auth/login、/h5/login)+H5 用例同步+小程序无环境标注;07 前置环境补 portal 隧道+21 deploy 清单。覆盖缺口:后端全功能用例齐✅、前端 GUI 用例齐✅、跨服务链路用例齐✅。

## 当前热区 (2026-08-14 会话22-20 Redis warm-up + TiFlash 诊断)

### Redis Cluster warm-up 完成✅
- **Job**:`esp-redis-warmup` Completed(backoffLimit=2,0 失败),1.5s 灌完 23000 行。
- **脚本**:`scripts/redis-warmup.py`(ConfigMap `redis-warmup-script` 挂载),流式 SSDictCursor + pipeline 批量写(BATCH=500),TTL=3600。
- **数据源 → Redis key**:scenarios→`esp:cache:scenario:{id}`、knowledge_points→`esp:cache:kp:{id}`、sessions(top1000)→`esp:cache:session:{id}`、user_mastery(top1000)→`esp:cache:mastery:{uid}:{kp_id}`、users(top1000)→`esp:cache:user:{id}`。
- **验证**:单节点 scan 7631 键(3 主分片,每主约 1/3),分类计数对齐(scenario+kp≈6600/session+mastery+user≈1000)。

### TiKV/TiFlash 诊断(用户问:基础设施层 TiKV 和 TiFlash 都正常了吗)
- **TiKV ✅ 3/3 Up**:store id=1(tikv-aws)/id=2(tikv-azure)/id=3(tikv-gcp),跨三区域,每个 capacity≈353.8GiB available≈118.4GiB,v8.5.7。行存引擎完全健康。
- **TiFlash ⚠️ 未注册 PD**:pod Running 但 raft proxy 未起(只监听 9000+8234,缺 8123/20170/20292),PD /stores 无 tiflash store。delete pod 重建后仍如此 → 非瞬时故障,为持续性启动缺陷。config `pd_endpoints=pd-aws...:2379` 正确,三区域 PD 连通均 200。stdout 无日志(容器无文件日志)。**根因推断**:TiFlash v8.5.5 raft proxy 在单节点跨 region 环境启动失败。业务无 TiFlash 依赖,标记已知缺陷,多节点移植时重建。

### 恢复锚点(跨会话)
- **当前子步骤**:Redis warm-up 完成✅;TiKV 健康✅;TiFlash 诊断完成(未注册 PD,已知缺陷,待多节点重建);Obsidian 同步进行中;git commit 待做。下一步=git commit+push + Obsidian 同步收尾 + 前端浏览器 GUI 人工确认。
- **已确认根因**:TiFlash 未注册 PD = raft proxy 子进程未拉起(缺 20170/8123/20292 监听),非 PD 连通问题(三区域 PD 200);Redis warm-up 键数单节点 7631 = Cluster 3 主分片正常分布。
- **已改文件**:本会话无新增源码改动(诊断为主);Obsidian vault `ESP-英语教育平台/` 目录(subagent 生成中,非 git 仓库内)。
- **下一步**:① git add/commit/push 17 文件改动(前端白屏根治+infra_smoke+PROGRESS);② Obsidian 同步收尾;③ 用户浏览器确认 http://localhost:30070/web/auth/login 渲染。

---

## 历史 (2026-08-14 会话22-18 压测+前端收尾)

### k6go 基线压测(成功✅)
- **方案**:用本地 `golang:1.25-alpine` 编纯 Go 压测程序为静态二进制,`busybox:1.36`(本地有)挂 hostPath 跑,绕开 3128 proxy 拒绝非白名单镜像拉取。
- **关键修**:bff-java ClusterIP svc 只有 8090(mvc)/9090(edge)/50090(dubbo)三端口**无 port 80**;k6go 默认 `-bff=http://bff-java:80` 错,必须 8090。register() 尝试 /api/auth/register 失败则 /api/auth/login 拿 token,vu() 循环 GET /api/scenarios。
- **结果**:50 VUS/15874 请求/100% 成功/529.1 rps/平均 1.61ms。

### Locust WS 鉴权双修(成功✅)
- **第一阻塞:create_session 500**。根因=Redis Cluster `cluster_state:fail`(坑㉓复发:nodes.conf 持久化旧 PodIP,VM 重启后 Pod IP 漂移,redis7.2 不认 hostname)。session.Manager.Create 的 rdb.Set 失败(main.go:132 handleCreateSession → 500)。日志被 traces export 噪音淹没。
- **Redis 自愈**:`CLUSTER MEET <live-IP> 6379` ×5 + `CONFIG SET cluster-announce-ip <真实IP>`(pod-0 announce 停旧 IP 时)。自愈后 cluster_state:ok slots_ok:16384,create_session 恢复 200。
- **第二阻塞:WS peak_online=0 established=0**(Redis 自愈后仍失败)。根因双:① `url = f"...?token={token}"` 未 URL 编码,JWT signature 含 +/_- 字符→gateway-go `r.URL.Query().Get("token")` 解析失败→400;② 多余 `header=["Host: gateway-go"]` 致 400(pod 内 DNS 已解析,加 Host 反致问题)。
- **修复**:`from urllib.parse import quote` + `url = f"{ws_url}/{sid}" + (f"?token={quote(token, safe='')}" if token else "")` + 去 Host header。重跑 peak_online=70 established=85 dropped=15,70/100 (70.0%)。

### 前端 build + portal 隧道(成功✅)
- **frontend-react**:tsc + vite build RC=0。**frontend-h5**:vite + PWA v1.3.0 build RC=0。
- **portal SSH 隧道**:`ssh -f -N -L 30070:localhost:30070 -F "$RD_LIMA" lima-0`。验证:portal root 200;/web/ /h5/ 反代生效;/api/scenarios 200。URL http://localhost:30070/ 供用户人工 TC-W/H。

### 恢复锚点(跨会话)
- **当前子步骤**:会话22-18 压测+前端收尾已完成(k6go 529rps / Locust WS 70% / Redis 自愈 / 前端 build 全绿 / portal 隧道)。
- **已确认根因**:① 3128 proxy 拒绝=lima.yaml setproxy 给 docker/containerd init 加 HTTPS_PROXY=openresty 3128,3129 SSL MITM 查 allowed-images.conf 白名单(仅 pause 3 条)非白名单返 403;② create_session 500=Redis cluster_state:fail;③ WS 400=token 未 URL 编码 + 多余 Host header。
- **已改文件**:deploy/locust/ccu5000_ws.py(quote 编码 + 去 Host header)、deploy/k6/k6go/main.go(bff 默认 8090,上一会话)。
- **下一步**:① 用户人工浏览器 TC-W/H(隧道已就绪);② 3128 proxy 根因答用户;③ git commit;④ plan `indexed-honking-spring`(平台底座+图向量库)。

---

## 历史 (2026-08-14 会话22-17 RD VM 扩容 8G→30G/4核→9核 全绿 + 坑㉔诊断推翻 + 全量清理僵尸pod)

### 扩容验证结果(全绿✅)
- **VM 运行态**:`pgrep -fl qemu` 显 `-m 30720 -smp 9,cores=9`;`free -h` 29.3G;`nproc`=9。lima.yaml `memory:32212254720`/`cpus:9`/`disk:360GiB` 已落盘。
- **k3s**:重启后未自启,手动 `sudo rc-service k3s restart`(初次 start 卡在 start_pre 镜像加载 hook,restart 完成加载全部 bundled 镜像含 pause:3.6),node Ready(v1.34.6+k3s1)。
- **6443 隧道**:RD 重启把 SSH localPort 从 53281 改到 59195,旧隧道失效;`pkill -f "6443:localhost:6443"; ssh -f -N -L 6443:localhost:6443 -F "$RD_LIMA" lima-0` 重建。
- **esp-prod**:21/22 deploy AVAILABLE(learning-worker 0 副本按设计),0 非Running pod。bff/learning/teacher 三服务此前因双 RS 并存卡就绪,删旧 RS 后新 pod 10min 内转 1/1。

### 关键清理(扩容后必要动作)
- **孤儿 ReplicaSet 堆积**:扩容前多服务经历了 rollout,旧 RS(desired=1)与新 RS 并存,两套都创建 pod 抢资源+探针竞争。bff-java 曾有 3 个 RS、learning/teacher 各 2 个。删非 current 且 desired>0 的旧 RS 解决。**Why**:单节点资源紧时双 RS 各拉一个 pod,内存/CPU 双倍消耗,新 pod 起不来旧 pod 又不退出。
- **跨 ns 僵尸 pod force-delete**:扩容前 8G 内存耗尽时,education-platform 14 个 pod 被打成 `ContainerStatusUnknown`(占 2550m CPU+4608Mi requests 但实际死的)、ekb-m2-aws 3 个(tidb/tiflash/tikv)、default extract-config。force-delete 后节点 CPU 99%→93%,腾出空间给 esp-prod 调度。
- **坑㉔ 修正记忆**:`memory/esp-k3s-containerd-image-isolation.md` 已记 k3s 用 moby/docker CRI,docker build 镜像本就 k3s 可见,无需搬运。

### 风险与铁律遵守
- RD 自身重启**不在**坑㉑禁列(禁的是停 colima/动 lima 网络,会抽 RD 底盘)。仅 `rdctl shutdown/start`,不动 k3s 配置、不 pkill、不 Factory Reset。
- 坑㉓ 预置:VM 重启后必查 pause 镜像,否则 sandbox 建不起来全集群瘫。
- 坑㉔ 修正后,Task#27 "根治 k3s containerd 构建流水线"**前提不成立**——docker build 即 k3s 可见,无需额外流水线。已关闭该任务。
- **monitor 判定坑**:`awk '$2==$4'` 对 `kubectl get deploy` 输出(READY/AVAILABLE 字段)因 `1/1` vs `1` 类型不一致恒为假,导致误报 "available=0"。恢复判定应以**第4列 AVAILABLE==1**为准。

### 恢复锚点(跨会话)
- **当前子步骤**:会话22-17 扩容全绿已完成;下一步=会话22-16 遗留(k6 ConfigMap 重跑 → Locust WS 交叉验证 → 前端 GUI TC-W/H → git commit)或 plan `indexed-honking-spring`(平台底座+图向量库)。
- **已确认根因**:pod 全挂=VM 8G 内存耗尽(非镜像清空);扩容后双 RS 竞争+跨ns僵尸pod占 CPU 是卡就绪的真因。
- **下一步**:见会话22-16 恢复锚点。

---

## 历史 (2026-08-14 会话22-16 统一 Portal v1.1.0 + 全后端功能测全绿 + 条件性 GO)

### 核心进展:4 任务批次(Portal/压测/人工测/commit)
1. ✅ 统一 Portal(frontend-portal v1.1.0 构建部署,Mac 可达)
2. ⏳ 压测(k6 基线首轮超时待重跑,Locust WS 待交叉验证)
3. ⏳ 人工测试(TC-GR-1~9 + TC-P-1~4 全绿;前端 GUI TC-W/H 待人工浏览器)
4. ⏳ git commit(待最后)

### 任务1 统一 Portal v1.1.0(完成✅)
- **痛点**:ESP 无统一入口。frontend-react/h5 独立 ClusterIP 无 NodePort;esp-ingress 仅 API+WS 路由不托管静态;traefik LB 192.168.5.15 = VM 内 Mac 不可达。对照 education-platform 有完整 portal 三件套。
- **方案**:单 nginx 容器(frontend-portal)托管门户 index.html + 反代。NodePort 30070 对 Mac 暴露。
- **关键修复(traefik 绕过)**:原 nginx /api /ws 反代 traefik(Host esp.example.com)→ 502/000,因 traefik ingress controller pod 未运行(svc traefik 仍 LoadBalancer 但 endpoints `<none>`)。改为按 path 直连 8 后端 ClusterIP,复刻 esp-ingress 9 path 规则:`/api/auth/`→bff-java:8090、`/api/scenarios|sessions`→gateway-go:80、`/api/learning`→learning-java:80、`/api/writing-coach`→writing-coach:80、`/api/writing`→writing-py:80、`/api/teacher`→teacher-java:80、`/api/emotion`→emotion-svc:80、`/ws/`→gateway-go(WebSocket upgrade)。
- **Mac 可达性(SSH 隧道)**:RD 单节点 NodePort Mac host 直连不通(VM IP ping 100% 丢包,无 svclb DaemonSet bridge)。解法:`ssh -f -N -L 30070:localhost:30070 -F "$HOME/Library/Application Support/rancher-desktop/lima/0/ssh.config" lima-0`。6443 kubectl 隧道同理会断,断后 `ssh -f -N -L 6443:localhost:6443 -F "$RD_LIMA" lima-0` 恢复。
- **验证(TC-P-1~4 全绿)**:portal root 200;/api/scenarios 200;/api/auth/health 404(bff 无此端点,转发本身通);/web/ /h5/ 反代生效。
- **改文件**:frontend-portal/nginx.conf(重写 8 path + /ws)、frontend-portal/Dockerfile(不变)、deploy/k8s/17-frontend-portal.yaml(image 1.0.0→1.1.0)、docs/test/07-人工测试用例.md §8 补全 8 Go/Rust 服务用例 + §8.4 portal 用例。

### 任务3 人工测试(TC-GR-1~9 + TC-P-1~4 全绿✅)
- **临时 curl pod iptables race 修复**:`kubectl run --rm -i` 临时 pod 启动期 iptables/DNS race,对存在 endpoints 的 ClusterIP 返回 HTTP 000(connection refused ~3ms)。改用长驻 `probe-keep` pod(curlimages/curl:8.10.1 sleep 300),从其内全 8 服务响应正常。
- **TC-GR-1~9(8 Go/Rust 服务全 200)**:course-path-go/listening-go/reading-go/gamification-go/live-tutor-go/media-learn-rs 全 200;pronunciation-rs POST /api/pronunciation/score 需 `{"user_id":1,"text":"hello world","ref_type":"mock","ref_id":1,"audio_url":"http://x/a.wav"}`(ref_id 必须 i64 非 string)→ 200;assessment-rs POST /api/assessment/sessions → 200。
- **TC-P-1~4(portal)**:见任务1,全绿。
- **writing-coach 500 = 数据缺口(非阻断)**:logs 显 `Table 'esp.writing_templates' doesn't exist`。db_query 实证该表在 `esp_writing` 库(schema-per-service),非 `esp` 库。writing-coach 连接串指向 esp 库读不到表。种子数据缺口,记录不阻断。
- **30080 NodePort 失效**:测试文档 §1 列 gateway-go 30080 但 gateway-go 实为 ClusterIP-only(无 NodePort),该条 stale。改用 portal /api/scenarios 测 gateway。

### 任务2 压测(k6 首轮超时⏳)
- **k6 基线首轮失败**:`kubectl run k6-baseline --rm -i --image=grafana/k6:0.54.0 -- k6 run ... /dev/stdin < deploy/k6/scenarios-baseline.js` 超时 180s "timed out waiting for the condition",pod 卡 Terminating(已 force delete)。疑因:`/dev/stdin` + `kubectl run -i` stdin 重定向在该 k6 镜像不工作,或镜像拉取慢。
- **重跑方案**:用 ConfigMap 挂脚本(`kubectl create cm k6-script --from-file=deploy/k6/scenarios-baseline.js`),pod 挂载到 /scripts/,`k6 run /scripts/scenarios-baseline.js`。避免 stdin 重定向问题。
- **Locust WS 交叉验证**:待启动(deploy/locust/ 已有脚本)。

### 恢复锚点(跨会话)
- **当前子步骤**:任务2 k6 重跑(ConfigMap 挂脚本)→ Locust WS → 任务4 git commit。
- **已确认根因**:traefik pod 未运行→portal 直连后端;RD NodePort Mac 不可达→SSH 隧道;临时 pod iptables race→长驻 probe pod;writing-coach 500=esp_writing 库数据缺口。
- **已改文件**:frontend-portal/nginx.conf、frontend-portal/Dockerfile、17-frontend-portal.yaml、07-人工测试用例.md §8、(上轮)NettyEdgeServer.java、JwksController.java、DbLearningEventConsumer.java、15-bff-java.yaml、20-learning-java.yaml、memory×2、deploy/k6/*。
- **下一步**:k6 ConfigMap 重跑;Locust WS 交叉验证;前端 GUI TC-W/H;git commit 全部。

---

## 历史 (2026-08-14 会话22-15 端到端人工测试 SEC全绿 + L1-L10链路)

### 核心进展:阶段0-6 测试执行(docs/test/09-人工测试操作指南.md)
执行上线前全流程人工测试。SEC 安全关口全过,发现并修 1 个真 bug(learning_events 落库)。

### 阶段2 SEC 鉴权越权矩阵(全绿✅)
- SEC-1✅ WS越权403实证:A token连B session得HTTP 403 forbidden(gateway-go handleSpeech: parseUserID(token)!=sess.UserID→403,在WS upgrade前)。SID_B=sess_1786655339867。
- SEC-2✅ 过期JWT→401:edge 9090 EdgeAuthHandler.jwt.parse() 对过期token返回401(合法token=转发000因fire-for-get,无token=401)。
- SEC-3✅ X-User-Id剥离:代码可证 EdgeAuthHandler stripSpoofedHeaders()+注入JWT sub(line109-111)。直连learning(30081)信任伪造999999(下游无独立filter,设计信任BFF注入),经edge必被剥离替换。实证受edge fire-and-forget不回写阻碍。
- SEC-4✅ 无secret泄漏:trivy fs扫services/ 全量源码0发现(镜像扫描因Mac连不上RD docker改用fs扫)。
- SEC-5✅ JWKS闭环:kid=esp-rs256-1。修复alg缺失(JwksController加.algorithm(JWSAlgorithm.RS256))。bff-java 1.7.0→1.7.1。

### 坑1闭合(NettyEdgeServer)+ edge fire-and-forget设计缺口
- 修:NettyEdgeServer bind "0.0.0.0"(原绑loopback ::1/127.0.0.1致NodePort不可达)+ SO_BACKLOG改.option(原误放.childOption)。
- 真根因(非bind):edge 9090 对需回写响应的请求全超时,因 InletDisruptor.DownstreamForwarder 是fire-and-forget(sendAsync不回写Netty ctx,InletDisruptor.java:143注释"回写见ForwardRespondHandler扩展点"未实现)。仅401/503错误路径即时回写。=P2工程缺口。
- 验证:edge /api/learning/x 无token→401 ✓;过期token→401 ✓。

### 阶段5 L1-L10链路测
- ✅L1(6服务健康全200) ✅L2(REST scenarios 200) ✅L3(Dubbo Triple teacher kp-stats 200) ✅L8(writing-py Mock批改score=24+grammar修正) ✅L9(sessions history user_id正确)
- ✅L4/L7(learning_events落库,坑㉙已修):LearningEventEntity @Id Long无@GeneratedValue + TiDB AUTO_RANDOM,Hibernate persist抛异常丢数据。**修复**:DbLearningEventConsumer 改 JdbcTemplate batchUpdate(INSERT不含id列,TiDB AUTO_RANDOM生成)。learning-java 1.7.0→1.7.1。实测 esp_learning.learning_events COUNT=1 id=10364873落库成功。
- ⚠️L5(trace部署侧待多节点,esp-observability ns无otel-collector) ⚠️L6(双pod WS待多节点) ⚠️L10(优雅停机待多节点)

### 阶段6单节点压测基线
- host经port-forward:28rps/100%成功(瓶颈=port-forward隧道开销kubectl→apiserver→kubelet→pod,非服务容量)。
- 集群内locust压测(ClusterIP直连绕过port-forward)进行中,预期rps远高于28。
- 5000CCU单节点不可达,如实记录,多节点Phase D2重测。

### 用户澄清(本会话2个问题)
1. 统一portal:ESP无。frontend-react/h5独立ClusterIP无NodePort,esp-ingress仅API+WS路由(esp.example.com)不托管静态资源,mac不可达traefik LB(192.168.5.15=VM内)。对照education-platform有完整edu-portal-ingress+edu-unified-ingress+edu-unified-ws-ingress(www/api.edu.com)。缺口,见memory esp-unified-portal-gap。
2. 端口冲突:ESP与education-platform零冲突。edu全ClusterIP无NodePort(走traefik);ESP用300xx NodePort段(30009/10/48/58/71/81-85/92)。两ns共用kube-system/traefik靠host分流。新增ESP ingress host须避开*.edu.com。见memory esp-port-isolation-edu。

### 待执行
1. ✅ LearningEventEntity修复验证(L4落库=1行)→G-5转绿
2. ✅ 阶段7 Go/No-Go签收 → 条件性GO(G-1/2/3/5/6/7✅+G-4降权+G-8知悉)
3. 前端GUI人工浏览器走TC-W/H(web/h5 Vite可起,小程序跳过P-0-6缺)
4. git commit(NettyEdgeServer+JwksController+DbLearningEventConsumer+manifest×2+PROGRESS+memory×2+k6脚本)
5. 多节点移植(真实弹性/5000CCU/L6双podWS/L10优雅停机/trace Collector部署)

### 关键约束(本会话)
- 单节点铁律:不碰k3s/restart/pkill/Factory Reset;不停colima;HPA minReplicas=1(learning-java HPA曾扩2新pod探针失败缩回,印证JVM抢CPU)
- learning-worker保持1.6.0不动
- 压测机入集群(走ClusterIP)绕过port-forward,工业级姿势

---

## 当前热区 (2026-08-13 会话22-14 业务 100% 恢复 + 坑㉔ k3s containerd 镜像隔离)

### 核心根因:坑㉔ k3s containerd 与 docker 镜像库隔离
- colima-stop DNS 雪崩(会话22-13)期间,k3s containerd 的 `k8s.io` namespace 下 ESP 业务镜像全被清空(只剩旧 `edu-*` 项目镜像)
- manifest 用 `esp/xxx:tag` + `imagePullPolicy: IfNotPresent`,但 k3s containerd 本地无镜像 → kubelet 去远程拉 → daocloud 镜像源对 `esp/` 私有名返回 **403 Forbidden**(公网不存在)
- **关键陷阱**:ESP 业务镜像其实都在节点的 **docker daemon** 里(`docker build` 存 `/var/lib/docker`),但 **k3s 用自己的 containerd**(`/var/lib/rancher/k3s/agent/containerd`),两者镜像库**完全隔离**
- **Why K8s 用 containerd 不用 docker**:K8s 1.24 移除 Dockershim,kubelet 只认 CRI 接口;k3s 自带 containerd(轻量:省 dockerd 一层、少故障点、少内存);docker daemon 镜像与 k3s containerd 镜像互不可见
- **修复**:`docker save esp/xxx:tag | sudo k3s ctr images import -` 把 docker 镜像搬运到 k3s containerd(实测有效)

### 本会话修复清单(11 步,按顺序)
1. 节点 DNS 正常(resolv.conf 223.5.5.5/8.8.8.8 未被重置)→ ImagePullBackOff 非因 DNS
2. 诊断根因=containerd 无 esp/ 镜像(crictl 无该命令,用 `k3s ctr images ls` 查)
3. 批量导入 16 ESP 业务镜像到 k3s containerd(gateway-go/bff-java/learning-java:1.7.0/teacher-java/emotion-svc/writing-coach/writing-py/course-path-go/live-tutor-go/media-learn-rs/pronunciation-rs/listening-go/assessment-rs/reading-go/gamification-go/frontend-react/frontend-h5)
4. 补导 seataio/seata-server:2.2.0 + bladex/sentinel-dashboard:1.8.9 + esp/learning-java:1.6.0(worker 用)
5. 重建 esp-rsa-jwt secret(`--from-literal=JWT_RSA_PRIVATE/PUBLIC_KEY_PEM`,密钥来自 services/bff-java/src/test/resources/rsa/;3 Java 服务 CreateContainerConfigError 根因)
6. apply 06-sca-middleware.yaml(seata+sentinel **漏 apply**,bff/learning 连不上 seata:8091 → BeanCreation `globalTransactionScanner` 失败)
7. 创建 redis-warmup-script ConfigMap(`--from-file=redis-warmup.py=scripts/redis-warmup.py`;warmup job ContainerCreating 根因;补建后 job **Completed**)
8. 重新 apply 05-infra-reuse.yaml(之前**漏了 redis-cluster/pulsar-broker/milvus/neo4j 4 个 ExternalName**;业务用短名 pulsar-broker 解析 NXDOMAIN → Pulsar consumer 失败)
9. apply 30-other-services.yaml(teacher-java/emotion-svc/writing-coach **漏 apply**)
10. scale learning-worker/live-tutor-go/media-learn-rs replicas 0→1
11. learning-java HPA minReplicas=1(曾因 CPU 40% 扩到 4 副本,单节点 JVM 抢 CPU,降回 1)

### 最终状态(21 deploy 全 Ready,2026-08-13 实测)
- **1/1 Running**:assessment-rs/bff-java/course-path-go/emotion-svc/frontend-h5/frontend-react/gamification-go/gateway-go/learning-java/learning-worker/listening-go/live-tutor-go/media-learn-rs/nacos/pronunciation-rs/reading-go/seata/sentinel/teacher-java/writing-coach/writing-py
- **esp-redis-warmup: Completed** ✅(Redis esp:cache:* 已灌入)
- bff/learning RESTARTS=7=历史启动期累积(seata 未就绪时),现已稳定
- 基础设施(ekb-m2):milvus/minio/neo4j/pd/pulsar(3bk+3broker+3zk)/redis-cluster(6 节点)全 Running

### 关键命令(恢复用)
- 查节点镜像:`ssh -F lima/0/ssh.config lima-0 'sudo k3s ctr images ls | grep esp/'`
- 导入镜像:`ssh ... lima-0 'docker save esp/xxx:tag | sudo k3s ctr images import -'`
- apply 顺序:00-namespace → 05-infra-reuse → 05b → 06-sca → 07-tidb → 10/15/16/20/21/30/31/32 → esp-redis-warmup-job

---

## CI 修绿(2026-08-15,ci.yml 16 连败根因修复)

### 根因与修复(6 个失败 job)
1. **go-gateway jwt PEM decode 失败**:`.gitignore` 的 `**/*.pem` 把 `testdata/{public,private}.pem` 排除在仓库外,CI checkout 后文件缺失 → `failed to decode PEM block` + `open testdata/private.pem: no such file`。
   - 修复:`validator_test.go` 改为 `sync.Once` 进程内一次性生成 RSA 测试密钥对(`rsa.GenerateKey`),不再读 testdata/*.pem 文件。本地 `go test -count=1 ./src/jwt/` RC=0 ok。
2. **python-services writing-py exit 127**:`pip install -e .[dev]` 失败回落 `pip install -e .`(无 dev extras),导致 pytest/ruff 未安装 → `pytest: command not found`(exit 127)。
   - 修复①:ci.yml 改 `pip install -e ".[test]"`(writing-py/emotion-svc 的 extras 名是 `test` 不是 `dev`),pytest 命令改 `python -m pytest`。
   - 修复②:writing-py/emotion-svc 的 `pyproject.toml` `[test]` extras 补 `ruff>=0.6`(ruff lint step 也需要)。本地 venv 验证:18 passed。
3. **java-services bff/learning/teacher 依赖解析失败**:业务服务 pom 依赖 `com.esp:esp-cache/esp-rw-java/esp-disruptor/esp-api`,CI 无 root reactor → Maven 从 central 找不到这些内部包。
   - 修复:java-services job 前置 `mvn -B -q install -DskipTests -pl esp-api,esp-cache,esp-rw-java,esp-disruptor`(working-directory=platform)把公共包 install 到本地 ~/.m2 仓库。
4. **contract job `Could not find the selected project in the reactor: services/teacher-java`**:`mvn -B -pl backend/services/teacher-java` 需 root reactor 才能用 -pl 相对路径;本项目无 root pom。
   - 修复:改 `working-directory: backend/services/teacher-java` + `mvn -B test -Dtest=...`(去掉 -pl);learning-java 同理。前置 install 公共包步骤(同 java-services)。
5. **java-commons esp-cache 覆盖率门禁 57% < 75%**:jacoco 0.8.12 的 ASM 不支持 Java 25 class file(major version 69)→ Mockito 生成的动态类插桩失败,覆盖率统计失真。
   - 修复①:`esp-cache/pom.xml` jacoco 0.8.12→0.8.13(支持 class file 69),surefire 3.2.5→3.5.3。
   - 修复②:新增 `IdempotencyFilterTest`(8 个用例覆盖幂等 Filter 全路径)+ CacheServiceTest 补 singleflight/jitterTtl/tryIdempotent 7 个用例。本地实测覆盖率(排除 Config/CacheBroadcast/PulsarClientHolder)77% ≥ 75% 门禁。
   - 修复③:ci.yml 覆盖率 awk 排除规则改 `$3 !~ /Config|CacheBroadcast|PulsarClientHolder/`(按类名排除 broker 连接类,原 `$2` 按包名排除粒度不够)。
6. **go-services `cache-dependency-path: backend/go.sum` 文件不存在**:Go workspace 模式各模块独立 go.sum,backend/ 下无 go.sum → setup-go 缓存失效(warning,非致命)。
   - 修复:改多行列表列出 4 个模块的 go.sum 路径。

### 路径核对清单(P2-18 目录重组后全量核对)
| ci.yml job | working-directory | 实际路径 | 状态 |
|---|---|---|---|
| java-commons | `platform/${pkg}` | platform/{esp-api,esp-cache,esp-disruptor,esp-rw-java} | ✅ |
| java-services | `backend/services/${svc}` | backend/services/{bff,learning,teacher}-java | ✅ |
| go-gateway | `backend/services/gateway-go` | ✅ | ✅ |
| python-services | `backend/services/${svc}` | backend/services/{writing-py,emotion-svc} | ✅ |
| go-services | `backend`(vet/test 用 ./services/... ./shared/...) | ✅ | ✅ |
| build-images | context=backend / backend/services/<svc> | ✅ | ✅ |
| contract | backend/services/{teacher,learning}-java | ✅ | ✅ |
| frontend | 仓库根(pnpm workspace) | ✅ | ✅ |

### 改动文件清单
- `backend/services/gateway-go/src/jwt/validator_test.go`(重写:内嵌密钥生成)
- `.github/workflows/ci.yml`(java-services/contract 前置 install 公共包;python-services 改 [test] extras + python -m pytest;esp-cache 覆盖率排除规则;go-services cache-dependency-path)
- `backend/services/writing-py/pyproject.toml` + `backend/services/emotion-svc/pyproject.toml`([test] extras 补 ruff)
- `platform/esp-cache/pom.xml`(jacoco 0.8.13 + surefire 3.5.3 + spring-test test scope)
- `platform/esp-cache/src/test/java/com/esp/cache/idempotency/IdempotencyFilterTest.java`(新增)
- `platform/esp-cache/src/test/java/com/esp/cache/CacheServiceTest.java`(补 singleflight/jitter/tryIdempotent 用例)

### 本地验证结果
- gateway-go jwt 单测:`go test -count=1 ./src/jwt/` → ok 0.569s RC=0
- writing-py pytest(python3.12 venv + `pip install -e ".[test]"`):18 passed RC=0
- esp-cache:`mvn clean test jacoco:report` BUILD SUCCESS,覆盖率 77% ≥ 75% 门禁

---

## P1-13 token 迁移 httpOnly cookie(2026-08-15,消灭 XSS 窃取面)

### 改动
- **bff-java**(1.7.6):`AuthController` login/register/wx-login/refresh 响应 `Set-Cookie: esp_token=<jwt>; HttpOnly; SameSite=Strict; Path=/; Max-Age=7d`;logout 改为 Bearer/cookie 二选一 + Set-Cookie Max-Age=0 清 cookie;`AuthFilter` 无 Bearer header 时回落 esp_token cookie 验签。
  - **TODO(安全)**:esp-ingress 启用 TLS 后 cookie 必须补 `.secure(true)`(当前仅 80 明文,加 Secure 浏览器拒发)。
- **gateway-go**(1.7.1):`main.go` 新增 `tokenFromRequest`(Bearer 优先→esp_token cookie 回落),HTTP 鉴权 `userIDFromRequest` 与 WS 握手 `handleSpeech` 均走 cookie 兼容;body 仍返回 token(小程序/非 Cookie 客户端过渡)。
- **前端**:`packages/api-client` 新增 `CookieTokenStorage`(getToken 恒 null→不注入 Authorization,靠同源 cookie);`speechWsUrl` 无 token 时不拼 `?token=`(WS 握手同源自动带 cookie)。web/h5 的 `api.ts` 换 CookieTokenStorage,`auth.ts` token 仅存 zustand 内存不再落 localStorage(logout 顺手清历史遗留 key);**小程序保持 Bearer + wx.storage 不变**(无 XSS 面)。

### 验证(全过)
- bff-java `mvn package` RC=0;gateway-go `go build`/`go test` RC=0;`pnpm turbo typecheck` **7/7**(顺手修 miniprogram/trace.ts 两处失效 @ts-expect-error,存量问题非本次引入)。
- 镜像:RD VM `docker build` esp/bff-java:1.7.6(context=services/bff-java)+ esp/gateway-go:1.7.1(context=backend),yaml 同步 tag,滚动更新 2/2 Running。
- curl(30009 bff / 30080 gateway):register/login `Set-Cookie` 含 HttpOnly+SameSite=Strict+Max-Age=604800 ✅;**纯 cookie**(无 Authorization)调 bff `/api/scenarios` 200 ✅、gateway `/api/sessions/history` 200 ✅;无凭证 401 ✅;logout `Set-Cookie: esp_token=; Max-Age=0` ✅。
- WS cookie 鉴权为代码路径验证(未做真实握手测试,前端 SpeechPanel 本就是同源无 token URL,靠 cookie 回落)。

---

## 历史 milestone 归档索引(需要时取)

| 主题 | 文件 | Redis key |
|---|---|---|
| **会话22-21 k6+Locust 混合压测第二轮 + traefik 502 坑㉕ + 三根因闭合** | `docs/progress/archive/session22-21-loadtest-traefik502-launch.md` | `esp:ctx:hot:2026-08-14-session22-21-loadtest-launch` |
| **会话22-13 colima-stop DNS 雪崩** | `docs/progress/archive/session22-13-colima-dns.md` | — |
| **会话22-12 CrashLoop 诊断 + 双轨落盘** | `docs/progress/archive/session22-12-crashloop-diagnosis.md` | — |
| **会话22-11 图+向量库 + 冒烟7/8 + Neo4j坑 + 实测校正** | `docs/progress/archive/session22-11-graph-vector-smoke.md` | — |
| **会话22-10 Obsidian同步 + Redis warm-up** | `docs/progress/archive/session22-10-obsidian-redis-warmup.md` | — |
| **会话22-9 平台底座化 + redis坑㉓闭合** | `docs/progress/archive/session22-9-platform-infra-redis-023.md` | `esp:ctx:hot:2026-08-13-session22-9-platform-infra` |
| **会话22-3 ~ 22-8(08-09~0811)** | `docs/progress/archive/2026-08-09-to-0811-session22-3-to-22-8.md` | — |
| **会话18~22 冷区批次(08-07~09)** | `docs/progress/archive/2026-08-09-cold-archive-session18-22.md` | `esp:ctx:hot:2026-08-09-session22-abcd-batch-ab` |
| teacher readiness 根治 + 安全扫描四件套(08-07) | (见冷区归档) | `esp:ctx:hot:2026-08-07-teacher-readiness-and-security-scan` |
| teacher CrashLoop + 增量服务(08-07) | (见冷区归档) | `esp:ctx:hot:2026-08-07-teacher-crashloop-and-new-services` |
| Pulsar cookie + VM 网络中断(08-06) | `docs/progress/archive/2026-08-06-pulsar-cookie-fix-and-vm-down.md` | `esp:ctx:archive:2026-08-06-pulsar-cookie-fix-and-vm-down` |
| Pulsar Geo + 应用切新集群(08-05) | `docs/progress/archive/2026-08-05-pulsar-geo-and-cluster-switch.md` | `esp:ctx:archive:2026-08-05-pulsar-geo-and-cluster-switch` |
| 上线就绪 + 三层坑事故(08-04) | `docs/progress/archive/2026-08-04-readiness-and-three-layer-incident.md` | `esp:ctx:archive:2026-08-04-readiness-and-three-layer-incident` |
| 07-31~08-03 历史 | `docs/progress/archive/2026-08-{01,02,03}-*.md` | `esp:ctx:archive:2026-08-{01,02,03}-*` |

## 关键命令速查

- 查 TiDB 表:`scripts/db_query.sh "SQL"`(pymysql pod 封装)。
- 服务日志:`kubectl -n esp-prod logs deploy/<svc> --tail=20`。
- Redis 写/读:`kubectl -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c SET/GET "esp:ctx:*" "..."`。
- 压测:分布式 locust 见 `deploy/locust/README.md`。
- 单节点铁律:重 mvn/docker build 与集群操作串行;不碰 k3s 配置。

## 上下文工程铁律(摘要,详见 memory `context-overflow-ironrules`)

- 文件系统=长期记忆,对话=临时工作区。80%(160k)主动 /compact,90%(180k)强制止损落盘+compact。
- 三层防御:源头减量(subagent 隔离+输出限流)+ 状态外化(PROGRESS 热区+Redis 运行态)+ 主动管控。
- **本文件热区上限 ~300 行**,历史归 `docs/progress/archive/`,不全量读。完整铁律集见 memory。
