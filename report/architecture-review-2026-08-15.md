# 全服务架构审查(2026-08-15)

> 审查标准:互联网大厂资深架构师/技术总监(10亿用户/100万并发对标)。
> 只读审查,不改代码。4 个 subagent 并行采集(Go/Java/Rust+Python/测试覆盖率),主线程复核压测瓶颈根因。

---

## 服务清单与语言栈

| # | 服务 | 语言 | 框架 | 端口 | K8s 副本/资源(limits) | 角色 |
|---|---|---|---|---|---|---|
| 1 | gateway-go | Go 1.25 | net/http + gorilla/websocket | 8080 | 1 pod / 512Mi 1CPU | WS 网关,鉴权,会话接入 |
| 2 | conversation-go | Go 1.25 | library(gorilla WS) | - | 随 gateway | 对话引擎,Hub,scenario |
| 3 | course-path-go | Go 1.25 | net/http | 8093 | - | 学习路径(stub) |
| 4 | gamification-go | Go 1.25 | gin 1.10 | 8092 | - | 积分/排行榜 |
| 5 | listening-go | Go 1.25 | gin 1.10 | 8089 | - | 听力练习(stub) |
| 6 | live-tutor-go | Go 1.25 | net/http + gorilla | 8094 | - | 直播辅导(echo stub) |
| 7 | reading-go | Go 1.25 | gin 1.10 | 8091 | - | 阅读练习(stub) |
| 8 | vocab-srs-go | Go 1.25 | gin 1.10 + zap | 8087 | - | 词汇间隔重复 |
| 9 | bff-java | Java 21 | Spring Boot 4.0.7 + Netty edge + Jetty MVC | 9090(edge)/8090(mvc) | - / - | BFF 边缘接入+鉴权+聚合 |
| 10 | learning-java | Java 21 | Spring Boot 4.0.7 + Jetty + Disruptor 4.0 | 8081 | 1 pod / 1536Mi 1CPU | 学情核心,事件落库 |
| 11 | teacher-java | Java 21 | Spring Boot 4.0.7 + Jetty | 8083 | - | 教师端 |
| 12 | assessment-rs | Rust | tokio full + axum 0.7 + sqlx | - | - | 测评(stub) |
| 13 | llm-orchestrator-rs | Rust | tokio full + axum 0.7 + sqlx | - | - | LLM 编排 |
| 14 | media-learn-rs | Rust | tokio + axum | - | - | 媒体学习(stub) |
| 15 | pronunciation-rs | Rust | tokio + axum + sqlx | - | - | 发音评测 |
| 16 | emotion-svc | Python | Litestar 2.24 + Granian | 8084 | workers=2 | 情绪感知(落 learning_events) |
| 17 | writing-coach | Python | Litestar + Granian + anthropic | 8085 | workers=2 | 写作辅导 |
| 18 | writing-py | Python | Litestar + Granian + LangGraph | 8082 | workers=2 | 写作批改 AI |
| 19 | integration-tests | Java | JUnit IT | - | - | 4 契约/集成测试 |

**共享模块**:`platform/esp-disruptor`(5 类 3 测试)、`platform/esp-cache`(多类 7 测试)、`platform/esp-rw-java`(读写分离 4 测试)、`backend/shared/esp-common-go`(5 类 1 测试)、`backend/shared/esp-common-rs`(5 类 0 测试)、`backend/shared/esp-cache-py`(0 测试)、`backend/shared/esp-db-py`(0 测试)。

---

## 各语言框架与并发模型评分

