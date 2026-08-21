# English-Study-Platform — 系统架构设计文档 (SAD)

| 项 | 值 |
|---|---|
| 文档编号 | ESP-SAD-001 |
| 版本 | 5.1 |
| 日期 | 2026-08-21 |
| 状态 | 数据面叙事修正稿(P0-A1 审计修复:回写 CockroachDB 实情) |
| 编制 | 高校 AI 与大数据专业教授级高工 |
| 上游 | ESP-SRS-001 需求分析文档 |

> **修订说明(v5.1,2026-08-21)**:本版据 P0-A1 架构审计修复数据面叙事。原 v5.0(2026-07-31)通篇以 **TiDB+TiFlash HTAP+TiCDC 异地多活**为数据面叙事,但实际生产自 #140 全服务切 CockroachDB 起已 **100% 走 CockroachDB**(`ESP_RW_STORAGE_TYPE=cockroachdb`,连接 `crdb-public:26257`;见 `platform/esp-rw-java/.../CockroachDbDataSourceConfig.java` 与各服务 deploy env)。TiDB 3区域已于 2026-08-20 下电止血(memory `esp-tidb-decommissioned-scale0-2026-08-20`),`tidb-primary`/`tidb-follower` ExternalName 仍指向已下电集群(`deploy/k8s/05b-platform-infra-externalnames.yaml:13-30`)。本次据实修正第 2/3/4/6/13 章数据面表述,TiDB 相关内容降级为"历史演进/可回切保留"段(注明回切条件)。关联 **ADR-24(新建,CRDB 作为主数据面)**、ADR-10/16(Superseded)。工程铁律⑦诚实优先:本版不再以 TiDB HTAP 为数据面叙事。

---

## 第 1 章 架构目标与原则

### 1.1 架构目标
支撑 5000 CCU,5 大功能模块,高并发、高可用、可水平扩展、可观测、可快速迭代。核心 SLO:口语引导 P95<800ms、学情事件入口 ≥5000 ev/s、可用性 ≥99.9%。

### 1.2 设计原则
1. **领域驱动 + 微服务**:按业务能力划分服务边界。
2. **分层 + 读写分离**:接入 / 编排 / 领域 / 基础设施分层;数据面读写分离。
3. **事件驱动**:高频学情/写作/情绪事件走 Pulsar + Disruptor 异步化,削峰填谷。
4. **无状态服务**:会话外置 Redis Cluster,服务可任意扩缩。
5. **最优多语言**:实时面 Go、业务面 Java(Disruptor)、AI 面 Python。
6. **契约先行**:OpenAPI 3 + WS 帧契约,前后端 / 服务间解耦。
7. **可观测优先**:日志 + 指标 + 追踪三位一体。
8. **安全默认**:JWT RBAC、Secret 管理、TLS。

---

## 第 2 章 总体架构

### 2.1 架构总览图
```
                         ┌──────────────────────────────────────────┐
   Learner / Teacher ───►│   React Frontend (Vite + TS + SWR)       │
                         └────────────────────┬─────────────────────┘
                                              │ HTTPS / WSS
                         ┌────────────────────▼─────────────────────┐
                         │   Ingress (K8s Traefik + cert-mgr)        │
                         └──────┬──────────────────────────┬────────┘
                                │ WSS(长连接)              │ HTTPS
                  ┌─────────────▼──────────┐    ┌──────────▼────────────┐
                  │ gateway-go (实时面)     │    │ BFF (Spring Cloud GW) │
                  │ WebSocket / 音视频流    │    │ 聚合 REST,JWT 校验    │
                  │ 口语会话编排 + Mock ASR │    └──────────┬────────────┘
                  └────────┬───────────────┘               │
                           │ Redis(会话状态/跨 Pod Pub/Sub)  │
                           │ Pulsar speech.session          │
   ┌───────────────────────┼───────────────────────────────┼──────────────────┐
   │                       ▼                                ▼                  │
   │ ┌───────────────────────────────┐   ┌─────────────────────────────────┐   │
   │ │ learning-java (业务面)         │   │ teacher-java (业务面)            │   │
   │ │ Spring Boot 3 + Disruptor     │   │ Spring Boot 3                   │   │
   │ │ 知识图谱/复习/动态难度          │   │ 教案/学情分析/作业评阅           │   │
   │ │ (读副本 / 写主库)              │   │ (读副本 / 写主库)               │   │
   │ └─────────┬─────────────────────┘   └──────────────┬──────────────────┘   │
   │           │ Disruptor RingBuffer                     │                      │
   │           ▼                                          ▼                      │
   │ ┌──────────────────────────────────────────────────────────────────────┐  │
   │ │            Apache Pulsar(事件总线,计算存储分离)                       │  │
   │ │ topics: learning.event / writing.task / emotion.signal / teacher.task │  │
   │ └─────┬───────────────────────┬──────────────────────┬─────────────────┘  │
   │       ▼                       ▼                      ▼                    │
   │ ┌─────────────┐     ┌─────────────────┐    ┌──────────────────┐          │
   │ │ writing-py  │     │ emotion-svc     │    │ learning-worker  │          │
   │ │ FastAPI     │     │ (Python)        │    │ (Java consumer)  │          │
   │ │ Claude LLM  │     │ 情绪+视觉辅助    │    │ 事件落库/算法触发 │          │
   │ └─────────────┘     └─────────────────┘    └────────┬─────────┘          │
   │                                                     │                    │
   │ ┌───────────────────────────────────────────────────▼──────────────────┐ │
   │ │                 数据面(读写分离,实际 CockroachDB)                      │ │
   │ │  ┌─────────────────────┐        ┌─────────────────────────────────┐   │ │
   │ │  │ CRDB Write Pool (写) │───────►│ CRDB Read Pool (读)              │   │ │
   │ │  │ crdb-public:26257    │        │ crdb-public:26257                │   │ │
   │ │  │ (leaseholder 写)     │        │ follower-read AS OF SYSTEM TIME  │   │ │
   │ │  └─────────────────────┘        └─────────────────────────────────┘   │ │
   │ │  RoutingDataSource:@DataSource(WRITE)/默认→写池;@DataSource(READ)→读池│ │
   │ │                                                                       │ │
   │ │  Redis Cluster:会话/缓存/限流/分布式锁(3 主 3 从)                      │ │
   │ └───────────────────────────────────────────────────────────────────────┘ │
   └───────────────────────────────────────────────────────────────────────────┘
```

