# platform-infra 局域网通用平台底座 —— 多租户隔离设计

> 评估视角:教授级高级工程师 / 互联网大厂资深架构师(10亿用户/100万并发工业级基线)。
> 关联:`deploy/k8s/00-platform-infra-namespace.yaml`、`deploy/k8s/05b-platform-infra-externalnames.yaml`、`deploy/k8s/05-infra-reuse.yaml`、`docs/design/architecture/multi-node-topology.md`、`docs/design/schema-per-service-phase0.sql`。
> 日期:2026-08-13。
>
> **2026-08-20 更新**:Neo4j 已从集群 `ekb-m2-aws-us-east-1` ns 下沉宿主(brew install neo4j 5.x,7474/7687),ESP 指向改 `host.lima.internal:7474`(orchestrator-rs `NEO4J_URL`)+ llm-cache-proxy 直连 `127.0.0.1`。邻居 education-platform 的 Neo4j 已弃用(改 NebulaGraph),删 ekb deploy 零影响。Milvus 仍集群 ns(scale=0,Phase2 待下沉宿主)。本文 §1/§6 中 Neo4j "集群 ns / scale=0 / 容器内" 描述为下沉前快照,Neo4j 部署形态以本注为准。下沉计划见 `report/milvus-neo4j-sink-plan.md`。

## 1. 背景与目标

ESP 当前基础设施复用模式是"雏形共享":TiDB/Redis Cluster/Pulsar/Milvus/Neo4j 物理集中在 `ekb-m2-aws-us-east-1` ns,ESP 业务(`esp-prod`)通过 4 个 ExternalName 短名访问。三个缺口驱动平台化:

1. **图库/向量库未启用**:Milvus+Neo4j+minio 均 scale=0,语义检索/知识图谱缺底座。
2. **伪读写分离**:`tidb-primary`/`tidb-follower` 同指 `tidb-cluster-tidb`(单节点 tidb replicas=1)。
3. **无多租户模型**:基础设施绑死 ESP 私有 ns,无法接纳其他项目。

**目标**:抽 `platform-infra` ns 作为统一入口层,设计五组件多租户隔离矩阵,ESP 作为首批租户,为新项目预留 3 步接入路径。单节点为验证拓扑,多节点移植零租户改动。

## 2. 架构:二跳解耦入口层

```
租户A(esp-prod)──┐                              ┌── tidb-cluster-tidb.ekb-m2
租户B(tenantB) ──┼──> platform-infra 入口层 ──> ├── redis-cluster-new.ekb-m2
租户C(...)     ──┘  (6 ExternalName 短名)       ├── pulsar-broker.ekb-m2
                                                 ├── milvus.ekb-m2
                                                 └── neo4j.ekb-m2
```

- 租户 ns 内建 6 个 ExternalName(如 `tidb` → `tidb.platform-infra.svc.cluster.local`),与平台入口同名。
- 平台入口(`05b`)指向底层 `ekb-m2` 真实 svc。
- **组件迁移隔离**:底层 svc 从 ekb-m2 搬到 platform-infra(多节点),只改 `05b` 的 externalName,租户侧 ExternalName 指向 `*.platform-infra.svc.cluster.local` 不变。
- **DNS 二跳代价**:租户→平台→底层两跳 CNAME 解析,CoreDNS 缓存后 <1ms,可忽略。

## 3. 多租户隔离矩阵

每租户独立 K8s ns + 每组件独立隔离单元。ESP 为首批租户实例:

| 组件 | 隔离单元 | ESP 租户实例 | 其他租户扩展 | 隔离强度 |
|---|---|---|---|---|
| **TiDB** | database(schema-per-service) | `esp` / `esp_learning` / `esp_writing` ...(23 个 esp_* DB)| `tenantB_*` | 强(独立 DB,权限可收口)|
| **Redis Cluster** | key 前缀 | `esp:ctx:*` / `esp:cache:*` | `tenantB:*` | 中(共享 cluster,前缀约定 + ACL)|
| **Pulsar** | tenant + namespace | tenant `esp`,ns `esp/<module>` | tenant `tenantB` | 强(原生多租户)|
| **Milvus** | collection(同 db) | collection `esp_*` | collection `tenantB_*` | 中(同 db,collection 命名约定)|
| **Neo4j** | label + 节点命名空间 | 默认 db 内 `:Tenant` 标签 + `esp_*` 前缀 | `tenantB_*` 前缀 | 弱(社区版无多 db,见下)|