| 服务 | 语言 | 框架 | 并发模型 | 评分 | 依据 |
|---|---|---|---|---|---|
| gateway-go | Go | net/http + gorilla WS | goroutine-per-WS(阻塞读循环) | 4/10 | session ID 用 `sess_<UnixMilli>`(`main.go:362`)毫秒级碰撞;WS 无 ping/pong/deadline,慢消费者阻塞 Hub 广播 goroutine(头线阻塞);`session.Manager.End` 每会话 `go func()` 无上限(`session/manager.go:144`)可致 goroutine 风暴;Hub 写无并发保护(gorilla Conn 非并发写安全) |
| conversation-go | Go | library | Hub dispatch goroutine + sync.RWMutex | 5/10 | 复用 gateway-go 的 WS 缺陷;DB 池 env 驱动(默认 10/2)尚可;archive goroutine 无上限 |
| gamification-go | Go | gin 1.10 | gin 默认(每请求 goroutine) | 5/10 | 框架选型合理;但 stdlib `log` 非结构化,leaderboard key 硬编码,DB 池 hardcoded 10/2 |
| vocab-srs-go | Go | gin + zap | gin 默认 | 6/10 | 唯一用 zap 结构化日志;DB 池自配 20/5 高于共享默认;但 trace ID `v-<UnixNano>`(`main.go:128`)非 UUID |
| listening-go/reading-go/live-tutor-go/course-path-go | Go | gin/net.http | 阻塞 | 3/10 | stub 级实现,几乎空壳;course-path/live-tutor 无优雅关闭;path_id/booking_id 非_uuid |
| bff-java | Java 21 | SB 4.0.7 + Netty edge | Netty edge(1 boss + 2×CPU worker)+ Jetty MVC thread-per-request(虚拟线程开启) | 5/10 | SB4+JDK21+虚拟线程是先进栈;Netty edge SO_BACKLOG=8192 合理。**但**:Dubbo Triple 全链路配置了却**未接线**(bff 无任何 `@DubboReference`,下游全走同步 `RestClient` `ProxyController.java:88`);Netty bootstrap 用裸 `new Thread`(非线程池,`NettyEdgeServer.java:56`);edge 与 MVC 双端口职责重叠;无 `server.tomcat.threads` 调优(走默认 200) |
| learning-java | Java 21 | SB 4.0.7 + Jetty + Disruptor 4.0 | Jetty 400 线程 + 虚拟线程 + Disruptor 4 consumer 分片 | 6/10 | 栈先进(SB4/虚拟线程/Disruptor 4 多 EventHandler 分片)。**但**:Disruptor consumer 线程**同步** `batchUpdate` 阻塞(`DbLearningEventConsumer.java:60`),DB 延迟直接反压 RingBuffer → DROP;无独立 DB writer 线程池;batch 失败仅 log 丢数据(`:64-67`);OutboxRelay 单线程逐行 INSERT(`OutboxRelay.java:68`)N+1;Jetty 400 线程 + 虚拟线程双轨易混淆(虚拟线程下 servlet 线程池本应收敛) |
| teacher-java | Java 21 | SB 4.0.7 + Jetty | 同 learning | 6/10 | 同构优缺点;无 Disruptor;领域模型 `Submission` 有充血方法(grade/confirm/rollback) |
| assessment-rs | Rust | tokio full + axum 0.7 | tokio multi-thread(默认) | 4/10 | 框架选型最优(tokio+axum)。**但**:sqlx 池无 `max_connections`(默认 10);无 spawn/channel/背压;纯 stub |
| llm-orchestrator-rs | Rust | tokio + axum + reqwest SSE | tokio multi-thread | 5/10 | 有真实 LLM 编排逻辑;idem Redis 兜底;但无并发原语,LLM 调用 inline await(长尾阻塞 worker),SSE helper 死代码 |
| pronunciation-rs | Rust | tokio + axum + sqlx | tokio multi-thread | 4/10 | `ORDER BY id DESC LIMIT 1` 取 last insert id(`main.rs:128`)并发竞态;sqlx 池默认 10 |
| media-learn-rs | Rust | tokio + axum | - | 2/10 | 纯 stub,无 DB |
| emotion-svc | Python | Litestar 2.24 + Granian | async(Litestar)+ 后台 daemon 线程(pymysql app_logs) | 7/10 | 选型先进(Litestar>FastAPI);全 async 端点;DB 走 esp-db-py(aiomysql 异步);app_logs 用 pymysql 但隔离在 daemon 线程 + 有界 queue(正确 fire-and-forget);G-06 路径 `persist_emotion` 是异步非阻塞 |
| writing-coach | Python | Litestar + anthropic | async | 6/10 | 全 async;但仅 1 测试文件,LLM 调用无超时/熔断可见 |
| writing-py | Python | Litestar + LangGraph | async + 幂等 SETNX | 7/10 | LangGraph 多步 agent + Redis 幂等 + 异步 DB;设计较完整;测试 4 文件覆盖 service/agent/api |

