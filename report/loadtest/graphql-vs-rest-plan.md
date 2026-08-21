# ESP BFF GraphQL vs pre-GraphQL REST 聚合性能对比压测方案

> Phase1 Task C(2026-08-21)。本方案只定义脚本与执行计划,实际执行由主线程串行控制(单节点铁律,避免 VM 雪崩)。
> 脚本:`deploy/k6/graphql-vs-rest.js`(未执行)。

## 1. 目标

量化 BFF 引入 GraphQL(spring-graphql 2.0.4,端点 `/graphql`)后,对"前端一屏聚合取数"场景的端到端时延与吞吐收益:

- **GraphQL 单请求聚合** 5 个下游数据片(5 下游 RTT → 1 客户端 RTT),减少 N+1 往返与 over-fetching。
- **REST 多端点串行往返** 同等 5 数据片(客户端 5 次 RTT)。
- 对比维度:`http_req_duration` P50/P95/P99、`http_reqs`、失败率、自定义 `graphql_screen_duration_ms` vs `rest_screen_duration_ms`。

## 2. 可压测 Query(经源码核实)

来源:`backend/services/bff-java/src/main/resources/graphql/schema.graphqls` + `graphql/*Fetcher.java`。

| GraphQL Query 字段 | 下游 REST(经 bff WebClient 直连) | REST 等价端点(经 bff RouteTable,客户端可达) |
|---|---|---|
| `rankings(type,scope,scopeId,period,limit)` | gamification-go `/api/gamification/rankings` | `/api/gamification/rankings?type=xp&scope=global&scope_id=all&period=weekly&limit=20` |
| `rankingGap(type,scope,scopeId,period)` | gamification-go `/api/gamification/rankings/gap` | `/api/gamification/rankings/gap?type=xp&scope=global&scope_id=all&period=weekly` |
| `rankingRegions` | gamification-go `/api/gamification/rankings/regions` | `/api/gamification/rankings/regions` |
| `reviewList` | learning-java `/api/learning/review` | `/api/learning/review` |
| `weakPoints` | learning-java `/api/learning/mastery/weak` | `/api/learning/mastery/weak` |

聚合 query 文本见脚本 `GRAPHQL_QUERY`(5 字段一次取全,对应上表 5 个 REST 端点)。`avatar` 子字段走 BatchLoader(N+1 演示字段),本压测不展开,避免 DataLoader 批次干扰聚合收益基线。

## 3. 端点与鉴权(核实结论)

- **GraphQL 端点**:`POST http://bff-java:8090/graphql`。
  - spring-graphql `http.path: /graphql`(application.yml:21,ASV 核实 `GraphQlProperties$Http.getPath`)。
  - 复用 MVC 端口 8090(非 Netty edge 9090)。
- **bff-java 暴露方式**:ClusterIP(`deploy/k8s/15-bff-java.yaml`,无 NodePort,注释明确"不擅自加 NodePort 以免端口冲突")。
  - **Blocker(已知)**:`/graphql` 未在 traefik ingress(`deploy/k8s/40-ingress.yaml`)声明路由,ingress 仅映射 `/api/**`。故 `/graphql` **只能集群内 ClusterIP 直连**,不可经 ingress/NodePort 从宿主打。压测必须在集群内 k6 pod 执行(符合单节点铁律:压测机入集群)。
  - 如需宿主可达,执行方需先给 ingress 加 `/graphql` path(回写 yaml,DGO 铁律)或 `kubectl port-forward deploy/bff-java 8090:8090`。
- **鉴权**:JWT **RS256**(非 HS256),私钥在 K8s Secret `esp-rsa-jwt-sign`(`00b-rsa-jwt-secrets-split.yaml`),`JwtService.issue` 用 `Jwts.SIG.RS256` + `subject=userId`。
  - k6 无法在脚本内重签 RS256(无私钥),故**复用 BFF `/api/auth/register` + `/api/auth/login`**(AuthFilter 白名单免鉴权)拿真实 JWT,与生产链路一致(AuthFilter parse → `X-User-Id` 注入 → fetcher 透传下游算 `mine`)。
  - 每 VU 独立邮箱避并发冲突(register 幂等,已存在则 login 回落,见 `scenarios-baseline.js` 既有模式)。

## 4. Scenario 设计

两个 scenario **错峰串行**(单节点铁律:禁两 scenario 并行抢 VM CPU,`startTime: '150s'` 错开),各走相同阶梯负载:

| 档位 | VU | 时长 |
|---|---|---|
| 1 | 10 | 30s |
| 2 | 50 | 30s |
| 3 | 100 | 30s |
| 4 | 200 | 30s |
| 回落 | 0 | 10s |

