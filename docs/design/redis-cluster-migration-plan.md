# Redis Cluster 升级 + 跨区同步方案

> ESP 缓存层架构升级设计。路线(用户已确认):Redis 升级真 Cluster + RedisShake 跨区同步。
> **状态(2026-08-06):单节点已落地** —— `redis-cluster-new` StatefulSet 6/6 Running(Cluster 模式 3主3从,无密码),全应用经 ExternalName `redis-cluster`(→`redis-cluster-new`)以 cluster 模式访问。下文 §1"现状"为迁移前历史,§7/§8 应用侧与 manifest 草案为已执行记录 + 多节点扩展蓝本。多区域 Cluster 同步仍属多节点阶段任务。

---

## 1. 现状与问题(迁移前历史,2026-08-05 前状态)

| 维度 | 现状 | 问题 |
|---|---|---|
| 部署形态 | 名为 `redis-cluster`,实为**主从复制**;aws 区 1 master + azure/gcp 各 1 slave(`--replicaof redis-cluster.ekb-m2-aws-us-east-1.svc.cluster.local 6379`) | 名字误导,无 Cluster 模式能力 |
| 版本 | Redis 7.2.13,`CLUSTER NODES` 返回 `cluster support disabled` | 非 cluster 模式,无 slot 分片 |
| 高可用 | **无 Sentinel**,master 挂需人工 `replicaof no one` + 改 ExternalName | failover 不可自动,影响 SLA |
| 分片 | 无分片,单实例承载全量 key | 受单实例内存上限约束,无法水平扩展缓存容量 |
| 跨区 | master→slave **纯异步复制**,slave `master_link_status:up` | 跨区失效窗口存在数据不一致(主挂丢未复制写) |
| 三区分布 | 三区 Pod 实际都在 lima-rancher-desktop 同一 node(10.42.0.134/133/138),跨区为逻辑模拟 | 单节点 RD 环境的拓扑验证性质,非真跨区 |

**核心结论**:当前是"伪 Cluster + 无自动 failover 的主从",扛不住工业级可用性与扩展性要求。

---

## 2. 目标架构

### 2.1 拓扑

Redis Cluster 模式,**3 shard × 3 replica = 9 节点**,主从跨区分布保证任一区全挂仍有多数 shard 可用:

| Shard | 主 | 从1 | 从2 |
|---|---|---|---|
| shard-1 | aws | azure | gcp |
| shard-2 | azure | aws | gcp |
| shard-3 | gcp | aws | azure |

- 16384 slot 哈希分片(`CLUSTER SLOTS` 自动均衡)。
- 任一区全挂:每 shard 至少 1 副本在存活区,集群多数派(≥ shard 半数 master 在线)可继续服务。
- Gossip 协议自动故障检测 + 从节点选举升主。

### 2.2 跨区同步(可选,见 §4 决策)

- 开源方案:RedisShake v4 sync 模式,跨 Cluster 单向链式(aws→azure→gcp)或星型。
- 商业方案:Redis Enterprise Active-Active(不选,开源栈)。

---

## 3. 分片 key 设计

ESP 现有 key 前缀分片均衡性评估:

| Key 模式 | 分布特征 | 分片评估 | 处置 |
|---|---|---|---|
| `user:{id}` | id 数值/雪花,hash 后均衡 | 均衡 ✅ | 无需 hashtag |
| `learning:due:{uid}` | 按 uid hash | 均衡 ✅ | 无需 hashtag |
| `session:*` | 同 prefix,可能热点 shard | 热点风险 | 加 hashtag `{uid}` 让同用户 session 落同 shard(便于 pipeline/MULTI/事务) |
| `esp:ctx:*` | 运行态 key,量小 | 单 shard 即可 | 可放固定 shard 或不强分片 |
| `esp-cache:*`(L2 缓存) | key 含业务 id | 均衡 | 默认 hash |

**hashtag 规则**:Redis Cluster 按 `{...}` 内子串分片。ESP 约定:涉及同用户多 key 原子操作的场景,统一用 `{uid}` hashtag。

---

## 4. 跨区同步方案(关键决策)

### 4.1 两个候选

| 方案 | 说明 | 优点 | 缺点 |
|---|---|---|---|
| A. RedisShake 跨区同步 |各区 Cluster 间单向/双向同步 | 读本地命中降延迟 | 双向 Active-Active 有写冲突;运维复杂;同步延迟 |
| B. 各区独立 Cluster + cache-aside 回源 | 不跨区同步 Redis,各区内独立 Cluster;miss 回 DB | 简单;无冲突;DB 是唯一真相源 | 跨区读 miss 需回 DB(可接受) |

### 4.2 决策:ESP 选方案 B(各区独立 Cluster,不强同步)

理由(经得起架构拷问):
1. ESP Redis 存的是**缓存**(cache-aside 模式),DB(TiDB)是 source of truth,缓存可重建。
2. 跨区同步 Redis 增加复杂度与延迟,收益仅"跨区读命中",但跨区读 miss 回 DB 的成本在 ESP 量级(5000 CCU)可接受。
3. TTL 兜底(60s)保证缓存最终新鲜,跨区不一致窗口可接受。
4. 双向 Active-Active 的冲突解决(last-write-wins)对缓存是过度设计。

> RedisShake 仍作为**应急数据迁移/校验工具**保留(如 Cluster 扩容 slot 迁移、主从→Cluster 一次性数据搬迁),不做常态跨区同步。

---

## 5. failover

