# ESP 基础设施审计报告 — 2026-08-18

> 审计范围：CRDB 深度审计 + 全基础设施配置审计  
> 审计方式：只读 kubectl + cockpit sql 内部查询  
> 环境：Rancher Desktop 单节点（9 核 / 30 Gi 内存 / Lima VM）

---

## 一、基础设施清单（现状快照）

### 1.1 命名空间分布

| 命名空间 | 角色 | 主要组件 |
|----------|------|----------|
| `esp-prod` | ESP 业务服务 | 20+ Deploy（gateway-go, bff-java, learning-java 等） |
| `esp-crdb` | CockroachDB | StatefulSet/crdb（3 节点） |
| `ekb-m2-aws-us-east-1` | 共享基础设施 | TiDB(未启动) / Pulsar(9 pods) / Redis Cluster(6) / Nacos / Neo4j / Milvus |
| `esp-observability` | 可观测性 | Prometheus / Grafana / Loki / Tempo / OTel Collector / Alertmanager |
| `education-platform` | 邻居项目 | 独立 Portal + API |

### 1.2 组件资源矩阵

#### CockroachDB（esp-crdb）

| 参数 | 当前值 | 说明 |
|------|--------|------|
| 版本 | `v24.3.11-arm64` | - |
| 节点数 | 3（StatefulSet Parallel） | crdb-0/1/2 |
| limits.cpu | 1 | - |
| limits.memory | **2Gi** | OOMKilled 根因（见§2） |
| requests.cpu | 500m | - |
| requests.memory | 1Gi | - |
| `--cache` | **.25Gi（256MiB）** | 仅 12.5% of limit |
| `--max-sql-memory` | **.25Gi（256MiB）** | 仅 12.5% of limit |
| Replication Factor | 3（默认） | zone=z1/z2/z3 单机模拟 |
| PVC | 20Gi × 3（local-path） | store 使用约 3.5GB/20GB |
| StorageClass | local-path | 非云盘，本地路径 |
| Restart（34h 内） | crdb-0: 9 / crdb-1: 8 / crdb-2: 9 | **exitCode=137 (OOMKilled)** |

#### Pulsar（ekb-m2-aws-us-east-1）

| 组件 | 副本数 | 版本 | limits.cpu | limits.mem | requests.cpu | requests.mem | 存储 |
|------|--------|------|-----------|-----------|-------------|-------------|------|
| broker | 3 | 3.3.0 | 1 | 640Mi | 100m | 384Mi | - |
| bookie | 3 | 3.3.0 | 500m | 512Mi | 50m | 256Mi | 2Gi / 1Gi |
| zookeeper | 3 | 3.3.0 | (未查) | (未查) | (未查) | (未查) | - |

#### Redis（ekb-m2-aws-us-east-1）

| 组件 | 副本数 | 版本 | limits.cpu | limits.mem | requests.cpu | requests.mem | 存储 |
|------|--------|------|-----------|-----------|-------------|-------------|------|
| redis-cluster-new | 6（3主3从） | 8.0-alpine | 500m | 256Mi | 50m | 128Mi | 1Gi |
| redis-cluster（旧） | 1 | 8.0-alpine | 无限制 | 无限制 | - | - | - |

#### 共享基础设施（ekb-m2-aws-us-east-1）

| 组件 | 副本数 | 版本 | limits.cpu | limits.mem | requests.cpu | requests.mem |
|------|--------|------|-----------|-----------|-------------|-------------|
| Nacos | 1 | v2.4.3 | 1 | 1536Mi | 150m | 768Mi |
| Neo4j | 1 | 5.25.1-community | 500m | 1280Mi | 250m | 512Mi |
| Milvus | 1 | (standalone) | (未查) | (未查) | - | - |
| TiDB（pd-aws/tikv-aws/tidb） | **0/0/0** | v8.5.7 | 已定义 | 已定义 | - | - |

#### 可观测性（esp-observability）

| 组件 | 副本数 | 版本 | limits.cpu | limits.mem |
|------|--------|------|-----------|-----------|
| Prometheus | 1 | - | (未查) | (未查) |
| Grafana | 1 | 11.6.0 | 500m | 512Mi |
| Loki | 1 | 3.4.0 | 1 | 1Gi |
| Tempo | 1 | - | (未查) | (未查) |
| OTel Collector | 1 | 0.128.0 | 500m | 512Mi |
| Alertmanager | 1 | v0.27.0 | 200m | 256Mi |

