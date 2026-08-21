# k6 GraphQL vs REST 压测结果 + bff-java 容量拐点(2026-08-21)

## 执行
- Job: k6-graphql-vs-rest(esp-k6:0.54.0 本地派生镜像,IfNotPresent)
- 场景: graphql(单 POST /graphql 5字段聚合) vs rest(5串行 GET),错峰串行(rest startTime 150s)
- VM: 单节点 9 核,执行期 load 17.99(2x超载)

## 结果(诚实,非"通过")

### graphql scenario: **完整跑完 100%**
- 5648 complete, 192 interrupted iterations
- 193/200 VUs 稳定,2m10s 全跑完
- 健康阶段(85 iter,25 VUs,32% 时点)bff-java 无 ERROR,Jetty 线程池正常

### rest scenario: **73% 时 bff-java 崩,未完成**
- 73% 时(115/200 VUs)bff-java:8090 全 connection refused
- 根因:bff-java 活 pod 被 OOM kill(exitCode 137)+ HPA 扩容新 pod 启动太慢

## 根因定位(P0 级发现,影响后续 P1-D10)

### 发现1: bff-java JVM 冷启动 **151 秒**(P0)
```
Started BffApplication in 151.414 seconds (process running for 158.565)
```
- HPA 扩容触发 → 新 pod 151s 才 Ready → 流量早过去了
- **HPA 对 bff-java 这种重型 JVM 服务在突发流量下根本来不及扩容**
- 冷启动卡点:Nacos gRPC 注册 `TimeoutException: Waited 3000 milliseconds for grpc ClientCalls`
  - Nacos gRPC 超时 3s,VM load 18 时 gRPC 慢超 3s 反复重试

### 发现2: bff-java OOMKilled(P0)
- 活 pod pbbm7 exitCode 137(SIGKILL,内核 OOM kill)
- limits memory 1536Mi 在 k6 并发下不够
- pbbm7 OOM 后重启,又进 151s 启动期 → deploy 0 AVAILABLE 持续

### 发现3: HPA 扩容的 3 新 pod 没到 ready 就被缩容
- 2ccgz/bpfmd/qxgjx 3m34s-3m49s 前 HPA 扩出
- 全 ContainersNotReady(startup 阶段被压)→ 终止
- 典型"扩容来不及 + 扩出来也接不住"

## 单节点 9 核容量拐点(实测)
- graphql 单请求聚合多字段,bff-java 1 副本 1536Mi 可扛 ~193 VUs(graphql 跑完)
- rest 5 串行请求/VU + 高并发 → bff-java 1 副本扛不住,OOM
- **拐点:bff-java 1 副本 + 1536Mi + 151s 冷启动 = 单节点无法支撑突发并发流量**

## 与 P1-D10 的关系
- bff-java 是 P1-D10 阶梯压测的前端入口,151s 冷启动 + OOM 意味着:
  - 5000 CCU 档 bff-java 必崩(远早于 5000 就崩)
  - 阶梯早期(500-1000 CCU)可能就开始 OOM
- **决策:不在 bff-java 崩溃状态下跑 P1-D10**(会污染 HPA 数据 + 雪崩 VM)
- 先等 bff-java 回到稳态 + VM 降温,再决定 P1-D10 是否跑或调整 bff-java 容量后跑

## 已做止血
1. 清 3 个卡 Terminating 死 pod(--force --grace-period=0)
2. 删 k6 job + configmap
3. 停 HPA watch 后台脚本(hpa-snapshots.log 采到完整 k6 期数据)

## 数据资产
- report/loadtest/hpa-snapshots.log(128KB,k6 压测期完整 HPA 扩缩容 + top pod)
- report/loadtest/graphql-vs-rest-plan.md

## DEFERRED / 代码债
- bff-java Nacos gRPC 超时 3s 偏短(VM 高负载时)→ 考虑调到 5-10s(需查 Nacos client 配置,ASV)
- bff-java limits memory 1536Mi → 多节点生产应调大(单节点资源紧暂不动)
- bff-java 冷启动 151s → 预热/启动优化(GraalVM native image 或 Spring AOT,工程量大 DEFERRED)
- k6 rest scenario 未完整跑 → bff-java 容量恢复后可补跑(优先级低,graphql 对比已够说明问题)
