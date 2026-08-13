# ESP 跨服务连调测试前并行优化方案

> 输入:`docs/design/架构评分与重构方向.md §0` 评分增量 + `docs/design/architecture/togaf-assessment.md §三` TOGAF 补救清单。
> 目的:把"评分派生的优化项 + TOGAF 补救项"转化为可在 **D 种子补灌完成后、统一跨服务连调测试之前**并行执行的落地方案。
> 原则:
> - **只读/小改优先**:补灌期不抢集群资源(单节点铁律),manifest 改动打包到补灌完成后批量 apply。
> - **可并行性明确标注**:每个工作包标注"是否可与补灌并行 / 是否需 scale 恢复 / 是否需部署"。
> - **最小可验证产出**:每项含改动文件、验证方式、预期结果(Done Definition)。
> 评估视角:大厂资深架构师,工业级基线。
> 日期:2026-08-12。

---

## 〇、执行总览(并行调度图)

```
阶段 1: 补灌期(learning_events v4 → 100M)—— 仅写文档,不动集群
  并行分支 A: TOGAF 治理文档补齐 (G1/A1/B1/B2/B3/C1/C3/D1/D2/D3/G2/H1)
  并行分支 B: 测试代码增量 (N-3 公共包覆盖率 / N-4 Pact 扩量 / N-6 安全测试回填)
  并行分支 C: 依赖升级与配置增量 (N-5 依赖升级 pom/go.mod / N-1 探针三分离 manifest / N-2 preStop)

阶段 2: 补灌完成、12 表核查达标 → scale 恢复 18 个 P0(小批,单节点铁律)

阶段 3: 统一跨服务连调测试 (此时探针三分离+preStop+Pact 全契约+安全回填+依赖升级已就绪)
  L1: 集群内单服务健康+(冒烟)
  L2: 跨服务 REST/Dubbo Triple/Pulsar 链路联调
  L3: 三端联调(Web/H5/小程序 → BFF → 下游)
  L4: 压测基线重测 (本阶段或单独阶段)
  L5: 安全测试 SEC-1~4 实跑
  L6: 混沌故障注入回归
```

---

## 一、阶段 1 工作包(补灌期并行,不动集群)

### 工作包 A:TOGAF 治理文档补齐(P0 最小可见集)

每个产出物均为新建 markdown,工作流:写 → 自审 → 入仓 docs/design/architecture/。

#### A-1 利益相关者诉求矩阵【A1】
- **文件**:`docs/design/architecture/stakeholder-concerns.md`
- **内容**:学员/教师/运营/法务/运维/SRE/财务 七维 × (核心诉求 / 影响力 high-med-low / 关键担忧 / 解决方案指引 / 验收 KPI)矩阵。
- **数据来源**:PDF 业务愿景 + SRS §2 + README + 现有 SAD §1 架构目标。
- **完成验证**:7 个角色行齐,每行至少 1 条诉求 + 1 条担忧 + 1 条方案;同事交叉审。
- **可并行补灌**:✅
- **预估**:0.5h

#### A-2 业务能力地图【B1】
- **文件**:`docs/design/architecture/business-capability-map.md`
- **内容**:5 大一级能力(口语陪练/自适应学习/写作批改/教师教研/情感多模态)× 二级能力分解(2-3 层),用 mermaid mindmap 或表格,标注哪些服务实现该能力。
- **数据来源**:SAD §3 DDD 服务划分 + SRS §3 功能需求。
- **完成验证**:覆盖现有 27 服务工程的能力归属,无 orphan 服务(每服务都能归到能力树下)。
- **可并行补灌**:✅
- **预估**:1h

#### A-3 价值流图(3 条核心)【B2】
- **文件**:`docs/design/architecture/value-streams.md`
- **内容**:① 学员一次完整学习闭环 ② 一次写作批改闭环 ③ 一次教师教研闭环;每条价值流:trigger → stage 列表 → 每个 stage 的入口/产出/执行者/价值增量。
- **数据来源**:SRS §4 用例 + SDD 各服务章端到端流程注释。
- **完成验证**:3 条价值流图齐,与 SRS §4 UC 对应;每 stage 标注同步 REST/异步 Pulsar/WS 类型。
- **可并行补灌**:✅
- **预估**:1h

