# scripts/chaos — 混沌实验脚本

模拟故障,验证自愈 / 降级 / 重试 / 监控告警(测试报告 04 用)。

## 脚本
| 脚本 | 故障 | 验证点 |
|---|---|---|
| `pod_kill.sh <deploy> [count]` | 随机杀 pod | K8s 自愈、HPA 扩容、客户端重试 |
| `redis_down.sh <sec>` | Redis 不可达(改 env) | 会话状态降级、错误率上升、恢复后自愈 |
| `crdb_down.sh <sec> [deploy]` | CRDB 不可达(改 env) | 读写分离降级到不可用(无降级路径,需显式 503)、恢复后自愈 |
| `pulsar_down.sh <sec>` | Pulsar broker 下线(scale 0) | rankings feed 中断、learning-worker 消费暂停、恢复后续消费 |
| `dep_cascade.sh <target> [caller] <sec>` | 依赖服务不可达 | 调用方超时降级/快速失败 |
| `latency_inject.sh <deploy> <ms> <sec>` | tc netem 注入延迟 | 超时降级、P95/P99 飙升 |
| `net_partition.sh <a> <b> <sec>` | NetworkPolicy 断网 | 异地多活分区、跨服务调用失败 |

## 执行
```bash
NS=esp-prod bash scripts/chaos/pod_kill.sh learning-java 1
NS=esp-prod bash scripts/chaos/crdb_down.sh 60 learning-java
NS=esp-prod bash scripts/chaos/pulsar_down.sh 30
NS=esp-prod bash scripts/chaos/dep_cascade.sh emotion-svc learning-java 60
```
每脚本起止打时间戳,恢复后 `rollout status` 确认自愈。配合 `scripts/chaos/run_all.sh` 可顺序执行并记录恢复时间到 `report/chaos_result.md`。

## 注意
- 单节点 Rancher Desktop:部分混沌(tc/netem 需 NET_ADMIN、NetworkPolicy 需 CNI)可能不可用,脚本遇缺权限会跳过并 WARN,如实记录"跳过原因"。
- 破坏性:redis_down/crdb_down/pulsar_down 会临时改环境变量或 scale,属可控可逆,但执行前确认无生产流量。
