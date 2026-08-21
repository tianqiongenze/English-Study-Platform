# P1-D10 HPA / 容量阶梯压测方案

> RWT(微服务韧性接线铁律)· P1-D10。**本文件是准备阶段方案,不含执行结果**。
> 执行由主线程串行调度(单节点 VM 雪崩防护,见 `esp-singlenode-host-cluster-contention`)。
> 关联:`esp-loadtest-probe-pitfalls`(探针 timeout 误杀)· HSA(单节点限制如实)· BRR(爆炸半径最小化)。

## 1. 目标

验证 HPA 在阶梯负载下的扩缩容响应时间、失败率、AI 长尾影响。**单节点只验三件事**:
1. HPA 是否按阶梯触发扩容(CPU 超 target 60-70% → replicas 增长);
2. 扩容后失败率是否下降(扩容有效性的唯一可观测信号);
3. AI 长尾(>10s)占比随 CCU 变化(对标 `esp-loadtest-probe-pitfalls`:AI 长尾 HPA 跟不上)。

**不在单节点验证(DEFERRED 多节点)**:绝对吞吐、5000 CCU 真实承载、真水平扩展(HPA 扩到 N 副本但都挤单节点,CPU/内存共享物理资源,N 副本是"切片"非"倍增")。

## 2. 现状基线(已侦察,2026-08-21)

- **HPA**:19 个,`deploy/k8s/50-hpa.yaml` + 各服务 yaml。CPU target 60-70%,memory 75%(部分)。
  - maxReplicas 单节点 cap:3-6(原生产 8-20,`# Phase2:单节点 RD cap` 注释)。
  - learning-worker 已 3 副本(`21-learning-worker.yaml`),核心服务 1 副本起步。
- **前次实测**:533 rps,7.3% 失败,AI 长尾 HPA 跟不上(来源:任务背景)。
- **探针现状**(已读 `30-other-services.yaml`):timeout 已非 1s 误杀档——
  - writing-py(8082)/emotion(8084)/writing-coach(8085):readiness `timeoutSeconds: 3`,`failureThreshold: 3`。
  - teacher-java(8083):readiness `timeout 5s / period 30s`,liveness `timeout 15s`(已放宽容忍单节点 GC 抖动)。
  - **结论**:`esp-loadtest-probe-pitfalls` 的 1s 误杀坑已在历史修复,但 3s readiness 对 AI 推理长尾(writing/emotion 8-34s/次,见 `esp-lco-embedding-vm-snowball`)仍偏紧,压测期间需监控探针失败重启。

## 3. 压测方案

### 3.1 阶梯档位(CCU → 每档时长)

| 档 | 目标 CCU | ramp | hold(采 HPA) | 单档总时长 |
|---|---|---|---|---|
| 1 | 100 | 30s | 120s | 150s |
| 2 | 500 | 30s | 120s | 150s |
| 3 | 1000 | 30s | 120s | 150s |
| 4 | 2000 | 30s | 120s | 150s |
| 5 | 5000 | 30s | 120s | 150s |

- 总时长 ~12.5min。5000 档单节点很可能 OOM/CPU 饱和,**记录失败率拐点即停,不追求"通过"**。
- 每档 ramp 30s 线性爬升(非突发,与 spike.py 区分),让 HPA 有反应窗口。
- spawn-rate 上限 50/s(防单档过陡打垮 VM),环境变量 `SPAWN_RATE` 可调。
- 环境变量覆盖:`STAIR_USERS` / `STAIR_HOLD` / `STAIR_RAMP`(轻量化降档调试用)。

### 3.2 负载模型(复用现有)

复用 `locustfile.py` 的 4 个用户类(同全量基准条件,只换负载形态):
- `SpeechUser`(口语会话,~60% 在线 CCU 主体)
- `LearningEventUser`(学情事件 Disruptor 吞吐,G-06)
- `WritingUser`(写作批改 AI 长尾源)
- `TeacherUser`(教师端)

鉴权:复用 `mint_jwt` RS256(env `JWT_RSA_PRIVATE_KEY_PEM` 注入),与 gateway-go/bff 一致。

### 3.3 分布式 locust(对标 `deploy/locust/README.md`)

单节点 RD 不 apply `distributed-locust.yaml`(资源不足,该 yaml 顶部已注明)。单节点用单 locust-runner pod 跑;**5000 CCU 真分布式 = DEFERRED 多节点**(worker 调度到压测专用节点,物理隔离被测集群,见 yaml §nodeSelector 注释)。

### 3.4 运行(主线程串行调度,本准备阶段不执行)

```bash
# 终端1:HPA 快照采集(与 locust 同时起)
bash scripts/p1-d10-hpa-watch.sh

# 终端2:阶梯压测(locust-runner pod 内)
locust -f /locust/p1_d10_hpa_staircase.py --headless \
    --host http://traefik.kube-system.svc.cluster.local:80 \
    --csv=report/p1d10 --html=report/p1d10.html
```

