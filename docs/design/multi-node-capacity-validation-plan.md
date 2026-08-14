# 多节点容量验证方案(G-01/G-04/G-06 多节点达标验证)

> 定位:单节点 Rancher Desktop 已验证拓扑与功能,本方案给出**多节点集群上的分层容量验证**执行方案,目标证明 5000 CCU 端到端可达。
> 数据来源:单节点实测产物(`report/ws-ccu-retest-2026-08-14*.json`、`report/launch-readiness-2026-08-14.md`、`report/soak-summary.md`、`report/chaos-rto.txt`)+ 目标拓扑(`docs/design/architecture/multi-node-topology.md` D1/D2/D3)。
> 日期:2026-08-15。评估视角:教授级高工 / 大厂架构师(工业级方法论,项目尺度 5000 CCU)。

---

## 1. 单节点实测基线(外推锚点)

> 本节全部为**实测数字**,来源标注到文件,是多节点容量模型的唯一合法锚点。凡未在本节出现的数字,后文一律标注"假设"。

| 指标 | 实测值 | 来源 |
|---|---|---|
| WS 并发(bff 1 核) | ~62 并发(register 串行 ~230ms 瓶颈) | `report/ccu5000_ws_result.json` + launch-readiness §1.3 |
| WS 并发(bff 2 核补丁后) | **≥372 持续并发**(target 500,established 436,dropped 64,sustain 74.4%) | `report/ws-ccu-retest-2026-08-14-tier2.json` |
| WS 并发(target 200) | 158 并发(sustain 79.0%) | `report/ws-ccu-retest-2026-08-14-tier1.json` |
| 2 核 bff CPU 峰值 | ~1.3 core(密集建连突发,CPU 未饱和,瓶颈在 register 会话速率/线程模型) | `ws-ccu-retest-2026-08-14.json` verdict |
| 混合压测吞吐(200VU) | 140,420 reqs / avg 780.6 rps | launch-readiness §1.2 |
| 混合压测延迟 | agg P95=95ms / P99=150ms(读路径) | 同上 |
| 单节点全量上限 | 533 rps / P95 25s / 7.3% 失败 | `multi-node-topology.md` §2 引用 5000 CCU 实压 |
| writing 单实例 | 24 rps P95=95ms(此前口径 47 rps P95 73s) | launch-readiness G-04 |
| learning/event 背压 | ~760 ev/s(503 accepted:false,Disruptor 单消费者;另一口径 277 ev/s 为更早轮次) | launch-readiness §1.4 |
| soak 20min | 354.4 rps 稳定 / P95 8.5s(AI 长尾)/ 无内存泄漏迹象 | `report/soak-summary.md` |
| 混沌 RTO | pod_kill 1s / Redis down 42~44s / TiDB down ~100s(注册慢) | `report/chaos-rto.txt` |
| 单节点资源设置 | 多数服务 requests 100m/128Mi,limits 500m/256Mi(紧凑验证档);learning-worker 100m/512Mi→1C/2Gi | `deploy/k8s/*.yaml` |

**结论(单节点未达项,多节点待验)**:G-01(5000 CCU,WS 372)、G-04 rps 档(24<50)、G-06(~760<5000 ev/s)。根因均为单实例/单节点资源约束,非架构缺陷——这是本方案要验证的核心命题。

---

## 2. 容量模型(实测外推 + 显式假设)

### 2.1 每 CCU 负载模型

| 参数 | 取值 | 性质 | 依据 |
|---|---|---|---|
| CCU 定义 | 同时在线 WebSocket 会话 | 实测口径 | WS retest 即此口径 |
| 每 CCU REST 请求率 | 0.4 rps/CCU(思考时间模型) | **假设** | 教育类产品典型值;压测 780rps@200VU 是无思考时间的压力档,不代表真实用户 |
| 每 CCU WS 消息率 | 0.5 msg/s(心跳+对话) | **假设** | 心跳 15s + 会话轮次 |
| 5000 CCU 总 REST | ~2000 rps(读:写 ≈ 8:2) | 推导 | 0.4×5000 |
| 5000 CCU WS 消息 | ~2500 msg/s | 推导 | 0.5×5000 |
| 学情事件 | 峰值 5000 ev/s(G-06 目标) | 目标 | 正常态估算 ~1500 ev/s,峰值 3x |
| 带宽/CCU | 下行 30 Kbps(音频流+UI) | **假设** | 5000 CCU ≈ 150 Mbps 出口,接入层非瓶颈 |

