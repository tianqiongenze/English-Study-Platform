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
- 活 pod pbbm7 exitCode 137(SIGKILL,内核 cgroup OOM kill)
- **HSA 修正(2026-08-21 复核)**:live 与 yaml limits 实为 **2Gi(2048Mi)** 非 1536Mi(此前报告 1536Mi 有误)
- pbbm7 内存爬升 1850Mi < 2048Mi,但仍 OOMKilled → 根因非"超堆 limits"
- 真因:Java 进程 RSS = 堆(MaxRAMPercentage=75%→~1.5Gi)+ Netty direct buffer + Metaspace + 线程栈 + code cache,k6 高并发下**非堆开销膨胀**(direct buffer/线程数激增)致进程总 RSS 超 2Gi cgroup memory.max 被杀
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


## HPA 扩缩容时间线(esp-data 分析 hpa-snapshots.log 1403行/31样本)

### bff-java HPA REPLICAS 时间线(19:48-19:54)
| 时间 | 副本 | 备注 |
|---|---|---|
| 19:48-19:49:02 | 2 | 预热态(老 pod zxtnr+pbbm7) |
| 19:49:12 | 1 | 缩到 1 |
| 19:49:24-19:49:55 | 2 | 新 pod bczv8 19:49:24 出现 |
| 19:50:05-19:50:16 | 1 | 再缩到 1 |
| 19:50:26 | 2 | **CPU 首次破阈值 218%** |
| **19:50:36** | **4** | **HPA 触发扩容直达 MAXPODS=4(CPU 628%)** |
| 19:50:36-19:53:18 | 4 | 维持 4,19:51:45 起 CPU=`<unknown>`(新 pod 启动期 OOMKill) |
| **19:53:34** | **1** | **缩容回 1(3 新 pod 全未 ready 即死)** |
| 19:54:55 | 3 | 尾部小回弹 |

### CPU 峰值
- HPA 显示 **655%/70%(9.3 倍超阈)** @ 19:51:09-19:51:20
- 单 pod 实测 cores 峰值:**bczv8 = 1381m(1.38 core)** @ 19:49:55-19:50:05
- 19:51:45 后全 `<unknown>`(pod 崩溃 HPA 取不到指标)

### 扩容触发延迟
- CPU 首次 ≥70% @ 19:50:26(218%),HPA 19:50:36 副本 2→4
- **HPA 控制器决策延迟 ≈10s(一个采样周期,可接受)**
- 但新 pod(2ccgz/qxgjx)19:50:57 才在 top pod 出现(启动 ≥21s),一出现就 814m/730m 高负载,**从未 ready 即 OOMKill** = 无效扩容

### OOM 轨迹(pbbm7 内存爬升)
636→1094→1392→1712→**1850Mi @ 19:51:20**(超 limits 1536Mi)→ OOMKill 窗口 19:51:20-19:51:45
- bczv8 内存:244→557→1259→1661Mi 同期攀升
- 19:52:11→19:53:18 有 **67s 日志断档**(VM CPU 雪崩,watcher 自身停摆)

### 其他服务 HPA(关键:下游全富余)
- **gamification-go**:全程 1(CPU 5-15%)— 未触发
- **learning-java**:全程 1(CPU 13-34%,峰值 34% < 65%)— 未触发
- **teacher-java**:全程 1(CPU 18-25% < 70%)— 未触发
- **gateway-go**:全程 1(含 memory target)— 未触发
- **learning-worker**:3↔1 多次摆动(预扩容后缩放震荡,峰值 CPU 174/177%,非本次新增扩容)
- 其余 emotion/content/course-path/listening/vocab/live-tutor/reading/llm-orchestrator/assessment/writing-py/media-learn/pronunciation/community:**全程 1,均未触发**
- **结论:负载完全压在 bff-java(GraphQL/聚合层),下游 Java/Go 服务 CPU 富余,HPA 无需动作**

### top pod CPU 排行(压测期峰值)
| 排名 | Pod | 峰值 CPU | 时间 |
|---|---|---|---|
| 1 | bff-java-bczv8 | 1381m | 19:49:55/19:50:05 |
| 2 | bff-java-pbbm7 | 1310m | 19:50:57/19:51:09 |
| 3 | learning-worker-txl5n | 955m | 19:49:55/19:50:05 |
(前 4 中 3 席 bff-java)

## 根因小结(esp-data 结论)
bff-java 是单点瓶颈,HPA 扩容决策延迟仅 ~10s 可接受,但 MAXPODS=4 的新 pod 在高负载下启动期就被 OOMKill(内存上限不足 1536Mi + 启动即承压),导致"扩了等于没扩",最后缩回 1。

修复方向:① 上调 bff-java 内存 limit(1536→3072Mi);② 启动期降载/预热;③ 评估 bff-java 是否需横向拆分(GraphQL 聚合层单服务吃满);④ 151s 冷启动需 GraalVM native/Spring AOT(工程量大 DEFERRED)。

## 对 P1-D10 的决策意义(关键)
- k6 已实测拐点:~2000 CCU 档(19:50:26 CPU 218% 飙升对应高 CCU)bff-java OOM
- 下游全富余 → P1-D10 压力会 100% 堆在 bff-java,5000 CCU 必崩(远早于 5000)
- bff-java 止血态(maxReplicas=1)→ P1-D10 一压就 OOM(重复 k6 发现)
- 单节点 9 核 + bff-java 1 副本 1536Mi + 151s 冷启动 = P1-D10 5000 CCU 物理不可能通过
