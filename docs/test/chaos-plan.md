# 混沌实验执行计划(预检 + 待执行)

> 状态:**预检完成,未执行**。实际混沌执行须在种子数据就绪 + 压测基线建立之后进行,否则故障无法与压测基线对比,且会干扰尚未完成的数据/压测任务。
>
> **2026-08-17 更新**:TiDB 已下电(三区 pd/tidb/tikv/tiflash --replicas=0),ESP 全服务切 CockroachDB(`crdb-public:26257`)。原 `tidb_down.sh` 替换为 `crdb_down.sh`。

## 1. 脚本清单与预检结论

| 脚本 | 破坏对象 | 恢复方式 | 预估 RTO | 前置条件(须 Running) | 单节点可行性 |
|---|---|---|---|---|---|
| `pod_kill.sh <deploy> [count]` | 指定 deployment 的 pod(`kubectl delete pod -l app=...`) | K8s 控制面自动拉起新 pod,脚本 `rollout status --timeout=180s` 确认 | 15-90s(取决于镜像本地缓存与探针) | 目标 deploy 必须 Running;**副本数 >= 2** 才能验证自愈期间服务不中断 | 需副本>=2,否则杀 pod 期间服务完全中断(单节点仍可跑,但有断服窗口) |
| `redis_down.sh <sec>` | gateway-go 的 `REDIS_HOST` 改为 `127.0.0.1:1`(不可达),触发 rollout | 到时还原 `REDIS_HOST=redis-cluster` 并 `rollout status --timeout=120s` | 60-120s(env 改动触发滚动重启) | gateway-go Running;Redis Cluster(ekb-m2-aws-us-east-1 ns)可达 | 可行(纯 env patch,不依赖 CNI) |
| `crdb_down.sh <sec>` | learning-java 的 CRDB 连接不可达(将 `ESP_RW_COCKROACHDB_HOST` 改为 `127.0.0.1:1`),触发 rollout | 到时还原 env 并 rollout | 60-120s | learning-java Running;CRDB(crdb-public:26257)可达 | 可行(纯 env patch) |
| `pulsar_down.sh <sec>` | Pulsar broker 不可达(gamification-go rankings feed + learning-worker Pulsar 订阅中断) | 到时还原,`rollout status --timeout=120s` | 60-120s | gamification-go/learning-worker Running | 可行(纯 env patch 或 scale pulsar-broker 0→恢复) |
| `dep_cascade.sh <svc>` | 模拟依赖故障:emotion-svc 不可达→学习链路降级;teacher-java 不可达→学生端不受影响 | `kubectl rollout undo` 或 env 还原 | 30-60s | 目标依赖 Running;调用方 Running | 可行(通过 env patch 不可达地址) |
| `latency_inject.sh <deploy> <ms> <sec>` | 容器内 `tc qdisc add dev eth0 root netem delay` | `tc qdisc del dev eth0 root` | 即时(删 qdisc 即恢复) | 目标 deploy Running;**容器须有 NET_ADMIN cap**,且镜像含 `iproute2`/`tc` | **大概率不可行**:Rancher Desktop 默认不给 NET_ADMIN,无特权容器脚本会 WARN 跳过 |
| `net_partition.sh <a> <b> <sec>` | 给 deploy_a 应用全 deny NetworkPolicy(Ingress+Egress 空) | 到时 `kubectl delete networkpolicy` | 即时(删策略即恢复) | 两个 deploy 均 Running;**CNI 须支持 NetworkPolicy** | **不可行/无效**:Rancher Desktop 默认 CNI(bridge)不强制 NetworkPolicy,策略 apply 成功但不生效,无法真实断网 |

## 2. 执行顺序建议(基线就绪后)