### 2.2 WS/接入层外推(核心:G-01)

- 实测单 bff 2 核支撑 372 WS 并发,且 CPU 未饱和(1.3 core),瓶颈在 register 速率/线程模型 → **单副本保守按 500 WS 并发规划**(372 实测 × 1.34 余量,验证项之一)。
- 5000 CCU × 40% 口语占比 = **2000 WS 并发** → bff **4 副本即理论达标,取 6 副本**(N+2 容错:任意 2 副本故障仍承载 2000/4=500/副本,恰好等于单副本保守上限)。HPA 3/10,指标用"每副本 WS 连接数 > 400"自定义指标(非 CPU——实测证明 CPU 不是先导指标)。
- gateway-go WS 接入在 62→372 提升中始终未饱和,多副本 + hub 分片(8 shard)即可,副本数跟 bff 对齐取 3~6。
- **验证锚点:L2 阶段须实测单 bff 副本 WS 上限是否 ≥500**,若 register 速率仍是先导瓶颈,先修 bff(异步 register/连接接纳限速)再扩容,不许靠堆副本掩盖单副本缺陷。

### 2.3 业务服务副本与 HPA

基于 D2 容量表(`multi-node-topology.md` §2)并结合本次实测微调:

| 服务 | 多节点副本 min/max | 计算 | 性质 |
|---|---|---|---|
| bff-java | 3/10(4C8G) | §2.2 推导 | 实测外推 |
| gateway-go | 3/10(4C8G) | 对齐 bff | 实测外推 |
| learning-java 入口 | 4/12(4C8G) | 读路径 P95 95ms 余量大,按吞吐线性外推 2000rps×20% 学习占比 | 实测外推 |
| learning-worker | 3/8(4C8G) | 760 ev/s 单消费者 → 5000 ev/s 需 ≥7 消费并行;**分区键扩容 + 副本数 = 消费并行度,须验证消费者可水平分片**(当前单 worker 消费是已证瓶颈) | 实测外推+验证项 |
| writing-py | 4/10(4C8G) | 24rps/实例 → 50rps 需 ≥3 实例,写作占比 30% 峰值 600rps 总量 → 但 LLM 外部长尾才是 P95 8.5s 根因,rps 档 4 副本、P95 档靠 LLM 并发配额 | 实测外推 |
| teacher-java | 3/8(4C8G) | 教师占比 10%,负载最轻 | 假设 |
| emotion/writing-coach/Rust4/Go7 | 各 1~2 起,HPA 1/4~1/6 | 轻路径 | 假设 |

**关键推导口径**:读路径单实例在 200VU 混合下未现瓶颈(P95 95ms),故读路径副本数由**吞吐线性外推 + 30% 余量**决定;写路径(LLM/事件)副本数由**实测单实例上限**决定,二者不可混用。

### 2.4 数据层规格

| 组件 | 多节点规格 | 依据 |
|---|---|---|
| TiDB | 2 TiDB Server(4C16G)+ PD×3(2C4G 跨节点)+ TiKV×3(8C32G NVMe) | 30 亿测试数据已灌;坑㉑ TiKV 单节点 region 压力 → 3 TiKV 分摊 leader |
| TiKV | region leader 均衡度纳入通过标准(见 §4) | 坑㉑实测教训 |
| Redis Cluster | 6 主 6 从(4C16G),**各区独立不强同步** | ADR redis-cluster-migration |
| Pulsar | Broker×3(4C16G)+ Bookie×3(4C32G NVMe) | 5000 ev/s + Geo Replication |
| 可观测 | OTel DaemonSet 每节点 + GW×2 | P2-4 分层 |

### 2.5 集群拓扑(验证环境,非生产三地域)