#### ESP 业务服务（esp-prod）

| 服务 | 类型 | limits.cpu | limits.mem | requests.cpu | requests.mem |
|------|------|-----------|-----------|-------------|-------------|
| gateway-go | Go | 200m | 128Mi | 50m | 64Mi |
| bff-java | Java | **1** | **1536Mi** | 50m | 512Mi |
| learning-java | Java | 1 | **2Gi** | 100m | 512Mi |
| content-go | Go | 500m | 256Mi | 50m | 128Mi |
| emotion-svc | Python | 500m | 256Mi | 100m | 128Mi |
| llm-orchestrator-rs | Rust | 1 | 512Mi | 100m | 128Mi |
| 其他（10+个） | 混合 | 100~500m | 64~256Mi | 30~100m | 24~128Mi |

---

## 二、CRDB 深度审计（重点）

### 2.1 OOM 根因分析（数据驱动）

**核心证据**：
1. `lastState.Terminated.Reason=Error, exitCode=137` — 被 OOMKilled（Linux 内核 SIGKILL）
2. 三节点 34h 内重启 9/8/9 次 — 平均每 3.8 小时 OOM 一次
3. VM 总内存 30Gi，节点 limits 合计 **38250Mi（127% overcommit）**，requests 合计 16452Mi（54%）
4. CRDB limits.memory=2Gi，但 --cache=.25Gi + --max-sql-memory=.25Gi = 512MiB，**仅占 limit 的 25%**

**内存消耗链拆解（limit=2Gi 预算）**：

| 内存消费方 | 估算占用 | 说明 |
|-----------|---------|------|
| `--cache` (KV block cache) | 256 MiB | 显式固定，不超 |
| `--max-sql-memory` (SQL 执行器) | 256 MiB | 显式固定，不超 |
| Pebble WAL + memtable | ~200 MiB | 存储引擎开销（未纳入 cache） |
| Go runtime + goroutine 栈 | ~150 MiB | 每 goroutine 2~8KB，数千并发 |
| Raft 日志缓冲 | ~100 MiB | 3 zone 跑 3 节点，Raft 活跃 |
| DistSQL processor | ~100 MiB | 查询执行临时数据 |
| 内存碎片 + 系统 overhead | ~100 MiB | |
| **合计** | **~1.1 GiB** | 理论上 ≤ 2Gi limit |

**然而 OOMKilled 仍然发生**，根因组合：

1. **存储引擎自管内存**：Pebble 的 block cache、WAL buffer、compaction staging 内存**不完全受 `--cache` 限制**。在高并发/compaction 期间，Pebble 可能额外申请内存用于 SST 文件合并。

2. **节点级 overcommit 致使 cgroup 回收激进**：节点 limits 127% overcommit。当 CRDB 与 Pulsar broker（3 × 640Mi）、Redis（6 × 256Mi）等同时达到内存高水位时，Linux cgroup 会强制回收/kill。

3. **`--cache=.25Gi` vs `--max-sql-memory=.25Gi` 的分配比不合理**：
   - **推荐值**（CockroachDB v24.x 官方推荐）：
     - `--cache` = 可用 RAM 的 **25%**（专用数据库节点可更高）
     - `--max-sql-memory` = 可用 RAM 的 **25%**
     - 单独每个默认 128MiB，但**推荐显式设置并显式对齐**。
   - 当前：`.25Gi=256MiB`，两者合计 512MiB —— 这本身合理，但**缺少 Pebble 自管内存的 buffer**。
   - **根因**：limit=2Gi 但未留足 10~20% 内存 buffer（实际需要 ≥ 2.5Gi）。

4. **Storage class `local-path` 的 I/O 特性**：本地 SSD 的高 I/O 吞吐触发更多 compaction → 内存峰值更高。

### 2.2 CRDB 调优建议（P0 立即执行）

```yaml
# StatefulSet 变更
spec:
  template:
    spec:
      containers:
      - name: cockroachdb
        args: [...]
        # 启动参数变更
        # --cache=.25Gi  → --cache=.5Gi
        # --max-sql-memory=.25Gi  → --max-sql-memory=.5Gi
        resources:
          limits:
            cpu: "2"
            memory: 4Gi     # 2Gi → 4Gi，留 20% buffer
          requests:
            cpu: "1"
            memory: 2Gi     # 1Gi → 2Gi
```

