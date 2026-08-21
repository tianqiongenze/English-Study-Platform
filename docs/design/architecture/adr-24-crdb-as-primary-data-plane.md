# ADR-24 CockroachDB 作为主数据面(覆盖 ADR-10/16)

| 项 | 值 |
|---|---|
| 编号 | ADR-24 |
| 标题 | CockroachDB 作为 ESP 主数据面 |
| 状态 | Accepted |
| 日期 | 2026-08-21 |
| Supersedes | SAD 第 5 章内联 ADR-10(TiDB 多分区拓扑)/ADR-16(TiKV 异地多活)/ADR-03(TiDB HTAP)— 注:此为 SAD 本地编号,**非**本索引中的 ADR-10(writing-coach)/ADR-16(content-go) |
| 关联 | ADR-11(Redis Cluster 迁移) |

---

## 背景

原 SAD v5.0(2026-07-31)通篇以 **TiDB+TiFlash HTAP+TiCDC 异地多活** 为数据面叙事(原 ADR-03/10/16)。`docs/design/cockroachdb-vs-tidb-multi-region-plan.md`(2026-08-15)给出"不值得引入 CRDB"的 ADR 结论。

**实际(2026-08 起生产)**:ESP 已 **100% 切 CockroachDB**:
- 连接:`crdb-public:26257`(Postgres wire,driver `org.postgresql.Driver`)
- 开关:`ESP_RW_STORAGE_TYPE=cockroachdb`(各服务 deploy env;见 `deploy/k8s/20-learning-java.yaml:30-37` 等)
- 实现:`platform/esp-rw-java/src/main/java/com/esp/rw/CockroachDbDataSourceConfig.java`(`@ConditionalOnProperty(esp.rw.storage.type=cockroachdb)` 激活,双 HikariCP 池 + `RoutingDataSource`)
- Go 侧:`gamification-go/src/main.go:52-53` 等用 `READ_DB_URL`/`WRITE_DB_URL` 默认指向 `crdb-public:26257`

**TiDB 现状(2026-08-20)**:3区域(aws/azure/gcp 逻辑 ns)下电止血,memory `esp-tidb-decommissioned-scale0-2026-08-20`。`deploy/k8s/07-tidb-tikv.yaml` 中 PD/TiKV/TiDB replicas 现 =1(实际底层已不可用);`deploy/k8s/05b-platform-infra-externalnames.yaml:13-30` 仍声明 `tidb`/`tidb-follower` ExternalName 指向已下电集群(作为可回切保留)。

## 决策

**CockroachDB 作为 ESP 主数据面。**

- 读写分离走 **CRDB follower-read(`AS OF SYSTEM TIME`)** + esp-rw-java `CockroachDbDataSourceConfig`(`crdbWriteDataSource` 写池 leaseholder / `crdbReadDataSource` 读池 follower-read)。
- `@DataSource(READ)`/`@DataSource(WRITE)`/`@DataSource(value=READ, consistent=true)` 注解语义与 TiDB 路径完全复用(`RoutingHolder` ThreadLocal + `RoutingDataSource`)。
- 多区域:当前 3 节点 zone=z1/z2/z3 同台 RD VM,无 region 层;multi-region(GLOBAL/REGIONAL BY ROW + SURVIVE REGION FAILURE)为 Phase1 DEFERRED。

## 迁移动因

1. **TiDB mirror pingcap 镜像源 403**:拉取 `pingcap/pd`、`pingcap/tikv` 等被阻(memory `esp-registry-mirror-outage-2026-08-16`),TiDB 运维受阻;CRDB 单二进制无此问题。
2. **CRDB 单二进制 + 多 region 能力贴合 ESP 5000 CCU 尺度**:一个进程同时是 SQL 网关与存储,部署简单;声明式多区域语法(GLOBAL/REGIONAL BY ROW)为后续多区域路径铺路。
3. **Postgres 兼容 + esp-rw-java 已实现 CRDB 路径**:`CockroachDbDataSourceConfig` 与 TiDB `DataSourceConfig` 互斥激活,切换成本已前置投入(`#140 全服务切 CRDB`)。

## 代价 / 技术债

- **HTAP 学情分析路径丢失**:原 SAD v5.0 的 FR-4.2 学情大数据分析(learning_events 30亿行聚合)靠 TiFlash 库内列存 MPP 加速。CRDB Pebble LSM 行存只能外推数仓方案(无列存 MPP)。**补 ADR-25(数据面迁移决策复盘,记 ClickHouse/物化视图补偿方案,DEFERRED)**。
- **RoutingDataSource 双池语义退化**:CRDB 读池与写池常指向同 host(`crdb-public`),follower-read 在连接参数层而非物理独立端点,与原 RoutingDataSource "写主库/读副本" 双池语义不符(读写分离维度据此从 8.0 降为 5.0,见架构评分文档)。
- **TiDB ExternalName 残留**:`05b-platform-infra-externalnames.yaml` 仍声明 `tidb`/`tidb-follower`,应用 env 仍保留 `WRITE_DB_URL`/`READ_DB_URL` 的 `tidb-primary`/`tidb-follower` jdbc:mysql 回切串(`20-learning-java.yaml:38-39`),属 DGO 漂移待清理(或显式标注"可回切保留")。

## 回切条件

多节点服务器恢复时,若 mirror pingcap 镜像源修好(当前 403),可回 TiDB:
1. `deploy/k8s/07-tidb-tikv.yaml` replicas 改回 3(PD/TiKV/TiDB 各 3)。
2. `deploy/k8s/05b-platform-infra-externalnames.yaml:13-30` 指向真实 TiDB 集群。
3. 应用 env `ESP_RW_STORAGE_TYPE=tidb`(`CockroachDbDataSourceConfig` 失效,`DataSourceConfig` 生效)。
4. **schema 兼容性需重验**:`AUTO_RANDOM`(TiDB)↔ `unique_rowid()`(CRDB)、`ON UPDATE CURRENT_TIMESTAMP`(TiDB)↔ 触发器/应用层维护(CRDB)等差异。
5. 回切决策需新 ADR,不可静默切换(工程铁律⑦诚实优先 + DGO GitOps)。

## 被覆盖 ADR 状态变更

- **ADR-10**(原"TiDB 多分区拓扑与同步/读写分离真相"):状态 Superseded by ADR-24。本 ADR 描述的 TiDB 单点环境已下电。
- **ADR-16**(原"真 TiKV 异地多活 TiDB 集群"):状态 Superseded by ADR-24。TiCDC 异地多活方案未上线即被覆盖。
- **ADR-03**(原"TiDB + 读写分离 + TiFlash HTAP"):状态 Superseded by ADR-24。TiDB HTAP 路径保留可回切。

> 注:ADR-10/16 原文散落引用于 SAD v5.0 第 5 章(非独立文件),状态标注已在 SAD v5.1 第 5 章内联标记 "Superseded by ADR-24"。ADR-03 同理。