生产目标态见 `multi-node-topology.md`(3 地域×多 AZ)。**本验证方案在单地域多节点集群执行**,规模:

- 控制面:3 master(或 1 托管 control plane + 2 etcd),4C8G——验证 etcd/apiserver 在压测高变更率(HPA 扩缩/Pod 重建)下的稳定性。
- 工作节点:**6 × 8C32G**(其中 2 台带 NVMe 跑 TiKV/Bookie,4 台跑业务+接入;压测机独立 2 × 4C8G,不复用被测节点——坑:压测机与被测同节点会互相污染)。
- CNI:**首选 Cilium(eBPF 绕 kube-proxy,2000 WS 并发下 conntrack 表规模是 kube-proxy/iptables 模式的真实风险点,需在 L1 验证);保守回退 Calico**。(决策记录:memory `esp-cni-multinode-upgrade-decision`;注:docs/design 尚无独立 CNI 文档,本条为首个落盘。)
- 合计 8C×(3+6)+2×4C ≈ 84C / ~280G,验证档够用;生产按 D2 表另计。

---

## 3. 验证分层(L1→L4,逐层放行)

> 原则:**下层不绿不放行上层**;每层产出落 `report/multinode/`,数字回填本文档附表。

### L1 组件级(数据层各自 benchmark)

| 对象 | 工具 | 场景 | 观测 |
|---|---|---|---|
| TiDB | sysbench oltp_read_only / oltp_write_only / go-tpcc | 30 亿数据下 2000rps 读 + 400rps 写混合,P95<QPS 目标对应阈值 | region leader 均衡、TiKV CPU、raft region 9005 报错(坑㉑复发监测) |
| Redis Cluster | redis-benchmark + memtier | 20 万 ops/s 目标(会话态+幂等+黑名单),pipeline 1/16 | cluster_state、MOVED、连接数、eviction |
| Pulsar | pulsar-perf producer/consumer | 5000 ev/s 持续 30min,分区≥8 | backlog、bookie writable、Geo 复制 lag |
| K8s 网络(Cilium) | iperf3 + netperf + conntrack 压测 | 2000 并发长连接建连速率 | conntrack 表占用、Cilium drop 指标 |

### L2 服务级(每服务独立 locust,测单副本上限)

- **bff/gateway WS**:阶梯 200/500/1000/2000 WS 并发打单副本网关组,确认 §2.2 的"单副本 500 WS"保守值;register 速率单独压。
- **learning 入口**:REST 事件上报 1000→5000 ev/s 阶梯,观测 503 accepted:false 复现点。
- **writing-py**:24rps/实例复测多节点下是否提升(LLM 真实/Mock 双模分别测,隔离外部依赖)。
- **teacher/emotion/go 系**:冒烟 + 各 200rps 档即可(负载占比低)。
- **learning-worker**:1→3→5 副本消费者水平分片验证,G-06 关键前置。

### L3 全链路混合(用户行为加权)

- 场景分布:**口语 40% / 写作 30% / 学习 20% / 教师 10%**。
- 阶梯:1000 → 2000 → 3500 → 5000 CCU,每档稳态 30min;5000 CCU 档要求 WS 在线 ≥4750(95%)。
- 分布式 locust(压测机独立节点,`deploy/locust/`,见 `docs/design/p2-3-distributed-locust.md`)。
- 观测:全链路 trace 采样 10%,重点看跨服务 P99 分解(bff→gateway→业务→DB)。

### L4 稳态 soak + 故障注入

- **Soak 4h**(生产前 8h):5000 CCU 的 70% 负载持续 4h,观测内存斜率(Go/Java 服务 RSS)、GC、连接泄漏、Pulsar backlog、TiDB 慢查询。复用 `deploy/locust/soak.py`(SOAK_TIME 环境变量控时长)。
- **故障注入**(5000 CCU 负载中执行,对标单节点混沌基线):

