# ESP 架构评审与重构方向报告 (2026-08-14)

> 视角:互联网大厂资深架构师 / 教授级高工(工业级 10亿用户/100万并发对标)。
> 依据:6 路并行静态审查(后端架构/DDD、事件驱动/读写分离/多级缓存、安全封控、前端多端、测试综合)+ WS 重测(bff 扩核后)+ 联网大厂最佳实践对标 + 已有压测/上线就绪报告。
> 目的:给当前实现打分,指出与大厂标准的差距,输出按优先级的重构 roadmap。
> 注:所有"私钥扩散"等高危发现已 kubectl/Grep **核实属实**(非 subagent 臆断)。

---

## 0. 综合评分(满分 10)

| 维度 | 评分 | 评级 | 一句话 |
|---|---|---|---|
| 架构与技术选型 | **8.0** | A- | 多语言微服务+Dubbo Triple+BFF+Disruptor+读写分离 骨架是工业级的 |
| DDD 落地深度 | **5.0** | C | domain 包是"包装层",全贫血 JPA 实体,无聚合根/值对象/仓储接口 |
| 事件驱动(EDA) | **6.5** | B | Disruptor+Pulsar 双轨完整,但单消费者同步落库是瓶颈,无 outbox 有丢数据风险 |
| 读写分离 | **6.0** | B- | 注解 AOP 完备,但当前是 TiDB 单节点**伪分离**+ThreadLocal 对异步失效 |
| 多级缓存 | **6.5** | B | 防穿透+双删到位,但**零命中率指标**(无法量化)+缺防雪崩/击穿 |
| **安全封控** | **3.5** | **D** | **最致命:RSA 私钥扩散到 5 服务 + 12 服务裸奔 + IDOR 越权 + 无限流/幂等/TLS** |
| 前端多端 | **4.0** | C- | 三套独立代码零共享、零设计系统、零前端 CI、token 存 localStorage |
| 工程化(CI/单测) | **5.0** | C | 后端有 CI 无前端 job,公共包单测覆盖缺口大,零 E2E |
| 目录组织 | **5.5** | C+ | services/ 平铺 27 目录,公共包与业务服务混放,无分层依赖约束 |
| 可观测性 | **7.0** | B | OTel 全链路+Prometheus+Grafana 到位,但缓存/Disruptor 缺 Micrometer 指标 |
| 测试覆盖 | **6.5** | B | 后端集成 21 服务+三端 GUI+契约测试到位,但单测/E2E/安全测试缺 |
| **加权综合** | **5.9 / 10** | **C+** | **骨架优秀但封控是红线,距大厂上线标准有明确差距,需重构** |

> 综合判定:**"架构蓝图达到工业级水准,但安全封控存在致命缺口,DDD/前端/工程化为半成品"**。骨架值 8 分,被安全(3.5)+前端(4.0)+DDD(5.0)拉到 5.9。可作为**灰度/内部测试**,距**大厂生产上线**需完成 P0 重构。

---

## 1. 后端架构与 DDD(评分 8.0 / 5.0)

### 1.1 优点(骨架工业级)
- **多语言按域选型**:Java(I/O 密集的 bff/learning/teacher)+ Go(网关/高频微服务 8 个)+ Python(AI 写作/情绪)+ Rust(高性能 assessment/llm/pronunciation/media)。选型正确。
- **Dubbo Triple 跨服务契约**:`esp-api` 定义 `LearningRpcService`/`KpStat`,gRPC over HTTP/2 + Nacos 注册。契约优先,符合大厂标准。
- **BFF 聚合层**:`bff-java` = NettyEdgeServer + RouteTable + InletDisruptor(边缘高并发)+ JwtService(JWKS)。职责清晰。
- **Disruptor 入口**:bff 用 LMAX Disruptor 做边缘流水线,网关层抗尖峰的设计正确。

### 1.2 DDD 差距(贫血模型,5.0)
- `learning-java/domain/UserMastery.java`(64 行,16/18 方法是 getter/setter)、`teacher-java/domain/Submission.java`(39 行,全 getter/setter)—— **@Entity 贫血 POJO,零行为**。
- **无聚合根边界、无值对象语义、无仓储接口抽象**(直接 `@Autowired JpaRepository`,无 domain repository 接口)。
- "事件"(Pulsar topic)是基础设施消息,不是 DDD DomainEvent。
- SAGA(`TeacherSummarySaga` 187 行)是亮点,但属架构层非 DDD 层。

