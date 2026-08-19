# Milvus/Neo4j 下沉宿主机方案与可行性评估(2026-08-19)

> 状态:**评估完成,建议 DEFERRED**(联网不可用无法验证官方最佳实践;ESP 零引用无即时收益;Milvus 多组件高难度)
> 联网限制:WebSearch 200/200 配额耗尽 + WebFetch 被网络策略拦,无法查 Milvus/Neo4j 官方下沉最佳实践

## 1. 当前状态(实测)

### 集群内
| 组件 | ns | pod | 镜像 | 年龄 |
|---|---|---|---|---|
| milvus | ekb-m2-aws-us-east-1 | 1/1 | milvusdb/milvus:v2.5.0 | 127d |
| milvus-minio | ekb-m2-aws-us-east-1 | 1/1 | minio/minio:RELEASE.2024-05-28 | 127d |
| neo4j | ekb-m2-aws-us-east-1 | 1/1 | neo4j:5.25.1-community-bullseye | 128d |

### 多区域部署(多项目群共用)
- ekb-m2-aws-us-east-1 / ekb-m2-azure-eastasia / ekb-m2-gcp-europe-west1 三区域都有 milvus+neo4j
- 符合 `esp-shared-infra-ironrule`:多项目群共用底座,ESP 操作不得损害

### ESP 引用情况(关键)
- ESP 服务 env 零引用 MILVUS/NEO4J(grep deploy/k8s + kubectl get deploy env 实证)
- esp-prod 有 milvus ExternalName → `milvus.platform-infra.svc.cluster.local`(不是 ekb-m2,是 platform-infra ns)
- **结论:ESP 当前不直接用 Milvus/Neo4j,它们是 LLM 缓存 L2(语义向量)/L3(知识图谱)预留**

## 2. 下沉可行性评估(基于架构原理,未联网验证)

### Neo4j 下沉宿主(难度:低)
- 架构:单进程,Java,社区版单机
- macOS 支持:官方有 darwin arm64 distribution(tarball)
- 下沉方式:宿主起 neo4j 进程(:7474 HTTP + :7687 Bolt),esp-prod 加 ExternalName `neo4j` → host.lima.internal
- 数据迁移:`neo4j-admin dump` 集群内导出 → 宿主 `neo4j-admin load` 导入
- 风险:单进程无 K8s 自愈(同 Nacos 下沉硬伤)

### Milvus 下沉宿主(难度:高)
- 架构:依赖 etcd(元数据)+ minio(对象存储)+ milvus-server(查询引擎)多组件
- 集群内已是精简单机版(milvus + milvus-minio,无独立 etcd 用内嵌)
- macOS 支持:Milvus 官方有 darwin arm64 release,但需 etcd + minio 都装宿主
- 下沉方式:宿主起 etcd + minio + milvus 3 进程,端口规划(19530 milvus / 9000 minio / 2379 etcd)
- 数据迁移:milvus-minio 数据 dump → 宿主 minio 导入;milvus 元数据在 etcd,需 etcd dump/restore
- 风险:3 组件都需起,端口/数据迁移复杂,单进程无自愈

## 3. 与 Nacos 下沉的对比(为何不建议)

| 维度 | Nacos 下沉(已执行) | Milvus/Neo4j 下沉(建议 DEFERRED) |
|---|---|---|
| ESP 引用 | bff-java 等实际注册(心脏) | **零引用**(预留) |
| 即时收益 | 消除集群 Nacos 单点(形式) | **无**(ESP 不用) |
| 下沉难度 | 单进程 + CRDB 存储(已验证) | Milvus 3 组件 / Neo4j 单进程 |
| 联网验证 | 基于实测 + Nacos 架构知识 | **联网不可用,无法查官方最佳实践** |
| 多区域 | 单点(esp-prod 用) | 3 区域都有(多项目共用) |

## 4. 建议(DEFERRED,带触发条件)

**不建议当前下沉 Milvus/Neo4j**,理由:
1. ESP 零引用,下沉无即时收益(纯形式腾退)
2. 联网不可用,无法验证 Milvus/Neo4j 官方下沉最佳实践(违反 ASV 精神:不臆造方案)
3. Milvus 多组件下沉复杂度高,单节点 VM 资源紧张(9 核已 99% CPU requests)
4. 三区域多项目共用,下沉 ekb-m2 一份影响其他区域架构一致性

**触发下沉的条件**(满足任一才做):
1. LLM 缓存 Phase2(语义向量)启动,ESP 服务真用 Milvus → 下沉有即时收益
2. LLM 缓存 Phase3(知识图谱)启动,ESP 服务真用 Neo4j → 下沉有即时收益
3. 联网恢复,查到 Milvus/Neo4j 官方下沉最佳实践 → 方案有据
4. 多节点迁移(消除单节点资源约束)→ 下沉复杂度降低

## 5. 若强制下沉的执行预案(参考 Nacos 流程,需联网验证后执行)

### Neo4j 下沉(低难度,若真要做)
```bash
# 1. 备份集群 Neo4j 数据
kubectl -n ekb-m2-aws-us-east-1 exec deploy/neo4j -- neo4j-admin database dump --database=neo4j --to=/data/dump
kubectl cp ekb-m2-aws-us-east-1/<neo4j-pod>:/data/dump ~/neo4j-dump

# 2. 宿主装 Neo4j(tarball 或 brew install neo4j)
# 3. 导入:neo4j-admin database load --from=~/neo4j-dump
# 4. 起 neo4j 进程(:7474 + :7687)
# 5. esp-prod 加 ExternalName neo4j → host.lima.internal
# 6. 删集群 neo4j deploy/svc/pvc
```

### Milvus 下沉(高难度,若真要做)
```bash
# 1. 备份 milvus-minio 数据(mc mirror 集群 minio → 本地)
# 2. 备份 etcd 数据(若用独立 etcd)
# 3. 宿主起 etcd + minio + milvus 3 进程
# 4. 导入数据到宿主 minio + etcd
# 5. esp-prod ExternalName milvus → host.lima.internal
# 6. 删集群 milvus/milvus-minio deploy/svc/pvc
```

## 6. 已完成的下沉(对照)

| 中间件 | 下沉状态 | 方式 |
|---|---|---|
| Nacos | ✅ 已下沉宿主(本轮) | ExternalName → host.lima.internal,集群 deploy 已删 |
| Prometheus/Loki/Alertmanager/Grafana | ✅ 已下沉宿主 | ExternalName,集群无 pod |
| OTel Collector | ✅ 已下沉宿主 | ExternalName,集群内 daemonset/gateway 残留已删(本轮) |
| Tempo | ⚠ 下沉配置在,宿主端口冲突未起 | DEFERRED(report/tempo-port-conflict-deferred.md) |
| **Milvus** | ❌ 未下沉(建议 DEFERRED) | 多组件+零引用+联网不可用 |
| **Neo4j** | ❌ 未下沉(建议 DEFERRED) | 零引用+联网不可用 |

## 7. 铁律遵守
- 未碰 Milvus/Neo4j 集群内资源(esp-shared-infra-ironrule,多项目共用底座不动)
- 联网不可用诚实告知,不臆造最佳实践(ASV 精神)
- 方案基于架构原理 + 已有 Nacos/Prometheus 下沉经验,标注"未联网验证"