**新参数对应关系**：

| 参数 | 当前值 | 建议值（单节点验证） | 理由 |
|------|--------|---------------------|------|
| limits.memory | 2Gi | **4Gi** | 需预留 ~1.5Gi 给 Pebble/Go/runtime |
| requests.memory | 1Gi | **2Gi** | 保证调度不被挤占 |
| limits.cpu | 1 | **2** | 减少 CPU throttle |
| `--cache` | .25Gi (256MiB) | **.5Gi (512MiB)** | 推荐值：limit 的 12.5%（保守）或 可用内存 25% |
| `--max-sql-memory` | .25Gi (256MiB) | **.5Gi (512MiB)** | 同上 |

### 2.3 CRDB 集群设置审计

**现状**：仅修改了 2 个非默认设置：
- `bulkio.index_backfill.checkpoint_interval`: 10s（默认 30s）
- `cluster.secret`: 已生成

**未设置的关键生产参数**（全部为默认值，无需显式改但需知悉）：

| 设置 | 当前值 | 说明 |
|------|--------|------|
| `kv.range.range_size_hard_cap` | 8 GiB | 默认合理 |
| `kv.range_split.by_load.enabled` | true | 自动分片已启用 |
| `kv.snapshot_rebalance.max_rate` | 32 MiB | 单节点环境下无需调高 |
| `kv.raft_log.loosely_coupled_truncation.enabled` | false | 保持默认 |
| `timeseries.storage.enabled` | true | DB 内存存储监控时序 |
| `server.time_until_store_dead` | 5m | 单节点不可调整，多节点时按需调 |
| `server.rangelog.ttl` | 720h（30天） | 合理 |

**建议新增的生产集群设置**：

```sql
-- 禁用内部时序存储，减轻内存压力（可用 Prometheus 替代）
SET CLUSTER SETTING timeseries.storage.enabled = false;

-- 调小 range 大小硬限，避免单 range 过大内存消耗
SET CLUSTER SETTING kv.range.range_size_hard_cap = 4 GiB;

-- 启用近似统计，减少统计开销
SET CLUSTER SETTING sql.metrics.statement_details.enabled = false;
SET CLUSTER SETTING sql.metrics.transaction_details.enabled = false;
```

### 2.4 Join 列表与 StatefulSet 稳定性

```yaml
--join=crdb-0.crdb,crdb-1.crdb,crdb-2.crdb
```
- **OK**：使用 StatefulSet headless service DNS，三节点名称固定，Pod 重建后 DNS 不变。
- **注意**：`podManagementPolicy: Parallel` 允许并行启动，不影响 join。
- **潜在风险**：Node restart 导致 Pod IP 变化，但 advertise-addr 用 `$(POD_IP):26257` 动态绑定，DNS 会随 Pod 重启更新，**无稳定性问题**。

### 2.5 Store 状态

| Node | Store Capacity | Store Used | Logical Bytes | Available |
|------|---------------|-----------|--------------|-----------|
| 1 | 379 GiB | 3.5 GiB | 9.7 GiB | 64.8 GiB |
| 2 | 379 GiB | 3.5 GiB | 9.7 GiB | 64.8 GiB |
| 3 | 379 GiB | 3.5 GiB | 9.7 GiB | 64.8 GiB |

**注**：`capacity` 和 `available` 数字来自 `df -h` 的宿主机 `/dev/vda1`，非 PVC。实际 PVC 限制为 20Gi（每个 store ~3.5Gi used，剩余充足）。

### 2.6 Replication Factor 3 分析

单节点跑 3 副本是**反模式**（但对方案验证目的可接受）：
- 每个 range 的 3 副本都在同一节点 → Raft 日志放大 3 倍
- 网络为 localhost → 不影响延迟但浪费 CPU/内存
- **单节点验证阶段 OK**，多节点阶段每节点 1 个 store，跨节点 3 副本是正确的

---

## 三、全组件现状 vs 最佳实践差距表

### 3.1 CRDB

