# ESP 全量审计报告(2026-08-21)

> 7 维度并行 Workflow 审计:24 微服务 + 6 共享包 + 3 前端 + 40 K8s yaml + 4 CI workflow + 完整设计文档。
> 704k tokens / 282 tool uses / 7 agent 并行 / 63 条结构化发现(14 P0 + 28 P1 + 21 P2)。
> 编排:[workflow-first-ironrule](../../.claude/projects/-Users-joelgong-claude-Projects-English-Study-Platform/memory/workflow-first-ironrule.md)(全量审计是 Workflow 典型场景)。

## 综合评分

| 维度 | 分数 | 大厂级评价 |
|---|---|---|
| 安全治理 | **8.5/10** | 较好。JWT RS256/IDOR 收敛/SCS 门禁齐全,仅纵深防御(llm-orchestrator 会话无鉴权、3 Go 服务纯信任头)有缺口 |
| 韧性可观测 | **6.0/10** | 中。gateway-go 样板(Sentinel+CB+降级),但 2 P0:LLM 裸调无超时 + 11 服务 Prometheus 抓取断链 |
| 架构设计 | **5.5/10** | 中下。骨架有大厂雏形,但 SAD 与实现严重矛盾(TiDB 叙事 vs 全量 CRDB),HTAP 学情分析路径丢失,自评 8.08 虚高 |
| 代码质量 | **5.5/10** | 中下。工程化意识强,但多处"编译过运行期崩"隐患:Python 限流信号量错乱、conversation-go 归档 schema 不一致、DefaultClient 旁路 |
| 数据一致性 | **5.5/10** | 中下。读写分离架构扎实,但 3 P0 事务原子性破洞:EventPublisher fire-and-forget 丢关键事件、content-go 非事务、vocab-srs read-modify-write 竞态 |
| 配置部署 | **4.5/10** | 差。RWP 回写好(learning/teacher/bff 已修),但 4 P0:argocd 管道断裂、TiDB 仓库漂移、孤儿 redis-cluster deploy、5 服务 readOnly 无 emptyDir |
| 测试覆盖 | **4.5/10** | 差。CI 骨架完整,但 2 P0:esp-common-rs + 3 Rust 服务零单测、L1 缓存层零测;07 大量 Mock 路径不能作上线证据 |

**加权总分:6.3/10**(等权平均 5.57,安全 8.5 拉高)。**大厂级及格线 7.0,差 0.7 分。距"可上线运营标准"有真实差距**,主要集中在**测试覆盖(4.5)和配置部署(4.5)**。

**核心结论**:ESP 工程化骨架(分层/共享包/契约/CI/ADR 体系/韧性接线)有大厂雏形且在同类项目中属中上,但存在**一处架构级致命失真**(SAD TiDB vs 实现 CRDB)与**多处运行期正确性隐患**(事务原子性/超时/竞态),叠加**测试债严重**(公共包近零单测)与**GitOps 管道断裂**。按"10亿/100万并发"基线,真实可水平扩展能力为零(单节点 5000 CCU 实测 533rps/7.3% 失败未达标),多区域/异地多活纯纸面。

---

## P0 清单(14 条,阻断上线,立即修)

### 架构设计 P0(2)

**P0-A1 SAD 与实现严重矛盾(数据面)**
- 服务:部署/SAD/ADR(全栈数据面)
- 位置:`docs/design/02-系统架构设计文档-SAD.md:1-419`(第2/4/6/13章 + ADR-03/10/16);`docs/design/cockroachdb-vs-tidb-multi-region-plan.md:1-355`;`deploy/k8s/05b-platform-infra-externalnames.yaml:15-30`
- 问题:SAD v5.0 通篇以 TiDB+TiFlash HTAP+TiCDC 异地多活为数据面叙事,但实际生产 100% 走 CRDB(crdb-public:26257,TiDB 已下电 scale=0)。且有 ADR 结论"不值得引入 CRDB"与实际全量切 CRDB 直接矛盾,无新 ADR 记录此次重大转向。文档反映的不是当前系统。
- 修复:SAD 第 2/3/4/6/13 章整体改写为 CockroachDB(3 节点 zone=z1/z2/z3,无 region 层,multi-region Phase1 DEFERRED),删 TiDB/TiFlash/TiCDC 表述降级为"历史演进/可回切保留"。新增 ADR-24"CRDB 作为主数据面(覆盖 ADR-10/16)",ADR-10/16 标 Superseded。读写分离章节改 CRDB follower-read(AS OF SYSTEM TIME)+ esp-rw-java CockroachDbDataSourceConfig 实情。架构评分按实情重评(TiDB HTAP 优势已不存在,应降分而非维持 8.0)。