> **数据面修正注(v5.1)**:上图数据面已由 v5.0 的 TiDB Primary/Follower+TiFlash 改为 CockroachDB 实情。CRDB 单二进制多 region 能力贴合 ESP 5000 CCU 尺度;读写分离走 CRDB follower-read(`AS OF SYSTEM TIME`)+ esp-rw-java `CockroachDbDataSourceConfig`(双 HikariCP 池 + `RoutingDataSource`)。原 TiDB/TiFlash/TiCDC 异地多活叙事见第 13 章"历史演进/可回切保留"。

### 2.2 逻辑分层
| 层 | 组件 | 职责 |
|---|---|---|
| 接入层 | Ingress / BFF / gateway-go | 协议接入、鉴权、路由、长连接管理 |
| 编排层 | learning-java / teacher-java | 业务流程编排、领域逻辑、事件入口 |
| AI 能力层 | writing-py / emotion-svc | LLM 调用、算法推理 |
| 异步处理层 | Pulsar + Disruptor + learning-worker | 削峰、落库、算法触发 |
| 数据层 | CockroachDB(读写分离)+ Redis Cluster | 持久化、缓存 |
| 可观测层 | Prometheus / Grafana / Loki / OTel | 指标 / 日志 / 追踪 |

### 2.3 部署拓扑(当前实现)
- 集群:Rancher Desktop 单节点(6 CPU / 14GB),命名空间 `esp-prod`。
- 数据面:**CockroachDB**(CRDB 3 节点,zone=z1/z2/z3,单台 RD VM;无 region 层,multi-region Phase1 DEFERRED),短名 `crdb-public:26257`,应用经 `ESP_RW_STORAGE_TYPE=cockroachdb` 走 `CockroachDbDataSourceConfig`。
- 基础设施复用:Redis Cluster / Pulsar / Neo4j / Milvus / Nacos 已运行于 `ekb-m2-aws-us-east-1` 或下沉宿主,通过 `05b-platform-infra-externalnames.yaml` 的 ExternalName 引用,`esp-prod` 内以短名 `redis-cluster`/`pulsar`/`neo4j`/`nacos` 等访问。其中 `redis-cluster` ExternalName 指向真 Redis Cluster(`redis-cluster-new` StatefulSet,3主3从 Cluster 模式),应用以 cluster 模式连接。
- **TiDB 已下电(2026-08-20)**:`tidb`/`tidb-follower` ExternalName 仍声明于 `05b-platform-infra-externalnames.yaml:13-30`,但底层 `tidb-cluster-tidb.ekb-m2-aws-us-east-1` 已不可用;`07-tidb-tikv.yaml` 中 PD/TiKV/TiDB 副本现 replicas=1,多节点恢复时可改回(详见第 13 章历史演进段与 ADR-24 回切条件)。mirror pingcap 镜像源 403,回切前需先修镜像拉取。
- 入口:Traefik Ingress(host=`esp.example.com`),NodePort `30080`(gateway-go)、压测直连 NodePort `31032`。

---

## 第 3 章 服务划分(领域驱动)

> **服务清单修正(v5.1)**:`backend/services/` 下实际 24 个子目录(23 服务 + conversation-go 内嵌库)。conversation-go **非独立部署服务**,是 gateway-go 通过 `replace esp/conversation-go => ../conversation-go` 引入的会话/ASR/TTS 领域库(provider 代码物理在 `conversation-go/src/{asr,tts,llm}`,运行时由 gateway-go 调用)。下表按语言栈与领域归类,端口为容器端口。

| 服务 | 语言 | 端口 | 职责 | 状态 | 数据访问 |
|---|---|---|---|---|---|
| gateway-go | Go | 8080 | WebSocket 口语会话、Mock ASR/TTS、实时流编排;内嵌 conversation-go 会话/LLM/ASR/TTS 领域库 | 无状态(会话在 Redis) | Redis(读写)/ CRDB(读会话历史) |
| bff-java(bff-gateway) | Java | 8090 | REST 聚合、JWT 校验、限流、RouteTable 动态化 | 无状态 | Redis / CRDB(esp_auth) |
| learning-java | Java | 8081 | 知识图谱、艾宾浩斯复习、动态难度、学情事件入口(Disruptor) | 无状态 | CRDB 读写分离(esp_learning + esp_shared.app_logs) |
| learning-worker | Java | - | Disruptor/Pulsar 消费 → 落库 + 算法 | 无状态 | CRDB 写主库 |
| teacher-java | Java | 8083 | 教案生成、学情分析、作业评阅 | 无状态 | CRDB 读写分离 |
| writing-py | Python | 8082 | 写作批改、润色、翻译精读、Claude(Litestar+Granian+LangGraph) | 无状态 | CRDB 写主库 |
| emotion-svc | Python | 8084 | 情绪感知、视觉辅助 | 无状态 | 无(纯计算) |
| content-go | Go | - | 课程策展、内容作答(esp_content) | 无状态 | CRDB |
| community-go | Go | - | 圈子/UGC/举报(esp_community) | 无状态 | CRDB |
| gamification-go | Go | - | 排行榜 CQRS(热 Redis ZSET + 冷 CRDB 快照) | 无状态 | Redis / CRDB(esp_gamification) |
| listening-go | Go | - | 听力训练 | 无状态 | CRDB |
| reading-go | Go | - | 阅读训练 | 无状态 | CRDB |
| vocab-srs-go | Go | - | 词汇间隔重复 | 无状态 | CRDB |
| course-path-go | Go | - | 学习路径编排 | 无状态 | CRDB |
| live-tutor-go | Go | - | 实时辅导 | 无状态 | CRDB |
| admin-go | Go | - | 运维/治理面板 | 无状态 | CRDB |
| asr-adapter-go | Go | - | ASR 引擎 HTTP 适配(消费方为 gateway-go/conversation-go) | 无状态 | 无 |
| tts-adapter-go | Go | - | TTS 引擎 HTTP 适配 | 无状态 | 无 |
| writing-coach | Python | - | 写作增强聚合(LCO 三层缓存接入点) | 无状态 | CRDB |
| llm-orchestrator-rs | Rust | - | LLM 编排/统一 Provider 抽象(LCO 接入点) | 无状态 | Redis / Milvus / Neo4j |
| assessment-rs | Rust | - | 评测引擎 | 无状态 | CRDB |
| pronunciation-rs | Rust | - | 发音评分 | 无状态 | 无(纯计算) |
| media-learn-rs | Rust | - | 媒体学习 | 无状态 | CRDB |
| **conversation-go** | Go | **(非独立部署)** | **gateway-go 内嵌会话领域库**(ASR/TTS/LLM/cache),非独立 Deployment | 内嵌 | 复用 gateway-go |
| frontend | React | 3000 | 学习者 + 教师前端(Web/H5/小程序三端) | - | - |