| 维度 | 主从无 Sentinel(现状) | Sentinel | Redis Cluster(目标) |
|---|---|---|---|
| 自动 failover | 否(人工切) | 是(Sentinel 仲裁) | 是(Gossip + 从选举) |
| 分片 | 否 | 否 | 是(16384 slot) |
| 客户端复杂度 | 低 | 中( Sentinel list) | 中(cluster nodes + moved/ask 重定向) |
| 运维 | 低 | 中 | 中高 |

Cluster 自带 failover 优于 Sentinel:无额外组件、原生分片、分区少数派自动降级。Sentinel 仅作"不升 Cluster 的轻量 fallback"决策备选(用户已选 Cluster,Sentinel 不采用)。

---

## 6. 读写分离

- 现状:ESP 读写分离在 **TiDB 层**(esp-rw-java),Redis 层不分读写(全走 master/ExternalName)。
- Cluster 后:从节点默认拒读,需 `READONLY` 命令开启。Lettuce 用 `ReadFrom.REPLICA_PREFERRED` 让 L2 读走副本,降主压力。
- 写仍走各 shard 主(强一致)。
- 注意:从节点读是**最终一致**(异步复制延迟),ESP 缓存场景可接受。

---

## 7. 应用侧改动

| 项 | 现状 | 改动 |
|---|---|---|
| 连接配置 | `spring.data.redis.host=redis-cluster:6379`(单 host) | `RedisClusterConfiguration`:`spring.data.redis.cluster.nodes=redis-cluster-0:6379,redis-cluster-1:6379,...` + `max-redirects=3` |
| RedisTemplate | StringRedisSerializer key + GenericJackson2JsonRedisSerializer value + default-typing | **不变**,序列化逻辑兼容 Cluster |
| esp-cache CacheConfig | 单 host Lettuce | 加 cluster 配置;Lettuce `topologyRefresh` 开启(动态感知 slot 迁移) |
| 连接池 | 默认 | Lettuce pool 调整:`max-active` / `min-idle` 按 shard 数缩放 |
| 读策略 | 默认 master | `ReadFrom.REPLICA_PREFERRED` |

---

## 8. K8s manifest 草案

草案文件:`deploy/k8s/06c-redis-cluster.yaml`(见同目录)。

要点:
- 3 个 StatefulSet(每 shard 1 主 2 从跨区),或 1 个 StatefulSet 9 副本 + init 脚本分配 shard。
- Headless Service `redis-cluster` 供 Pod DNS 解析(`redis-cluster-0.redis-cluster` 等)。
- `redis-cli --cluster create` 初始化 Job(一次性,`--cluster-replicas 2`)。
- RedisShake Deployment(每区一个,仅作迁移工具,非常态同步)。
- manifest 内注释说明初始化步骤顺序(先建 StatefulSet → 跑 init Job 建簇 → 校验 `CLUSTER INFO cluster_state:ok`)。

---

## 9. 迁移步骤(灰度)

1. **双写**:应用同时写老主从 + 新 Cluster(双写开关,CacheConfig 增加 dual-write flag)。
2. **校验**:RedisShake `dump`/`restore` 比对 key 数与抽样值一致。
3. **切读**:应用读切到新 Cluster(`ReadFrom.REPLICA_PREFERRED`)。
4. **切写**:关闭老主从写,写全切新 Cluster。
5. **下线**:观察 N 天后下线老主从 Deployment + PVC。

**回滚**:任一步骤异常,回退 switch 到老主从(数据双向可逆,因双写期新老一致)。

---

## 10. 风险与成本

| 项 | 评估 |
|---|---|
| 资源成本 | 9 节点 vs 当前 3 节点:内存 ×3、CPU ×3、PV ×3。按每节点 2Gi 内存估,9 节点 ≈ 18Gi + 9 PV(5Gi)。工业级可接受,单节点 RD 跑不动。 |
| 单节点 RD | **跑不动 9 节点 Cluster**。单节点继续用主从验证拓扑连通性,Cluster 落地记为多节点移植项。 |
| RedisShake 运维 | 仅迁移用,运维负担低;常态同步不采用(见 §4)。 |
| Cluster 客户端重定向 | moved/ask 重定向增加 RTT,`max-redirects` 兜底,Lettuce 自动处理。 |
| slot 迁移影响 | 扩缩容时 slot 迁移期间部分 key 短暂不可用,选低峰执行。 |

---

## 11. 结论与建议(分阶段路线)

| 阶段 | 动作 | 理由 |
|---|---|---|
| 当前(单节点 RD) | **维持主从 + 加 Sentinel 补 failover** | RD 跑不动 9 节点 Cluster;Sentinel 轻量补自动 failover 短板 |
| 多节点移植(目标) | **升 Redis Cluster(3×3)+ 各区独立,不强同步** | 工业级分片 + 自动 failover;cache-aside 不需跨区强一致 |
| 应急/迁移 | RedisShake 作一次性数据搬迁工具 | 主从→Cluster、Cluster 扩容 slot 迁移 |

**明确标注:单节点 RD 阶段不落地 Cluster,多节点移植时执行。**

---

## 关键架构决策点(一句话结论)

1. **分片策略**:3 shard × 3 replica,主从跨区分布;`session:*` 加 `{uid}` hashtag,其余按 hash 自然均衡。
2. **跨区同步选型**:各区独立 Cluster + cache-aside 回源,不强同步(缓存可重建,DB 是真相源,TTL 兜底)。
3. **读写分离**:Cluster 后 L2 读走副本(`ReadFrom.REPLICA_PREFERRED`),写走主。
4. **failover**:Cluster Gossip 自带,淘汰现状无 Sentinel 的手动切换;Sentinel 仅作不升 Cluster 时的 fallback。
5. **迁移灰度**:双写→校验→切读→切写→下线,五步可回滚。