**并发模型总体评分:5.2/10**。栈选型本身先进(SB4/JDK21 虚拟线程/Disruptor 4/tokio+axum/Litestar),但工程化落地不彻底:Dubbo Triple 配而未接、Disruptor consumer 同步阻塞 DB、WS 无背压/无 deadline、DB 池普遍未调优。

---

## 测试覆盖率矩阵

| 服务 | 主源文件数 | 测试文件数 | 覆盖率定性 | 缺口 |
|---|---|---|---|---|
| gateway-go | 3 | 1 | minimal(<20%) | main.go/tracing.go 未测;WS/jwt 仅 validator 测 |
| conversation-go | 15 | 8 | good(≥80%) | 8/9 包有配对测试,表现最好 |
| course-path-go | 1 | 1 | good | 单文件全配对 |
| gamification-go | 1 | 1 | good | 单文件全配对 |
| listening-go | 1 | 0 | none | stub,零测试 |
| live-tutor-go | 1 | 0 | none | stub |
| reading-go | 1 | 0 | none | stub |
| vocab-srs-go | 4 | 1 | minimal | 仅 sm2 算法测,3 文件未测 |
| bff-java | 20 | 7 | partial(~50%) | auth/edge/filter/handler 有测,Netty edge/InletDisruptor 未测 |
| learning-java | 24 | 10 | good(≥80%) | disruptor/service/domain/rpc/web 全有测,表现好 |
| teacher-java | 12 | 4 | partial(~50%) | controller/service/domain/contract 有测 |
| integration-tests | 0 | 4 | N/A | 4 IT:Dubbo Triple/读写分离/多级缓存/Disruptor pipeline |
| assessment-rs | 1 | 0 | none | stub |
| llm-orchestrator-rs | 5 | 1(inline) | minimal | 仅 cache/mod.rs 6 个 inline test,provider/dialogue 未测 |
| media-learn-rs | 1 | 0 | none | stub |
| pronunciation-rs | 1 | 0 | none | 零测试 + 并发竞态未覆盖 |
| emotion-svc | 7 | 2 | good | service+API 测 |
| writing-coach | 6 | 1 | minimal | 仅 test_api.py,service/db/llm 未测 |
| writing-py | 11 | 4 | partial(~50%) | agent/service/api 有测 |
| **esp-disruptor** | 5 | 3 | good | DisruptorWorker/Pipeline/Stats 测,但 learning-java 侧 BatchEventHandler/DbLearningEventConsumer/OutboxRelay 零单测 |
| **esp-cache** | 多类 | 7 | good | CacheService/Broadcast/idempotency/event 全测 |
| **esp-rw-java** | 多类 | 4 | good | DataSourceAspect/Config/Routing 全测 |
| **esp-common-go** | 5 | 1 | minimal | 仅 config 测,db/redis/tracing 未测 |
| **esp-common-rs** | 5 | 0 | none | 零测试 |
| **esp-cache-py** | 1 | 0 | none | 零测试 |
| **esp-db-py** | 1 | 0 | none | 零测试 |

**覆盖率总体定性:部分(约 40%)**。Java 共享模块(esp-disruptor/esp-cache/esp-rw-java)达标,learning-java/conversation-go/emotion-svc 达标;但 7 个服务是 stub 零测试,4 个共享模块(esp-common-go/rust、esp-cache-py、esp-db-py)零测试,**距离大厂 ≥80% 标准仍有显著差距,尤其是核心基础设施代码(读写分离池/缓存/DB 池)零单测是上线阻断项**。