#### A-4 业务事件目录【B3】
- **文件**:`docs/design/architecture/business-event-catalog.md`
- **内容**:Pulsar 5 topic(speech.session/learning.event/writing.task/emotion.signal/teacher.task)+ REST 接口(11 服务 43 路由 from openapi-go-rust.yaml)+ Dubbo Triple 接口(esp-api 包列出的接口)三类的全量登记表:名称/producer/consumer/语义/Schema/SLA/幂等性/重试策略。
- **数据来源**:SAD §7 通信与契约 + bff API Portal(226 行 OpenAPI)+ esp-api Dubbo interfaces + ADR-new-services。
- **完成验证**:5 Pulsar topic + 43 REST 路由 + N 个 Dubbo 接口条目齐。
- **可并行补灌**:✅
- **预估**:0.5h

#### A-5 应用-数据矩阵【C1】
- **文件**:`docs/design/architecture/app-data-matrix.md`
- **内容**:11 业务服务行 × 12 张业务表列,每格标 C/R/U/D + Schema owner(对应 schema-per-service);并识别跨域写反模式(若某服务写非自己 schema)。
- **数据来源**:schema-per-service.sql(0/1)+ SDD 各章 + seed_3b.py 的 TARGETS。
- **完成验证**:矩阵无空行;Owner 列与 schema-per-service 对应;识别异常 = 待修记录。
- **可并行补灌**:✅
- **预估**:1h

#### A-6 ER 三层图【C3】
- **文件**:`docs/design/architecture/er-diagrams.md`
- **内容**:概念层(学员/教师/学习内容/会话/事件 5 大主实体关系)、逻辑层(11+ 业务实体的 cardinality 1:1/1:N/N:M)、物理层(直接引 schema.sql)三层 ER 图,mermaid erDiagram。
- **数据来源**:schema.sql + schema-per-service.sql。
- **完成验证**:三层图齐;N:M 关系(如 user×knowledge_point 的 user_mastery)正确标关联表。
- **可并行补灌**:✅
- **预估**:1h

#### A-7 多节点目标拓扑 + 容量规划 + SLO 矩阵【D1/D2/D3 合并】
- **文件**:`docs/design/architecture/multi-node-topology.md` + `capacity-plan.md` + `sla-slo-matrix.md`(三份可合一节或三份)
- **内容**:
  - **拓扑**:3 地域(AWS us-east-1 / Azure / GCP,见 SAD infra-reuse)× 多 AZ,TiDB PD 跨域 Raft、Redis Cluster 各区独立不强同步(ADR-redis-cluster-migration)、Pulsar Geo Replication。
  - **Capacity Plan**:5000 CCU 目标下所需节点数、每节点规格(8C16G 起)、HPA min=2/max=10、PDB minAvailable=1、扩缩容阈值(CPU 70%、CCU、队列深度)。
  - **SLO 矩阵**:per 组件 RPO/RTO/Availability 目标(TiDB 99.95% RPO 0 RTO 5min、Redis 99.9% RPO 0、Pulsar 99.95%、k8s API 99.9%、ArgoCD 99%、Prometheus 99.5%)。
- **数据来源**:多区域数据同步方案.md + pulsar-geo-replication-plan.md + redis-cluster-migration-plan.md + SAD §11。
- **完成验证**:有拓扑图、capacity 表、SLO 表;说明单节点→多节点切换标准(同 E1 过渡架构)。
- **可并行补灌**:✅
- **预估**:2h