### 1.3 职责越界
- **gateway-go 职责严重泄漏**:除路由+鉴权+WS,还塞了 `dialog/engine`(对话引擎)、`scenario/`(剧本)、`session/`(会话态)、`llm/provider`(LLM 调用)、`asr/`、`tts/`。已是"对话网关+业务编排器"二合一,与 bff-java 的编排职责重叠,且 gateway-go 自己也做 JWT(`src/jwt/`)与 bff 签发权冲突。

---

## 2. 事件驱动 EDA(评分 6.5)

### 2.1 现状
- `esp-disruptor`:RingBuffer 65536 + ProducerType.MULTI + BlockingWaitStrategy;`publish()` → `tryPublishEvent`,满则返回 `DROPPED`(**无重试、无 outbox、无事务消息,直接丢弃**)。
- 单消费者模型:仅 learning-java 用,`DbLearningEventConsumer` 单线程同步 `JdbcTemplate.batchUpdate` 写 TiDB → **~760ev/s 背压**。
- Pulsar 双轨:缓存广播(`esp-cache-invalidate`)+ 业务 topic(`learning.event.report`/`user.registered`/`teacher.finalized`/`teacher.autograde.requested`/`session.ended`/`writing.graded`)。

### 2.2 缺陷
- **数据丢失风险**:tryPublishEvent 满即 DROP,无 outbox。对"学习事件"这类可容忍丢失的埋点尚可,但若扩展到订单/支付类业务则是红线。
- **拆 worker 的循环陷阱**:`21-learning-worker.yaml`(replicas:0)已设计水平扩展,但 `LearningEventConsumer` 是 `@Lazy` 无条件订阅 `learning.event.report`,入口关 disruptor 而 consumer 仍开 → `handleEventReport`→`reportEvent`→再发 Pulsar → **无限循环**。拆分时入口必须同步 `PULSAR_CONSUMER_ENABLED=false`。

### 2.3 容量提升路径(达 G-06 5000ev/s)
- 拆 learning-worker 独立 Deployment(N 副本水平扩消费者),入口发 Pulsar topic。
- 消费者侧 batch 并行化(当前单线程顺序)。
- TiDB 写入分片(learning_events 已 AUTO_RANDOM + idx_user_time,写入友好)。

---

## 3. 读写分离(评分 6.0)

### 3.1 代码层完备
- `esp-rw-java`:`RoutingDataSource` + `LazyConnectionDataSourceProxy` + `@DataSource(READ/WRITE, consistent/tiflash)` AOP 切面。`consistent=true` 强制走主库(写后读一致),`tiflash=true` 走列存分析。设计正确。
- Python `esp-db-py`:aiomysql 双连接池(`_write_pool`→primary, `_read_pool`→follower),`execute_write`/`execute_read` 分流。
- HikariCP write/read 各 50/10。

### 3.2 两个硬伤
- **⚠️ 伪分离(已 kubectl 实证)**:`05b-platform-infra-externalnames.yaml` 中 `tidb` 与 `tidb-follower` 两个 ExternalName **指向同一 ClusterIP**(tidb-cluster-tidb)。单节点 TiDB 无主从,所有 tidb-server 共享 TiKV,**无写吞吐收益,仅分摊连接池**。真分离需 tidb replicas≥2 + 独立 follower svc。
- **⚠️ ThreadLocal 对异步失效**:`RoutingHolder` 用 ThreadLocal 持有路由键,而 bff-java 是 **Netty + Disruptor 异步模型**,换线程后 ThreadLocal 丢失 → 异步链路读写分离路由有隐患。应改 Reactor Context 或 SQL Hint。

---

## 4. 多级缓存与命中率提升空间(评分 6.5)

### 4.1 到位的防护
- `esp-cache` 手动 Cache-Aside:L1 Caffeine → L2 Redis → loader。
- **防穿透**:空值 sentinel(空对象也缓存,empty-ttl=30s)。
- **防脏读**:Pulsar 广播失效 + 延迟双删 100ms(double-delete-delay)。
- TTL 分层:l1=60s/maximumSize=10000,l2=300s,聚合=1800s。