---

## 压测瓶颈根因复核(G-04/G-06 + 新发现)

### G-04 WS:peak 955 CCU, FAILED 5000 —— PROGRESS 根因准确但漏了更深的瓶颈

PROGRESS 记载根因:① gateway-go 单 pod 512Mi 撑不住 1000+ WS;② session ID 碰撞 `sess_<unix_ms>` → 403;③ bff-java 探针 timeout。

**复核:准确,但代码层有更深瓶颈被遗漏**

1. **session ID 碰撞已坐实**(`gateway-go/src/main.go:362`):
   ```go
   func newID() string { return fmt.Sprintf("sess_%d", time.Now().UnixMilli()) }
   ```
   并发建会话同毫秒 → Redis key 碰撞 → `Get` 返回他人 session → `uid != sess.UserID` → 403。PROGRESS 诊断正确。建议 UUIDv7(时间有序 + 随机尾)。

2. **WS 无 ping/pong/deadline(新发现,PROGRESS 未提)**:`upgrader`(`main.go:35`)无 ReadDeadline/WriteDeadline;`writeConn`(`main.go:314`)与 `hub.sendLocal` 无 `SetWriteDeadline`。慢消费者/半开连接永不回收,Hub dispatch goroutine 在写阻塞时**头线阻塞整个分片所有会话**。这是 955 CCU 跌落的隐性主因之一 —— 不是 pod 内存先爆,而是连接积压先拖垮 goroutine。

3. **Hub 写并发不安全(新发现)**:gorilla `websocket.Conn` 非并发写安全,但 gateway-go 的 `writeConn`(主读循环)与 Hub 广播(`hub.sendLocal`)可能并发写同一 conn,Hub 持 RLock 快照后释放锁再写(`hub.go:137-141`),**无外部互斥**。会导致偶发 write panic / 数据帧交错。

4. **`session.Manager.End` 无上限 goroutine(`session/manager.go:144`)**:每会话结束 `go func()` 归档 TiDB,5000 CCU 退出风暴 → goroutine 风暴打 TiDB。

5. **单 pod 512Mi**:K8s `deploy/k8s/10-gateway-go.yaml:47` limits 512Mi。gorilla 每 WS 连接 ~40-80KB(两 buf 4096 + goroutine 栈),1000 连接 ~80MB 本够,但 Hub 分片 + archive goroutine + Redis pool 叠加,512Mi 在 GC 抖动时易 OOM。建议 1Gi + 3 replicas。

### G-06 events:peak 370 ev/s, FAILED 5000 ev/s —— PROGRESS 根因基本准确,Disruptor 同步阻塞是核心

PROGRESS 记载根因:① BFF+learning 同步 thread-per-request 链;② Disruptor consumer 同步批量 DB 写 ~300-500 ev/s;③ scale 2 replica CPU 争抢。

**复核:准确,但"同步 thread-per-request"表述需修正**

1. **BFF 不是纯 thread-per-request**:bff-java 已开 `spring.threads.virtual.enabled=true`(`application.yml:13`),虚拟线程下 I/O 并发上限已大幅提升。**真正瓶颈是**:
   - bff→learning 下游走**同步 `RestClient`**(`ProxyController.java:88` `retrieve().toEntity`),虚拟线程虽不占平台线程,但每请求仍阻塞一个虚拟线程 + 一个 Jetty 线程 slot(RestClient 同步);
   - **Dubbo Triple 配置了却未接线**(bff 无 `@DubboReference`),设计文档声称"Java 服务经 Dubbo Triple"但代码未实现,所有下游调用走 HTTP 同步。