#### A-8 架构治理委员会章程 + ADR 索引【G1/G2】
- **文件**:`docs/design/architecture/architecture-board-charter.md` + `adr-index.md`
- **内容**:
  - **章程**:成员(用户=首席架构师/CPO + 多语言代表 + SRE + 安全 + 数据)、职责、节奏(双周)、决策权、否决权、ADR 流程、合规模板。
  - **ADR 索引**:7 现有 ADR 编号甲起(ADR-01~07)+ 规则(requirement 背景举证、ADR-XX 编号永久不给出)。
- **可并行补灌**:✅
- **预估**:0.5h

#### A-9 技术债务登记【H1】
- **文件**:`docs/design/architecture/tech-debt-register.md`
- **内容**:从 PROGRESS "遗留项"、本文档 §0 滞后纠正、esp-cache vs esp-cache-py 契约同步、单节点容量、Pact 仅覆盖 1 对、38 trivy vulns 中提取债项 × (优先级 P0/P1/P2 / 估值人日 / 偿还时间 / 状态)。
- **完成验证**:债项 15+ 条,有优先级 + 估值 + 计划。
- **可并行补灌**:✅
- **预估**:0.5h

**工作包 A 合计**:~8h,在补灌完前可完成(短 dependence loop,纯写)。

---

### 工作包 B:测试代码增量(只写代码不部署)

#### B-1 公共包严格覆盖率量化【N-3】
- **文件**:
  - `services/esp-disruptor/src/test/java/.../*Test.java`(补单测,核心 RingBuffer tryNext/published/dropped 覆盖)
  - `services/esp-rw-java/src/test/java/...`(`RoutingHolder` ThreadLocal 路由、`@DataSource(consistent=true)` 走主库用例)
  - `services/esp-cache/src/test/java/...`(L1+L2 失效广播)
  - `services/esp-db-py/tests/`(读写分离、read_timeout 边界)
  - `services/esp-cache-py/tests/`(L1+L2)
- **完成验证**:jacoco/pytest-cov 跑通,覆盖率 ≥ 80%,生成报告落 `report/coverage/`。
- **可并行补灌**:✅(只写代码不部署,不抢集群 CPU)
- **预估**:中(1-2h/component × 5 component)

#### B-2 Pact 契约测试扩展全 11 路由【N-4】
- **文件**:`services/integration-tests/src/test/.../pact/*.java`(consumer)pact + 各服务 provider side `@PactVerify` 端点扩展,从 teacher-learning 一对扩到全 11 路由。
- **完成验证**:CI `contract` job 全绿,生成 pact json 落 `target/pacts + pacts/`(分流)。
- **可并行补灌**:✅
- **预估**:1.5h/对 × 11 对 ≈ 大,分批做

#### B-3 安全测试 SEC-1~4 执行回填【N-6】
- **文件**:
  - `docs/test/06-安全测试报告.md`(回填 SEC-1 WS 越权 / SEC-2 过期 JWT 401 / SEC-3 X-User-Id 伪造剥离 / SEC-4 镜像 secret 扫描,每项从 `_` 改为实测结果)。
- **依赖**:需 scale 回服务后跑(WS 越权需实跑 ws),所以**阶段 3 执行**,文档模板可在补灌期先准备好。
- **可并行补灌**:✅(仅文档模板) / 阶段 3 实跑
- **预估**:0.5h 模板 + 1h 实跑

---

### 工作包 C:依赖升级与配置增量(写不部署)

#### C-1 依赖 CRITICAL/HIGH 升级【N-5】
- **文件**:
  - Java pom.xml:`io.netty 4.2.15.Final → 4.2.16.Final`、`jackson 2.19.4 → 2.21.4`、`bouncycastle 1.78.1 → 1.84`(CRITICAL CVE-2025-14813 优先)。
  - 前端 package.json:`swiper 11.1.15 → 12.1.2`(CRITICAL CVE-2026-27212)、`react-router 7.18.2 → 8.3.0`。