### 4.2 缺陷与提升空间(直接回答"命中率提升")
- **⚠️ 零命中率指标(最关键缺口)**:`CaffeineCacheManager` **未开 `recordStats()`**,无 Micrometer `CacheMetrics` 绑定,actuator `/caches` 只有基本信息。**无法量化命中率 → 优化无依据**。这是命中率提升的第一前提:先加 `Caffeine.builder().recordStats()` + `CacheMetricsAutoConfiguration` 自动暴露 `cache.gets{result=hit,miss}`。
- **⚠️ 缺防雪崩**:TTL 无随机抖动,批量 key 同时过期会打穿。
- **⚠️ 缺防击穿**:无 singleflight 互斥,热点 key 过期瞬间并发回源。
- **该缓存没缓存**:`BehaviorController` 的 scenarios/funnel/heatmap 走 `readJdbc` **未缓存**(高频分析查询)。
- **提升路径**:① 加 recordStats + 指标;② TTL 加 ±10% 随机抖动;③ 热点 key(scenarios/writing templates)用 `getOrLoad` 显式缓存 + singleflight;④ 扩展 `esp-redis-warmup-job` 预热热点;⑤ 多副本靠 Pulsar 广播(已具备,Shared 订阅全副本收)。

---

## 5. 安全封控(评分 3.5 — 最致命维度)

> 两个架构级红线 + 多项高危。此维度决定"能否上线"。

### 5.1 红线一:RSA 签发私钥扩散到 5 服务(已核实)
- secret `esp-rsa-jwt` 同时含 `JWT_RSA_PRIVATE_KEY_PEM` + `JWT_RSA_PUBLIC_KEY_PEM`(kubectl 实证)。
- `envFrom secretRef` 注入 secret **全部 key** → **bff/gateway/learning/learning-worker/teacher 5 个服务全部握有私钥**(grep 实证 5 处 `secretRef: {name: esp-rsa-jwt}`)。
- 只有 bff-java 需要私钥签发 JWT,其余 4 个只需公钥校验却拿到了私钥。**任一服务 RCE 即可伪造任意用户(含 admin)JWT**。
- 私钥明文存 etcd,无 SealedSecret/KMS。

### 5.2 红线二:12 个服务完全裸奔无鉴权
| 有鉴权(2) | 裸奔(12+) |
|---|---|
| bff-java AuthFilter(白名单合理)、gateway-go userIDFromRequest() | **learning-java**(有 JwtValidator 类却**无任何 Filter 调用它**,信任 `X-User-Id` header,伪造即越权,NodePort 30081 对外)、writing-py/emotion(NodePort 30084 对外)/writing-coach/course-path/vocab/gamification/reading/listening/live-tutor… |

- **teacher-java 降级即裸奔**:`JwtAuthFilter` 在 `!jwt.enabled()`(公钥未注入)时直接放行。

### 5.3 IDOR 越权
- `gateway-go:177` history 缺 X-User-Id 时回落用 `?user_id=` query(注释"便于 Swagger 测试"——生产即可查任意用户会话历史)。
- teacher `/api/teacher/{userId}/kp-stats`、emotion `/api/emotion/recent/{user_id}`、writing-coach `/api/writing-coach/plan/{user_id}`:路径参数即查任意用户,无归属校验。

### 5.4 角色提升与登录漏洞
- `AuthController:62` register 的 `role` 由请求体控制 → 用户可自注册为 teacher/admin(权限提升)。
- `AuthController:111` wx-login:code 直接当身份、**无微信服务端校验**、`hashCode` 可碰撞。

### 5.5 防重放/防刷/限流/幂等
- **限流**:仅 bff-java 有 Sentinel(且"规则未加载则默认不限流"),其余 12 服务零限流,无 traefik 层限流。
- **幂等**:仅 writing-py grade 有 SETNX;learning/event、sessions、writing/polish 无幂等 key。
- **JWT**:`JwtService:41` 无 jti、无黑名单、无 refresh;注销无效。1h 硬编码过期。
- **风控**:无设备指纹/IP 频次/行为埋点/封禁。
- **WS 鉴权**:`main.go:215` `?token=` 校验且比对 session 归属——此项**正确**。

