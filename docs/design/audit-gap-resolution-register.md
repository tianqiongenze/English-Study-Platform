# ESP 文档-落地差距审计与处置登记册

> 审计日期:2026-08-15。覆盖 46 份设计文档 + 测试/部署/工程化资料,逐项 grep 代码与集群实测。
> 目的:找出"宣称了但未落实"的架构决策,逐项处置。

## 一、统计总览

| 口径 | 数量 |
|---|---|
| 可验证架构承诺 | 31 |
| ✅ 已落地 | 16 |
| ⏳ 部分落地 | 7 |
| ❌/📄 未落地或仅方案 | 8 |

## 二、差距表(按处置优先级排序)

### A 类:可在单节点立即落地(本批次处置)

| 承诺 | 现状 | 处置 | 任务号 |
|---|---|---|---|
| Disruptor 多级调度队列 | DisruptorPipeline 仅测试用,生产单 RingBuffer 单消费者(bff BlockingWaitStrategy 65536) | bff InletDisruptor 升两级 pipeline(规整→转发) | #109 |
| esp-cache 跨语言纯 JSON 契约(去 DefaultTyping) | CacheConfig.java:96 仍 activateDefaultTyping | 改纯 JSON + 兼容策略 | #110 |
| llm-orchestrator-rs / vocab-srs-go 部署 | 代码+DB 在,无 Deployment | 补 manifest+镜像+rollout | #111 |
| CI 流水线 | ci.yml 16 连败(PEM 测试失败+writing-py 路径重组后失效) | 修测试密钥内嵌+修路径 | #112 |
| CRDB 实测落地+TiDB 双跑对比 | 仅方案文档,集群无 CRDB | ✅ 已落地:esp-crdb ns 3 节点(arm64)+建库建表+种子+双跑写吞吐对比(report/crdb-vs-tidb-app-bench.json) | #108✅ |
| DB 吞吐优化落地 | 池 210→130 连接+批量 50 配置就绪未部署 | 待统一部署窗口(配置变更需回归) | #105 |
| OTel 导出超时 | endpoint 指向不存在 ns | otel-collector 部署+14 服务接入✅ | #104 |

### B 类:需多节点/独立窗口(环境约束,不强行落地)

| 承诺 | 约束 | 处置 |
|---|---|---|
| 多节点 Cilium CNI | 单节点 k3s,决策多节点才上 | 方案备 `multi-node-capacity-validation-plan.md`,条件触发 |
| Pulsar 三区 Geo-Replication | 单 broker | 方案备 `pulsar-geo-replication-plan.md`,多节点执行 |
| 异地多活/TiCDC 跨区容灾 | 文档自标"预留本期不实施" | 保留预留状态 |
| 真 Follower 物理读写分离 | tidb-primary/follower 同指单实例 | 单节点环境妥协,ADR-10 已认;多节点落地 |
| 多节点容量验证执行 | 需 3master+6worker 集群 | 方案备,回填表全"未执行" |

### C 类:基础设施组件(需独立引入决策)

| 承诺 | 现状 | 处置 |
|---|---|---|
| Loki+Fluent Bit 日志链路 | 全集群无资源 | 未落地,后续引入(ETC 范围) |
| ArgoCD GitOps | deploy/gitops/*.yaml 存在无 ns | 未落地,后续引入 |
| Milvus/Neo4j 语义检索+知识图谱 | L2 Milvus(`esp_llm_semantic` 384维 minilm)+ L3 Neo4j 已接入(conversation-go/orchestrator-rs/writing-coach/llm-cache-proxy);Neo4j 2026-08-20 下沉宿主 | ✅ 已落地(原"未落地"状态反转);L2 维度对齐 + Neo4j 下沉见 `llm-cache-proxy-sdd.md` / `report/milvus-neo4j-sink-plan.md` |

### D 类:测试/工程化执行滞后

| 项 | 现状 | 处置 |
|---|---|---|
| 人工测试 73 条 | 0 条已测 | 待服务全就绪后执行 |
| 5000 CCU 压测 G-04/G-06 | 写作 1rps/学情 2482ev/s 未达标 | 待性能优化部署后复测 |
| 分布式 Locust 常态化 | manifest 在集群无资源 | 按需起 |
| 探针三分离+prePost drain | tech-debt 自标待 apply | 待 apply |
| 单测覆盖率回填 | 01 报告未填实测 | 待回填 |
| 数字人唇形/TTS 情绪驱动 | 仅 SVG 静态表情 | 原型阶段 |
| 个性化推荐系统 | 代码仅文案提及 | 未实现 |

## 三、核心组件实证结论

### Disruptor(用户点名核查)
- **本体**:platform/esp-disruptor 5 类:`DisruptorWorker`(单队列)、`DisruptorPipeline`(多级 stage 串联)、`WaitStrategyType`(5 策略)、`PublishResult`(DROPPED 背压)、`DisruptorStats`。
- **差距**:多级能力仅在 integration-tests `T4_DisruptorPipelineIT.java`;生产 bff `InletDisruptor.java` 是单 RingBuffer(65536,MULTI 生产,BlockingWaitStrategy,单 DownstreamForwarder,fire-and-forget)。
- **处置**:#109 升两级 pipeline 兑现承诺。

### Netty(用户点名核查)
- **实证落地**:bff-java `pom.xml` 依赖 netty-all 4.2.16.Final;`NettyEdgeServer.java` boss(1)+worker(2*CPU) NioEventLoopGroup,SO_BACKLOG 8192,手写 ChannelInitializer(HTTP 解码→EdgeAuthHandler→投递 Disruptor)。非 Reactor Netty,是原生 Netty edge 层 + Spring MVC 并存。gateway-go 用 Go 标准库+gorilla/websocket(与 Netty 无关)。

### 其他高性能组件
- **Rust 四服务**:axum 0.7 + tokio full;pronunciation/llm-orchestrator 用 sqlx(tokio-rustls)+ redis tokio cluster-async。
- **emotion-svc(Python)**:granian(Rust 内核 ASGI server,2 workers),非 uvicorn/gevent;缓存栈 esp-cache-py(cachetools L1 + redis.asyncio L2)。
- **esp-api**:Dubbo Triple(gRPC over HTTP/2,Nacos)学习域契约包,learning-java 提供方,teacher-java/bff 消费方,含 Pact 契约测试。

## 四、处置进度

- A 类(#109/#110/#111/#112 + CRDB):进行中,本批次闭合。
- B/C 类:方案文档已备,环境/决策到位后执行,不强行单节点落地。
- D 类:测试执行待服务全就绪+性能优化部署后批量推进。