- **完成验证**:`mvn package` 各 Java 服务 RC=0、`npm run build` 前端 RC=0,rebuild 镜像(在补灌完后)apply。
- **可并行补灌**:✅(只改 pom/package.json;**build 镜像阶段要在补灌完之后,避免 mvn/docker 星 CPU**)
- **预估**:中(每升级可能 API 兼容性需微调)
- **风险**:升级 breaking change,jackson 2.21 或 react-router 8 可能要改代码,需要 build 验证不卡壳。

#### C-2 探针三分离 manifest【N-1】
- **文件**:`deploy/k8s/*.yaml` 涉及服务,主要为高负载/长连接服务:gateway-go(WS)、learning-java(Disruptor)、teacher-java、writing-py(LLM 长尾)、learning-worker、emotion-svc、bff-java。
- **改动**:每服务加 `startupProbe`(initialDelaySeconds=0,periodSec=10,failureThreshold=30 → 给慢启动 5min)+ `livenessProbe` 不变、`readinessProbe` 不变(ipv 鼠标推荐 timeout≥3s period≥15s failureThreshold≥5 已做)。
- **完成验证**:manifest diff dry-run `kubectl diff -f` 通过。
- **可并行补灌**:✅(只改文件,apply 在阶段 2 末批量)
- **预估**:小(~20 min)

#### C-3 preStop graceful shutdown【N-2】
- **文件**:
  - gateway-go:`deploy/k8s/10-gateway-go.yaml` 加 lifecycle preStop(`sleep 15` 给 WS drain);gateway-go 源码确认有 `WS.closeAll()`/`hub.unregister` flow(若没有需小补);
  - learning-java:`deploy/k8s/20-learning-java.yaml` 加 preStop + terminationGracePeriodSeconds=45(给 Disruptor drain 时间);learning-java 确认 `@PreDestroy` 已挂 Disruptor shutdown(会话22 ADR-01 应已做)。
- **完成验证**:rollout 测试 pod 优雅退出无 502(阶段 3 验证)。
- **可并行补灌**:✅(写文件)
- **预估**:小(~30 min)

---

## 二、阶段 2:scale 恢复 18 P0 业务服务(补灌完成、12 表达标后)

> 单节点铁律补灌期不可 scale,补灌完成后逐批 small-batch scale(singleton stability 原则),每批 healthz 验证后再下一批;每批后 1 min 稳定观察,**内存 headroom** ≥ 20%。

**scale 顺序**(按依赖 + P0 优先):
1. infra-base:nacos(已 Running)/ seata / sentinel 保持;N/A 不 scale。
2. **批次 1 - 接入层**:bff-java → gateway-go(2 个)。
3. **批次 2 - 业务面 Java**:learning-java → teacher-java(2 个)。
4. **批次 3 - Rust 4**: pronunciation-rs / assessment-rs / llm-orchestrator-rs / media-learn-rs(P1 保持 replicas=0)。
5. **批次 4 - Go 7**:course-path-go / listening-go / reading-go / gamification-go / live-tutor-go / vocab-srs-go(6 个,gateway 已在批次1)。
6. **批次 5 - Python AI**:writing-py / emotion-svc / writing-coach(3 个)。
7. **批次 6 - 前端**:frontend-react / frontend-h5(2 个)。
8. **批次 7 - worker/:learning-worker / learning-java 异步消费(复用主服务 replica)。

**前端 batch 顺序设计原则**:接入先于业务,业务先于 P1,Python AI 因 LLM 长尾排后(动态资源),前端最后(纯静态 nginx)。

每批 healthz 验证命令:
```bash
for svc in <batch_services>; do
  kubectl -n esp-prod rollout status deploy/$svc --timeout=180s
done
# 集群内冒烟(用 listening-go pod 串行 wget):
kubectl -n esp-prod exec deploy/listening-go -- sh -c 'for u in <healthz_url_list>; do echo $u $(wget -qO- -T3 $u); done'
```

> 单节点 scale 失败(任何 pod CrashLoop)立即停跳过,Taillogs 而不是继续推下一批。

---

## 三、阶段 3:统一跨服务连调测试清单

