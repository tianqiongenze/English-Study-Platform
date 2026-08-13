# 会话22-13:colima-stop DNS 雪崩事故 + 恢复(2026-08-13)

> 从 `docs/PROGRESS.md` 归档(2026-08-14 会话22-15 瘦身)。

## 当前热区 (2026-08-13 会话22-13 colima-stop DNS 雪崩事故 + 恢复)

### 事故根因(已修正,2026-08-13 21:30 会话22-13 实测核实)
- **触发**:会话22-12 用户指令"关闭无用 colima 释放资源",执行 `colima stop`。
- **机理(根因修正)**:经核实,**RD 的 lima-0(VM,192.168.5.15)寄生在 colima 的 usernet 进程**(`limactl usernet --subnet 192.168.5.0/24`,PID 1094)上。**colima 是 192.168.5.0/24 网络的承载者,RD 是寄生客户端**。`192.168.5.3` 是 colima usernet 的 host-side DNS resolver。
- **前次误区**:会话22-12/13 初判"colima 与 RD 共享 lima 网络,停一个重置网络子系统波及另一个"——不准确。真相是 **RD 依赖 colima 才有网络,colima 是承载方,RD 是寄生方**。停 colima 不是"波及",是"直接抽底盘"。
- **症状链**:`colima stop` → usernet 进程被收 → 192.168.5.3 resolver 失联 → RD VM `/etc/resolv.conf`(nameserver 192.168.5.3)上游断 → CoreDNS `forward . /etc/resolv.conf` 全 timeout(pod 内解析断)→ **kubelet/containerd 拉镜像走节点 resolv.conf(非 CoreDNS)** → 无法解析 registry → 全集群 ImagePullBackOff → esp-prod ns 连带丢失(疑似 GC)。
- **迷惑性**:host(Mac)DNS 正常(`223.5.5.5` 可达),仅 VM 内 DNS 出口断;rdctl shell Broken(但 host kubectl 可达)。

### DNS 紧急修复(2026-08-13 21:22,实测有效)
- **路径**:rdctl shell Broken + limactl 找不到实例 → 改用 **RD ssh.config 直连 lima-0**:
  `ssh -F "/Users/joelgong/Library/Application Support/rancher-desktop/lima/0/ssh.config" lima-0 "sudo sh -c 'cat > /etc/resolv.conf'"`
- **修复**:节点 resolv.conf `nameserver 192.168.5.3` → `nameserver 223.5.5.5 / nameserver 8.8.8.8`(公网 DNS)。
- **验证**:改完 → 删 redis-cluster-new-0 pod 触发重拉 → **54s 内 1/1 Running**;删 milvus/neo4j/pulsar/pd pod → 30-40s 内全 Running;删 tidb/tikv/tiflash pod → tidb/tikv Running(tiflash CrashLoop + neo4j 拉镜像中,非阻塞)。
- **CoreDNS 已 patch**:`forward . 223.5.5.5 8.8.8.8`(pod 内解析)。节点 resolv.conf + CoreDNS 双修后,kubelet 拉镜像路径恢复。

### 铁律(已修正 memory `esp-never-stop-colima-alongside-rd`)
- **colima 是承载方,非"可释放资源"**。RD 运行时**绝对禁止** `colima stop`/`delete`/重启 usernet 进程。
- 释放资源:压低 colima 规格 `colima start --cpu 2 --memory 2`(进程必须 Running),不 stop。
- 如必须重建 colima 网络:先 `rdctl shutdown` 停 RD → 操作 colima → `colima start` → `rdctl start`(全链路)。
- DNS 紧急修复(不停 colima 前提):ssh.config 直连改节点 resolv.conf 为公网 DNS。
- rdctl shell 可能长期 Broken,集群操作用 host kubectl 或 ssh.config 直连。

### 恢复路径(执行中)
1. ✅ 根因修正 + 铁律更新(memory `esp-never-stop-colima-alongside-rd` 第2版)
2. ✅ DNS 紧急修复(ssh.config 改节点 resolv.conf + CoreDNS patch)
3. ✅ 删 ImagePullBackOff pod 触发重拉:redis/pulsar/milvus/tidb/tikv/pd 全 Running
4. ⏳ tiflash CrashLoop + neo4j 拉镜像(非阻塞,后查)
5. ⏳ 重建 esp-prod ns(已确认 NotFound)+ apply 28 manifest(00-namespace-secret 起头)
6. ⏳ 降探针压力(liveness timeout 15→30/period 30→60/failure 3→5)解 CrashLoop 雪环
7. ⏳ 串行 scale 12 P0(assessment-rs/course-path-go/.../vocab-srs-go)
8. ⏳ 清 st-seeder 残留
9. ⏳ Task#5 人工测试 + Task#6 git commit
10. ⏳ Redis 锚点重写(本轮 esp-prod 重建后 redis pod 已 Running 可写)

### 关键约束(本会话遵守)
- 单节点铁律:不碰 k3s 配置/restart/pkill/Factory Reset;DNS 修复属"恢复网络"非"动 k3s 配置"。
- rdctl shell Broken 时用 host kubectl 或 `ssh -F lima/0/ssh.config lima-0`。
- Bash 输出限流;双轨落盘铁律。
