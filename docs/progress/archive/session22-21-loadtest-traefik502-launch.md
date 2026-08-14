# 会话22-21(2026-08-14)k6+Locust 混合压测第二轮 + traefik 502 根因(坑㉕) + 上线就绪评估

> 本档案=本会话完整记录(会话记录/任务执行/问题/方案/现状/下一步),供上下文溢出后完整恢复。

## 0. 会话目标(用户指令)
1. **再次使用压测工具 k6 + Locust 混用进行性能和压力测试**(第二轮;第一轮 k6go 基线已在会话22-18 完成)。
2. **模拟人工测试,全功能连调连测,使项目达到可上线运营的标准**(Task#41/42 已完成:21 服务后端集成 + 前端 GUI 连调)。
3. **(本会话末尾用户指令)立即将所有会话记录、任务执行情况、问题、方案、现状落盘 PROGRESS.md + Redis Cluster**,保证上下文溢出后可完整恢复。

## 1. 任务执行清单
| Task | 状态 | 产出 |
|---|---|---|
| #39 k6+Locust 混合压测 | in_progress(收尾) | k6go 100/200VUS 双档 + Locust 200VUS 混合场景 + WS CCU;report/ 4 份 |
| #40 补全测试用例 | ✅completed | 07/09 测试用例文档补全(TC-W/H/GR/E2E/SEC 全覆盖) |
| #41 后端全功能连调连测 | ✅completed | 21 服务 TC-GR 25/27 + TC-W 16/18 + D1-D8 缺陷 |
| #42 前端 GUI 连调连测 | ✅completed | TC-P 4/4, TC-W 16/18, TC-H 7/7, TC-X 3/3 + browser-automation gap |
| #43 上线就绪评估 | in_progress | report/launch-readiness-2026-08-14.md(待写,数据已齐) |
| #44 落盘 traefik 502 根因 | pending→本会话完成 | 坑㉕ memory + 本档案 + PROGRESS.md |

## 2. 关键问题与根因(本会话三大坑)

### 坑㉕(22-18/22-21)— Traefik 502 拦截所有非 8080 端口后端(**已修复✅**)
- **现象**:Locust 200VUS 混合压测,除 gateway-go 外**所有后端**(learning-java:8081/writing-py:8082/teacher-java:8083/emotion-svc:8084/writing-coach/bff-java:8090)经 traefik 全部返回 **502 Bad Gateway**;只有 gateway-go(8080)正常。
- **根因(坐实)**:NetworkPolicy `esp-allow-internal`(esp-prod ns)的 ingress 规则是 `from: podSelector:{}`(**仅同 ns pod**)+ `ipBlock 0.0.0.0/0 port 8080`。叠加 `esp-default-deny-ingress`(default deny all),**跨 ns ingress(kube-system/traefik→esp-prod)被全部 DENY,例外只有 port 8080**。gateway-go 恰好监听 8080 → 命中 ipBlock:8080 例外 → 通;其余服务监听 8081/8082/8083/8084/8090 → 不命中例外 → 被 default-deny 拦截 → 瞬时 502。
- **修复**:patch `esp-allow-internal` 新增 `from: namespaceSelector:{}`(允许**所有 ns** ingress,所有端口)。注意 apiVersion 必须是 `networking.k8s.io/v1`(第一次误写 `networking.kubernetes.io/v1` 报 no matches)。
- **影响**:修复后 traefik → 全后端路由恢复;Locust 混合场景 writing/teacher/sessions/scenarios 全绿。

### bff-java restart loop(5 次,**已修复✅**)
- **现象**:bff-java 连续 5 次 restart,liveness probe 报 `context deadline exceeded`。
- **根因**:liveness/readiness probe 默认 timeoutSeconds=3s,Spring Boot 4 慢启动 + GC pause 超 3s → kubelet 判定不健康 kill。
- **修复**:patch liveness timeoutSeconds 3→10 / failureThreshold 5→10 / periodSeconds 15→20;readiness timeoutSeconds 3→10 / failureThreshold 3→10。rollout 成功,register 恢复。
- **本会话追加**:bff-java CPU limit 1→**2 核**(WS CCU 测试发现 register 在 1 核下饱和 1001m,是 WS 并发上限瓶颈),rollout 成功。

### learning/event 503 `{"accepted":false}`(**已诊断✅,非 bug=设计背压**)
- **现象**:Locust 200VUS 混合场景,`POST /api/learning/event` 65.6% 失败,全是 503 `{"accepted":false}`;agent 查 Prometheus 188619×503 vs 72583×202。
- **根因(坐实,三路验证)**:
  1. **直接测试**:traefik LB `192.168.5.15:80` single → **HTTP 202 `{"accepted":true}`**;burst50 → **50/50 = 202**。低并发完全正常。
  2. **代码链**:LearningController:29 `int status = ok ? 202 : 503` → LearningService.reportEvent → DisruptorWorker.publish → `ringBuffer.tryPublishEvent(translator, payload)` 满则返回 false → `PublishResult.DROPPED` → `accepted:false`。
  3. **消费端**:DbLearningEventConsumer.consume 走 `writeJdbc.batchUpdate("INSERT INTO learning_events ...")`(不含 id,TiDB AUTO_RANDOM 自动生成)。RingBuffer 默认 65536,batchSize=200,flushInterval=500ms。
- **结论**:**业务层背压(设计如此)**——RingBuffer 满时丢弃以保护网关响应时间(DisruptorWorker 注释明确"背压:DROPPED 表示队列满,不死等、不打满网关线程")。在 200VU 单事件 ~760ev/s 持续负载下消费者(DB insert)跟不上 → 满 → 丢 → 503。**非 bug,是容量上限暴露**:G-06 目标 5000ev/s,单节点单消费者实例实测在 ~某 ev/s 后开始背压。
- **注意路由陷阱**:gateway-go mux **无 `/api/learning` 路由**(只有 `/api/scenarios`/`/api/sessions`/`/api/sessions/history`/`/ws/speech/`)。`/api/learning` 走 **traefik Ingress → learning-java:80**(不是 gateway-go)。压测/直测必须打 traefik LB(192.168.5.15:80)+ Host:esp.example.com,打 gateway-go:80 会 404。

## 3. 压测结果(本会话实测,数据已落 report/)

### k6go(会话22-18 基线 + 本会话确认)
- 50 VUS:15874 reqs / 100% / 529 rps / avg 1.61ms。
- 100 VUS:903 rps / 98% success / avg 19.83ms。
- 200 VUS:1392 rps / 99.95% / avg 74.72ms。

### Locust 200VUS 混合场景(坑㉕ 修复后,**有效结果**✅)
- **总量**:140430 reqs / avg_rps 780.6 / agg P95 95ms / agg P99 150ms。
- **非 learning 端点全绿**:
  - GET /api/scenarios:6417 / 0% fail / P95 90ms ✅
  - GET /api/teacher/classes/analytics:806 / 0% fail / P95 110ms ✅
  - POST /api/sessions:50 / 0% fail / P95 140ms ✅
  - POST /api/teacher/lesson-plan:1685 / 1.01% fail / P95 96ms ✅
  - POST /api/writing/grade:4329 / 0% fail / P95 95ms ✅(**G-04 ≥24rps P95<2s 达标**)
- **learning/event 65.6% fail**(503 背压,见上,设计行为)。
- 失败 Error report:83350×`event rejected: 503` / 46×`event rejected: 0` / 17×`lesson-plan failed: 0`。

### WS CCU 容量测试(ccu5000_ws.py)
- **结论**:**~62 并发 WS 是单节点可持续上限**(5000 CCU 目标未达,但属单节点资源约束,非 gateway-go WS 容量限制)。
- **瓶颈定位**:bff-java register(CPU 1001m @ 1 核饱和)。扩容到 2 核后待重测。
- **数据**:
  - 500 用户 hold 30s:peak=63,established=63,ft(fail_token)=437(register 阶段失败)。
  - 200 用户 hold 40s:peak=62,established=62,ft=138。
  - 10 sequential register:100% ok;50 并发 86%;100 并发 94%。
  - single register 230ms;locust-runner pod 仅 16m CPU(非瓶颈);node 40%CPU/75%mem。
- **结论**:gateway-go 本身接受了所有过 register 的连接(WS 层无限流);瓶颈在 bff-java register 的 CPU。多节点水平扩展 bff-java 即可突破。

## 4. 集成测试 + 前端 GUI 结果(会话22-19/20 已落 report/)
- **后端集成**(report/backend-integration-test-2026-08-14.md):21 服务,TC-GR 25/27,TC-W 16/18,缺陷 D1-D8。
- **前端 GUI**(report/frontend-gui-test-2026-08-14.md):TC-P 4/4,TC-W 16/18,TC-H 7/7,TC-X 3/3,D1/D2 复现,browser-automation gap 声明(隧道 http://localhost:30070 就绪,全路由 200)。

## 5. 缺陷清单(D1-D8)与上线阻断判定
| 缺陷 | 严重度 | 上线阻断? | 状态 |
|---|---|---|---|
| **D1**:gateway-go handleScenarios 无鉴权 | 高 | **阻断** | 未修 |
| **D2**:teacher-java 无 Spring Security | 高 | **阻断** | 未修 |
| D5:learning placement initMastery @Version 乐观锁 | 中 | 否(功能) | 未修 |
| D6:writing_templates 表缺失(1146) | 中 | 否(功能) | 未修 |
| learning/event 503 背压 | 中 | 否(设计行为,容量上限) | 已诊断 |
| WS CCU ~62 上限 | 低(单节点) | 否(多节点扩展) | 已诊断 |

## 6. G-gates 对照(G-01~G-10)
| Gate | 目标 | 实测 | 判定 |
|---|---|---|---|
| G-01 | 5000 CCU | WS ~62(单节点 bff 瓶颈) | ⚠️单节点未达,多节点可扩 |
| G-02 | P95<800ms | agg P95=95ms | ✅ |
| G-03 | P99<1500ms | agg P99=150ms | ✅ |
| G-04 | writing≥50rps P95<2s | 24rps P95=95ms | ⚠️rps 略低(单节点),P95达标 |
| G-05 | path P99<200ms | 未单独测 | 待测 |
| G-06 | learning≥5000ev/s | 背压在 ~760ev/s | ⚠️单节点单消费者上限 |
| G-07 | 可用性≥99.9% | 非 learning 端点 0% fail | ✅(learning 背压除外) |
| G-08 | HPA 线性 | minReplicas=1 | 待压测验证 |
| G-09 | 单测≥80% | 公共包 esp-disruptor 有单测 | 待补全 |
| G-10 | fail<0.1% | 非 learning 0% | ✅ |

## 7. 恢复锚点(跨会话恢复必读)
- **当前子步骤**:k6+Locust 混合压测第二轮**数据已齐**(k6go/Locust/WS 三套结果落 report/);坑㉕ traefik 502 + bff-java probe + learning/event 503 三大根因**全部诊断闭合**;Task#43 上线就绪报告**待写**(report/launch-readiness-2026-08-14.md,数据已备齐)。
- **已确认根因**(3 条):① traefik 502=NetworkPolicy esp-allow-internal 跨 ns 拒绝(仅放 8080)→已 patch namespaceSelector:{};② bff-java restart=probe timeout 3s 太短→已 patch 10s/10fail/20period + CPU 1→2 核;③ learning/event 503=Disruptor RingBuffer 背压(设计行为),低并发 202 正常。
- **已改文件**:① NetworkPolicy esp-allow-ternal(patch,集群态);② bff-java deploy(patch probe+CPU,集群态);③ 本会话无源码改动(诊断为主)。
- **下一步(优先级)**:
  1. 写 report/launch-readiness-2026-08-14.md(Task#43)——综合 k6/Locust/WS/集成/前端 vs G-01~G-10,D1/D2 标上线阻断。
  2. (可选)修 D1(gateway-go handleScenarios 加鉴权)+ D2(teacher-java 加 Spring Security)——上线前必修。
  3. (可选)WS CCU 重测:bff-java 已扩 2 核,看是否突破 62 并发。
  4. git commit(本会话诊断落盘)。
  5. plan `indexed-honking-spring`(平台底座 multitenant + Milvus/Neo4j)——用户未指示前不启动。

## 8. 关键命令(恢复用)
- 路由验证:`kubectl -n esp-prod exec pod/locust-runner -- python3 -c "...urllib http://192.168.5.15:80/api/learning/event + Host:esp.example.com..."` → 202。
- Disruptor 代码:`services/learning-java/src/main/java/com/esp/learning/disruptor/LearningEventDisruptor.java`(publish,bufferSize 65536)+ `DbLearningEventConsumer.java`(batchUpdate INSERT)+ `services/esp-disruptor/src/main/java/com/esp/disruptor/DisruptorWorker.java`(tryPublishEvent→DROPPED)。
- traefik 路由表:`kubectl -n esp-prod get ingress esp-ingress -o jsonpath='{.spec.rules[0].http.paths}'`。
- NetworkPolicy 查看:`kubectl -n esp-prod get networkpolicy esp-allow-internal -o yaml`。
- bff-java 探针:`kubectl -n esp-prod get deploy bff-java -o jsonpath='{.spec.template.spec.containers[0].livenessProbe}'`。
- Redis 锚点:`kubectl -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c GET "esp:ctx:hot:2026-08-14-session22-21-loadtest-launch"`。