> 前置:阶段 1 工作包 A 文档已交付,阶段 2 scale 恢复 18 个 P0 全 1/1,探针三分离 + preStop + Pact 全契约(本阶段扩展版)+ 安全回填 + 依赖升级已就绪。此时跨服务连调基础最稳。

详细执行见 `docs/test/09-人工测试操作指南.md`(以下只列与优化方案对接的连调测点):

| L | 测点 | 验证 | 关联优化 |
|---|---|---|---|
| L1 | 单服务健康+冒烟(18 服务 healthz) | 每服务返回 200(或对应路径) | C-2 探针三分离 |
| L2 | 跨服务 REST 链路(bff-java→gateway / learning / teacher / writing) | 调用一条真实业务流(如 listScenarios),全链 200 | A-4 业务事件目录对照 |
| L3 | 跨服务 Dubbo Triple(teacher-java→learning-java) | teacher 学情汇总调学习,正确返回数据 | A-4、A-5(应用-数据矩阵) |
| L4 | 跨服务 Pulsar 事件(learning.event 下游 worker 落库) | 发一学习事件,worker 字段落库,延迟 < 1s | A-4(pulsar topic) |
| L5 | 三端 trace 贯通(Web/H5/小程序 → BFF → gateway → DB) | Tempo 命中 traceId,前后端 traceparent 串联 | 旧评分 §一 8.5 可观测,断点 6 已实证 |
| L6 | WS 口语陪练会话(gateway-go WS→ redis 会话状态) | 完整一次会话(发起点→ 多轮 → 结束),双 pod 互发广播 ok | P2-1 WS 分片 hub shard=8 |
| L7 | Disruptor 学习链路(learning-java 发布 RingBuffer→worker 落库) | learning 发进 RingBuffer 字段立刻在 DB 可见 | ADR-01、P2-2 worker 拆分 |
| L8 | writing-py LangGraph 写作闭环(Claude Mock 模式批改→润色→笔记) | 调 /api/writing/run 返回 thread_id,最终 notes 出 | ADR-18、P1-3 langgraph 1.x |
| L9 | 三端 Profile 会话历史下拉(Web/H5/小程序) | 显示历史数据行(种子 sessions 50M 抽样) | 会话22-5 frontend sessions/history |
| L10 | 优雅停机验证(rollout gateway-go / learning-java) | rollout 期间 WS 不挂、Disruptor 不丢字段 | C-3 preStop drain |

---

## 四、并行可行性总览矩阵(回答用户"可并行则并行")

| 工作包 | 阶段 | 可与补灌并行 | 可与 scale 恢复并行 | 内部子项并行 | 备注 |
|---|---|---|---|---|---|
| A-x (TOGAF 9份文档) | 1 | ✅ | - | ✅ | 纯写,9 份之间相互独立 |
| B-1 公共包覆盖率 | 1 | ✅ | - | ✅ | 5 component 独立 |
| B-2 Pact 扩量 | 1 | ✅ | - | ✅ | 11 对独立扩展 |
| B-3 安全测试文档 | 1 部分 | ✅ | - | - | 实跑在阶段 3 |
| C-1 依赖升级 | 1 改文件 | ✅ | - | ✅ | 多 module 独立;build 在补灌后 |
| C-2 探针三分离 manifest | 1 | ✅ | - | ✅ | per 服务独立 |
| C-3 preStop manifest | 1 | ✅ | - | ✅ | per 服务独立 |
| 阶段 2 scale 恢复 | 2 | ❌ | - | ⚠ 小批串行 | 单节点铁律串行 |
| 阶段 3 L1-L10 连调 | 3 | - | - | ⚠ 大部分可并行子项 | 部分依赖前置(L1 → L2) |

