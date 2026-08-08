# P1-5 traceId 贯通 — 探查地图与断点(2026-08-03)

> esp-explorer 产出。结论:贯通**真断了**(2 个 P0 断点 + 2 个 P1 bug),P1-5 需修复+实测,非纯实测。

## 各端埋点现状

| 端 | OTel 集成 | span 创建 | 备注 |
|---|---|---|---|
| gateway-go | ✅ SDK 完整 | ✅ `otelhttp.NewHandler`(main.go:83) | W3C propagator 配好,exporter→otel-collector:4317 |
| writing-py / emotion-svc | ✅ SDK 1.30.0 | ✅ Litestar `OpenTelemetryPlugin` ASGI | `_current_trace_id()` 正确取 span |
| bff/learning/teacher-java | ✅ javaagent 加载 | ⚠ Spring MVC 可靠;**Netty 边缘层盲区** | 见断点1 |
| esp-observability | ✅ Collector+Tempo+Grafana 部署 | — | **Tempo 故障**,见断点2 |

## 断点清单(按严重度)

### [P0] 断点1:BFF Netty 边缘层 span 盲区
- BFF 是自建 Netty pipeline:`EdgeAuthHandler → InletDisruptor → DownstreamForwarder`,**不经 Spring MVC Servlet**。
- OTel javaagent 的 Netty 4 instrumentation 对自定义 pipeline 覆盖不全 → Netty 入口**无 server span**。
- 后果:`DownstreamForwarder` 的 JDK `HttpClient.sendAsync()`(InletDisruptor.java:112)无 parent span → 创建**新根 span** → gateway→bff 链第一个 span 就断。
- **修复**:在 Netty handler 显式建 span(手动 `tracer.spanBuilder("bff.edge").startSpan()` 包住请求处理 + W3C extract/inject),或把 traceparent 注入下游 header(EdgeAuthHandler 现在只注 X-User-Id/X-Role,没注 traceparent)。

### [P0] 断点2:Tempo 管道全线故障 —— ⚠ 已修 OOM,单节点下偶发导出超时
- 原:Tempo **OOMKilled**(exit 137,11 次重启,vParquet 压缩 OOM)→ Collector `DeadlineExceeded` → 丢 trace(已丢 3360)。
- **修复**(2026-08-03):Tempo 内存 limit 512Mi→**1Gi**、request 256Mi→512Mi(kubectl patch + `deploy/k8s/monitoring/10-observability.yaml` 已同步)。新 pod **1/1 Running 0 重启**,不再 OOM 崩溃。
- 残留:单节点内存 90% 压力下,Collector→Tempo 导出**偶发** DeadlineExceeded(90s 窗口 4 次,非持续)—— Tempo 不再崩但导出延迟在负载下偶超 Collector timeout。**完整可靠性需多节点**(架构评分 §五 P2-4 OTel Collector 分层 + tail-sampling 降量)。不再深调(收益递减)。
- traceId 贯通实测在代码修复(断点1)部署后做,届时验证 trace 是否能在 Tempo 查到。

### [P1] 断点3:gateway-go traceId 恒空 BUG
- `log/client.go:96-98` `currentTraceID()` 用 `context.Background()` 取 span context → **永远返回空** → app_logs 里 gateway 的 trace_id 恒 null。
- **修复**:把请求 ctx 传进 currentTraceID(改为 `currentTraceID(ctx)`),从 ctx 取 span。

### [P1] 断点4:Java 服务日志无 traceId
- OTel javaagent 会写 MDC `trace_id`,但**无 logback.xml** → Spring Boot 默认 pattern 不含 `%X{trace_id}` → stdout 无 traceId,日志无法关联 trace。
- **修复**:为 bff/learning/teacher 加 `src/main/resources/logback-spring.xml`,pattern 含 `%X{trace_id} %X{span_id}`。

### [P2] 断点5:Dubbo Triple trace 传播未验证
- javaagent 有 Dubbo 3.x instr,但 3.3.6+Triple+javaagent 的 RPC attachment 传播可靠性未验证。可能需 `OTEL_INSTRUMENTATION_DUBBO_ENABLED=true` 或显式 `dubbo.tracing`。

### [P2] 断点6:前端→BFF 无 traceparent
- 前端未集成 OTel,请求不带 traceparent。BFF 作边缘入口需自建根 span(依赖断点1 修好)。

## 实测验证步骤(修复后)
1. 重启 Tempo,确认 Collector DeadlineExceeded 消失。
2. `curl -H "Authorization: Bearer <jwt>" localhost:30080/api/learning/...` 走完整链。
3. 查 Tempo(Grafana→Tempo 数据源)是否一条 trace 贯穿 gateway→bff→learning。
4. 查 app_logs:`service='gateway'` 的 trace_id 非空(验证断点3修复)。
5. Java stdout 日志含 traceId(验证断点4修复)。

## 实现范围(给 builder,需集群稳定后做)
- BFF Netty 显式 span + traceparent 注入下游(断点1)。
- gateway-go `currentTraceID(ctx)`(断点3)。
- Java 三服务 logback-spring.xml(断点4)。
- Tempo 重启 + 排障(断点2,集群操作,需 pods 健康)。
- 修复后实测 + 回填 docs/test/05。