| 注入 | 期望 RTO | 单节点基线 |
|---|---|---|
| kill 单 pod(每服务) | <5s(多副本无感) | 1s(gateway) |
| kill bff 副本 ×2 | WS 重连恢复 <30s,错误率尖峰 <3% | 无基线 |
| Redis 单节点 down | <60s failover,期间写路径降级不雪崩 | 42~44s |
| TiDB down(单 TiKV) | Region 调度完成 <3min,无 9005 风暴 | ~100s |
| 网络分区(工作节点隔离) | Pod 驱逐 + 流量重调度 <2min | 未执行(单节点无法测) |
| Pulsar broker down | 消费者重平衡 <30s,无消息丢失 | 未执行 |

---

## 4. 通过标准(量化阈值)

### 4.1 性能阈值

| 层 | 指标 | 阈值 |
|---|---|---|
| L1 TiDB | 混合 2400rps 下 P95 | <50ms(单条 SQL) |
| L1 Redis | P99 | <5ms;零 MOVED 错误(客户端拓扑感知) |
| L1 Pulsar | 5000 ev/s 端到端 | produce P99<50ms,零丢失 |
| L2 WS 单副本 | 持续并发 | ≥500(sustain ≥95%,低于则修 bff 而非扩容) |
| L2 learning | ev/s 上限 | 单 worker 副本 ≥1700(3 副本线性达标 5000) |
| L3 读路径 | agg P95 / P99 | <800ms / <1500ms(G-02/G-03,单节点已达标,多节点不回退) |
| L3 写作 | rps / P95 | ≥50rps 且 P95<2s(G-04,LLM 真实模式) |
| L3 学情 | 事件吞吐 | ≥5000 ev/s 零丢失(G-06,以 Pulsar 零积压+消费确认数核对) |
| L3 G-01 | WS 在线 | 5000 目标下 ≥4750 持续在线 30min |
| L4 soak | 4h 趋势 | 吞吐衰减 <5%,内存增长 <10% 且趋于平台,无 5xx 累积 |

### 4.2 资源水位红线(5000 CCU 稳态)

| 维度 | 红线 | 动作 |
|---|---|---|
| 业务 Pod CPU | <70%(limit 口径) | 超 → HPA 已扩满则记容量缺口 |
| 业务 Pod 内存 | <80% | 超 → 上调 limit 或查泄漏 |
| 节点整体 allocatable CPU | <75% | 超 → 加节点 |
| TiDB | QPS 达标前提下 TiKV CPU<65%,**region leader 均衡度:最忙/最闲 TiKV leader 数比 <1.3** | 坑㉑监测项 |
| Redis | 单分片内存 <70%(maxmemory),零 eviction | 超 → 扩分片 |
| Pulsar | backlog 消费 lag <10s 持续 | 超 → 扩 worker 消费并行 |
| 错误率 | 5xx <0.1%;WS 断线率 <5%/h | 超红线即判该档不通过 |

---

## 5. 单节点 → 多节点迁移风险清单