> 当前实现注:learning-worker 的落库消费已内置于 learning-java 进程(`DbLearningEventConsumer`),亦可通过 `ESP_ROLE=worker` 独立 Deployment(`21-learning-worker.yaml`);Pulsar 已就绪并预留 topic,本期学情事件走 Disruptor 直落库,Pulsar 用于跨服务事件广播(speech.session / writing.task 等)。独立服务数 23 + 内嵌库 1(conversation-go)。"24 微服务"计数含 conversation-go,但独立部署数为 23。

---

## 第 4 章 读写分离架构(重点)

> **实现修正(v5.1)**:本章 v5.0 以 TiDB Primary/Follower + TiFlash 列存为读写分离实现。实际生产自 #140 全服务切 CockroachDB 后,读写分离走 **CRDB follower-read(`AS OF SYSTEM TIME`)** + esp-rw-java `CockroachDbDataSourceConfig`(双 HikariCP 池 + `RoutingDataSource`)。TiDB/TiFlash 路径的 `DataSourceConfig` 与 `@DataSource` 注解机制保留可回切(见第 13 章),但当前生产数据面是 CRDB。

### 4.1 设计目标
5000 CCU 下,学情事件高频写、学习路径/学情分析高频读。若读写均打写节点,写节点成为瓶颈且分析查询影响 OLTP。读写分离将:
- **写**:始终命中 CRDB leaseholder(写池),保证强一致。
- **读**:按一致性要求分流——强一致读(个人最新状态)走写池;弱一致读(报表/榜单/列表)走 follower-read(`AS OF SYSTEM TIME` 历史读,卸载写节点)。

### 4.2 路由策略
```
                    ┌──────────────────────────┐
   业务请求 ───────►│  RoutingDataSource (Java) │
                    └────────────┬─────────────┘
              ┌──────────────────┼──────────────────┐
              │ 标注 @DataSource(READ)│ 默认 / @DataSource(WRITE)│
              ▼                  ▼
     ┌────────────────┐   ┌────────────────┐
     │ Read Pool       │   │ Write Pool      │
     │ → CRDB read     │   │ → CRDB primary  │
     │   host(follower │   │   (leaseholder) │
     │   -read AS OF   │   │ (强一致)        │
     │   SYSTEM TIME)  │   │                 │
     └────────────────┘   └────────────────┘
```