2. **Disruptor consumer 同步阻塞 DB(核心瓶颈,已坐实)**:
   - `BatchEventHandler.onEvent`(`BatchEventHandler.java:55`)缓冲到 batchSize(200)或 500ms flush;
   - flush 调 `DbLearningEventConsumer.consume`(`DbLearningEventConsumer.java:48`)→ `writeJdbc.batchUpdate`(`:60`) **同步阻塞**当前 consumer 线程;
   - 4 个 consumer 线程各自 1:1 做 DB 写,**无独立 DB writer 线程池**;
   - DB 延迟升高 → 4 线程全阻塞 → RingBuffer(65536)满 → `publish()` 返回 DROP(`LearningEventDisruptor.java:139`)→ 503。
   - 单节点 1 CPU + TiDB 同 VM,RTT 低但 CPU 争抢,scale 2 replica 反而恶化(每 pod ~0.5 core)。

3. **OutboxRelay N+1 + 单线程(新发现,PROGRESS 提到"SQL grammar bug"但实际是性能问题)**:
   - `OutboxRelay.drain()`(`OutboxRelay.java:47-103`)在单个 `@Scheduled` 线程里**逐行** `writeJdbc.update`(2 次:INSERT + UPDATE status),BATCH=200 行 = 400 次 DB 往返/周期;
   - 非 batchUpdate,与主路径 DbLearningEventConsumer 的批量写形成鲜明对比;
   - OutboxRelay 注释提到"绕过 Disruptor 避免死循环",设计合理但实现低效。

4. **batch 失败丢数据(`DbLearningEventConsumer.java:64-67`)**:`catch (Exception ex) { log.error(...) }` —— 批量写失败仅记错,无重试/死信,数据丢失。注释自认"生产环境:转死信队列/重试。此处仅记错"。

5. **Jetty 400 线程 + 虚拟线程双轨(新发现)**:`application.yml:5` `server.jetty.threads.max: 400` 与 `spring.threads.virtual.enabled: true` 并存。虚拟线程模式下 servlet 线程池本应收敛到很小(如 20),400 平台线程 + 虚拟线程双轨易造成线程数膨胀 + 调度抖动。

### 新发现:emotion-svc 在 G-06 路径上但是异步(非瓶颈)

`emotion-svc/app/service.py:64` `persist_emotion` 写 `learning_events` 走 esp-db-py(aiomysql 异步),非同步阻塞。G-06 瓶颈不在 emotion-svc,而在 learning-java Disruptor consumer 同步 DB 写 + bff 同步 RestClient。

---

## 代码质量通病 Top 10

1. **Dubbo Triple 配而未接(架构债 P0)**:bff/learning/teacher 三服务 `application.yml` 全配 `dubbo.protocol: tri` + Nacos registry,但 bff 无任何 `@DubboReference`,`ProxyController` 走同步 `RestClient`。设计文档与代码脱节,三服务间同步 HTTP 链是 G-06 瓶颈根因之一。

2. **WS 缺 ping/pong/deadline + 并发写不安全(P0)**:gateway-go/conversation-go/live-tutor-go 全无 `SetReadDeadline`/`SetWriteDeadline`/PingHandler;Hub 与主读循环并发写同一 gorilla Conn 无互斥。慢消费者头线阻塞 + 偶发 panic。

3. **ID 生成用时间戳非 UUID(P0)**:gateway-go `sess_<UnixMilli>`(毫秒碰撞)、vocab-srs `v-<UnixNano>`、course-path `path-<UnixNano>`。大厂标准应 UUIDv7/XID。

4. **Disruptor consumer 同步阻塞 DB + 无独立 writer 池(P0)**:learning-java 4 consumer 线程 1:1 同步 `batchUpdate`,DB 延迟直接反压 RingBuffer。应拆分:consumer 只入队列,独立 DB writer 线程池(背压有界 queue)异步批量写。

5. **零全局异常处理(P1)**:三 Java 服务无 `@ControllerAdvice`/`@RestControllerAdvice`/`@ExceptionHandler`。错误响应 ad-hoc(bff `ProxyController` 内联 catch → 503),无统一错误码/envelope。

6. **共享基础设施零单测(P1)**:esp-common-go(仅 config 测)、esp-common-rs、esp-cache-py、esp-db-py 全零或近零测试。读写分离池/缓存/DB 池是全栈基石,零单测是上线阻断。