| # | 风险 | 已知坑源 | 缓解 |
|---|---|---|---|
| 1 | **Redis nodes.conf 持久化旧 PodIP** → cluster_state:fail | 坑:VM 重启/nodes.conf 短名覆盖 FQDN | 迁移时清 nodes.conf 重建集群;runbook 固化 CLUSTER MEET 自愈步骤 |
| 2 | **Redis 公告短名致跨 ns 客户端 MOVED 解析失败**(坑㉓) | redis-cluster-hostname-announce-023 | 公告 FQDN/IP,迁移后专项回归 MOVED=0 |
| 3 | **k3s → 标准 K8s 差异**:本 RD 配置 k3s 用 moby/docker 作 CRI(坑㉔),标准集群是 containerd;镜像可见性假设全部失效 | esp-k3s-containerd-image-isolation | 统一走私有镜像仓库(Harbor),CI 推镜像 + 节点 imagePullSecrets,禁止手工 import |
| 4 | **CNI 切换**:flannel(单节点)→ Cilium/Calico | esp-cni-multinode-upgrade-decision | 新集群全新装 Cilium(非原地切换);NetworkPolicy 语义回归(坑㉕:跨 ns traefik 放行 patch 在新 CNI 下重验) |
| 5 | **ExternalName DNS 间歇失败**(坑㉒:短名 DNS) | seed-job TIDB_HOST 改 FQDN 修过 | 多节点统一 FQDN service 直连(同 ns)或 Private DNS;ExternalName 逐个替换 |
| 6 | **Traefik Ingress 跨 ns + NetworkPolicy** | 坑㉕ | 迁移后全端点 502 回归清单(逐服务 curl) |
| 7 | **TiKV 单节点 region 压力**(坑㉑ ERR) | seed_3b THROTTLE+重试 | 3 TiKV 起步 + PD scheduler 平衡;补灌数据限速重验 |
| 8 | **JPA findAll 全表扫描 OOM**(坑㉖) | esp-jpa-findall-oom-trap | 大表路径投影查询已在 1.8.5 修复,迁移后回归 initMastery 大用户路径 |
| 9 | **压测机与被测同节点污染** | esp-loadtest-probe-pitfalls | 独立压测节点 + 探针 timeout 调 ≥3s |
| 10 | **主机/集群资源争用** | esp-singlenode-host-cluster-contention | 多节点后风险自然消解,但构建(CI)与压测仍须错峰 |
| 11 | **数据迁移**:30 亿测试数据重灌成本高 | seed_3b 历史 | 优先 TiDB Backup/Restore(BR)跨集群恢复;失败才重灌(锚点见 PROGRESS 补灌记录) |
| 12 | **PDB/反亲和缺失** | 新增项 | 迁移清单:每 deploy 加 PDB minAvailable=1 + topologySpreadConstraints 跨节点 |

---

## 6. 执行计划

| 阶段 | 内容 | 机器增量 | 预计工时 | 放行/回滚点 |
|---|---|---|---|---|
| P0 集群搭建 | 3 master + 6 worker + 2 压测机;Cilium;Harbor;ArgoCD(ADR 已有) | 全部 | 1~2 天 | 回滚点:单节点 esp-prod 全量保留不动,新集群独立 ns esp-staging |
| P1 数据层就绪 | TiDB(BR 恢复 30 亿)/Redis Cluster(6主6从)/Pulsar×3;坑 1/2/5/7 专项回归 | 0 | 2 天 | 回滚点:BR 快照可重放;Redis 重建脚本化 |
| P2 L1 组件压测 | §3 L1 四项 | 0 | 1 天 | 放行标准 §4.1 L1 行 |
| P3 业务迁移灰度 | 按 optimization-plan 批次 1-7 deploy,每批 healthz + 内存 headroom≥20%;PDB/反亲和补齐 | 0 | 1~2 天 | 回滚点:每批 ArgoCD rollback(镜像 tag 已固定) |
| P4 L2 服务级 | WS 单副本上限 + worker 分片 + writing 双模 | 0 | 1~2 天 | 放行:§4.1 L2 行;**bff 单副本 <500 WS 则先修后扩** |
| P5 L3 全链路 | 1000→5000 CCU 阶梯 | 0 | 2 天(含分析) | 放行:§4.1 L3 全行 + §4.2 水位 |
| P6 L4 soak+混沌 | 4h soak + 6 类注入 | 0 | 2 天 | 放行:§4.1 L4 + RTO 表 |
| P7 报告归档 | 结果回填本文档附表 + PROGRESS.md + G-01/04/06 判定 | 0 | 0.5 天 | — |

**总计:约 9~12 个工作日,机器 9 节点(84C/280G 档)**。风险缓冲:P4 若 bff 单副本上限不达标,+2 天修复窗口。

---

## 附:结果回填表(执行后填写,未执行留空并注明)

| 项 | 目标 | 多节点实测 | 判定 | 日期 |
|---|---|---|---|---|
| G-01 WS CCU | ≥4750/5000 | 未执行 | — | — |
| G-04 writing | ≥50rps P95<2s | 未执行 | — | — |
| G-06 学情 | ≥5000 ev/s | 未执行 | — | — |
| bff 单副本 WS | ≥500 | 未执行 | — | — |
| worker 单副本 ev/s | ≥1700 | 未执行 | — | — |
| soak 4h | 衰减<5% | 未执行 | — | — |
| 混沌 RTO(表 §3 L4) | 各自阈值 | 未执行 | — | — |