### 4.3 实现机制(Java 侧,核心业务面,CRDB 路径)
1. **双 DataSource(CRDB)**:`crdbWriteDataSource`(jdbc:postgresql://crdb-public:26257/...)与 `crdbReadDataSource`(同 host 或独立 read host,读池连接参数启用 follower-read),由 Spring `AbstractRoutingDataSource`(`RoutingDataSource`)按 Key 路由,经 `LazyConnectionDataSourceProxy` 包裹延迟获取连接。实现在 `platform/esp-rw-java/src/main/java/com/esp/rw/CockroachDbDataSourceConfig.java`,`@ConditionalOnProperty(esp.rw.storage.type=cockroachdb)` 激活。
2. **注解驱动**:`@DataSource(READ)` 标注 Service 方法走读池;`@DataSource(WRITE)`/默认走写池。`DataSourceAspect`(`@Around`)在方法入口设置 `RoutingHolder` ThreadLocal 路由 Key,finally 清除。注解机制与 TiDB 路径完全复用。
3. **强一致读兜底**:写后立即读场景,通过 `@DataSource(value=READ, consistent=true)` 强制走写池(leaseholder),避免 follower 复制延迟读到旧值。
4. **follower-read 历史读**:弱一致读走读池,CRDB 用 `AS OF SYSTEM TIME` 提供近实时历史读,卸载写节点。**注意**:CRDB 为 Pebble LSM 行存,无 TiFlash 式列存 MPP,学情大数据聚合(learning_events 30亿行)已退化为外推数仓方案(见第 13 章技术债与 ADR-25 DEFERRED)。
5. **Go / Python 侧**:gateway-go 读会话历史走 read host;writing-py 落库走 write host。各自在连接串层面区分;Go 服务(gamification-go 等)用 `READ_DB_URL`/`WRITE_DB_URL` env 指向 `crdb-public:26257`。

### 4.4 读写分离在各模块的应用
| 模块 | 写 | 读 |
|---|---|---|
| 口语陪练 | session_turns 落写池(learning-worker 异步) | 会话历史读读池 |
| 自适应学习 | learning_events 落写池(Disruptor→worker) | 学习路径/mastery 读读池(写后强一致读走写池) |
| 写作批改 | writing_tasks 落写池 | 历史作文读读池 |
| 教师学情分析 | - | 共性难题聚合读读池(无列存加速,依赖索引+应用层聚合) |
| 作业评阅 | submissions 落写池 | 列表读读池 |

### 4.5 一致性权衡
- 复制延迟:CRDB Raft 多副本, follower-read(`AS OF SYSTEM TIME`)容忍秒级历史读延迟。
- 关键写后读:走写池(leaseholder),牺牲少量延迟换一致性。
- 报表:容忍秒级延迟,走读池 follower-read,换吞吐。

---

## 第 5 章 关键架构决策(ADR 摘要)

### ADR-01 Java 采用 LMAX Disruptor
**背景**:学情事件高频(5000 CCU × 每分钟数十事件)。
**决策**:learning-java 作为 Producer 发布事件到 RingBuffer;`DbLearningEventConsumer` 作为 EventProcessor 消费 → 批量写 TiDB(批量 200 条或 500ms 触发)+ 可发 Pulsar。
**收益**:无锁(RingBuffer + 序列屏障)单线程 10M+ ops/s,解耦接入吞吐与落库/算法延迟。
**版本**:Disruptor 4.0+(Java 21)。RingBuffer size=65536,`tryNext()` 满即丢并记 `dropped`,保护网关响应时间。

### ADR-02 Pulsar 而非 Kafka
计算存储分离(Broker 无状态,K8s 横向扩容简单)、多租户、百万 Topic、Geo-replication 预留。

### ADR-03 TiDB + 读写分离 + TiFlash HTAP(Superseded by ADR-24)
**状态**:Superseded by ADR-24(2026-08-21)。CRDB 作为主数据面后,TiDB HTAP 路径保留可回切,生产不再走 TiDB。
MySQL 协议兼容(零改造)、HTAP(OLTP 行存 + OLAP 列存)、水平扩缩容;读写分离卸载主库(见第 4 章)。主键 AUTO_RANDOM 避免写热点。**实情(2026-08-21)**:TiDB 3区域 2026-08-20 下电,生产全量切 CockroachDB(见 ADR-24)。

### ADR-04 Go 承担实时面
goroutine per connection,单进程 hold 数万 WS;会话状态外置 Redis Cluster → 网关无状态 → K8s 任意 Pod 接连接;跨 Pod 广播走 Redis Pub/Sub。

### ADR-05 LLM Provider 抽象 + Mock 模式
`LlmProvider` 接口各语言实现,默认 Anthropic Claude;`LLM_MODE=mock` 切定性 stub,压测规避费用/限流。

### ADR-06 读写分离路由(见第 4 章)

### ADR-07 基础设施复用而非重复部署
集群已存在 TiDB/Redis/Pulsar(`ekb-m2-*` 命名空间),通过 ExternalName 复用,避免重复占用资源,符合"容器中已有则重用"约束。

### ADR-08 BFF 网关:Spring Cloud Gateway + Spring Cloud Alibaba(Sentinel/Nacos),保留 Gateway 但引入 Alibaba 治理组件
**背景**:用户提出"BFF | Spring Cloud Gateway | 4.x 里能否使用 Dubbo 相关的 Spring Cloud Alibaba 替换掉 Spring Cloud Gateway 组件"。

**决策**:**不替换 Spring Cloud Gateway,而是在其之上叠加 Spring Cloud Alibaba 治理组件(Nacos 配置/注册中心、Sentinel 流控熔断、Dubbo Triple 作为 Java 服务间 RPC)**。技术选型依据:
1. **职责不同,不可互替**:Spring Cloud Gateway 是基于 Reactor Netty 的**反应式 API 网关**,职责是边缘接入(HTTP/WS 路由、JWT 校验、限流、跨域、协议转换);Dubbo 是**服务间 RPC 框架**,职责是内部服务通信。两者是"南北向(入口)vs 东西向(内部)"关系,非同一层,无替换关系。把 Dubbo 放在 BFF 边缘位置既不合规也无法承载 WS 长连接与边缘鉴权。
2. **组合最优**:网关层用 Spring Cloud Gateway 承载边缘职责;Java 服务间(learning-java ↔ teacher-java 等)用 **Dubbo Triple(gRPC over HTTP/2)** 替代 REST,获更高吞吐与强类型契约;用 **Nacos** 做服务注册与配置中心;用 **Sentinel** 做网关与服务级流控熔断(替代手写令牌桶)。这正是 Spring Cloud Alibaba 的标准组合,与 Spring Cloud Gateway 完美互补而非互斥。
3. **成本**:Spring Cloud Alibaba 2023.x(对应 Spring Boot 3.x / Spring Cloud 2023.x)与现有 Spring Boot 3.3.4 兼容,引入成本可控。
**结论**:保留 Spring Cloud Gateway 4.x 作 BFF/边缘网关;引入 Spring Cloud Alibaba(Nacos + Sentinel + Dubbo Triple)做治理与服务间 RPC。**不因引入 Dubbo 而移除 Gateway**。

### ADR-09 AI 服务框架:FastAPI → Litestar + Granian(全异步、更高吞吐)
**背景**:用户提出"AI 服务(Python)或整个项目能否用比 FastAPI 更优秀的组件替换掉 FastAPI"。

**决策**:**将 writing-py 与 emotion-svc 的 Web 框架由 FastAPI 迁移到 Litestar,ASGI 服务器由 Uvicorn 迁移到 Granian(Rust 实现)**。技术选型依据(对标大厂 AI 服务标准):
1. **性能**:Granian(Rust HTTP/2 ASGI/WSGI 服务器)在多数基准下吞吐与并发显著高于 Uvicorn/HTTPTOOLS;Litestar 在路由解析、依赖注入与 Pydantic 校验路径上有微优化,配合 Granian 的 Rust 内核,端到端 RPS 与尾延迟优于 FastAPI+Uvicorn。
2. **异步原生**:FastAPI 路由支持 `async def`,但本项目原实现用同步 `def` + 同步 `httpx.post`,LLM 调用阻塞线程池。Litestar 全异步范式 + `httpx.AsyncClient` + Anthropic SDK 异步客户端,可在 I/O 等待期释放事件循环,单进程支撑更高并发 LLM 调用(对 G-04 写作批改吞吐与 G-02 口语引导延迟有直接收益)。
3. **工程能力对等或更优**:Litestar 同样基于 Pydantic v2、支持 OpenAPI 3.1 自动生成(优于 FastAPI 的 3.0)、内置依赖注入、生命周期钩子、消息打包优化(msgspec 可选)、更好的类型安全。学习曲线对已熟悉 FastAPI 的团队极低。
4. **生态与稳定性**:Litestar(原 Starlite)已发布 2.x 稳定版,社区活跃,被多家公司生产采用;Granian 由 Pylint 作者维护,Rust 内核稳定。两者均为 Apache-2.0。
5. **不替换 gateway-go 与 Java**:Go 网关用标准库 + gorilla 已是最优实时面选型(ADR-04);Java 用 Spring Boot 生态成熟。**仅替换 Python AI 面的 Web 框架与 ASGI 服务器**,精准提升 AI 服务并发能力。
**结论**:writing-py / emotion-svc 改用 Litestar + Granian;LLM 调用改异步(`httpx.AsyncClient` / anthropic async);保持 Pydantic v2 模型与 OpenAPI 契约不变,前后端契约零影响。

### ADR-10 TiDB 多分区拓扑与同步/读写分离真相(含 TiKV/TiFlash)(Superseded by ADR-24)
**状态**:Superseded by ADR-24(2026-08-21)。本 ADR 描述的 TiDB 单点环境已于 2026-08-20 下电,生产全量切 CockroachDB。原文保留作历史溯源与回切参考。
**背景**:用户提出"容器中有多个分区的 TiDB,它们之间是否进行数据同步、读写分离,是否有 TiKV?多个分区之间的 Pulsar MQ、Redis Cluster 各自是否有数据同步、读写分离等"。

**调研事实**(2026-07-30 对 `ekb-m2-{aws-us-east-1, azure-eastasia, gcp-europe-west1}` 三命名空间实测):
1. **三"分区"实为同一单节点 k3s 上的三个命名空间,仅逻辑隔离,非物理多区域**。
2. **TiDB**:三分区各跑一个**单进程 `tidb-server`(v8.5.5)**,**无 PD、无 TiKV、无 TiFlash**(无任何 tikv/tiflash pod 与 CRD,`/stores` 为空)。即仅有 SQL 前端,**无底层 KV 存储引擎**,本质是单机 SQL,Pod 重启即丢数据。
3. **跨分区同步**:**无**。无 TiKV 故无 TiCDC/BR;三实例互不连接。
4. **读写分离**:`esp-prod` 的 `tidb-primary` 与 `tidb-follower` 两个 ExternalName **指向同一 svc**,所谓"主从"是同一单点,无物理 Follower 可读。

**决策与处置**:
- **当前环境定位为 demo/压测环境**:不强求多区域容灾。复用 `ekb-m2-aws-us-east-1` 的单点 TiDB 作为统一数据面(aws 分区最稳定,见 ADR-07)。
- **读写分离架构不退化**:`RoutingDataSource` 双池 + `@DataSource` 注解 + TiFlash hint 的**设计与代码保持完整**,是面向真实分布式 TiDB 集群的标准实现;当前单点环境下 primary/follower 指向同一实例属环境妥协,生产部署到真实 TiDB Operator 集群(PD 3 + TiKV 3 + TiDB 2 + TiFlash 2)后,读写分离与 HTAP 立即生效,无需改码。
- **生产演进路径(写入部署文档)**:生产用 TiDB Operator 部署真实集群:`pd.replicas=3`、`tikv.replicas=3`、`tidb.replicas=2(primary+follower)`、`tiflash.replicas=2`;开启 `tidb_enable_follower_read=ON` 使 Follower Read 生效;跨区域容灾用 TiCDC 同步到异地 TiDB(预留,本期不实施)。
- **是否引入 TiKV**:生产**必须有 TiKV**(TiDB 的行存引擎,OLTP 基石),否则 TiDB 无存储能力;**必须有 TiFlash**(列存引擎,FR-4.2 学情 OLAP 依赖)。当前容器无 TiKV/TiFlash 是环境降配,文档与代码按"有 TiKV/TiFlash"设计。

### ADR-11 Redis / Pulsar 多分区拓扑与同步真相
**调研事实**:
1. **Redis**:三分区各跑一个 `redis:7.2-alpine` 单实例,`CLUSTER NODES` 返回 `cluster support disabled`,`INFO replication` 显示 `role:master, connected_slaves:0`。即**单机 master,无 Cluster、无从节点、无跨分区同步**。
2. **Pulsar**:三分区各跑一个 `pulsar standalone`(单进程,broker+bookie+zk 内嵌),无独立 bookie StatefulSet、无持久卷。**无 Geo-Replication**(standalone 不支持)。三分区 Pulsar 互不连接;且三实例均高频 OOM 重启(内存占用 ~2.3Gi,节点内存紧张)。

**决策与处置**:
- **Redis**:复用 aws 分区单实例作会话/缓存/限流。**生产演进**:用 Redis Cluster Operator 部署 6 主 6 从集群(`cluster.nodes=6`),开启持久化与哨兵/集群故障转移;跨区域用 Redis Replication 或 Redis CRDT(预留)。
- **Pulsar**:复用 aws 分区 standalone 作事件总线。**生产演进**:用官方 Helm 部署 Pulsar 集群(broker 3 + bookie 3 StatefulSet + 持久卷 + ZooKeeper 3),开启 Geo-Replication 做跨区域复制(预留)。
- **架构代码不绑定单点**:gateway-go 用 `redis.Client`(单点)+ 预留 Cluster 模式开关;learning-java 的 Pulsar producer/consumer 接口预留,本期学情走 Disruptor 直落库(ADR-01),Pulsar 用于跨服务事件广播。生产切到真实集群时仅需改连接串与拓扑配置。

### ADR-12 多分区"同步/读写分离"总体结论(直接回答用户三问)
| 组件 | 当前容器是否多分区 | 是否有 TiKV/TiFlash | 跨分区数据同步 | 读写分离 |
|---|---|---|---|---|
| TiDB | 三命名空间逻辑分区(同节点) | **无**(仅单进程 tidb-server) | **无**(无 TiCDC/BR) | **当前无**(primary/follower 指向同一单点);**架构已就绪**,生产换真实集群即生效 |
| Redis | 三命名空间逻辑分区 | — | **无**(单机 master 无从) | **无**(单点);生产换 Redis Cluster 即生效 |
| Pulsar | 三命名空间逻辑分区 | — | **无**(standalone 无 Geo-Replication) | broker 无主从概念;**无跨区复制**;生产换集群 + Geo-Replication 即生效 |

**结论**:当前容器环境的三"分区"不具备跨分区数据同步与读写分离能力(均为单点玩具级部署,非物理多区域)。ESP 的**读写分离/HTAP/集群高可用能力体现在架构设计与代码实现**(RoutingDataSource、TiFlash hint、Disruptor、Redis Cluster 预留、Pulsar 接口预留),生产部署到真实分布式集群后立即生效。本期按"复用 aws 分区单点 + 架构代码就绪"执行,并在部署文档给出生产演进路径。

### ADR-13 WebSocket 流量旁路 BFF
**背景**:压测发现 WS 经 Traefik Ingress(Host 路由)返回 400/404,直连 gateway-go NodePort 可用;反应式网关(SCG)承载长连接成本高。
**决策**:**WS 流量不经 BFF,直连 gateway-go**(Ingress `/ws/` → gateway-go,或压测直连 NodePort 30080)。BFF 仅负责 REST 鉴权聚合,会话由 REST `POST /api/sessions` 创建,WS 用返回的 session_id 直连 gateway-go。
**依据**:SCG 基于 Reactor Netty,长连接占事件循环线程;旁路避免网关成为 WS 瓶颈;Traefik WS 透传在无 Host 规则下可用。前端三端统一此路径。

### ADR-14 三端前端选型
**决策**:Web=React18+Vite+TS+react-router+SWR+zustand;H5=React18+Vite+antd-mobile;小程序=Taro4(React 语法)。
**依据**:Taro 与现有 React 技术栈一致(uni-app 为 Vue,不一致);一套 React 代码编译微信/支付宝多端;小程序 WS 用 `Taro.connectSocket`(生产需 wss+备案域名)。详见 SDD 第 7 章。

### ADR-15 混沌工程范围
**决策**:单节点 RD 用脚本化混沌(kubectl delete pod / scale 0 / tc 延迟注入);生产用 Chaos Mesh。
**依据**:单节点无法真实网络分区;脚本化覆盖 pod kill / 依赖下线 / 延迟 / BFF kill / 跨区断网等核心场景,记录恢复时间与一致性。

### ADR-16 真 TiKV 异地多活 TiDB 集群(Superseded by ADR-24)
**状态**:Superseded by ADR-24(2026-08-21)。TiDB 异地多活路径未实际落地即被 CRDB 主数据面决策覆盖。
**背景**:原 ADR-10 指出容器内 TiDB 为 unistore 单点(无真 TiKV/TiFlash)。用户要求部署含真 TiKV 的异地多活集群。
**决策(已被覆盖)**:用 TiDB Operator 在 K8s 部署真实 TidbCluster CR(PD3 + TiKV3 + TiDB2 + TiFlash2,开启 `tidb_enable_follower_read=ON`);跨 aws/azure/gcp 三区用 **TiCDC** 异步同步(aws 主写区 → azure/gcp 只读副本),实现异地多活读写分离。原 unistore 单点保留只作迁移源,数据迁移后下线。
**实情(2026-08-21)**:TiDB 3区域 2026-08-20 下电(memory `esp-tidb-decommissioned-scale0-2026-08-20`),mirror pingcap 镜像源 403 致运维受阻,生产改走 CockroachDB。原 TiCDC 异地多活方案未上线即被 ADR-24 覆盖。回切条件见 ADR-24。

### ADR-17 数字人与智能体 + LLM 端点
**决策**:三端前端接入数字人(唇形/表情随 TTS+情绪驱动);四类智能体(口语/写作/学习路径/教案)在各服务编排;LLM 统一 `LlmProvider` 抽象直连 Anthropic API,复用本机代理端点(`ANTHROPIC_BASE_URL`、模型 `claude-opus-4-8[1M]`/`glm_for_coding`),`LLM_MODE=mock` 供压测。
**依据**:数字人增强"人格化"体验(对齐 PRD 愿景);智能体编排使各模块具备主动建议能力;复用本机 Claude Code 已配置的模型端点避免额外 key。详见 SDD 第 7 章。

### ADR-18 智能体编排框架:LangGraph(Python AI 面)+ 轻量编排(Java/Go 面)
**背景**:用户提出"智能体编排方面是否可以/需要引入 LangGraph 及相关组件,进行论证和实施"。

**论证(分类施策)**:
ESP 四类智能体分布在三种语言栈,统一用单一框架不现实。按"复杂度 × 语言栈"分类决策:

| 智能体 | 语言栈 | 复杂度 | 编排方案 | 理由 |
|---|---|---|---|---|
| 写作批改智能体 | Python | 中(多步:评分→语法→连贯→建议) | **LangGraph** | 多步有状态图,LangGraph 的 StateGraph + 条件边天然适配;Litestar 异步 + LangGraph checkpoint 可断点续跑 |
| 学习路径智能体 | Java | 中(规划:薄弱点→复习排序→难度) | 轻量自研状态机 + `LlmProvider` | Java 生态无 LangGraph 等价物;规划逻辑确定性强,自研状态机足够,避免跨语言调用 Python 的网络开销 |
| 教案智能体 | Java | 低(单轮结构化生成) | 单轮 `LlmProvider` 调用 + 模板 | 结构化输出,无需多步图 |
| 口语陪练智能体 | Go | 中(实时:纠错→LLM→hint,低延迟) | 轻量自研编排 + `llm.Provider` | 实时面 P95<800ms,引入跨语言 RPC 增延迟;Go 自研足够 |

**决策**:
1. **Python AI 面(writing-py)引入 LangGraph**:将 `WritingService` 的 grade/polish/translate 重构为 LangGraph `StateGraph`,实现"**批改 → 润色 → 生成学习笔记**"多步链,并支持**教师人工干预(human-in-the-loop)**。详见下方"写作智能体 LangGraph 链路设计"。emotion-svc 情绪分类为单步,不引入 LangGraph。
2. **Java/Go 面不引入 LangGraph**:学习路径智能体用自研 `LearningPathStateMachine`(状态:WEAK_DETECT→PLAN→RANK→DIFFICULTY);教案与口语智能体保持单轮/轻量编排。理由:LangGraph 是 Python 库,Java/Go 无原生实现;跨语言 gRPC 调 Python 会增加 ~20-50ms 尾延迟,违反实时面 SLO;且 Java/Go 智能体逻辑确定性强,自研编排更轻、更可控。
3. **共享抽象**:所有智能体经统一 `Agent` 概念封装(Python 用 LangGraph,Java/Go 用自研),对外暴露一致的 `run(input)->output` 契约,LLM 调用统一 `LlmProvider`/`llm.Provider` 抽象。
4. **可观测**:LangGraph 的 step 级事件接入 OpenTelemetry(Python distro),与 Java/Go 智能体的 trace 串联。

**写作智能体 LangGraph 链路设计**(批改→润色→学习笔记 + 人工干预):
```
StateGraph(WritingState):
  START → grade(评分+语法+连贯)
        → polish(基于批改结果润色)
        → [human_review 节点: 教师介入]  ← interrupt_before,挂起等待教师反馈
        → notes(生成学习笔记:生词/语法点/改进清单)
        → END
```
- **State**:`WritingState { prompt, essay, exam, grade_result, polished, teacher_feedback, notes, status }`。
- **批改节点 grade**:LLM 评分官角色 → score/feedback/grammar/coherence。
- **润色节点 polish**:LLM editor 角色 → 基于 grade_result 给 polished + suggestions。
- **人工干预节点 human_review**:用 LangGraph `interrupt_before` 在 polish 后挂起,持久化 checkpoint(`MemorySaver`/生产用 Redis checkpointer);教师经 teacher-java `POST /api/writing/review` 提交 `teacher_feedback`(可改写润色结果/补充评语)→ 恢复执行。
- **学习笔记节点 notes**:LLM 综合 grade_result + polished + teacher_feedback → 生词表/语法点/个性化改进清单。
- **断点续跑**:checkpointer 持久化每步状态;教师可在任意时间介入,`Command(resume=...)` 恢复。
- **降级**:无教师介入时,`teacher_feedback` 为空,notes 节点直接基于 grade+polish 生成(自动模式,供压测/学习者自助)。

**收益**:写作智能体获得多步推理、条件分支、断点续跑、人工干预能力;教师可在润色后介入调整,再生成最终学习笔记,形成"AI 批改 → 教师把关 → 个性化笔记"闭环。**不盲目全栈引入** LangGraph,避免跨语言开销与过度工程化。

**结论**:Python AI 面引入 LangGraph(writing-py 写作批改智能体:批改→润色→人工干预→学习笔记);Java/Go 面用自研轻量编排;统一 `LlmProvider` + OTel 可观测。详见 SDD 第 9.6 节。

---

## 第 6 章 数据架构

### 6.1 ER 概览(详见 schema.sql)
```
users ──< sessions ──< session_turns
users ──< user_mastery >── knowledge_points
users ──< learning_events
users ──< writing_tasks
users(teacher) ──< classes ──< assignments ──< submissions
users(teacher) ──< lesson_plans
```

### 6.2 读写分离下的表归属(CRDB 路径)
| 表 | 写池 | 读池 | 备注 |
|---|---|---|---|
| users | ✓ | ✓ | 读多写少 |
| sessions / session_turns | ✓(异步) | ✓ | 会话历史读读池 |
| learning_events | ✓(Disruptor→worker) | ✓(个人,follower-read) | 高频写,CRDB 用 `unique_rowid()` 替代 AUTO_RANDOM |
| user_mastery | ✓ | ✓(写后强一致读走写池) | PK(user_id,kp_id) |
| writing_tasks | ✓ | ✓ | feedback JSON 列 |
| lesson_plans / classes / assignments / submissions | ✓ | ✓ | 无列存加速 |

### 6.3 索引与分区
- learning_events:CRDB 行存无 TiDB 式分区表,按 `created_at` 建索引 `idx_user_time` 加速查询;30亿行聚合走外推数仓(ADR-25 DEFERRED)。
- 高频读路径覆盖索引:`user_mastery(user_id, next_review_at)`。

---

## 第 7 章 通信与契约
- **实时**:WebSocket JSON frames(口语);预留 WebRTC 升级。
- **业务 REST**:OpenAPI 3(`docs/api/`)。
- **内部事件**:Pulsar(JSON/Avro),topic 命名 `{domain}.{action}`。
- **鉴权**:JWT(RS256),Ingress/BFF 校验,透传 `X-User-Id` / `X-Role`。

---

## 第 8 章 高并发与性能设计
1. **接入层**:gateway-go WS 长连接 + Redis Pub/Sub;BFF 限流(令牌桶,Redis)。
2. **业务层**:Disruptor 削峰学情事件;批量化写 CRDB(批量 200)。
3. **数据层**:CRDB 读写分离(follower-read)+ 连接池(HikariCP max 50)。无 TiFlash 列存,OLAP 聚合外推数仓(ADR-25 DEFERRED)。
4. **缓存层**:Redis Cluster 缓存学习路径、场景模板、热点教案。
5. **LLM 层**:流式输出降首字延迟;mock 模式压测;三层语义缓存(LCO 铁律,L1 Redis/L2 Milvus/L3 Neo4j)。

---

## 第 9 章 高可用与容错
- 服务多副本 + readiness/liveness 探针;HPA 自动扩缩。
- Pulsar 消息持久化 + 消费失败重试 + 死信队列。
- CockroachDB Raft 多副本(3 节点 zone=z1/z2/z3,单台 RD VM;multi-region Phase1 DEFERRED);Redis Cluster 主从 + 故障转移(3主3从)。
- 优雅停机:WS drain / Disruptor drain(`@PreDestroy`)/ Pulsar consumer 关闭。

---

## 第 10 章 可观测性
- **日志**:JSON 结构化,统一 trace_id(slog / logback)。
- **指标**:各服务 `/metrics`(Prometheus);gateway-go `/metrics`(promhttp),Java `/actuator/prometheus`(micrometer)。关键 histogram:WS 延迟、Disruptor 吞吐(published/dropped)、LLM 延迟、DB 连接池。
- **追踪**:OpenTelemetry,跨服务 trace 串联。
- **仪表**:Grafana(CPU/内存/延迟/错误率/CCU)。
- **监控栈部署**:见 `docs/test/05-监控告警部署说明.md`。

---

## 第 11 章 部署架构
### 11.1 K8s 生产
- 每服务 Deployment + HPA(CPU/自定义指标 CCU/队列深度)。
- 数据面:CockroachDB(3 节点单台 RD VM,zone=z1/z2/z3);Redis Cluster(3主3从);Pulsar standalone。本期复用已存在集群/下沉宿主组件。
- Ingress(Traefik)+ cert-manager(Let's Encrypt);Secret 管理 API Key/DB 密码。
- 命名空间:`esp-prod`、`ekb-m2-aws-us-east-1`(复用基础设施)。

### 11.2 本地验证(Docker Compose)
- 全栈:CockroachDB、Redis Cluster、Pulsar standalone、各服务、frontend。
- `docker compose up` 一键起;CRDB init 自动建库建表(`scripts/apply_crdb_schema.py`)。

---

## 第 12 章 安全架构
- TLS 全链路;JWT RS256 + 刷新令牌;RBAC(learner/teacher/admin)。
- Secret 化:ANTHROPIC_API_KEY、DB 密码、JWT 私钥。
- PII 脱敏:邮箱等敏感字段存储加密。
- 限流与防刷:Redis 令牌桶,按 user / IP。

> 安全现状注:本期网关 `userIDFromContext` 返回固定 1(鉴权为占位),压测/演示环境 JWT 以 mock 模式运行;生产须在 BFF/Ingress 强制校验。详见 SDD 第 1 章 & 安全测试文档。

---

## 第 13 章 历史演进与可回切保留(TiDB 路径)

> **本章为 v5.1 新增**,记录 TiDB HTAP/TiCDC 异地多活路径的历史决策与回切条件,生产当前不走此路径。

### 13.1 历史决策(已被 ADR-24 覆盖)
- v5.0 SAD 以 TiDB+TiFlash HTAP+TiCDC 异地多活为数据面叙事(原 ADR-03/10/16)。
- 实际 2026-08 起生产全量切 CockroachDB(`ESP_RW_STORAGE_TYPE=cockroachdb`,`crdb-public:26257`)。
- TiDB 3区域(aws/azure/gcp 逻辑 ns)于 2026-08-20 下电止血:`deploy/k8s/07-tidb-tikv.yaml` 中 PD/TiKV/TiDB replicas 现 =1(实际底层已不可用),memory `esp-tidb-decommissioned-scale0-2026-08-20`。
- `deploy/k8s/05b-platform-infra-externalnames.yaml:13-30` 仍声明 `tidb`/`tidb-follower` ExternalName 指向已下电集群,作为可回切保留(应用 env `WRITE_DB_URL`/`READ_DB_URL` 仍保留 `tidb-primary`/`tidb-follower` jdbc:mysql 回切串,见 `20-learning-java.yaml:38-39`)。

### 13.2 多区域实情
- 当前无物理多区域:CRDB 3 节点同处单台 RD VM(zone=z1/z2/z3),无 region 层。
- multi-region(GLOBAL/REGIONAL BY ROW + SURVIVE REGION FAILURE)为 Phase1 DEFERRED,见 `docs/design/cockroachdb-vs-tidb-multi-region-plan.md`(注:该文档原"不值得引入 CRDB"的 ADR 结论已被 ADR-24 覆盖,保留作选型溯源)。

### 13.3 回切条件(多节点恢复时)
- 多节点服务器恢复 + mirror pingcap 镜像源修好(当前 403)后,可回 TiDB:`07-tidb-tikv.yaml` replicas 改回 3 + `05b-platform-infra-externalnames.yaml` 指向真实集群 + 应用 env `ESP_RW_STORAGE_TYPE=tidb`。
- schema 兼容性需重验:`AUTO_RANDOM`(TiDB)↔ `unique_rowid()`(CRDB)、`ON UPDATE CURRENT_TIMESTAMP`(TiDB)↔ 触发器/应用层维护(CRDB)等差异。
- 回切决策需新 ADR,不可静默切换(工程铁律⑦诚实优先 + DGO GitOps)。

---

## 第 14 章 技术栈清单
| 类别 | 选型 | 版本 | 选型依据 |
|---|---|---|---|
| 实时网关 | Go + gorilla/websocket + go-redis | Go 1.22+ | goroutine per connection,单进程 hold 数万 WS;无状态(会话外置 Redis) |
| 业务服务 | Java 21 + Spring Boot 3 + Disruptor | Spring Boot 3.3.4 / Disruptor 4.0 | Disruptor 无锁削峰学情事件 |
| BFF/边缘网关 | Spring Cloud Gateway | 4.x | 反应式边缘网关,承载 JWT/限流/路由/WS(ADR-08) |
| 服务治理 | Spring Cloud Alibaba(Nacos + Sentinel + Dubbo Triple) | 2023.x | Nacos 注册/配置、Sentinel 流控熔断、Dubbo Triple 服务间 RPC(ADR-08) |
| AI 服务 | Python 3.12 + **Litestar + Granian** + anthropic SDK | Litestar 2.x / Granian 1.x | 全异步 + Rust ASGI 内核,吞吐/尾延迟优于 FastAPI+Uvicorn(ADR-09) |
| MQ | Apache Pulsar | 3.x | 计算存储分离、多租户、Geo-Replication 预留(ADR-02/11) |
| DB | **CockroachDB(主数据面)** | 22.x/23.x | Postgres 兼容、Raft 多副本、声明式多区域;读写分离走 follower-read(ADR-24);TiDB+TiFlash 路径保留可回切(ADR-03/10/16 Superseded) |
| 缓存 | Redis Cluster | 7.x/8.x | 会话/缓存/限流/分布式锁,3 主 3 从(ADR-11) |
| 前端 | React 18 + Vite + TS + SWR | — | 轻量数据请求 + 快速构建 |
| 可观测 | Prometheus + Grafana + Loki + OpenTelemetry | — | 指标/日志/追踪三位一体 |
| 压测 | Locust | 2.x | 5000 CCU HTTP+WS 压测 |
| 部署 | Docker + K8s + Helm | — | 容器化 + 声明式编排 + HPA |

---

## 第 15 章 与需求追溯
| SRS 需求 | 架构落点 |
|---|---|
| G-01 5000 CCU | gateway-go 无状态 + K8s HPA |
| G-06 学情事件 5000 ev/s | Disruptor + 批量写 |
| NFR-PERF-2 读写分离 | 第 4 章 + CRDB 双 DataSource(CockroachDbDataSourceConfig) |
| FR-2.4 学情事件流 | Disruptor RingBuffer → worker |
| FR-4.2 学情大数据分析 | 原靠 TiFlash 列存(Superseded);CRDB 行存无 MPP,改走外推数仓(ADR-25 DEFERRED) |
| NFR-MAINT-* 可观测 | 第 10 章 |
| NFR-DEPLOY-* | 第 11 章 |