### 5.6 网络与传输
- **NetworkPolicy 放太宽**:坑㉕ 修复时 patch 成 `namespaceSelector:{}` 放行**所有 namespace** → 跨项目 pod 可达 esp-prod 任意服务。
- 无 TLS / 无 mTLS / 全仓库零 CORS 配置 / DB root 无密码。

### 5.7 好消息
- SQL 全参数化(esp-db-py `%s`+args、JPA `:param`,无 `${}` 拼接)。
- SSRF base_url 来自 env 不可控。
- 依赖版本较新(spring-boot 4.0.7/jjwt 0.13.0/golang-jwt v5.3.1),无已知 CVE。

---

## 6. 前端多端(评分 4.0)

### 6.1 三重基建缺失(最大问题)
- **三套独立代码、零共享、零 monorepo**:无根 package.json/workspace。API 层三份拷贝**已漂移**(react 31 接口最全 / h5 缺 writingReview / 小程序签名少参数);types.ts 注释"三端内联同一份"靠人肉同步。
- **WS 客户端三处各写**:裸 `new WebSocket`,**无重连/心跳/消息队列**,断线即死。
- **无设计系统**:styles.css 15-30 行硬编码 hex,无 tokens/暗黑主题/i18n,三端色值各写。

### 6.2 工程化空白
- 无 ESLint/Prettier/单测(E2E 零),无前端 CI(ci.yml 里 frontend 出现 0 次),无路由懒加载/错误边界/骨架屏。
- TS 类型质量差:`LessonPlan.exercises/games` 大量 `unknown[]`;小程序 api.ts any 滥用。

### 6.3 安全
- token 存 localStorage(`auth.ts:24-37`)→ XSS 可偷,长寿命无 refresh。

---

## 7. 目录组织(评分 5.5)

### 问题:services/ 平铺 27 目录,公共包与业务服务混放
- 业务服务(learning-java/gateway-go…)与**公共库**(esp-cache/esp-disruptor/esp-common-go/esp-common-rs/esp-db-py/esp-cache-py/esp-api/esp-rw-java)和 **integration-tests** 全平铺在 `services/` 下。
- 大厂 monorepo 会做 `backend/services/` + `backend/shared/`(或 `platform/`) + `frontend/` + `infra/` 顶层分层。
- 当前依赖拓扑靠命名前缀(esp-)约束,**无编译级保障**(shared 可反向依赖 services 而不被发现)。
- **符合软件工程吗?**:部分符合(有公共包抽取意识),但**分层不清晰**。公共包应与可部署服务目录隔离,否则新人无法一眼区分"哪个能独立部署、哪个是 lib"。

---

## 8. 可观测性与测试(评分 7.0 / 6.5)
- **优点**:OTel 全链路(Java agent + Python SDK + Go/WS traceparent)+ Prometheus + Grafana 到位;后端集成 21 服务 + 三端 GUI + 契约测试。
- **缺口**:缓存/Disruptor 缺 Micrometer 指标;单测覆盖率缺口大(业务服务几乎无单测);零 E2E;零安全测试(无 DAST/SAST)。

---

## 9. 重构 Roadmap(按优先级)

### P0 — 上线前必须(安全红线 + 数据安全)【阻断生产】
1. **私钥收敛**:secret 拆为 `esp-rsa-jwt-sign`(含私钥,仅 bff-java envFrom)+ `esp-rsa-jwt-verify`(仅公钥,gateway/learning/teacher/worker envFrom)。立即消除"任一 RCE 伪造 admin JWT"。
2. **learning-java 接鉴权**:已有 `JwtValidator`,补 `JwtAuthFilter`(仿 teacher)+ 移除信任 `X-User-Id` header,改从 JWT subject 取 userId。
3. **IDOR 修复**:移除 gateway-go history 的 `?user_id=` 回落;所有 `/{user_id}` 路径参数接口加"调用者==资源归属"校验。
4. **register 锁 role**:服务端强制 role=student,teacher/admin 仅后台开通。
5. **NodePort 收敛**:learning 30081/emotion 30084 等对公网 NodePort 收为 ClusterIP(仅 traefik ingress 暴露)。
6. **NetworkPolicy 收窄**:把坑㉕ 的 `namespaceSelector:{}` 改为精确 podSelector/namespaceSelector 白名单。