## 4. 关键指标与验收门禁

| 指标 | 单节点目标 | 多节点目标(DEFERRED) |
|---|---|---|
| HPA 扩容触发延迟 | <60s(从 CPU 超 target 到 replicas+1) | <60s |
| 失败率 | 扩容后下降(无绝对阈值) | <1% |
| P99 延迟 | 记录拐点 CCU 档 | <5s |
| AI 长尾(>10s)占比 | 记录随 CCU 增长曲线 | <5% |

- **扩容触发延迟**算法:对照 `report/loadtest/hpa-snapshots.log`,同档内 REPLICAS 从 N→N+1 的时间戳减去该档 ramp 起点时间戳。
- **失败率拐点**:记录哪一档 CCU 失败率突升(预期 2000 或 5000 档,单节点 CPU 饱和)。这是单节点最有价值的产出——定位"单节点承载上限"。

## 5. 探针 timeout 调整建议(对标 `esp-loadtest-probe-pitfalls`)

| 服务 | 现状 readiness | 压测期建议 | 理由 |
|---|---|---|---|
| writing-py / emotion / writing-coach | timeout 3s | 调到 5s + failureThreshold 3→2 | AI 推理 8-34s/次,3s 探针在压测 GC 抖动下易误杀触发重启,重启反而加剧失败率 |
| teacher-java | timeout 5s / period 30s | 维持(已优化) | 已放宽容忍单节点抖动 |
| 非 AI 服务 | timeout 3s | 维持 | 无长尾,3s 足够 |

**注意**:探针调整属 L1 可逆变更,但**必须回写 yaml**(DGO 铁律)。压测期临时调需在 PROGRESS 记"待回写",压测后还原或固化。本准备阶段不改动 yaml,仅记录建议,执行阶段由主线程决定。

## 6. 单节点限制(HSA 铁律,如实)

1. **无法真测水平扩展**:HPA 扩到 N 副本挤单节点,CPU/内存共享 9 核物理资源,N 副本"切片"非"倍增",扩容后吞吐不会线性增长,失败率下降幅度有限。
2. **5000 CCU 档预期失败**:单节点无法承载 5000 CCU(前次 533rps 已 7.3% 失败),5000 档记录拐点即停,不代表生产不可达。
3. **压测机与被测同节点**:`distributed-locust.yaml` 明确要求物理隔离(压测专用节点 nodeSelector),单节点做不到,locust 本身吃 VM CPU 污染被测数据。**这是单节点压测数据的固有噪声,结论需打折**。
4. **HPA 扩容触发可验证**:这是单节点唯一能确证的——CPU 超 target 60-70% 时 replicas 是否增长(看 `hpa-snapshots.log`)。这才是 P1-D10 单节点核心交付。

**多节点 DEFERRED 清单**:分布式 locust(5 worker × 1000 CCU)、压测机物理隔离、5000 CCU 真实承载、绝对吞吐验收、HPA 跨节点调度延迟。落地条件:迁移多节点服务器后 apply `distributed-locust.yaml` + nodeSelector `role: loadtest`。

## 7. 风险与爆炸半径(BRR 铁律)

- **VM 雪崩风险**:5000 档 + locust 自身负载可能触发单节点 PLEG 雪崩(对标 `esp-crdb-probe-timeout-1s-pitfall` 事故链)。**主线程串行调度,不与编译/embedding 并行**(`esp-singlenode-host-cluster-contention`)。
- **回滚**:HPA 副本数由 HPA 自治回缩(流量回落后),无需人工回滚;探针临时调整压测后还原。
- **降级**:若 2000 档 VM load 飙升(load>50),主线程应中断 locust(对应 `esp-lco-embedding-vm-snowball` 雪崩恢复期纯等不加压原则)。

## 8. 交付物清单

- 方案文档:`report/loadtest/p1-d10-hpa-capacity-plan.md`(本文件)
- 阶梯压测脚本:`deploy/locust/p1_d10_hpa_staircase.py`(LoadTestShape 阶梯 ramp+hold,复用 locustfile 用户类)
- HPA 采集脚本:`scripts/p1-d10-hpa-watch.sh`(每 10s `kubectl get hpa` + `top pod` 到 `report/loadtest/hpa-snapshots.log`)

## 9. 执行后待补(执行阶段,非本准备阶段)

- `report/loadtest/hpa-snapshots.log`(实际采集)
- `report/p1d10_stats.csv` / `report/p1d10.html`(locust 输出)
- 扩容触发延迟实测表(每档)
- 失败率拐点 CCU 档结论
- 单节点承载上限结论 + 多节点 DEFERRED 确认
