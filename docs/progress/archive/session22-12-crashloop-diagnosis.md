# 会话22-12:colima-stop 前的 CrashLoop 诊断 + 双轨落盘(2026-08-13)

> 从 `docs/PROGRESS.md` 归档(2026-08-14 会话22-15 瘦身)。

## 会话22-12:colima-stop 前的 CrashLoop 诊断 + 双轨落盘(2026-08-13)

### 用户指令(本会话)
1. "完整完成后续所有的任务(如果可以并行就并行),VM已经正常了" —— 持续推进 Task#4/5/6
2. "把这个没用的 Colima 关闭释放资源,只保留 Rancher Desktop" —— ✅ 已执行 `colima stop`
3. "立即自动将会话记录/任务执行/问题/方案/现状更新至 PROGRESS.md + 存入 Redis cluster,使上下文溢出后能完整恢复" —— 本节即此指令落盘

### VM/连接真实状态(2026-08-13 20:15~20:30 实测)
- **colima**:✅ 已 `colima stop` 关闭(lima/colima VZ 4核/8G 停),释放给 RD QEMU(30G)。docker context 仍指向 colima(已停),但 k3s 用 cri-dockerd 不依赖 host docker context。
- **rdctl shell**:仍 Broken(QEMU pid 61457 活,lima-guestagent 控制通道断)。**铁律:不自行 rdctl start/Factory Reset**。
- **host kubectl**:context `rancher-desktop` → `server: https://127.0.0.1:6443`。20:15 短暂可用(get nodes/pods 正常),20:25 起间歇 `connection reset by peer @6443`。
- **根因(节点事件)**:`ContainerGCFailed: dial unix /run/cri-dockerd.sock: connection refused` + `ImageGCFailed: context deadline exceeded` —— **cri-dockerd socket 间歇断**(VM soft lockup 余震,非新问题)。kubelet↔docker 通道不稳 → API 调用偶发 reset。
- **节点 conditions**:MemoryPressure/DiskPressure/PIDPressure 均 False,Ready=True(20:14:18)。即节点本身不压,问题在 cri-dockerd 通道。

### learning-java CrashLoop 根因(已确认,非代码 bug)
- **pod**:learning-java-5b8b7c6fd-w9srd,RESTARTS=10,Last State Exit Code 1(运行 2m32s 才死)。
- **日志证据**:启动正常推进到 `pulsar client connected` + `Connected to server`(12:14:57),Spring Boot **已基本启动成功**。
- **死因**:`Liveness probe failed: context deadline exceeded`(15s 超时)×6 → `Killing`。非启动崩溃,是**启动后被 liveness 探针超时误杀**。
- **探针配置**:liveness `delay=150s timeout=15s period=30s #failure=3`;readiness 同;startup `delay=0 period=10s #failure=60`。
- **resources**:req 200m/512Mi,lim 1/1536Mi。
- **本质**:单节点 9 核被 milvus+neo4j+tidb+tikv+tiflash+pulsar×3+redis×5+nacos+seata+... 抢占,Java 应用启动期勉强过 startup probe,但 liveness/readiness 周期探针在 CPU 高峰拿不到时间片响应 /actuator/health → 超时误杀 → 重启 → 再杀的雪环。**= memory `esp-loadtest-probe-pitfalls` 记录的"探针 timeout 太短误杀 + 单节点资源争抢"坑**。
- **gpxls pod**:Pending,Scheduled 到 node 但容器未创建(cri-dockerd 断,无法 create)。
- **关联次生**:`FailedSync: error reading from server: connection reset by peer`(kubelet↔docker 断);gateway-go exec 误报 "container not found"(同因,容器实际在)。

### learning-worker Error 根因(同因)
- `Communications link failure` to TiDB(HikariPool 起不来)+ OTel exporter `IOException: Canceled`。
- TiDB 实际可达(gateway-go `nc -zv tidb-primary 4000` 曾 RC=0 通过),是**启动慢被 Hikari connectionTimeout 拖垮**。

### 其他 pod 状态(20:25 实测快照)
- **1/1 Running 稳定**:emotion-svc/gateway-go/nacos/seata/teacher-java/writing-coach
- **不稳**:bff-java CreateContainerError(cri-dockerd 断致容器丢失,exit137)/learning-java 0/1/learning-worker ContainerCreating/sentinel 0/1 Running/writing-py 0/1 Running
- **st-seeder**:11 Error + 1 Completed(种子 Job 残留,待清)
- **dbq-21381**:Error(待查)

### 基础设施真实状态(20:15 实测)
- **图库 trio ✅**:milvus/milvus-minio/neo4j 均 1/1 Ready(前次"图库 Ready"声明属实)
- **TiDB ✅**:tidb-cluster-tidb/tikv-aws/tiflash/pd-aws 均 1/1
- **Pulsar ✅**:pulsar-broker+pulsar-bk+pulsar-zk sts 均 3/3(前次"pulsar 1/1"系 deploy 视角误判,sts 实 3/3 Ready —— 前次 Stage C FAIL 系 VM 断连误判,实已 Ready)
- **Redis ⚠️**:redis-cluster-new sts 6→5(1 未 Ready,待查因)
- **ExternalName 二跳 ✅**:esp-prod 6 短名 → platform-infra → ekb-m2;tidb-primary 经 getent/nc 验证可达

### 恢复锚点(精确子步骤)— 等 API 自愈后执行
1. **前置**:等 k3s API 自愈(Monitor b3r0pfl3w 后台等 `get node` 恢复 Ready)。cri-dockerd socket 断是间歇的,通常数分钟自愈。
2. **降探针压力(优先,解 CrashLoop 雪环)**:patch learning-java/bff-java/learning-worker liveness `timeoutSeconds 15→30`、`periodSeconds 30→60`、`failureThreshold 3→5`;readiness 同步放宽。单节点宽容(memory `esp-loadtest-probe-pitfalls` 已知解法)。
3. **learning-worker Hikari**:加 `connectionTimeout` 放宽(启动慢不被 TiDB 连接超时拖垮)。
4. **redis-cluster-new 缺 1 pod**:describe 查因(OOM/Unready/lost),必要时 force-delete pod 重建。
5. **12 P0 串行 scale**:assessment-rs/course-path-go/frontend-h5/frontend-react/gamification-go/listening-go/live-tutor-go/llm-orchestrator-rs/media-learn-rs/pronunciation-rs/reading-go/vocab-srs-go,每批等 Ready + load 回落(单节点铁律:不并行)。
6. **清 st-seeder 残留**:delete job st-seeder + 11 Error pod。
7. **Task#5**:端到端人工测试(docs/test/09-人工测试操作指南.md)。
8. **Task#6 git commit**:多租户文档/05b/infra_smoke/PROGRESS/Obsidian 未 commit。
9. **Redis 锚点重写**:`esp:ctx:hot:2026-08-13-session22-12-crashloop-diagnosis`(本节内容,本轮写入)。

### 关键约束(本会话遵守)
- host kubectl 间歇 reset 时,命令重试或等 API 自愈;不自行 rdctl start/restart k3s(铁律)。
- 单节点铁律:重操作串行;不碰 k3s 配置/pkill/Factory Reset;VM 重启后必预置 pause 镜像。
- colima 已关,不再占资源;如需重开 docker build 用 `colima start`。
- Bash 输出限流:`--tail=N`/`| head/grep`;大输出落 report/logs。
- 双轨落盘铁律:上下文达 10% 起,每有重大修改/里程碑即时更新 PROGRESS.md + Redis 锚点。
