# 全服务应共享/未接线代码审计(2026-08-15)

> 只读扫描,供 `engineering-ironrules.md` 证据附录使用。扫描范围:Java(3 服务 + platform 共享模块)、Go(8 服务 + esp-common-go)、Rust(4 服务 + esp-common-rs)、Python(3 服务 + esp-cache-py/esp-db-py)。
> 结论:Java 侧 JWT/异常/日志是最高密度重复;bff Dubbo Triple 配置是最大"配而未接"负债;跨语言 traceId 键名不一致是最直接观测阻断点。

---

## A. Java 服务间重复/近重复

| 重复类 | 份数 | 证据文件:行号 | 差异 |
|---|---|---|---|
| GlobalExceptionHandler | 3 | bff `handler/GlobalExceptionHandler.java:28`(envelope:41-55,含 RestClientException 503)、learning `web/GlobalExceptionHandler.java:26`、teacher `web/GlobalExceptionHandler.java:22` | 仅 basePackages 不同;bff 多 RestClientException handler;`currentTraceId()` 每份重写 |
| JwtValidator | 2 | learning `auth/JwtValidator.java:20` vs teacher `config/JwtValidator.java:20` | diff 仅 package/注释 + 一行 null 判空;`parse/parseUserId/parsePublicKey` 全同 |
| JwtAuthFilter | 2 | learning `auth/JwtAuthFilter.java:27` vs teacher `config/JwtAuthFilter.java:25` | 仅 package/注释/白名单细节;OncePerRequestFilter 逻辑全同 |
| AppLogClient | 3 | bff `common/AppLogClient.java`(75 行)、learning `common/AppLogClient.java`(101 行)、teacher `common/AppLogClient.java`(75 行) | 同构:异步 insert app_logs + OTel span 取 traceId;learning 多 service 参数 |
| jjwt+bouncycastle 依赖块 | 3 | 三 pom 各自重复声明 jjwt-api/impl/jackson + bcprov/bcpkix | 版本各服务单独 pin |
| Response 信封 | — | envelope 均为 GlobalExceptionHandler 内联 `Map<String,Object>`,无共享 `ApiResponse` | 无共享包 |

## B. 共享模块依赖矩阵

**Java(3 服务 pom)**

| 共享模块 | bff-java | learning-java | teacher-java |
|---|---|---|---|
| esp-api | 引 pom:150-151,**零代码 import** | pom:117 + `rpc/LearningRpcProvider.java:24` | pom:53 + `config/DubboConsumerConfig.java:19` |
| esp-cache | pom:82,CacheService/EventPublisher | pom:67,CacheService | pom:80,CacheService+tryIdempotent |
| esp-disruptor | pom:75,`edge/InletDisruptor.java` | pom:110,`disruptor/LearningEventDisruptor.java` | **未声明、未使用** |
| esp-rw-java | pom:88,`AuthController.java:86,123` | pom:79,`LearningService.java` 多处 + `LearningRpcProvider` | pom:86,`TeacherService.java` 多处 + Saga |

**Go(esp-common-go)**

| 服务 | 引用 | 备注 |
|---|---|---|
| gateway-go | config+redisx | **自写** tracing.go + healthz,未用 common-go tracing/health |
| conversation-go | 无 | 内嵌库;自写 DB `db/db.go:68-84` + 自写 log/client.go |
| course-path-go | config+health+redisx+tracing | |
| gamification-go | config+db+health+redisx | 继承默认池 |
| listening/reading-go | config+db+health | 继承默认池 |
| live-tutor-go | config+health+tracing | 无 db |
| vocab-srs-go | config+health | **自写** DB `dao/vocab.go:66-71`(20/5,与默认 10/2 不一致) |

esp-common-go/db 默认 MaxOpen=10/MaxIdle=2(`db/db.go:20-21`),无 env 覆盖。

**Rust(esp-common-rs)** — 4 服务全依赖(db/tracing_init);`src/health.rs` **零服务使用**。

**Python(esp-cache-py/esp-db-py)** — 3 服务 pyproject 全依赖,均用 esp_cache/esp_db。但 `emotion-svc/app/logging_client.py:16,58` 与 `writing-py/app/logging_client.py:23,66` 各**手写 pymysql** 写 app_logs(重复 Java AppLogClient 模式);`writing-py/app/main.py:34` 用 esp_cache 私有 API `_get_redis` 做幂等。

## C. 设计未接线

| 未接线项 | 证据 |
|---|---|
| **bff Dubbo 全空转** | application.yml:74-82 配 `dubbo: protocol tri port 50090 + nacos registry`;pom:151 引 esp-api;源码 **0 @DubboReference、0 com.esp.api import**,下游全走 RestClient(`ProxyController.java:43,58`)。全仓 @DubboReference 仅 3 处:teacher + integration-tests 测试 |
| **esp-cache 幂等 HTTP 过滤器未接线** | `IdempotencyFilter.java:35` 无 @Component/FilterRegistrationBean,仅自身单测;生产只走 teacher 编程式 `tryIdempotent` |
| **esp-common-rs/health.rs 未接线** | 4 个 Rust 服务均自写 axum health handler,无人 import esp_common::health |
| **Outbox 仅 learning 独享** | learning_event_outbox/OutboxRelay 只在 learning-java;其他服务不引用 |
| 已正常接线 | @DataSource(3 服务全用)、EventPublisher(bff/learning/teacher)、CacheService(3 服务全用) |

## D. 跨语言不一致

| 维度 | 现状 |
|---|---|
| 健康端点 | Go 6 服务 common-go/health,gateway 自写 /healthz;Java /actuator(框架级);Rust 路径**不一致**(/actuator/health vs /healthz);Python /healthz |
| 错误信封 | Java {code,message,traceId};Go 无统一格式;Python {error};Rust Json<Value> 零散 |
| traceId 键名 | Java `traceId`;Go 混用 `traceId`/`trace_id`;Rust `trace_id`;Python `trace_id`。**跨语言无法按统一 key 串联** |

---

## Top 5 最值得抽共享包

1. **esp-web-common(待建)** — 收编 3 服务 10 个重复文件:GlobalExceptionHandler + envelope + currentTraceId + AppLogClient(3 份,75-101 行)。
2. **JWT 边缘鉴权 starter** — JwtValidator+JwtAuthFilter learning/teacher 逐字节相同(仅 package),bff AuthFilter(签发侧)可一并纳入。
3. **esp-common-go 收编 gateway/conversation/vocab** — gateway 自写 tracing.go+healthz、conversation/vocab 自写 DB 池(20/5 vs 10/2 不一致),应收编并给 db 加 env 池覆盖。
4. **esp-common-rs 健康接线** — 4 个 Rust 服务统一用 esp_common::health 与 /actuator/health 路径。
5. **app_logs 写入契约跨语言统一** — Java 3 份 AppLogClient + Go 2 份 log/client.go + Python 2 份 logging_client.py(全手写同一 INSERT),抽统一契约并定 `trace_id` 键名消歧。

---

## 关联

- 工程铁律:docs/design/architecture/engineering-ironrules.md(铁律一~三直指本清单)
- 架构审查:report/architecture-review-2026-08-15.md