### P1 — 上线前应做(限流/幂等/容量)【质量门槛】
7. **全局限流**:traefik middleware RateLimit(入口)+ 各服务 Sentinel/令牌桶(细粒度)。WS/写接口单独配额。
8. **写接口幂等**:POST 类接口加 Idempotency-Key header + Redis SETNX(复用 writing-py grade 模式)。
9. **JWT 加固**:加 jti + Redis 黑名单(注销生效)+ refresh token。
10. **Disruptor outbox**:关键业务事件(非埋点)加 outbox 表 + 可靠投递,避免 DROP 丢数据。
11. **拆 learning-worker**:入口发 Pulsar、worker N 副本消费(**入口同步关 PULSAR_CONSUMER_ENABLED 防循环**),达 G-06 5000ev/s。
12. **缓存可观测**:`Caffeine.recordStats()` + Micrometer CacheMetrics,先量化命中率再优化。

### P2 — 架构深化(DDD/前端/工程化)【大厂标准对齐】
13. **DDD 充血化**:domain 包拆 aggregate/valueobject/domainevent/repository(接口);行为下沉(`UserMastery.applyEvent()`、`Submission.grade()`);repository 接口入 domain、实现入 infrastructure。
14. **gateway-go 瘦身**:dialog/scenario/llm/session 下沉为独立"对话服务",gateway 退回纯路由+鉴权+WS;JWT 签发权统一归 bff。
15. **前端 monorepo 重构**:pnpm workspace + turbo,三端收 `apps/{web,h5,miniprogram}` + 抽 `packages/{api-client(OpenAPI 生成),types,ui-tokens,ws-client}`。统一设计 tokens + i18n。token 转 httpOnly cookie。
16. **读写分离异步化**:ThreadLocal → Reactor Context / SQL Hint,适配 bff 异步模型;多节点 tidb replicas≥2 真分流。
17. **缓存防雪崩/击穿**:TTL 随机抖动 + singleflight 互斥 + BehaviorController 热点查询纳入缓存。
18. **目录重组**:`backend/{services,shared}` + `frontend/` + `platform/`(disruptor/rw/api/cache)+ Nx/Bazel 依赖图校验。
19. **工程化补齐**:前端 ESLint/Prettier/Vitest + CI frontend job;后端单测覆盖率≥80%;E2E(Playwright);SAST(Depcheck/Trivy)。

### P3 — 多节点容量验证(生产上线前置)
20. G-01(5000 CCU)/G-06(5000ev/s)单节点受资源约束未达,需多节点集群验证(bff 多副本 + learning-worker 拆分 + tidb replicas≥2)。

---

## 10. 关于"是否现在就完整重构"的建议

评分 5.9 → 与大厂生产标准有明确差距,但**重构范围极大**(P0~P2 共 19 项,横跨安全/DDD/事件驱动/前端/目录)。建议**分波次**执行,而非一次性"完整重构"(那样上下文与风险都不可控):

- **第一波(立即,本会话可启动)**:P0 安全红线 1-6(6 项),这是唯一阻断上线的硬伤,改动集中在 secret 拆分 + 加 Filter + 修 IDOR,可在单节点验证。
- **第二波(容量与质量)**:P1 限流/幂等/JWT 加固/outbox/拆 worker/缓存指标(6 项)。
- **第三波(架构深化)**:P2 DDD/gateway 瘦身/前端 monorepo/读写异步化/目录重组(7 项)——这部分工作量最大,建议独立专项。

我建议**先执行第一波 P0 安全重构**(最高价值、风险可控、可立即验证),完成后再评估是否继续第二波。是否同意此分波策略,或要求一次性全量重构?

---

## 附:数据来源
- 后端架构/DDD:explorer agent A(24 tool uses)。
- 事件驱动/缓存:explorer agent D。
- 安全封控:explorer agent C(已 kubectl/Grep 核实私钥扩散)。
- 前端多端:explorer agent B。
- WS 重测:esp-data agent(bff 扩 2 核,~62→372,6 倍提升,0 restart)。
- 压测/上线就绪:`report/launch-readiness-2026-08-14.md`、`docs/progress/archive/session22-21-loadtest-traefik502-launch.md`。
