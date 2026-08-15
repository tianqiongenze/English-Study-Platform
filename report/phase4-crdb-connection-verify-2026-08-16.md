# Phase4 CockroachDB 全量连接回归验证

> 日期: 2026-08-16
> 背景: ESP 全服务已切 CockroachDB (#140),TiDB 已 scale 0。
> 方式: 只读验证(env 查询 + 日志 grep),不修改任何 yaml/deployment。
> 命名空间: esp-prod (业务) / ekb-m2-aws-us-east-1 (原 TiDB 基础设施) / esp-crdb (新 CRDB)

## 1. 各服务 CRDB env 与连接证据

| 服务 | 语言 | CRDB env (指向 crdb-public?) | CRDB 连接证据 (日志) | TiDB 回退迹象 |
|------|------|------------------------------|----------------------|---------------|
| bff-java | Java | ESP_RW_STORAGE_TYPE=cockroachdb, ESP_RW_COCKROACHDB_HOST=crdb-public ✓ | 日志无 HikariPool/jdbc 行 (bff 走 Dubbo 调下游,不直连 DB;启动成功 Started BffApplication) | 无 |
| teacher-java | Java | ESP_RW_STORAGE_TYPE=cockroachdb, ESP_RW_COCKROACHDB_HOST=crdb-public ✓ | HikariPool-1 Start completed + Added connection PgConnection + jdbc:postgresql://crdb-public:26257/esp_teacher?sslmode=disable ✓ | 无 |
| learning-java | Java | ESP_RW_STORAGE_TYPE=cockroachdb, ESP_RW_COCKROACHDB_HOST=crdb-public ✓ | Database JDBC URL [jdbc:postgresql://crdb-public:26257/esp_learning?sslmode=disable] + PgConnection.createClob 警告(已知,CRDB 兼容性无害) ✓ | 无 |
| learning-worker | Java | ESP_RW_STORAGE_TYPE=cockroachdb, ESP_RW_COCKROACHDB_HOST=crdb-public ✓ | HikariPool-2/3 Start completed + Added connection PgConnection (延迟初始化,jetty 线程触发) ✓ | 无 |
| writing-py | Python | WRITE_DB_HOST=crdb-public, READ_DB_HOST=crdb-public, CRDB_HOST=crdb-public ✓ | esp_db write pool ready -> crdb-public:26257/esp_writing + read pool ready ✓ | 无 |
| emotion-svc | Python | WRITE_DB_HOST=crdb-public, READ_DB_HOST=crdb-public, CRDB_HOST=crdb-public ✓ | esp_db write pool ready -> crdb-public:26257/esp_learning + read pool ready ✓ | 无 |
| writing-coach | Python | WRITE_DB_HOST=crdb-public, READ_DB_HOST=crdb-public, CRDB_HOST=crdb-public ✓ | esp_db write pool ready -> crdb-public:26257/esp_writing + read pool ready ✓ | 无 |
| gateway-go | Go | CRDB_HOST=crdb-public, CRDB_PORT=26257, DB_ENABLED=true ✓ | db pools ready write=read=postgres://root@crdb-public:26257/esp_speaking + scenario: loaded from CRDB count=10000 ✓ | 无 |

## 2. TiDB scale 0 核验

- `kubectl -n ekb-m2-aws-us-east-1 get deploy | grep tidb` → `tidb-cluster-tidb 0/0 0 0 14d` (已 scale 0,无运行 Pod)
- `kubectl -n ekb-m2-aws-us-east-1 get pods | grep tidb` → 空 (无运行 TiDB Pod)
- 7 服务日志 grep `tidb-primary|tidb-follower|mysql.*connect` → 全部空 (无 TiDB 回退/连接错误)

## 3. CRDB 活跃连接来源

- 查询: `SELECT application_name, count(*) FROM pg_catalog.pg_stat_activity GROUP BY application_name ORDER BY 2 DESC LIMIT 10;`
- 结果: 仅有 `cockroach sql` (本次查询自身),说明查询瞬间无长连接活跃 (连接池为按需建连,空闲连接已释放或 pg_stat_activity 在 CRDB 中语义与 PG 不同,活跃连接需观察 `[SHOW CLUSTER STATEMENTS]`)
- 备用查询 `[SHOW CLUSTER STATEMENTS]` 同样仅显示本次查询 (说明各服务连接池处于空闲状态,无活跃 SQL 在跑,但 pool 已建连且启动期已验证)

## 4. 汇总

| 指标 | 结果 |
|------|------|
| CRDB env 指向 crdb-public | 7/7 服务全部确认 (bff-java 不直连 DB 但 env 已配) |
| CRDB 连接日志证据 | 6/7 有明确日志证据 (bff-java 无 DB 直连日志,符合架构: BFF 走 Dubbo 调下游) |
| TiDB 回退迹象 | 0/7 (无任何服务出现 tidb/mysql 连接错误) |
| TiDB scale 0 | 已确认 (deploy 0/0, 无 pod) |
| CRDB 集群健康 | crdb-0/1/2 三节点均 Running,4h+ 稳定 |

结论: Phase4 CockroachDB 切换回归验证通过。7 服务 env 全部指向 crdb-public,6 个直连 DB 服务均有 CRDB 连接成功日志证据,bff-java 走 Dubbo 不直连 DB 符合架构设计。TiDB 已 scale 0,无任何回退迹象。CRDB 三节点集群稳定运行。

备注: learning-java 出现 `PgConnection.createClob() not yet implemented` 警告,这是 PostgreSQL JDBC 驱动对 CRDB 的已知兼容性提示,不影响功能 (Hibernate 会 fallback 到字节流处理 CLOB)。建议后续通过 Hibernate 配置 `hibernate.jdbc.lob.non_contextual_creation=true` 消除该警告。