- `graphql`(`exec: graphqlScreen`):1 次 `POST /graphql` 带聚合 query,记 `graphql_screen_duration_ms`。
- `rest`(`exec: restScreen`):5 次串行 GET,记 `rest_screen_duration_ms`(5 端点总耗时)。
- 业务场景统一:"学生首页一屏取数"(排行榜三件套 + 学情两件套)。

## 5. 指标与 Thresholds

- 自定义 Trend:`graphql_screen_duration_ms`、`rest_screen_duration_ms`(端到端一屏取数耗时,直接对比)。
- Counter:`auth_errors`、`graphql_errors`、`rest_errors`。
- k6 内置:`http_req_duration`(P95/P99)、`http_reqs`、`http_req_failed{screen:graphql|rest}`。
- Thresholds:
  - `graphql_screen_duration_ms p(95) < 3000ms`
  - `rest_screen_duration_ms p(95) < 5000ms`
  - `http_req_failed{screen:graphql} rate < 0.05`
  - `http_req_failed{screen:rest} rate < 0.05`
  - **核心对比**:GraphQL P95 应 < REST 5 端点串行总耗时 P95(聚合减 RTT 净收益)。

## 6. 预期与净收益判据

- **理论**:GraphQL 把 5 次客户端 RTT 压成 1 次,减少网络往返 + over-fetching。聚合深度越深(下游越多)收益越大。
- **代价**:GraphQL 加 schema 解析 + fetcher 编排开销;fetcher 内 `block()` 同步阻塞(GraphQL 同步语义,并发由 edge 层消化),下游慢则 GraphQL 放大长尾。
- **净收益判据**:
  - 若 `graphql P95 < rest P95` 且失败率均 < 5% → 聚合收益成立,推广 GraphQL。
  - 若 `graphql P95 ≥ rest P95` → 聚合收益被 schema 解析/同步阻塞开销抵消(下游 RTT 本就低或 bff WebClient 4s 超时未触发),需评估 fetcher 异步化(`@BatchMapping`/DataLoader)再推广。
  - 若 `graphql P99` 显著高于 `rest P99` → 同步阻塞放大长尾,印证 RWT 铁律(fetcher 应异步化)。

## 7. 单节点限制(HSA 如实记录)

- **压测机入集群(同 VM)**:单节点唯一正确姿势,VUS 上限 ≤200 防 OOM(见 `deploy/k6/README.md`)。压测机与被测同节点 → 测得时延含 VM 调度噪声,P95 门槛低于生产(3s/5s 非生产标尺)。
- **AI 长尾 HPA 跟不上**(esp-loadtest-probe-pitfalls):本脚本只打聚合读路径(rankings/review/weak,**非 AI 推理**),避开 AI 长尾;故不验证 HPA 弹性,仅测聚合层。
- **bff-java HPA**:`minReplicas:1, maxReplicas:4, CPU 70%`(15-bff-java.yaml)。单节点单 pod 起步,200 VU 可能触 HPA 扩容,但 HPA 滞后 → 本测主要看单 pod 聚合层时延,非容量上限。
- **下游依赖**:gamification-go / learning-java 必须先 Ready(`kubectl -n esp-prod get deploy`),否则 GraphQL/REST 全 5xx。执行前主线程先 `kubectl get pods -l app -n esp-prod | grep -E 'gamification|learning-java|bff'` 确认。
- **`/graphql` 未挂 ingress**:宿主不可达,必须集群内 k6 pod 执行(ClusterIP 直连 `bff-java:8090`)。

## 8. 执行步骤(主线程串行控制,本代理不执行)

```bash
# 0. 前置:宿主中间件 + bff/gamification/learning Ready
kubectl -n esp-prod get deploy bff-java gamification-go learning-java 2>&1 | grep -E 'NAME|1/1'

# 1. 集群内 k6 pod 执行(ClusterIP 直连,绕过 host port-forward)
kubectl -n esp-prod run k6-gql-vs-rest --rm -i --restart=Never \
  --image=grafana/k6:0.54.0 --command -- k6 run - < deploy/k6/graphql-vs-rest.js \
  2>&1 | tee logs/k6-graphql-vs-rest-$(date +%Y%m%d-%H%M).log | tail -40

# 2. 结果落 logs/,主线程只读摘要(tail -40)
```

执行后主线程汇总:`graphql_screen_duration_ms` vs `rest_screen_duration_ms` 的 P50/P95/P99 + 失败率,回写 `docs/PROGRESS.md`。

## 9. 关联铁律

- esp-loadtest-probe-pitfalls(压测机同节点 + AI 长尾 HPA 滞后)
- esp-singlenode-host-cluster-contention(主机重负载与集群操作串行)
- DGO(`/graphql` 若加 ingress 必须回写 yaml)
- HSA(单节点限制如实记录,P95 门槛非生产标尺)
- ASV(spring-graphql 2.0.4 API 核实 `/graphql` path 在 `http:` 下)
- LCO(本压测不含 LLM 调用,缓存铁律不适用)