1. `redis_down.sh` —— 影响面最小(仅缓存降级),先验证监控/告警链路 + L1/L2→L3(CRDB)降级。
2. `crdb_down.sh` —— 验证 CRDB 不可达时学习链路降级(读写分离双池下行即降级)。
3. `pulsar_down.sh` —— 验证 Pulsar 不可达时 rankings feed pollLoop 兜底 + learning-worker 消费暂停不丢事件(SETNX 幂等)。
4. `dep_cascade.sh` —— 验证依赖级联:emotion 挂→学习降级;teacher 挂→学生端不受影响。
5. `pod_kill.sh` —— 验证 K8s 自愈(需先 scale 副本)。
6. `latency_inject.sh` —— 若 NET_ADMIN 可用再执行,否则跳过并记录原因。
7. `net_partition.sh` —— 单节点 Rancher Desktop 上**建议跳过**,记录"CNI 不支持,跳过";如需真实分区验证须迁多节点集群。

每脚本起止打时间戳,恢复后 `rollout status` 确认;结果回填到 `report/chaos_result.md`(待执行时创建)。

## 3. HPA 与副本数预检(2026-08-17 实测)

| 服务 | HPA max | 当前 replicas | CPU target | 单节点风险 |
|---|---|---|---|---|
| gateway-go | 20 | 1 | 60% | ⚠️ max=20 单节点 9 核会 OOM/CPU 抢占,坑㉟已现;建议 cap 4 |
| learning-java | 1 | 1 | 65% | ✅ 已 cap |
| teacher-java | 1 | 1 | 70% | ✅ 已 cap |
| writing-py | 3 | 1 | 70% | ⚠️ max=3 可能抢占 |
| emotion-svc | 3 | 1 | 70% | ⚠️ max=3 可能抢占 |
| gamification-go | 4 | 1 | 70% | ⚠️ max=4 可能抢占 |
| bff-java | - | 1 | - | 无 HPA,手动管理 |
| live-tutor-go | 4 | **0** | unknown | ❌ replicas=0 未启用 |
| media-learn-rs | 4 | **0** | unknown | ❌ replicas=0 未启用 |

**pod_kill 前置**:核心服务均为单副本,杀 pod = 完全断服窗口。执行前需先 `kubectl scale deploy/<svc> --replicas=2`。

## 4. 关键发现 / 风险

- **单节点 Rancher Desktop 限制**:
  - `net_partition.sh` 默认 CNI 不强制 NetworkPolicy → **不可行/无效**,建议跳过。
  - `latency_inject.sh` 需 NET_ADMIN cap → **大概率不可行**,脚本会自动 WARN 跳过,如实记录。
  - `pod_kill.sh` 在单副本时杀 pod = 断服,需先扩副本到 2+。
- `redis_down.sh` / `crdb_down.sh` 通过 env patch 触发 rollout,属可控可逆,单节点可行,但 rollout 期间该服务会有滚动重启窗口(老 pod 终止 → 新 pod 就绪),需确认 readinessProbe 配置正确避免提前切流。
- `pulsar_down.sh`:Pulsar 在 `ekb-m2-aws-us-east-1` ns,可通过 scale broker 到 0 或 env patch 不可达地址。gamification-go rankings feed 有 pollLoop 兜底(SETNX 幂等);learning-worker Pulsar 订阅中断后消费暂停,恢复后自动续消费(Shared 模式)。
- `dep_cascade.sh`:emotion-svc 挂→学习链路应降级(visual_for_word 返回本地 SVG);teacher-java 挂→学生端不受影响(Dubbo 超时快速失败)。
- TiDB 已下电:原 `tidb_down.sh` 替换为 `crdb_down.sh`;CRDB 是唯一数据源,下线即全量不可用(无降级路径,需显式 503)。

## 5. 执行前置总闸(全部满足才启动混沌)

- [ ] 种子数据就绪(CRDB esp_learning/esp_content/esp_community/esp_gamif 有足够数据量,可观察故障前后数据一致性)。
- [ ] 压测基线已建立,`report/` 下有基线 P95/P99/错误率。
- [ ] 监控/告警链路就绪(Prometheus + Grafana + OTel collector,告警能触发)。
- [ ] 核心服务副本数 >= 2(或 HPA minReplicas>=2,`pod_kill` 前必须 scale)。
- [ ] 已与团队约定混沌窗口,确认无真实用户流量。
- [ ] CRDB 备份/恢复方案就绪(混沌注入前做快照,异常时可回滚)。