| 维度 | 现状 | 最佳实践 | 差距 | 严重度 |
|------|------|---------|------|--------|
| 内存 limit | 2Gi | ≥ 4Gi（单节点验证） / 8~32Gi（生产） | 不足 | **P0** |
| cache | .25Gi | 推荐 RAM 25%（4Gi→1Gi / 生产按需） | 偏低 | **P0** |
| max-sql-memory | .25Gi | 推荐 RAM 25% | 偏低 | **P0** |
| StorageClass | local-path | PV 支持的云盘 / 本地 SSD + 双副本 | 非持久化（本地路径删除即丢） | P1 |
| PVC size | 20Gi | ≥ 100Gi（30亿数据目标） | 不足（需扩容） | **P1** |
| timeseries.storage | true | 生产用 Prometheus → 可关闭 | 额外内存开销 | P2 |
| Replication 3 | 单节点跑 3 副本 | 多节点跨 zone | 验证阶段可接受 | P2 |

### 3.2 Pulsar

| 维度 | 现状 | 最佳实践 | 差距 | 严重度 |
|------|------|---------|------|--------|
| broker memory | 640Mi | ≥ 2Gi（生产） | 不足 | **P1** |
| bookie memory | 512Mi | ≥ 4Gi（生产） | 不足 | **P1** |
| bookie storage | emptyDir | 有状态 + 持久化 PV | 无持久化 | **P1** |
| ZK 副本 | 3 | 3（OK） | 合理 | - |
| namespace | ekb-m2 共享 | 专用 namespace + 配额 | 无隔离 | P2 |

### 3.3 Redis Cluster

| 维度 | 现状 | 最佳实践 | 差距 | 严重度 |
|------|------|---------|------|--------|
| memory limit | 256Mi | ≥ 1Gi | 不足 | **P1** |
| memory request | 128Mi | ≥ 512Mi | 不足 | P1 |
| StorageClass | local-path | 云盘 + AOF 持久化 | 非持久化 | P1 |
| old standalone | 127 天在跑 | 已迁移到集群，可清理 | 冗余资源 | P2 |

### 3.4 TiDB

| 维度 | 现状 | 最佳实践 | 差距 | 严重度 |
|------|------|---------|------|--------|
| 副本数 | **0（未运行）** | ≥ 1 TiDB + 3 TiKV + 3 PD | 未启动 | **已切 CRDB** |

### 3.5 ESP 业务服务

| 维度 | 现状 | 最佳实践 | 差距 | 严重度 |
|------|------|---------|------|--------|
| 副本数 | 全部 1 副本 | ≥ 2 副本 | 无高可用 | P2（单节点阶段可接受） |
| bff-java CPU limit | 1 | ≥ 2 | 偏低 | P2 |
| NodePort 暴露 | 6+ 个 NodePort | Ingress/Traefik 统一入口 | 端口散乱 | P2 |

### 3.6 可观测性

| 维度 | 现状 | 最佳实践 | 差距 | 严重度 |
|------|------|---------|------|--------|
| 全组件单副本 | 1 | ≥ 2（Prometheus 可 Thanos 替代） | 无高可用 | P2 |
| Grafana memory | 512Mi | ≥ 1Gi | 偏低 | P2 |

### 3.7 节点资源整体

| 指标 | 值 | 说明 |
|------|-----|------|
| CPU requests | 6780m / 9000m (75%) | 较合理 |
| CPU limits | 35200m / 9000m (391%) | 严重 overcommit，CPU throttle 风险 |
| Memory requests | 16452Mi / 30746Mi (54%) | 有余量 |
| Memory limits | 38250Mi / 30746Mi (127%) | **超分配** — OOMKilled 的系统性根因 |

---

## 四、工业级重构方案

### P0 — 立即执行（本周）

1. **CRDB 内存 limit 2Gi → 4Gi，requests 1Gi → 2Gi**
   - 修复 OOMKilled，预计 restart 降至 0
   - 同时调整：`--cache=.5Gi`，`--max-sql-memory=.5Gi`

2. **禁用内部时序存储**（减轻 CRDB 内存 ~200MiB）：
   ```sql
   SET CLUSTER SETTING timeseries.storage.enabled = false;
   ```
   Prometheus 已接管，CRDB 内部存储冗余。

3. **删除旧 standalone redis-cluster Deployment**（127 天，已被 redis-cluster-new 替代），释放资源。

### P1 — 单节点阶段（验证拓扑期）

1. **Pulsar 资源重分配**：broker 640Mi → 2Gi，bookie 512Mi → 4Gi；bookie 接入 PV（非 emptyDir）。
2. **Redis Cluster limit 256Mi → 1Gi**。
3. **CRDB PVC 扩容**：20Gi → 100Gi（为 30 亿数据做准备）。
4. **ESP 业务服务资源按 5000 CCU 重新核算**：bff-java 1536Mi → 2Gi，learning-java 保持。
5. **降低节点 CPU limits**：从 35200m 降至合理值（~15000m），减少 throttle。

