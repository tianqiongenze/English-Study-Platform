# 混沌实验执行计划(预检 + 待执行)

> 状态:**预检完成,未执行**。实际混沌执行须在 30 亿种子数据 + 5000 CCU 压测基线建立之后进行,否则故障无法与压测基线对比,且会干扰尚未完成的数据/压测任务。

## 1. 脚本清单与预检结论

| 脚本 | 破坏对象 | 恢复方式 | 预估 RTO | 前置条件(须 Running) | 单节点可行性 |
|---|---|---|---|---|---|
| `pod_kill.sh <deploy> [count]` | 指定 deployment 的 pod(`kubectl delete pod -l app=...`) | K8s 控制面自动拉起新 pod,脚本 `rollout status --timeout=180s` 确认 | 15-90s(取决于镜像本地缓存与探针) | 目标 deploy 必须 Running;**副本数 >= 2** 才能验证自愈期间服务不中断 | 需副本>=2,否则杀 pod 期间服务完全中断(单节点仍可跑,但有断服窗口) |
| `redis_down.sh <sec>` | gateway-go 的 `REDIS_HOST` 改为 `127.0.0.1:1`(不可达),触发 rollout | 到时还原 `REDIS_HOST=redis-cluster` 并 `rollout status --timeout=120s` | 60-120s(env 改动触发滚动重启) | gateway-go Running;Redis Cluster(ekb-m2-aws-us-east-1 ns)可达 | 可行(纯 env patch,不依赖 CNI) |
| `tidb_down.sh <sec>` | learning-java 的 `READ_DB_URL` 改为 `127.0.0.1:1`,读副本不可达 | 到时删除该 env(`READ_DB_URL-`)并 rollout | 60-120s | learning-java Running;TiDB primary 可达(降级路径) | 可行(纯 env patch) |
| `latency_inject.sh <deploy> <ms> <sec>` | 容器内 `tc qdisc add dev eth0 root netem delay` | `tc qdisc del dev eth0 root` | 即时(删 qdisc 即恢复) | 目标 deploy Running;**容器须有 NET_ADMIN cap**,且镜像含 `iproute2`/`tc` | **大概率不可行**:Rancher Desktop 默认不给 NET_ADMIN,无特权容器脚本会 WARN 跳过 |
| `net_partition.sh <a> <b> <sec>` | 给 deploy_a 应用全 deny NetworkPolicy(Ingress+Egress 空) | 到时 `kubectl delete networkpolicy` | 即时(删策略即恢复) | 两个 deploy 均 Running;**CNI 须支持 NetworkPolicy** | **不可行/无效**:Rancher Desktop 默认 CNI(bridge)不强制 NetworkPolicy,策略 apply 成功但不生效,无法真实断网 |

## 2. 执行顺序建议(基线就绪后)

1. `redis_down.sh` —— 影响面最小(仅会话降级),先验证监控/告警链路。
2. `tidb_down.sh` —— 验证读写分离降级到 primary。
3. `pod_kill.sh` —— 验证 K8s 自愈(需先 scale 副本)。
4. `latency_inject.sh` —— 若 NET_ADMIN 可用再执行,否则跳过并记录原因。
5. `net_partition.sh` —— 单节点 Rancher Desktop 上**建议跳过**,记录"CNI 不支持,跳过";如需真实分区验证须迁多节点集群。

每脚本起止打时间戳,恢复后 `rollout status` 确认;结果回填到 `report/chaos_result.md`(待执行时创建)。

## 3. HPA 与副本数预检(待执行前用 kubectl 复核)

本次预检未执行 `kubectl get hpa`(按任务要求未跑实际命令,仅文档准备)。执行混沌前需先确认:

- `kubectl -n esp-prod get hpa` —— 检查 gateway-go / learning-java / writing-py / teacher-java / emotion-svc 是否有 HPA。
- `kubectl -n esp-prod get deploy -o wide` —— 检查各 deploy 当前副本数。
- **若核心服务均为单副本**:`pod_kill.sh` 会在杀 pod 期间产生完全断服窗口,无法验证"自愈期间服务可用"。执行前需先 `kubectl scale deploy/<svc> --replicas=2`(或配置 HPA minReplicas=2),否则 chaos-plan 标注"需先 scale 副本到 2+"。

## 4. 关键发现 / 风险

- **单节点 Rancher Desktop 限制**:
  - `net_partition.sh` 默认 CNI 不强制 NetworkPolicy → **不可行/无效**,建议跳过。
  - `latency_inject.sh` 需 NET_ADMIN cap → **大概率不可行**,脚本会自动 WARN 跳过,如实记录。
  - `pod_kill.sh` 在单副本时杀 pod = 断服,需先扩副本到 2+。
- `redis_down.sh` / `tidb_down.sh` 通过 env patch 触发 rollout,属可控可逆,单节点可行,但 rollout 期间该服务会有滚动重启窗口(老 pod 终止 → 新 pod 就绪),需确认 readinessProbe 配置正确避免提前切流。
- `tidb_down.sh` 恢复用的是 `READ_DB_URL-`(带末尾 `-` 表示删除该 env),还原后 learning-java 回落到镜像/ConfigMap 默认读库地址,执行前需确认默认值指向 `tidb-follower` 而非被 patch 的值。

## 5. 执行前置总闸(全部满足才启动混沌)

- [ ] 30 亿种子数据已写入(esp-data 阶段完成)。
- [ ] 5000 CCU 压测基线已建立,`report/` 下有基线 P95/P99/错误率。
- [ ] 监控/告警链路就绪(Prometheus + Grafana + Alertmanager,告警能触发)。
- [ ] 核心服务副本数 >= 2(或 HPA minReplicas>=2)。
- [ ] 已与团队约定混沌窗口,确认无真实用户流量。
