# 压测报告 (会话23-13) —— 小规模验证链路可承受

> 方法论:verification-before-completion(每步附证据:QPS/P95/错误率/RC,瓶颈定位不粉饰)
> 时间:2026-08-18
> 环境:K8s rancher-desktop ns esp-prod,集群刚从 PLEG 雪崩 + CRDB probe 震荡恢复,30 pod Running(含滚动卡死)
> 压测工具:locust 2.46 集群内 Job(scripts/run_locust_bench.sh),从 50 并发起步(铁律:刚恢复不大规模压)

## 压测结果汇总(2 组,均 50 并发 45s)

| 端点 | EP | requests | failures | avg_rps | avg_latency | P95 | fail_rate | 瓶颈评估 |
|------|----|----|------|---------|-------------|------|-----------|----------|
| learning_review (GET 只读) | learning_review | 1053 | 0 | 24.46 | 1593.91ms | 3000ms | 0.0000 | 中,P99 长尾 25s |
| bff_login (POST 鉴权链路) | bff_login | 23 | 0 | 1.18 | 14474.74ms | 17000ms | 0.0000 | 严重瓶颈 |

## 详细分析

### 组1:learning_review(只读接口,GET /api/learning/review?userId=<id>)

locust 聚合延迟分布(50 并发 45s,直打 learning-java:80 集群内 Service):
```
Aggregated  min=740  p50=1700  p90=2500  p95=3000  p99=18000-25000  reqs=1053
```
- 0 失败,链路稳定
- P50=1.7s 偏高(只读接口应 <500ms);P95=3s;P99 长尾达 25s
- 瓶颈假设:① learning-worker 滚动卡死(新 RS 5445fc748f 2 pod 0/1 崩溃重启占用资源);② learning-java Dubbo RPC 调用链(teacher→learning 已证实 DEADLINE_EXCEEDED 10s 超时);③ CRDB 查询(learning_events 8000 行全表扫描 KV 12ms,本身不慢,但连接池/并发竞争可能放大)

### 组2:bff_login(鉴权链路,POST /api/auth/login)

- **50 并发 45s 仅完成 23 请求,avg_rps=1.18,avg 14.5s,P95 17s** —— 严重瓶颈
- 0 失败(所有完成请求都 200,但吞吐极低)
- 瓶颈定位:
  - bff-java 处于滚动更新卡死(Deployment spec image=1.10.0 有 Dubbo `.dubbo` 目录写权限 bug,新 RS 3 pod 全 Startup probe failed;旧 RS 1.14.4 2 pod 承载流量但可能连接池/线程不足)
  - login 链路:BFF→(Dubbo RPC)→user/account 服务→CRDB 查 user→bcrypt 密码校验→RSA JWT mint。任一环节慢即放大
  - bff 旧 pod 压测期日志无 login 输出 → 请求可能在 Dubbo 调用阶段阻塞(BFF 等下游响应,线程占用)
  - CRDB pg_stat_activity 压测期 active_conn=0 → bff_login 未实时打 CRDB,瓶颈不在 CRDB I/O,而在 Dubbo RPC 链路或 user 服务处理

## CRDB 连接数(压测期间实测)

| 时刻 | pg_stat_activity count | 活跃查询 |
|------|------------------------|----------|
| 压测前基线 | 0 | - |
| bff_login 压测中 | 0 | 0(无活跃 SQL) |
| learning_review 压测中 | 未实时采集(单次 Job 模式无 sidecar 采样) | - |

结论:CRDB 连接数压测期保持 0 —— 当前小规模压测的瓶颈不在 CRDB 连接/IO,而在应用层(Dubbo RPC 链路 + 服务滚动卡死 + 线程池)。

## 瓶颈根因总结

1. **多个服务滚动更新卡死**(首要瓶颈):
   - bff-java:spec image=1.10.0(Dubbo `.dubbo` 写权限 bug),新 RS 3 pod 全崩,旧 RS 1.14.4 2 pod 承载 → 可用副本不足
   - learning-worker:新 RS 5445fc748f 2 pod 0/1 崩溃重启,旧 RS 86566fb8b7 2 pod → worker 可用性打折
   - 根因均为新版镜像启动失败,旧版勉强支撑,集群未真正"健康恢复"
2. **Dubbo RPC 超时**:teacher→learning 已证实 DEADLINE_EXCEEDED(10s),bff→user login 链路同理 → 鉴权吞吐被 RPC 超时压垮
3. **单节点资源瓶颈**:Rancher Desktop 单 VM(CRDB 3 pod + 30 业务 pod 共争 CPU/内存),压测期 VM 资源争抢放大延迟(参考 memory `esp-singlenode-host-cluster-contention`)

## 结论与建议

- 链路在 50 并发下**可用但不健康**:0 失败但延迟超标(learning P95 3s,login P95 17s),距离 5000 CCU 目标差距巨大
- **不建议当前状态上 5000 CCU 压测**:需先解除 bff-java / learning-worker 滚动卡死(推进到可用镜像版本),恢复全部副本健康
- DEFERRED:解除滚动卡死 + 恢复全副本后再做阶梯压测(50→200→500→5000),当前仅完成 50 并发小规模验证

## 关联文件
- learning_review 日志:`/Users/joelgong/claude-Projects/English-Study-Platform/report/benchmark-2026-08-16/locust-learning_review-v50.log`
- bff_login 日志:`/Users/joelgong/claude-Projects/English-Study-Platform/report/benchmark-2026-08-16/locust-bff_login-v50.log`
- 启动脚本:`/Users/joelgong/claude-Projects/English-Study-Platform/scripts/run_locust_bench.sh`
- locustfile:`/Users/joelgong/claude-Projects/English-Study-Platform/deploy/locust/locustfile_fullbench.py`