**P0-A2 自评 8.08 分虚高(诚实度)**
- 服务:架构评分文档
- 位置:`docs/design/架构评分与重构方向.md:11-99`;bff-java src/test 实测 7 文件;backend/shared/* 实测
- 问题:(1)bff-java 自称"21 测 100% 覆盖"实测仅 7 测试文件;(2)公共包标"done/单测齐",实测 esp-common-go=3/esp-common-java=1/esp-common-rs=0/cache-py=0/db-py=0;(3)读写分离维 8.0 但 TiDB 已不存在,CRDB 下 follower lag 防读后写语义失效;(4)conversation-go 计入"24 微服务"但无独立 Deployment(是 gateway-go 内嵌库)。
- 修复:按实情重评。bff 测试以文件计数 7 为准标覆盖率;共享包用 jacoco/cargo tarpaulin/pytest-cov 实测落报告,未达 ≥80% 标 P0 未完成而非 done;数据面维度 TiDB HTAP 优势消失应降分(8.0→5.0)。诚实是工程铁律⑦。

### 数据一致性 P0(3)

**P0-D1 EventPublisher 丢关键事件(teacher.finalized)**
- 服务:teacher-java + esp-cache
- 问题:EventPublisher.publish 用 sendAsync + blockIfQueueFull(false),队列满静默丢消息,失败仅 warn 不抛;teacher.finalized 在 tryFinalize 的 @Transactional 内**提交前先发**。该事件驱动 learning-java 更新 user_mastery 评分,丢失致掌握度评分永久缺失且无补偿。
- 修复:改 transactional outbox——@Transactional 内先落本地 outbox 表(同库事务原子)由独立 relay 投 Pulsar;或用 @TransactionalEventListener(AFTER_COMMIT) 提交后发且失败入重试队列。EventPublisher 去 blockIfQueueFull(false) 改 blockIfQueueFull(true)+有界超时,失败抛出触发补偿。

**P0-D2 vocab-srs read-modify-write 竞态**
- 服务:vocab-srs-go
- 问题:Review 流程 GetUserVocab 从 follower 读 SRS 状态→内存 sm2.Compute→UpsertUserVocab 用 ON CONFLICT DO UPDATE SET 全字段覆盖(无 WHERE 版本/时间戳)。并发复习同一(user,word)→丢失 review_count/ease_factor/interval 增量;follower 读旧值算 SM-2 基线错误。无 version 列无乐观锁。
- 修复:user_vocabulary 增 version 列,Upsert 的 ON CONFLICT DO UPDATE 加 WHERE version=$old,旧版本冲突返 0 行重读重算;GetUserVocab 用 WriteDB(consistent)或 follower strong 读;Review 端乐观重试(检测 0 行更新则重读重算)。

**P0-D3 content-go 非事务写**
- 服务:content-go
- 问题:submitAnswer 的 content_attempts INSERT 与 learning_event_outbox INSERT 是两个独立 ExecContext(无显式事务)。两步间崩溃/outbox 写失败(仅 warn)→作答已记但 XP/掌握度事件丢失,无补偿路径。违反 outbox 原子性。
- 修复:content_attempts 与 outbox 插入包进同一显式事务(BeginTx→insert attempts→insert outbox→Commit),任一失败整体回滚;或 content_attempts 本身作 outbox 源(单表 CDC 投影)。

### 韧性可观测 P0(2)

**P0-R1 LLM 裸调无超时(conversation-go + llm-orchestrator-rs)**
- 位置:`conversation-go/src/llm/anthropic.go:87`(http.DefaultClient.Do 无 Timeout,文件顶部声明的带 60s+30s 超时的 httpClient 被弃用);`llm-orchestrator-rs/src/provider/mod.rs:59/69/79`(3 provider 全 Client::new() 无 timeout)
- 问题:LLM 推理 8-34s+长尾,上游卡死/socket hang 时无 transport 超时兜底,goroutine/线程长期挂起耗尽 worker。
- 修复:L1 conversation-go Chat() 改用已声明的带超时 httpClient(文件顶部已有 60s+30s ResponseHeaderTimeout 的 httpClient 变量却未用);L2 llm-orchestrator-rs 的 3 个 Client::new() 改用 cache/mod.rs http_client()(已含 EXT_CALL_TIMEOUT_SECS)或 builder().timeout(60s).connect_timeout(5s)。按 RWT 超时分级 LLM 30-60s。**两路均为 LCO 铁律链路,裸调无超时属红线**。

**P0-R2 11 服务 Prometheus 抓取断链**
- 服务:community-go/content-go/admin-go/llm-orchestrator-rs/vocab-srs-go/listening-go/reading-go/assessment-rs/pronunciation-rs/media-learn-rs/course-path-go/live-tutor-go
- 问题:Deployment pod template 缺 annotations.prometheus.io/scrape:"true"。Prometheus scrape_configs 要求该 annotation 才 keep,无则丢弃不抓取。代码层已暴露 /metrics,但部署层断链致实际未采集。韧性指标(rate_limit_total/circuit_breaker_state/llm_cache_hits)在这些服务零数据,故障盲区。对照 gateway-go/learning-java/teacher-java 有 annotation 可正常抓。
- 修复:给这些 Deployment pod template 加 annotations:{prometheus.io/scrape:"true", prometheus.io/port:"<metrics端口>", prometheus.io/path:"/metrics"}(gateway-go/learning-java 可对照模板)。改后回写仓库 yaml+commit 禁只 patch live(DGO)。CI 加门禁:有 /metrics 路由的服务 yaml 必含 scrape annotation 否则红。

### 代码质量 P0(1)

**P0-C1 Python 限流信号量错乱(writing-py)**
- 服务:writing-py
- 问题:release() 先对旧 self.semaphore 调 release,若此时进入扩容分支会 self.semaphore = asyncio.Semaphore(target) 整体替换。原信号量上的 acquire 等待者继续等旧对象,新 release 的配额归旧对象;后续 acquire 读新对象,计数错乱,可突破并发上限或 permit 计数变负 RuntimeWarning。注释"原有等待者不受影响"与实际相悖。
- 修复:改用 RWMutex 守护计数 + acquire/release 原子,或用不变 Semaphore + 挂额外 permit 计数实现扩容,禁止重建替换。

### 配置部署 P0(4)

**P0-K1 argocd GitOps 管道断裂**
- 服务:argocd/GitOps
- 问题:argocd app esp-platform 处于 OutOfSync + SyncError 持续状态,自愈被手动关闭(live selfHeal=false)。sync 失败:①redis-cluster-new-init Job 不可变(argocd 反复 patch 已完成 Job 报 immutable);②course-path-go probe 历史 tcpSocket+httpGet 多 handler。仓库→集群单向事实源管道断裂,任何 yaml 改动不自动生效全靠手动 kubectl apply——正是 DGO 第③条要消灭的漂移温床(Redis 8.0 脑裂事故就是此管道失效期间无人发现)。
- 修复:①修 course-path-go/yaml probe 去重复 handler(确认当前 yaml 已 httpGet 单 handler,可能是旧 sync 残留→手动 sync 并确认 Synced);②给 redis-cluster-new-init Job 加 argocd.argoproj.io/hook: Skip 或 Sync hook 注解;③修完 live selfHeal 回写 true 与仓库对齐(DGO),不得关闭 selfHeal 掩盖 sync 失败。

**P0-K2 TiDB 仓库 yaml 漂移**
- 位置:`deploy/k8s/07-tidb-tikv.yaml`
- 问题:TiDB 3 区域下电后 live 9 deploy 全 scale=0,但仓库 9 处仍 replicas:1。下次 argocd sync 一旦 selfHeal 恢复或有人 kubectl apply -f,会试图重启全部 TiDB/PD/TiKV pod,pingcap 镜像源 403(mirror 挂)→ ImagePullBackOff 雪崩+抢 VM 资源。仓库与 live 漂移无人发现(DGO 第③条要求 CI kubectl diff 检测漂移,当前缺失)。
- 修复:立即回写 07-tidb-tikv.yaml 9 处 replicas:1→0(或文件头加注"已下电 DEFERRED,多节点恢复时改回")+commit(DGO);CI 加 argocd app diff/kubectl diff 非零即漂移红门禁。

**P0-K3 孤儿 redis-cluster deploy 声明**
- 位置:`deploy/k8s/06b-redis-pvc.yaml`
- 问题:仍声明 redis-cluster Deployment(replicas:1, image redis:8.0-alpine, probe timeoutSeconds:1——已修过的 CRDB probe 误杀坑模式)+ redis-cluster-data PVC,但该 deploy live 已删(NotFound),真集群是 06c 的 redis-cluster-new STS。下次 apply 会重建与真集群同短名混淆的 standalone Redis(曾致 esp:ctx: 锚点读取无输出事故)。孤儿子资源定时炸弹。
- 修复:删 redis-cluster Deployment + redis-cluster-data PVC 声明(或整文件移除/归档),仅保留 redis-cluster-new STS + 其 volumeClaimTemplates;CI 加"同 ns 无重名 deploy/sts 冲突"检查。

**P0-K4 5 服务 readOnly 无 emptyDir(RWP 遗漏)**
- 位置:`deploy/k8s/31-new-services.yaml`(course-path-go 无 volumeMounts)+ `32-batch2-services.yaml`(media-learn-rs/assessment-rs/gamification-go 无)+ `34-asr-stack.yaml`
- 问题:这些服务开 readOnlyRootFilesystem:true 但零 emptyDir 挂载,而同构服务(reading-go/listening-go/gamification-go/community-go/content-go/gateway-go/live-tutor-go/learning-worker)已全挂 /app/log+/tmp。esp-common-go/middleware/sentinel.go:63 明确"LogDir 不可写(readOnly)→改 /tmp(必挂 emptyDir)"。course-path-go 调 SetRateLimitService 初始化 Sentinel→写 /tmp 失败→init 崩;Rust 服务经 nacos-register.sh+OTel/gRPC 可能写 /tmp。RWP 第⑤条直接违反,潜在 CrashLoop。
- 修复:统一回写挂载(对标 reading-go/gateway-go 已修模式):readOnlyRootFilesystem:true 必挂 {app-log→/app/log}+{tmp→/tmp} emptyDir + pod fsGroup:1000。RWP 第⑤条"全局同步修禁只修出问题服务"——当前正是不一致状态。

### 测试覆盖 P0(2)

**P0-T1 esp-common-rs 零单测**
- 服务:esp-common-rs(共享包 db.rs/redisx.rs/health.rs/metrics.rs/tracing_init.rs 等 7 源文件)+ 4 Rust 服务(assessment-rs/media-learn-rs/pronunciation-rs 仅 llm-orchestrator-rs 有 1)
- 问题:零单测,CI rust-services job 对共享包无测试门禁(cargo test 空跑通过)。违反工程铁律⑥(CI 门禁覆盖率≥80%)与契约测试。
- 修复:esp-common-rs 每模块补 #[cfg(test)] 单测(db 连接池/重连、redisx cluster 路由、health/metrics 边界);CI rust-services job 加 cargo test -p esp-common-rs 且零测试即红(禁空跑);目标行覆盖≥70%。

**P0-T2 L1 缓存层零单测(LCO 铁律起因区)**
- 服务:llm-cache-proxy(L1 精确缓存 l1pool.go)
- 问题:LCO 三层缓存第一层无独立单测(l2/l3/manager/protocol/server 均有 test,唯独 l1 缺失)。**正是 CLC 铁律起因区**——Lookup 零值 Manager 解引用 nil cfg panic,manager_test.go 仅用 &Manager{} 测禁用路径未追踪真实执行路径。L1 是命中率与成本节省主力,零测无证据保障。
- 修复:补 l1pool_test.go(命中率统计/过期清理/容量驱逐/nil cfg 防御);CI llm-cache-proxy-ci.yml 加覆盖率门禁(go tool cover -func≥70%,当前只落盘 coverage.out 无 gate);静态门禁保留 nilness/nilerr。

---

## P1 清单(28 条,重要,重构阶段修)

### 数据一致性 P1(5)
1. **gamification-go pollLoop hwm 无条件推进**:userCache 未命中或 processedMark 已处理时事件被跳过但 hwm 仍推进→这些事件计分永久丢失。修复:仅整批成功 fanout 后才推进 hwm,未命中事件标记待处理不推进。
2. **learning-java LearningEventConsumer 乐观锁裸重试**:handleFinalized findById 无显式 @DataSource 标注依赖隐式默认,OptimisticLock 失败 catch 后 negativeAcknowledge 裸重试无退避,可能无限重试过期版本致掌握度更新卡死。修复:findById 标 @DataSource(READ,consistent=true) 走主库强一致读;乐观锁失败 exponential-backoff 重试,达上限转死信。
3. **teacher-java Submission 无 @Version**:compensate 读回 Submission→rollbackStatus→save 回滚,并发 finalize/重复补偿可覆盖 lost update。修复:Submission 增 @Version 列,compensate/confirm 走乐观锁防并发。
4. **learning-java handleRegistered 全表扫描**:`SELECT id FROM knowledge_points` 无 WHERE/LIMIT 全表加载,million 级 OOM/长暂停。修复:分页或启动时内存缓存(新增 kp 失效),或 cursor 分批,或按需初始化不一次性铺全量。属 esp-jpa-findall-oom-trap 同类。
5. **schema message_id 幂等列未入主 DDL**:learning_events/learning_event_outbox 的 message_id + UNIQUE INDEX 仅迁移脚本声明,schema.sql/crdb-schema-aux-services.sql 未含。新建集群 apply 主 schema 后无 message_id→OutboxRelay 的 ON CONFLICT (message_id) 报错或退化,幂等失效。修复:message_id 列+UNIQUE INDEX 正式写入主 DDL,CI 校验已部署 schema 含该索引。

### 架构设计 P1(5)
6. **多区域一致性纯纸面**:CRDB 3 节点 locality 仅 zone 无 region,全在单台 RD VM,primary_region=NULL。SAD ADR-16 却写"TiCDC 异步同步实现异地多活"——从未落地且 TiDB 已下电。读写分离在 CRDB 单端点退化 follower-read,与 SAD RoutingDataSource 双池语义不符。修复:阶段1 改 StatefulSet --locality 为 region+zone 回写 yaml;阶段2 SET PRIMARY REGION/ADD REGION/SURVIVE;阶段3 RBR 表加 region 列。落地前补 follower-read vs strong-read 一致性边界用例。回写 SAD 第4章。
7. **HTAP 学情分析路径丢失**:learning_events 30亿行聚合原靠 TiFlash 库内列存 MPP(结构性优势),切 CRDB 后丢失,CRDB Pebble LSM 行存只能外推数仓。SDD/ADR 未记录替代方案(ClickHouse?物化视图?离线批?),学情分析 OLAP 路径架构空白。修复:新增 ADR-25"数据面从 TiDB 迁移至 CRDB 决策复盘",记录迁移动因、覆盖 ADR-03/16、明确 HTAP 补偿方案(ClickHouse/物化视图+并行扫描)、回切条件。补 schema 迁移点(AUTO_RANDOM→unique_rowid/ON UPDATE→触发器)技术债登记。
8. **Redis 直连 Pod FQDN 反模式**:course-path-go/live-tutor-go/gamification-go/pronunciation-rs/assessment-rs 的 REDIS_CLUSTER 直指 redis-cluster-new-0.redis-cluster-new.ekb-...(跨 ns 单 Pod FQDN),而 gateway-go/learning/teacher 走 redis-cluster ExternalName。直连 Pod FQDN 绕过 Service 负载均衡,Pod 重建连接断,单 Pod 无法享 Cluster MOVED 重定向。修复:统一切 redis-cluster ExternalName(客户端 cluster 模式自动 MOVED),删直连 Pod FQDN。CI 加 lint 禁止 env 出现 redis-cluster-new-N. 形态。
9. **RWP 遗漏(与 P0-K4 重叠)**:course-path-go/media-learn-rs/assessment-rs/gamification-go readOnly 无 volumeMounts。修复:全局同步修补 /app/log+/tmp emptyDir(RWP 第⑤条)。CI 加 lint readOnly 服务必含 volumeMounts。
10. **HPA/容量差距**:HPA maxReplicas 普遍 3-6,minReplicas 全 1。对标 10亿/100万并发基线悬殊:5000 CCU 单节点实测 533rps/P95 25s/失败率 7.3% 未达标。AI 长尾(writing-py max latency 112s)在 maxReplicas=3 下 HPA 扩容跟不上。修复:多节点阶段分布式 Locust 重测 5000 CCU,按真实 SLO(G-04 写作 P95<2s 实测73s、G-06 学情5000ev/s 实测277ev/s)设 HPA 目标值与 maxReplicas 容量规划;多节点恢复 min≥2 并补 PDB minAvailable。

### 韧性可观测 P1(4)
11. **bff-java WebClient 聚合无超时**:WebClientConfig.java:15 WebClient.builder().build() 未配 connectTimeout/responseTimeout。用于 RankingFetcher/LearningFetcher/ProxyController 聚合下游 REST,下游慢响应卡死时 BFF 聚合请求永久挂起占满 Netty event loop。修复:WebClientConfig 配 responseTimeout(5s).connectTimeout(2s),或按下游分级(gateway LLM 90s/普通 5s);RankingFetcher/LearningFetcher/ProxyController 加 .timeout(Duration) per-request。
12. **gateway-go readyz 无真实依赖检查**:main.go:131 readyz 无条件返 200,但 gateway 强依赖 Redis。Redis 脑裂/host-middleware 未起时 pod 仍报 Ready 接流量,WS/LLM 缓存全 5xx 误报为应用故障。修复:readyz 加 Redis PING(2s 超时)失败返 503(区分 livez);readyz 失败不致 liveness 重启(已分离)。参照 admin-go probeAll 做轻量依赖探活。
13. **LCO 缓存命中率元数据透传断链**:gateway-go handleLLMChat 硬编码 cache_hit:false/cache_layer:""(注释承认 CachedProvider 不透传 cache 元数据)。LCO 铁律要求响应带 cache_hit+cache_layer,命中率>30%。gateway 直连 LLM 时 CachedProvider 命中层未暴露。修复:CachedProvider 加 LastLayer()/LastHit() 方法,handleLLMChat 取值透出;命中率 PromQL 上 admin-Grafana dashboard >30% 告警。
14. **熔断粒度粗 + Dubbo 下游无熔断**:Go 服务 HTTPCircuitBreaker 是全服务单实例(熔断时整服务返 503 非 per-dependency 舱壁);learning-java/teacher-java Resilience4j 仅配 dbSlowCall,对 Dubbo Triple 下游 RPC 未配熔断;BFF Sentinel 仅 flow 无 degrade。修复:Java 侧用 esp-common-java Resilience4j 注解包裹 Dubbo 调用,或 Dubbo consumer 配 timeouts+retries+sentinel degrade-rule;Go 侧演进 per-依赖 CB。

### 代码质量 P1(5)
15. **conversation-go 归档 schema 不一致**:End 异步归档用 INSERT INTO sessions(未显式 schema,依赖默认 search_path=esp_speaking),ListHistory 读 esp_speaking.sessions(显式)。表名引用不一致,默认 schema 变更时归档写错库,历史丢失无报错。修复:统一显式 schema INSERT INTO esp_speaking.sessions,schema.sql 落盘,CI 比对。
16. **llm-orchestrator-rs IDOR**:llm_chat/dialogue list_sessions/create_session/chat_sse/list_turns 均从 request body 取 user_id 无 JWT/X-User-Id 校验。调用方可任意指定 user_id 读写他人对话会话、记他人 llm_call_logs。修复:网关 EdgeAuthHandler 注入的 X-User-Id 作权威 uid,body user_id 仅日志兜底,二者不一致返 403;dialogue 必按 X-User-Id 过滤。
17. **conversation-go Anthropic 调用绕过自建 httpClient**(与 P0-R1 重叠):anthropic.go:87 用 http.DefaultClient.Do 绕过文件顶部声明带超时的 httpClient。修复:改用已声明的带超时 httpClient。
18. **community-go 邀请码有模偏差**:邀请码生成存在模偏差。修复:改用加密随机或无偏差算法。
19. **限流清理/缓存重连/降级路径加固**:多处并发安全与错误边界需加固。修复:逐项补防御。

### 测试覆盖 P1(4)
20. **admin-go/listening-go/reading-go/live-tutor-go 零测试**:关键治理服务(admin-go Phase1 刚接限流熔断)零测无法证明接线有效。CI go-coverage 对无测试服务因 bc 不可用或判空跳过。修复:admin-go 关键链路补单测+集成测试;CI go-extra 对零测试服务加最小冒烟或显式 DEFERRED;admin-go 有 go.mod 故要求≥70%。
21. **契约测试缺口**:T1/T2/T3/T4 四 IT 类覆盖 Dubbo/读写分离/多级缓存/Disruptor,但无独立 Pulsar producer/consumer 投递 IT、无 outbox 中转真实链路 IT(Pulsar consumer 在 T3 被 @Disabled)。02 报告日期 2026-07-31,近 3 周无真实 IT 执行证据。契约测试仅覆盖 userStats 单方法,weakCount 未覆盖。修复:补 Pulsar outbox 中转投递 IT(真实 broker);learning-java 补 PulsarTemplate 真投递 IT;恢复 @Disabled 的 T3 consumer;CI 增 Pulsar 接线验证 job。
22. **覆盖率回填**:01 报告覆盖率表格写"执行后填实测"占位,仅 go 基线(dialog 90.3%/llm 21.7%),Java/Python 各服务空白。conversation-go llm 包 21.7% 远低 80% 目标(LCO 核心缓存层却低覆盖)。SLO G-09(核心单测≥80%)无证据。修复:按 docs/test/00 表格补齐各服务实测覆盖率行;conversation-go llm 提测真实执行路径(mock provider 边界+真实 cache hit/miss);CI 落盘 coverage 回填 01。
23. **MOCK/REAL 标注不彻底**:07 人工测试大量默认 ASR/TTS/LLM Mock,大量 TC 项 PASS 基于 Mock 路径(image_url=placehold、作文批改 Mock 出分)。按 MPB 铁律 Mock 通过不得作上线证据。修复:07 每项用例强制标 MOCK/REAL 列;生产验收前跑全 REAL(配真实 key)出基线;mock 通过项补 REAL 重测。

### 配置部署 P1(3)
24-26:(P0-K2/K3/K4 的 P1 侧见上,已作为 P0 处理)。
27. **死配置 MySQL/TiDB env 保留**:teacher-java/bff-java 同时配 ESP_RW_COCKROACHDB_*(生效)和 WRITE_DB_URL=jdbc:mysql://tidb-primary(注释"可回切")。STORAGE_TYPE=cockroachdb 时 MySQL URL 被忽略,但保留死配置增加认知负担与误配风险(有人改 STORAGE_TYPE=tidb 即连已下电 TiDB CrashLoop)。05b 仍保留 tidb/tidf-follower ExternalName 指向下电集群。修复:若 ADR-24 收敛 CRDB 为唯一主数据面,删各 yaml 的 WRITE_DB_URL/tidb env 与 ExternalName,清 07-tidb-tikv.yaml(降认知负担与漂移风险,配而未接违反工程铁律②)。
28. **SAD 服务划分章节过期**:第3章仅列 8 服务,实际 24 个 backend/services 子目录、43 个 Deployment。16 个新服务(admin-go/content-go/community-go/gamification-go/vocab-srs-go/course-path-go/live-tutor-go/assessment-rs/pronunciation-rs/listening-go/reading-go/media-learn-rs/llm-orchestrator-rs/asr-adapter-go/tts-adapter-go/writing-coach)在 SAD 完全缺席。SAD 作为架构 Single Source of Truth 严重过期。修复:第3章服务表扩列到实际 24 个(区分"部署服务"vs"领域库"),明确 conversation-go 为 gateway-go 内嵌会话领域库(非独立服务)。

### 安全治理 P1(2)
29. **llm-orchestrator-rs 对话会话无鉴权**(与 P1-16 重叠):仅内部可达但属设计缺陷。见 P1-16 修复。
30. **三 Go 服务纯信任 BFF 注入头无自身 JWT 校验**:course-path-go 等。修复:纵深防御,关键写操作服务自身校验 JWT 或至少校 X-User-Id 存在。(硬编码 JWT_SECRET 死代码 + DB NodePort 暴露 TiDB root 无密码为 P2,见下)

---

## P2 摘要(21 条,建议/加固)

代表性:
- conversation-go:LLM 调用并发限制、缓存重连退避(3 条命中)
- llm-orchestrator-rs:多端点鉴权细化、对话会话归属校验、llm_call_logs 清理(3 条)
- gamification-go:pollLoop 退避、opt-out 缓存失效(2 条)
- esp-common-go:debug.go getDebugLogLevel unused lint、middleware 一致性(2 条)
- 安全:硬编码 JWT_SECRET(死代码)、DB NodePort 暴露(TiDB root 无密码)、三 Go 服务纯信任头(4 条)
- 部署:teacher-java/bff-java 读写分离可回切 env 死配置、SAD 服务划分过期、LCO 工具路径漂移(DGO)(3 条)
- 前端:i18n 占位符对齐、chunk 体积优化(recharts 420KB manualChunks)(2 条)
- LCO:embedding 服务无限流 + MPS 单点、缓存命中率元数据透传断链(P1-13 已记)(2 条)

---

## 问题密集服务 Top(按多维度命中)

| 服务 | 命中维度数 | 主要问题 |
|---|---|---|
| llm-orchestrator-rs | 3 | LLM 裸调无超时(P0)+ IDOR(P1)+ Prometheus 抓取断链(P0) |
| conversation-go | 3 | LLM 裸调无超时(P0)+ 归档 schema 不一致(P1)+ Anthropic 绕 httpClient(P0)+ LCO 元数据断链(P1) |
| gamification-go | 2 | Redis 直连 Pod FQDN(P1)+ RWP 遗漏(P0)+ pollLoop hwm(P1) |
| teacher-java | 3 | EventPublisher 丢事件(P0)+ Submission 无 @Version(P1)+ 死配置 MySQL env(P2) |
| course-path-go | 2 | RWP 遗漏(P0)+ Redis 直连 Pod FQDN(P1)+ Prometheus 抓取断链(P0) |
| content-go | 2 | 非事务写(P0)+ Redis 直连(P1)+ Prometheus 抓取断链(P0) |
| vocab-srs-go | 1 | read-modify-write 竞态(P0)+ Prometheus 抓取断链(P0) |
| writing-py | 1 | 限流信号量错乱(P0) |
| learning-java | 2 | 乐观锁裸重试(P1)+ handleRegistered 全表扫描(P1)+ schema message_id(P1) |
| esp-common-rs | 1 | 零单测(P0) |
| bff-java | 2 | WebClient 无超时(P1)+ GraphQL(本次新增)+ 死配置 env(P2) |
| argocd/GitOps | 1 | 管道断裂(P0)+ TiDB 仓库漂移(P0)+ 孤儿 redis deploy(P0) |

---

## 优化重构方向总览

### P0 立即修(14 条,本阶段执行,阻断上线)

**按可立即执行分组(无 L2/L3 风险,主线程串行做):**

1. **配置部署快速修(5 条,改 yaml 回写 commit)**:P0-K1 argocd 修 sync + selfHeal 回写 / P0-K2 TiDB replicas 1→0 回写 / P0-K3 删孤儿 redis-cluster deploy / P0-K4 补 5 服务 emptyDir / P0-R2 补 11 服务 prometheus annotation。**全 L1 可逆,改 yaml + apply + commit,DGO 回写**。

2. **LLM 超时红线(1 条,改 2 文件)**:P0-R1 conversation-go 用已声明 httpClient + llm-orchestrator-rs 用 http_client()。RWT 铁律红线。

3. **数据一致性事务原子性(3 条,需改代码+测试)**:P0-D1 EventPublisher outbox / P0-D2 vocab-srs 加 @Version / P0-D3 content-go 包事务。需契约测试证明。

4. **代码正确性(1 条)**:P0-C1 writing-py 限流信号量改 RWMutex。

5. **测试补 P0(2 条,长线)**:P0-T1 esp-common-rs 单测 / P0-T2 l1pool 单测。**这两条工作量大,Workflow 适合并行补**(每模块一 agent)。

6. **架构文档诚实(2 条)**:P0-A1 SAD 改写 CRDB / P0-A2 自评降分。文档工作,esp-doc agent 可并行。

### P1 出方案(28 条,重构阶段,压测前/多节点阶段)

- 数据一致性 5 条(乐观锁/全表扫描/schema DDL)
- 架构 5 条(多区域落地/HTAP 替代方案/Redis FQDN/HPA 容量)
- 韧性 4 条(WebClient 超时/readyz/CB 粒度/LCO 元数据)
- 代码 5 条(schema 一致/IDOR/加固)
- 测试 4 条(覆盖率回填/契约 IT/MOCK 标注)
- 配置 3 条(死配置清理/SAD 服务表)
- 安全 2 条(纵深防御)

### P2 加固(21 条,持续)

---

## 审计方法论与铁律对照

- **编排**:[workflow-first-ironrule](../../.claude/projects/-Users-joelgong-claude-Projects-English-Study-Platform/memory/workflow-first-ironrule.md) — 7 维度并行 Workflow,barrier 等齐汇总,schema 强制结构化输出。
- **核实**:[code-level-correctness-ironrule] 主线程对 P0/P1 逐条 WTV 核实(SAD TiDB vs CRDB、公共包零单测、Redis Pod FQDN、RWP 遗漏 全部属实)。
- **诚实**:工程铁律⑦——评分以可核验事实为准,不美化虚高项。
- **发现统计**:63 条(14 P0 + 28 P1 + 21 P2),7 维度全覆盖,0 agent 错误,0 空结果。

## 关联

- Workflow 脚本:`.claude/projects/-Users-joelgong-claude-Projects-English-Study-Platform/workflows/scripts/esp-full-audit-wf_5f3dfb94-3d4.js`
- Journal(7 agent 原始返回):`subagents/workflows/wf_5f3dfb94-3d4/journal.jsonl`
- memory:[[workflow-first-ironrule]]、[[engineering-ironrules]]、[[code-level-correctness-ironrule]]、[[declarative-gitops-ironrule]]、[[readonly-fs-writable-paths-ironrule]]、[[resilience-wiring-ironrule]]、[[llm-cache-optimization-ironrule]]、[[esp-jpa-findall-oom-trap]]、[[esp-shared-infra-ironrule]]
- `docs/PROGRESS.md` 会话23-27
