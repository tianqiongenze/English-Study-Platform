# G2 — ADR 索引与编号规范(独立副本)

> TOGAF ADM Phase G(实施治理)P0 补救产出物。
> 本文件是 ADR 索引的独立 Single Source of Truth(与 `architecture-board-charter.md` §G2 内容一致,单独成文便于检索)。
> 日期:2026-08-13。
> 更新:2026-08-17 追加 Phase 1-3 相关 ADR + ADR-19~22(Nacos/BFF 动态路由/限流熔断);2026-08-20 追加 ADR-23(llm-cache-proxy 三层语义缓存代理);2026-08-21 追加 ADR-24(CRDB 作为主数据面,Supersede ADR-10/16)。

## 编号规则

- 格式 `ADR-XX`(两位数,01 起,永久不回收)。
- 文件命名 `docs/design/adr-<kebab-topic>.md`。
- 状态:Proposed / Accepted / Superseded / Deprecated。
- Superseded 必注明替代编号,保留原文不删。

## ADR 索引(24 份)

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
| ADR-16 | content-go 服务选型 | (待补) | Accepted | Go 微服务+CRDB esp_content schema(5表:scenarios/courses/lessons/content_items/content_attempts);Transactional Outbox 投递学情事件;content_items.version+is_current 版本快照;REST API 课程策展+作答 | Phase 1 |
| ADR-17 | community-go 服务选型 | (待补) | Accepted | Go 微服务+CRDB esp_community schema(5表:circles/circle_memberships/circle_posts/circle_post_comments/content_reports);circle_memberships 复合主键实现 N:M;content_reports 多类型举报(post/comment/circle);举报阈值自动隐藏机制;REST API 圈子管理+UGC | Phase 2 |
| ADR-18 | rankings CQRS 选型 | (待补) | Accepted | gamification-go rankings 采用 CQRS:热数据 Redis ZSET(实时排行,查询 P95<100ms);冷数据 CRDB leaderboard_snapshots(日切快照,percentile 支持);ranking_opt_out 写端拦截(opt-out 不入榜);scope_type+scope_id+period 三维排行维度;日切 cron 定时快照落库 | Phase 3 |
| ADR-19 | Nacos 统一服务注册 | (本索引) | Accepted | 所有 19 个服务(Java/Go/Python/Rust)统一注册到 ekb-m2 共享 Nacos 实例;esp-prod 原 nacos Deployment 已删除,通过 ExternalName 桥接 `nacos.ekb-m2-aws-us-east-1`→共享 Nacos;消除 esp-prod 冗余 Nacos,减少 ~512MB 内存占用;Java 服务(nacos-discovery-config-starter)/Go(nacos-sdk-go)/Python(nacos-sdk-python)/Rust(nacos-rs) 各语言 SDK 对接;ephemeral 实例注册/注销由 SDK 自动管理 | 会话24 |
| ADR-20 | BFF RouteTable 动态化 | (本索引) | Accepted | bff-java RouteTable 从 Nacos 读取路由配置(dataId=esp-bff-routes,group=ESP_ROUTES);本地 DEFAULT_ROUTES 作为 fallback(Nacos 不可达时降级);volatile 保证内存可见性 + Nacos Listener 实现路由自动刷新(零重启生效);解决静态路由表新增端点需重启 BFF 的痛点 | 会话24 |
| ADR-21 | Go 共享限流熔断中间件 | (本索引) | Accepted | esp-common-go/middleware 提供统一 TokenBucket(令牌桶限流)+CircuitBreaker(熔断器)中间件;所有 Go 服务(gateway-go/content-go/community-go/gamification-go/listening-go/reading-go/vocab-srs-go/course-path-go)统一接入;默认参数:令牌桶 rate=100/s burst=200,熔断 threshold=50% error/10s window;Nacos 动态配置参数(无需重启);与 Java Sentinel 对齐语义(状态机/阈值/恢复策略) | 会话24 |
| ADR-22 | 多语言限流熔断策略 | (本索引) | Accepted | Java=Sentinel+Nacos( Dashboard 可视化+规则持久化);Go=esp-common-go 自研令牌桶+熔断器+Nacos 动态配置;Python=nacos-sdk-python+内存令牌桶(emotion-svc/writing-py);Rust=内存令牌桶+健康检查(assessment-rs/pronunciation-rs);四语言统一语义:限流(令牌桶)/熔断(半开-关闭-打开状态机)/降级(fallback 响应);统一 dashboard:Java Sentinel Dashboard 展示全 Java 链路,Go/Python/Rust 指标通过 OTel 推送到 Sentinel 或独立 Prometheus | 会话24 |
| ADR-23 | llm-cache-proxy 三层语义缓存代理 | adr-llm-cache-proxy.md | Accepted | 独立宿主 Go 代理(:15720)横截本机编码工具 LLM 调用,实施 LCO 铁律三层缓存:L1 Redis Cluster(SHA256 精确哈希)+ L2 Milvus(2560 维 Qwen 向量 cosine,双 collection 独立 lco_prompts 2560 vs esp_llm_semantic 384)+ L3 Neo4j(Prompt→Response 知识图谱,非阻断增强);调用方各自声明 embedding model 解 FORCE_MODEL 冲突(请求 model 优先 > 服务端 FORCE_MODEL > 语言路由);Neo4j 下沉宿主不进集群;全链路降级不阻断主链(RWT 铁律) | 2026-08-20 |
| ADR-24 | CockroachDB 作为主数据面 | architecture/adr-24-crdb-as-primary-data-plane.md | Accepted | ESP 主数据面由 TiDB+TiFlash HTAP 切为 CockroachDB(crdb-public:26257,ESP_RW_STORAGE_TYPE=cockroachdb);读写分离走 CRDB follower-read(AS OF SYSTEM TIME)+ esp-rw-java CockroachDbDataSourceConfig(双 HikariCP 池 + RoutingDataSource);TiDB 3区域 2026-08-20 下电(07-tidb-tikv.yaml replicas=1 实底层不可用,05b ExternalName 指下电集群保留可回切);迁移动因为 mirror pingcap 镜像源 403+CRDB 单二进制贴合 5000 CCU 尺度;代价=HTAP 学情分析路径丢失(learning_events 30亿行聚合改外推数仓 ADR-25 DEFERRED);回切条件=多节点+mirror 修好+schema 兼容重验(AUTO_RANDOM↔unique_rowid/ON UPDATE↔触发器);**Supersede SAD 第5章内联 ADR-03/10/16**(SAD 本地编号,非本索引 ADR-10/16);关联ADR-11 | 2026-08-21 |

## 维护

- 新 ADR 接续编号(下一个 ADR-25),本表追加。
- 状态变更原地更新,不删行。
- 与 `docs/design/adr-*.md` 双向校验,本索引为 SSoT。
- **ADR 编号双命名空间注意(2026-08-21)**:本索引 ADR 编号(ADR-01~24)与 SAD 第 5 章内联 ADR(ADR-01~18)是**两套独立编号**,ADR-24 Supersede 的是 SAD 第 5 章内联 ADR-03/10/16(TiDB 相关),**非**本索引中的 ADR-10(writing-coach)/ADR-16(content-go)。引用 ADR 时须注明命名空间(本索引 vs SAD 第 5 章)。