**隔离设计原则**(工业级对标):
- **TiDB**:已有 `esp_*` 与 `ep_*` 并存先例,沿用 `<tenant>_<service>` 命名。生产收口:每租户独立 DB 用户,GRANT 仅授权本租户 DB,杜绝跨租户读写。
- **Redis**:共享 Cluster 成本最优(6 节点已就绪),用 key 前缀 + Redis ACL(7.2+)限制用户只能操作本前缀。强隔离场景(合规要求)用独立 Cluster。
- **Pulsar**:原生 tenant 隔离最强,pulsar-admin 建租户 + 授 policy,topic 全限定名 `persistent://<tenant>/<ns>/<topic>`。
- **Milvus**:单 db 下 collection 隔离,生产可升 db 级隔离(Milvus 2.5+ 支持多 db)。
- **Neo4j**(⚠️ 社区版限制,2026-08-13 实测确认):Neo4j **Community Edition 不支持 `CREATE DATABASE`** —— HTTP API 返回 `Neo.ClientError.Statement.UnsupportedAdministrationCommand`。社区版只能用默认的 `neo4j`/`system` 两个库。多租户隔离改用 **label + 节点命名空间**:在默认 `neo4j` 库内,每租户节点打 `:Tenant {name:'esp'}` 标签,Cypher 查询带 `WHERE t.name=$tenant` 过滤。生产强隔离需升级 **企业版**(支持多 database + RBAC),或独立 Neo4j 实例/租户。

## 4. 新租户接入 SOP(3 步)

```
# Step 1: 建租户 ns
kubectl create ns tenantB
# Step 2: 在租户 ns 建 6 个 ExternalName 指向 platform-infra(复制 05-infra-reuse.yaml 模式,改 ns)
kubectl apply -f deploy/k8s/tenants/tenantB-infra-reuse.yaml
# Step 3: 申请各组件隔离单元
#   TiDB: CREATE DATABASE tenantB_* + GRANT
#   Redis: ACL CREATEUSER tenantB + 前缀 tenantB:
#   Pulsar: pulsar-admin tenants create tenantB + namespaces create tenantB/<ns>
#   Milvus: create collection tenantB_*
#   Neo4j: CREATE (:Tenant {name:'tenantB'})  -- 社区版 label 隔离(非 CREATE DATABASE)
```

## 5. 阶段C1:TiDB 读写分离真分流(多节点目标)

- **单节点现状**:`tidb-cluster-tidb` replicas=1,`tidb`/`tidb-follower` 同指一 svc,伪分离。
- **多节点目标**:tidb replicas=2+(见 `multi-node-topology.md` D2 容量表),拆 `tidb-cluster-tidb-primary` / `tidb-cluster-tidb-follower` 两个 svc,`05b` 的 `tidb-follower` externalName 改指 follower svc。
- **TiDB 读写分离机制**:TiDB server 无状态,任意节点可读写;真正分流靠 JDBC `READ_ONLY` hint 或 proxy 层(如 TiDB + HAProxy read-only 路由)。单节点验证阶段以注释标记,多节点实现。

## 6. 单节点约束与风险

| 风险 | 现状 | 缓解 |
|---|---|---|
| VM OOM | 9核/29G,可用 5.6G;milvus lim 6G + neo4j 2G 超 | 阶段B 压低 milvus/neo4j limit + 串行起 + 监控 free |
| Milvus/Neo4j 无 PVC | emptyDir / 无卷,重启丢数据 | 阶段B 补 PVC(local-path),平台底座不可无持久化 |
| Neo4j 无 resources | 社区版默认吃内存 | 显式设 heap=512m + limit 1280Mi(实测 768Mi OOMKilled,见坑㉒)|
| Neo4j 社区版无多 database | `CREATE DATABASE` 被拒(UnsupportedAdministrationCommand) | label 隔离;生产升企业版或独立实例 |
| Neo4j 容器内 cypher-shell OOM | cypher-shell 起额外 JVM 叠加主进程超 limit → exit 137 | 用外部 pod HTTP API 操作,不在容器内起 JVM |
| Pulsar admin OOM | broker JVM 堆不足执行 pulsar-admin | 建 tenant 用 broker-2 或调大 heap |
| ExternalName 二跳延迟 | 两跳 CNAME | CoreDNS 缓存,<1ms |

## 7. 多节点移植清单

单节点验证通过后,移植到多节点服务器:
1. 把 TiDB Operator / Redis Operator / Pulsar Helm / Milvus Helm / Neo4j 装到 `platform-infra` ns。
2. `05b` 的 6 个 externalName 改指本 ns svc(如 `tidb` → `tidb.platform-infra.svc.cluster.local` 一跳)。
3. 租户侧 ExternalName 零改动(仍指 `*.platform-infra.svc.cluster.local`)。
4. 按需扩容:TiDB tidb=2+ / Redis 6主6从 / Pulsar broker 跨 AZ / Milvus 集群版 / Neo4j Causal Cluster。
5. 跨地域:参照 `multi-node-topology.md` D1 三地域拓扑 + `pulsar-geo-replication-plan.md`。

## 8. ESP 租户接入收口

ESP 当前 `05-infra-reuse.yaml` 的 4 个 ExternalName 改指向 platform-infra 入口:
- `tidb-primary` → `tidb.platform-infra.svc.cluster.local`
- `tidb-follower` → `tidb-follower.platform-infra.svc.cluster.local`
- `redis-cluster` → `redis-cluster.platform-infra.svc.cluster.local`
- `pulsar-broker` → `pulsar.platform-infra.svc.cluster.local`

ESP 业务侧 env 不变(仍用短名 `tidb-primary`/`redis-cluster`/`pulsar-broker`),仅 ExternalName 目标改指向,业务零改动。新增 milvus/neo4j 短名待阶段B 启用后按需接入。