**并行最大集**:补灌期 同时可跑 9 份 TOGAF 文档 + 5 公共包测试 + 11 Pact 对扩展 + 5 依赖升级 + 7 探针改 manifest + 2 preStop = 理论 30+ 独立子项可并行(subagent 派发)。考虑 subagent timeout / 上下文 / 单节点 buildkit 资源,**实际并行度建议 4-6 并发**(单节点资源限制 subagent 不能同时拍 mvn/docker)。

---

## 五、关键风险与回滚

| 风险 | 触发条件 | 缓解 | 回滚 |
|---|---|---|---|
| 补灌 v4 在 learning_events 再次 ERR 中断(坑㉑ 第4次) | THROTTLE 0.5 + 10 retry 仍不够 | 接受实灌量,标记 PROGRESS "单节点如实记录",多节点 Phase D2 落地后重灌 | kubectl delete job → 接受当前行数,跑阶段 2 |
| 依赖升级破坏构建(netty 4.2.16 API 变/jackson 2.21 serialize 变) | C-1 build 失败 | 分模块升级 + 先 mvn package 验证再推 build 镜像 | git checkout pom.xml 回滚 |
| 探针三分离 manifest apply 后 startupProbe 配置错 → pod 启不来 | C-2 | apply 前用 `kubectl diff` 校验 | `kubectl rollout undo deploy/<svc>` |
| scale 恢复后单节点 OOM(坑⑰ 雪崩复发) | 批次过密 | 单节点铁律小批,每批 ≥ 1min 观察,内存 headroom < 20% 立即停 | `kubectl scale --replicas=0` 当批次 |
| Pact 扩量破坏现有 contract job CI 红 | B-2 | 新加 consumer 测试放主分支前先本地 mvn verify | 注释掉失败 consumer,提交 issue |
| preStop sleep 时间不够,rollout 仍 502 | L10 验证 | sleep 15 → 30,terminationGracePeriodSeconds 45 → 60 | 回滚 lifecycle |

---

## 六、本次优化的 Done Definition

阶段 1 完成(D 种子补灌仍在跑):
- [ ] docs/design/architecture/ 多 9 份文档:togaf-assessment.md(已 ✅) + stakeholder-concerns.md + business-capability-map.md + value-streams.md + business-event-catalog.md + app-data-matrix.md + er-diagrams.md + multi-node-topology.md(+ capacity/slo) + architecture-board-charter.md + adr-index.md + tech-debt-register.md
- [ ] 公共包单测 ≥80% jacoco/pytest-cov 报告落 report/coverage/
- [ ] integration-tests 新增 Pact consumer/provider,全 11 路由
- [ ] 06-安全测试报告.md 回填 SEC-1~4(模板就位,实跑暂留阶段 3)
- [ ] Java pom.xml 依赖升级(bouncycastle/netty/jackson)+ 前端 package.json(swiper/react-router)
- [ ] deploy/k8s 多 manifest 加 startupProbe + lifecycle preStop(diff 已 dry-run)

阶段 2 完成(补灌完 + 12 表达标):
- [ ] 18 个 P0 业务服务全 1/1 Running,无 CrashLoop
- [ ] 每批 healthz 验证记录
- [ ] 12 表最终行数 ≥ TARGETS(sessions 50M / session_turns 50M / learning_events 100M / 其余按 SRS)
- [ ] 镜像 rebuild + apply(依赖升级后)

阶段 3 完成(连调全绿):
- [ ] L1-L10 全绿(详 09-人工测试操作指南)
- [ ] SEC-1~4 安全测试实跑回填
- [ ] git commit push 闭环(模式⑩)

---

## 附:与已有评分/重构方向的对接关系

- 本方案是对 `架构评分与重构方向.md §0.5` N-1~N-7 优化项的**详细落地执行规划**(每项扩为可派 subagent 的工作包)。
- TOGAF 治理文档 A-1~A-9 是对 `togaf-assessment.md §三` P0 补救清单的转化。
- 跨服务连调测点 L1-L10 是对 `docs/test/07-人工测试用例.md` 冒烟骨架的扩展,完整可执行版见 `docs/test/09-人工测试操作指南.md`。