7. **DB 连接池普遍未调优(P1)**:esp-common-go hardcoded MaxOpen=10/MaxIdle=2;Rust sqlx 池未设 max_connections(默认 10);Python `DB_POOL_SIZE=10`。5000 CCU 下 Go 侧 10 连接偏小(虽然 learning-java 已收敛到 20 合理)。esp-common-go 池配置未 env 化。

8. **日志规范不统一(P2)**:Go 侧仅 gateway-go(slog)+ vocab-srs(zap)结构化,gamification/listening/reading/course-path/live-tutor 用 stdlib `log`;Java 全用 slf4j(一致);Rust 用 tracing(一致);Python 用 logging(一致)。跨语言日志格式不统一影响 Loki 聚合。

9. **7 个服务是 stub(P2)**:listening-go/reading-go/live-tutor-go/course-path-go + assessment-rs/media-learn-rs/pronunciation-rs 是空壳(部分有路由无逻辑)。生产前要么实现要么从拓扑摘除。

10. **OutboxRelay 逐行 INSERT + batch 失败丢数据(P2)**:OutboxRelay N+1(应改 batchUpdate);DbLearningEventConsumer batch 失败仅 log 无死信。与"关键事件不丢"的设计意图相悖。

---

## 综合评分与优先重构建议(按 ROI 排序)

### 综合评分

| 维度 | 评分 | 说明 |
|---|---|---|
| 语言/框架选型先进性 | 7/10 | SB4+JDK21虚拟线程+Disruptor4+tokio+axum+Litestar,栈本身是大厂先进水平 |
| 并发模型工程化落地 | 4/10 | Dubbo未接线/WS无背压/Disruptor同步阻塞,选型先进但落地不彻底 |
| 测试覆盖率 | 4/10 | 核心Java模块达标,但7 stub + 4共享模块零测试,距≥80%差距大 |
| 压测瓶颈根因准确性 | 7/10 | PROGRESS诊断方向正确,但漏了WS无deadline/Hub并发写/Dubbo未接线等更深因 |
| 代码质量通病 | 4/10 | 零全局异常/ID非UUID/DB池未调优/日志不统一 |
| **加权综合** | **5.2/10** | 栈选型拉分,工程化落地与测试拉跨,典型"先进栈+半成品落地" |

### Top 5 重构建议(按 ROI 排序)

1. **learning-java Disruptor consumer 拆独立 DB writer 线程池**:`BatchEventHandler` 改为只入有界 queue,独立线程池异步 `batchUpdate`(背压满则 DROP 不阻塞 consumer);同时修 `DbLearningEventConsumer` batch 失败转死信。直接解 G-06 瓶颈,ROI 最高。
2. **gateway-go WS 加 ping/pong + deadline + UUID session ID + Hub 写互斥**:`SetReadDeadline`/`SetWriteDeadline`/PingHandler;`newID` 改 UUIDv7;Hub `sendLocal` 与 `writeConn` 加 per-conn mutex。直接解 G-04 跌落 + 403 碰撞,ROI 次高。
3. **接线 Dubbo Triple 替换 bff→learning 同步 RestClient**:bff 加 `@DubboReference` 调 learning 的 `LearningRpcProvider`,消除同步 HTTP 链;或确认走 HTTP 则删 Dubbo 配置减少认知噪音。解 G-06 同步链 + 架构债。
4. **补齐共享模块单测(esp-common-go/rust、esp-cache-py、esp-db-py)+ learning-java Disruptor 侧单测**:读写分离池/缓存/DB 池是全栈基石,零单测是上线阻断;BatchEventHandler/DbLearningEventConsumer/OutboxRelay 零单测需补。解上线工程化阻断。
5. **统一全局异常处理 + DB 池 env 化 + 日志规范化**:三 Java 服务加 `@ControllerAdvice` 统一错误 envelope;esp-common-go DB 池改 env 驱动(对齐 conversation-go 的 `DB_POOL_SIZE`);Go 侧统一 slog/zap。提升可观测性与运维一致性。
