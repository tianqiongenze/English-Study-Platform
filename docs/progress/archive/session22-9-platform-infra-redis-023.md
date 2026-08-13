# 会话22-9:平台底座化(阶段A✅)+ redis 坑㉓闭合 + 阶段B/C(2026-08-13)

> 从 `docs/PROGRESS.md` 归档(2026-08-14 会话22-15 瘦身)。

## 会话22-9:平台底座化(阶段A✅)+ redis 坑㉓闭合 + 阶段B/C(2026-08-13)

用户指令:① 自动从 PROGRESS+Redis 恢复完整完成所有任务;② Redis 重建是否丢数据 + 把 Redis/Pulsar/TiDB/图库/向量库共享为局域网通用基础设施;③(决策)全部启用图+向量库 + 升级为局域网通用平台底座;④ 立即双轨落盘(PROGRESS+Redis)使上下文溢出后能完整恢复。

### 现状实查(2026-08-13)
- **Redis Cluster**:`redis-cluster-new` 6 pod 全 1/1 Running(18m),`cluster_state:ok slots_ok:16384 cluster_size:3 known_nodes:6`,全 FQDN announce。esp-prod 跨 ns `SET/GET esp:ctx:plat-test` 成功(坑㉓ 闭合)。
- **平台底座(阶段A✅)**:`platform-infra` ns 6 ExternalName 入口全在(tidb/tidb-follower/redis-cluster/pulsar/milvus/neo4j);ESP `05-infra-reuse.yaml` 二跳指 platform-infra。
- **阶段B(⏳)**:milvus(0/0)+milvus-minio(0/0)+neo4j(0/0) 均 scale=0,122d 前创建。VM 9核/29.3G,可用 6.0G,负载 26%。
- **esp-prod 业务**:gateway-go/learning-java/bff-java/learning-worker 已 1/1;余 ~14 deploy 待 scale up(Task#4)。
- **双轨落盘(✅)**:PROGRESS.md 本节 + Redis 锚点 `esp:ctx:hot:2026-08-13-session22-9-platform-infra`(2504B,STRLEN 验证)。

### 坑㉓(Redis 跨命名空间 hostname 阻断)✅ 闭合
- **根因**:redis sts `--cluster-announce-hostname $(POD_NAME).redis-cluster-new`(无 ns 后缀),cross-ns 客户端 esp-prod 解析失败 → MOVED 路由到短名节点超时。
- **修复**:sts args 改 FQDN `$(POD_NAME).redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local` + `--cluster-preferred-endpoint-type hostname`。
- **重建难点**:nodes.conf/dump.rdb/appendonlydir 在 PVC 持久化旧短名元数据,patch args 后仍公告短名。需 6 pod 逐一 `FLUSHALL + CLUSTER RESET HARD + rm -f /data/nodes.conf /data/dump.rdb + rm -rf /data/appendonlydir`(AOF 残留恢复旧集群元数据),再 force-delete pod + re-apply manifest 重建 init Job。
- **数据丢失**:重建 FLUSHALL 清空。Redis 仅存跨会话运行态/缓存/进度(非业务数据,可重灌)。
- **关联**:[[esp-redis-cluster-hostname-announce-023]] memory 已更新。

### 平台底座多租户设计(阶段A 文档+manifest,纯文件不动运行态)
- 新增 `deploy/k8s/00-platform-infra-namespace.yaml` — platform-infra ns(shared-base 标签)。
- 新增 `deploy/k8s/05b-platform-infra-externalnames.yaml` — 6 组件统一 ExternalName 入口层(二跳解耦:组件迁移只改 05b,租户零改动)。
- 修改 `deploy/k8s/05-infra-reuse.yaml` — ESP 4→6 ExternalName 改指 `*.platform-infra.svc.cluster.local`(业务 env 短名不变,零改动)。
- 新增 `docs/design/platform-infra-multitenant.md` — 5 组件隔离矩阵(TiDB DB / Redis 前缀 / Pulsar tenant / Milvus collection / Neo4j database)+ 新租户 3 步接入 SOP + 单节点风险 + 多节点移植清单。

### 恢复锚点(精确子步骤)— 阶段B 进行中
1. **当前**:阶段B,3 组件 scale=0。VM 可用 6.0G。
2. **B1 Milvus**:查 deploy/milvus spec(内嵌 etcd 或 standalone),压 limit req256/lim512,串行 scale milvus-minio→1(等 Ready+load 回落)→ milvus→1,验证 `:19530` 可连 + list collections 空。
3. **B2 Neo4j**:压 limit req256/lim768 + env NEO4J_dbms_memory_heap_max__size=512m + 补 PVC(local-path),scale→1,验证 `:7474` + cypher `CREATE DATABASE esp`。
4. **B3**:apply 05b(已含 milvus/neo4j ExternalName),esp-prod 跨 ns 测短名解析。
5. **阶段C**:TiDB 读写分离注释(单节点伪分离,多节点 tidb=2 真分流)+ `scripts/infra_smoke.sh` 6 组件冒烟 + 落盘 commit。
6. **Task#4**:恢复余下 ~14 P0 deploy + 验证 healthz(单节点串行,每批等 load 回落)。
7. **Task#5**:端到端人工测试(docs/test/09-人工测试操作指南.md)。

### 关键约束(本会话遵守)
- host kubectl 坏(EOF @ 127.0.0.1:6443),全用 `rdctl shell sudo kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml`。
- 单节点铁律:不碰 k3s 配置/restart/pkill/Factory Reset;主机重 mvn 与集群操作串行;VM 重启后必预置 pause 镜像。
- 文件传输到 VM 用 base64 内联(`B64=...; rdctl shell sudo sh -c "echo '$B64'|base64 -d>..."`)。
- Bash 输出限流:`--tail=N` / `| head/grep`;大输出落 report/logs。