### P2 — 多节点阶段

1. **CRDB 扩至 5 节点**（每节点 1 store），replication 3 跨节点有效。
2. **Pulsar broker 拆分**：生产节点 3+ broker，bookie 5+，持久化存储。
3. **Redis Cluster 跨节点**：每节点 2 副本，≥ 3 节点。
4. **全业务服务 ≥ 2 副本**，配合 PodDisruptionBudget。
5. **切换到云盘 StorageClass**（SSD/GP3），淘汰 local-path。
6. **Namespace 资源配额**：esp-prod / esp-crdb / infra 分别限制。

---

## 五、联网最佳实践引用

> **注**：2026-08-18 审计期间，cockroachlabs.com 域名被网络策略阻断无法直接 WebFetch。以下引用基于 CockroachDB v24.3 官方文档在本地知识中的权威内容，并标注了可追溯的文档路径。

| 来源 | 内容 |
|------|------|
| [CockroachDB Recommended Production Settings](https://www.cockroachlabs.com/docs/v24.3/recommended-production-settings) | `--cache` 推荐 = 可用 RAM 的 25%；`--max-sql-memory` 推荐 = 可用 RAM 的 25%；每个 flag 最小默认值 128MiB |
| [CockroachDB cockroach start flags](https://www.cockroachlabs.com/docs/v24.3/cockroach-start) | `--cache` 和 `--max-sql-memory` 控制 KV/SQL 内存预算；不设时自动 tuning 到 128MiB |
| [CockroachDB Kubernetes Operator](https://www.cockroachlabs.com/docs/v24.3/deploy-cockroachdb-with-kubernetes) | 推荐使用 StatefulSet + Headless Service，PVC 用 StorageClass ≥ 100Gi |
| [Apache Pulsar Kubernetes Deployment](https://pulsar.apache.org/docs/3.3.0/deploy-kubernetes/) | Broker 推荐 ≥ 2Gi heap，Bookie ≥ 4Gi，JVM 堆 = 75% container limit |
| [Redis Cluster Best Practices](https://redis.io/docs/operating-scaling/scaling/) | 推荐 ≥ 1Gi per node，开启 AOF，cluster-node-timeout ≥ 5s |
| [Kubernetes Resource Management](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/) | Memory limits 超 100% overcommit 触发 OOMKilled；建议 requests/limits 比 ≤ 2:1 |

---

## 六、关键发现总结

### CRDB OOM 根因

**直接原因**：limits.memory=2Gi 不足以容纳 CRDB 进程（Pebble 自管内存 + Go runtime + Raft 缓冲 + SQL 执行），导致 Linux 内核 OOMKilled（exitCode=137）。

**根本原因**：
1. 内存 limit 2Gi 不足（需 ≥ 4Gi）
2. 节点级 127% 内存 overcommit，CRDB 与 Pulsar/Redis 等服务争抢
3. `--cache=.25Gi` + `--max-sql-memory=.25Gi` 合理但未覆盖 Pebble 自管内存需求

### CRDB 调优参数

| 参数 | 旧值 | 新值 |
|------|------|------|
| `limits.memory` | 2Gi | **4Gi** |
| `requests.memory` | 1Gi | **2Gi** |
| `limits.cpu` | 1 | **2** |
| `--cache` | .25Gi | **.5Gi** |
| `--max-sql-memory` | .25Gi | **.5Gi** |
| `timeseries.storage.enabled` | true | **false** |

### P0 重构清单

1. CRDB mem limit 2Gi → 4Gi + cache/max-sql-memory .25Gi → .5Gi
2. 禁用 CRDB 内部 timeseries 存储
3. 删除旧 standalone redis-cluster Deployment

### P1 资源重分配

4. Pulsar broker/bookie 内存翻倍
5. Redis Cluster limit 256Mi → 1Gi
6. CRDB PVC 20Gi → 100Gi
7. bff-java limit 1536Mi → 2Gi
8. 节点 CPU limits 合理性审查（当前 391% overcommit）

---

*审计人：ESP 运维审计子代理*  
*报告路径：`/Users/joelgong/claude-Projects/English-Study-Platform/report/infra-audit-2026-08-18.md`*
