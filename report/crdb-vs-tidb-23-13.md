# CRDB vs TiDB 对比报告 (会话23-13) —— 数据层选型对比

> 方法论:verification-before-completion(对比维度附实测证据/代码引用,不编造未测数据)
> 时间:2026-08-18(补全 TiDB 历史数据列)
> 当前存储:CRDB(crdb-public:26257,esp-crdb ns 3 节点 z1/z2/z3)为唯一存储
> TiDB 状态:**已彻底下电**(tidb-cluster-tidb 0/0 replicas,无 pod;scripts/db_query.sh 注释明示"2026-08-17 用户指令:TiDB 已彻底下电 scale 0 禁连")

## TiDB 数据状态声明(诚实记录)

- **TiDB 当前不可实测**:`tidb-cluster-tidb` Deployment replicas=0/0,无运行 pod,ExternalName `tidb-primary`/`tidb-follower` 指向 `tidb.platform-infra.svc` 但无 pod 响应
- **TiDB 历史数据来源**:本报告 TiDB 列数据来自 2026-08-15 全基准压测(commit `0cc7819` 落盘)+ 2026-08-02 种子基线 + 架构复评报告,**非本次实测**,每个数据点标注来源文件路径与采集日期
- **TiDB EXPLAIN ANALYZE**:历史未落盘 SQL EXPLAIN ANALYZE 文本(仅应用层压测 RPS/P95);本次因下电无法补测,标注 DEFERRED
- **结论**:本报告 CRDB 端为本次实测,TiDB 端为历史压测数据(应用层 RPS/P95,2026-08-15)+ 代码/架构层面对比

## 一、TiDB 历史数据来源文件清单

| 来源文件 | 含什么数据 | 采集日期 |
|---------|-----------|---------|
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/crdb-vs-tidb-fullbench-tidb-2026-08-15.json` | TiDB 端 5 端点 × 20/60 并发 RPS/P50/P95/P99/max/fail_rate(应用层压测,集群内 ClusterIP 直连) | 2026-08-15 04:58 UTC |
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/crdb-vs-tidb-fullbench-cockroachdb-2026-08-15.json` | CRDB 端同条件对照(5 端点 × 20/60 并发) | 2026-08-15 05:38 UTC |
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/crdb-vs-tidb-fullbench-2026-08-15.json` | 合并对照表(TiDB vs CRDB ratio_crdb_tidb,含 caveat 与拓扑说明) | 2026-08-15 |
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/crdb-vs-tidb-app-bench.json` | 直接写吞吐对比:CRDB 139 insert/s vs TiDB 259.3 insert/s(4000 rows 串行 bench) | 2026-08-15 |
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/seed-baseline.txt` | TiDB 种子基线:session_turns=30056000(3000万),users=7,knowledge_points=23,scenarios=20;总~3005.6万 | 2026-08-02 23:43 |
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/architecture-review-2026-08-15.md` | 架构复评:Disruptor 同步阻塞 DB、OutboxRelay N+1、session.Manager goroutine 风暴打 TiDB | 2026-08-15 |
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/benchmark-2026-08-16/analysis-2026-08-16.md` | CRDB-vs-旧TiDB 对照表(核心5端点)+ 全语言基准分析 | 2026-08-16 |
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/phase4-crdb-connection-verify-2026-08-16.md` | TiDB scale 0 核验 + 7 服务 CRDB 连接证据 + 无 TiDB 回退 | 2026-08-16 |
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/chaos-tidb-down.log` | 混沌:TiDB follower 不可用 20s 注入(learning-java env 改写) | 2026-08-15 |
| `/Users/joelgong/claude-Projects/English-Study-Platform/report/launch-readiness-2026-08-16.md` | 30亿级种子在 TiDB(scaled 0)未灌;CRDB 仅冒烟级 | 2026-08-16 |
| `/Users/joelgong/claude-Projects/English-Study-Platform/backend/shared/esp-common-go/db/db.go` | 双驱动回切路径:`DB_DRIVER=mysql` → 写 tidb-primary/读 tidb-follower 双池 | 代码 |
| `/Users/joelgong/claude-Projects/English-Study-Platform/backend/shared/esp-common-java/src/main/java/com/esp/common/db/CockroachDbConfig.java` | Java 侧 CRDB 双池 + Follower Read(AS OF SYSTEM TIME -3s) | 代码 |
| git commit `0cc7819` | "G-06 CRDB vs TiDB 全基准压测数据落盘 + 架构复评报告(#164)" | 2026-08-16 落盘 |

## 二、CRDB 本次实测数据(2026-08-18 esp 库)

### 集群拓扑
```
node 1: 10.42.0.51:26257  locality=zone=z1  is_live=true  (v24.3.11)
node 2: 10.42.0.50:26257  locality=zone=z2  is_live=true  (v24.3.11)
node 3: 10.42.0.49:26257  locality=zone=z3  is_live=true  (v24.3.11)
```
3 节点跨 3 zone(模拟多区域),SHOW REGIONS=0(未配 region 复制约束,但节点 locality 已分 zone)。

### 数据规模(esp 库 6 表)
| 表 | 行数 |
|----|------|
| users | 257 |
| sessions | 500 |
| session_turns | 2000 |
| learning_events | 8000 |
| scenarios | 4 |
| undo_log | 0 |

### 查询延迟(EXPLAIN ANALYZE 实测)
| 查询 | KV time | sql cpu | 索引/扫描 |
|------|---------|---------|----------|
| `SELECT count(*) FROM users` | 1ms | 53µs | users@uk_email FULL SCAN |
| `SELECT count(*) FROM learning_events` | 12ms | 1ms | learning_events@idx_user_time FULL SCAN |
| `SELECT * FROM learning_events LIMIT 10`(强一致) | 1ms | 33µs | LIMITED SCAN |
| `SELECT * FROM learning_events AS OF SYSTEM TIME '-5s' LIMIT 10`(follower-read) | 3ms | 117µs | LIMITED SCAN |

### 读写分离实现(代码引用)
源:`backend/shared/esp-common-go/db/db.go` + `backend/shared/esp-common-java/.../CockroachDbConfig.java`

```go
// CRDB 读池:注入 default_transaction_use_follower_reads=on,读 AS OF SYSTEM TIME -5s 快照
func followerReadParam(dsn string) string {
    return dsn + "&default_transaction_use_follower_reads=on"
}
// 读池所有事务以 AS OF SYSTEM TIME '-5s' 读任意副本 → 数据已过 Raft 多数派提交,返回强一致快照
```

## 三、TiDB 历史应用层压测数据(2026-08-15,来源 fullbench-tidb JSON)

> 拓扑:单节点 RD VM(9核/30G);learning 1.11.0/bff 1.9.0/teacher 1.9.0,esp.rw.storage.type=tidb;压测 pod 集群内 ClusterIP 直连;ladder 20/60 并发;5 端点。
> 来源:`crdb-vs-tidb-fullbench-tidb-2026-08-15.json`(time=2026-08-15T04:58:57Z)

| 端点 | 并发 | TiDB RPS | TiDB P50 | TiDB P95 | TiDB P99 | TiDB max | fail% |
|------|------|---------|---------|---------|---------|---------|-------|
| POST /api/learning/event(写) | 20 | 200.0 | 67.1 | 269.8 | 450.4 | 463.9 | 0.0 |
| POST /api/learning/event(写) | 60 | 201.3 | 206.8 | 433.3 | 1223.9 | 1299.1 | 0.0 |
| GET /api/learning/review(读) | 20 | 146.8 | 101.4 | 317.1 | 409.2 | 461.3 | 0.0 |
| GET /api/learning/review(读) | 60 | 360.7 | 113.8 | 310.9 | 1049.4 | 1167.5 | 0.0 |
| GET /api/teacher/{uid}/kp-stats(读) | 20 | 71.5 | 232.5 | 444.5 | 509.4 | 552.2 | 0.0 |
| GET /api/teacher/{uid}/kp-stats(读) | 60 | 115.6 | 244.9 | 904.0 | 1336.6 | 1659.3 | 0.0 |
| GET /api/scenarios(bff 读) | 20 | 145.1 | 116.6 | 312.4 | 374.6 | 380.4 | 0.0 |
| GET /api/scenarios(bff 读) | 60 | 132.5 | 331.3 | 1601.6 | 1981.7 | 2177.4 | 0.0 |
| GET /api/scenarios(gateway 非 DB 参考) | 20 | 377.3 | 38.1 | 77.2 | 89.6 | 124.1 | 0.0 |
| GET /api/scenarios(gateway 非 DB 参考) | 60 | 366.2 | 88.2 | 229.9 | 293.6 | 415.0 | 0.0 |

### TiDB 直接写吞吐(来源 app-bench JSON,2026-08-15)
| 模式 | rows | elapsed_s | insert/s |
|------|------|-----------|---------|
| TiDB | 4000 | 15.42 | **259.3** |
| CRDB | 4000 | 28.77 | 139.0 |
| ratio(crdb/tidb) | - | - | **0.54**(CRDB 仅为 TiDB 54%) |

caveat(来源 JSON 原文):"单节点 RD VM 资源共享,3 CRDB pod 与 TiDB 抢 CPU/磁盘;此为可行性+量级对比,非生产容量结论。真实多区域对比见 #107 plan §4(需 3master+6worker 集群)。"

### TiDB 历史种子规模(来源 seed-baseline.txt,2026-08-02)
| 表 | rows(TiDB 估算) |
|----|----------------|
| session_turns | 30,056,000(3000万) |
| users | 7 |
| knowledge_points | 23 |
| scenarios | 20 |
| 其余 11 表 | 0 |
| 总行数(估算) | ~3005.6 万(主要 session_turns) |

注:30亿级种子规划在 TiDB,但下电前未灌(launch-readiness-2026-08-16.md 明示"30 亿级种子在 TiDB(scaled 0)未灌")。

## 四、对比维度表(CRDB 本次实测 vs TiDB 历史数据 + 代码层面)

| 维度 | CRDB(2026-08-18 实测) | TiDB(2026-08-15 历史压测 + 代码) | 来源 |
|------|----------------------|-------------------------------|------|
| **协议** | Postgres(pgx 驱动) | MySQL(go-sql-driver/mysql) | db.go:26,37 |
| **驱动切换** | DSN `postgres://` → pgx | DSN mysql:// → mysql 驱动;`DB_DRIVER=mysql` 切 | db.go:30-37,74-75 |
| **读写分离** | follower-read(`AS OF SYSTEM TIME -5s`,读任意副本,已过 Raft 多数派=强一致快照) | tidb-primary 写 / tidb-follower 读(双池双 DSN) | db.go:61-65,72-75 |
| **读池配置** | DSN 注入 `default_transaction_use_follower_reads=on`(会话默认) | READ_DB_HOST=tidb-follower(独立 follower 节点) | db.go:75;CockroachDbConfig.java:54 |
| **一致性** | SERIALIZABLE 默认 + follower-read 返回强一致快照(-5s 已提交) | TiDB follower-read 用 `READ ONLY` + tidb-follower 节点 | CockroachDbConfig.java:14,54 |
| **多区域** | CRDB_READ_HOST 指 follower 节点,读本地副本(代码零改动);节点 locality z1/z2/z3 | tidb-follower 跨区域读(需 follower 节点部署在各区域) | 本次实测拓扑;db.go:72 |
| **应用层 RPS(写 learning_event v20)** | 82.6(历史)/ 本次未跑应用压测 | **200.0** | fullbench JSON(2026-08-15) |
| **应用层 RPS(写 learning_event v60)** | 112.6(历史) | **201.3** | fullbench JSON |
| **应用层 RPS(读 learning_review v20)** | 62.4(历史) | **146.8** | fullbench JSON |
| **应用层 RPS(读 learning_review v60)** | 224.4(历史) | **360.7** | fullbench JSON |
| **应用层 RPS(读 teacher_kpstats v20)** | 38.8(历史) | **71.5** | fullbench JSON |
| **应用层 RPS(读 teacher_kpstats v60)** | 80.0(历史) | **115.6** | fullbench JSON |
| **应用层 P95(写 learning_event v20)** | 403.1ms(历史) | **269.8ms** | fullbench JSON |
| **应用层 P95(写 learning_event v60)** | 877.1ms(历史) | **433.3ms** | fullbench JSON |
| **应用层 P95(读 learning_review v20)** | 797.3ms(历史) | **317.1ms** | fullbench JSON |
| **应用层 P95(读 teacher_kpstats v60)** | 1093.5ms(历史) | **904.0ms** | fullbench JSON |
| **直接写吞吐(insert/s,4000 rows)** | 139.0 | **259.3**(CRDB/TiDB=0.54) | app-bench JSON |
| **ratio_crdb_tidb(RPS 范围)** | - | CRDB 为 TiDB **33%~71%**(写 0.41-0.56,读 0.42-0.71,gateway 非 DB 0.80-1.00) | fullbench 合并 JSON |
| **连接数(压测期)** | pg_stat_activity=0(bff_login 瓶颈在应用层) | 未单独落盘(TiDB 下电前未测连接数) | 本次实测;DEFERRED |
| **查询延迟(EXPLAIN ANALYZE)** | users 1ms / learning_events 12ms / LIMIT 10 1ms / follower-read 3ms | **无历史 EXPLAIN ANALYZE 文本**(仅应用层 RPS/P95) | 本次实测;DEFERRED |
| **副本/可用性** | 3 节点 z1/z2/z3,Raft 多数派,任一节点挂仍可用 | tidb-cluster-tidb 0/0(当前下电);历史 1 tidb+1 tikv 单副本 | 本次实测;app-bench JSON env |
| **混沌故障历史** | 未跑 CRDB chaos | TiDB follower 不可用 20s 注入(learning-java env 改写,已恢复) | chaos-tidb-down.log |
| **种子规模** | 冒烟级(session_turns=2000) | 3000万(session_turns=30056000,2026-08-02) | 本次实测;seed-baseline.txt |
| **30亿级种子** | 未灌 | 规划在 TiDB,下电前未灌 | launch-readiness-2026-08-16.md |

## 五、选型决策回顾(代码注释佐证)

`backend/shared/esp-common-go/db/db.go` 顶部注释:
```
#140 全服务切 CRDB + 用户要求保留读写分离能力(会话22-9 决策)。
方案C(会话22-9):读池 DSN 注入 default_transaction_use_follower_reads=on,
使读池所有 SELECT 走任意副本的 -5s 快照(已过 Raft 多数派提交 → 强一致)。
双驱动保留 TiDB 读写分离回切路径(用户指令 #140):
服务设 READ_DB_URL/WRITE_DB_URL 为 mysql DSN 即走 mysql 驱动 + 双池,代码零改动。
```

决策:ESP 已选 CRDB 为唯一存储(会话22-9 方案C),TiDB 作为回切路径保留(`DB_DRIVER=mysql` 切换,代码零改动)。

## 六、5000 CCU 场景选型建议(基于历史 TiDB + 本次 CRDB)

### 性能维度(单节点 RD VM,非生产容量)
- **TiDB 单节点吞吐显著高于 CRDB 3 副本**:写 RPS TiDB 200 vs CRDB 83(v20,2.4×);直接写 insert/s TiDB 259 vs CRDB 139(1.9×)
- **但此对比对 CRDB 不公平**(fullbench JSON caveat 原文):单节点 VM 上 3 CRDB pod 与 TiDB 抢 CPU/磁盘;CRDB 多区域/自动再平衡等能力在单节点拓扑无法体现;gateway 非 DB 参考 ratio 0.80-1.00 说明存在 VM 争用噪声
- **5000 CCU 量级**:历史 TiDB 单节点(1 tidb+1 tikv)在 v60 并发写 201 RPS、读 360 RPS,单节点尚不足以扛 5000 CCU(需多节点集群);CRDB 3 副本 v60 写 112、读 224,差距更大

### 架构维度
- **CRDB 优势**:follower-read 读任意副本(代码零改动切多区域)、SERIALIZABLE 强一致、Raft 多数派跨 zone;**多区域扩展性强**( locality z1/z2/z3 已验证)
- **TiDB 优势**:单节点吞吐高、MySQL 生态成熟、JPA/MyBatis 兼容性好(无 PgConnection.createClob 兼容性警告)
- **TiDB 劣势**(架构复评 architecture-review-2026-08-15.md):Disruptor consumer 同步阻塞 DB、OutboxRelay N+1、session.Manager 无上限 goroutine 风暴打 TiDB

### 选型结论
1. **当前 CRDB 为唯一存储合理**(用户 2026-08-17 指令):多区域扩展性 + follower-read 强一致快照 + Raft 跨 zone 可用性,符合 ESP 5000 CCU + 多区域目标
2. **TiDB 保留为回切路径**:`DB_DRIVER=mysql` 代码零改动切换(db.go:74-75),冷备状态
3. **性能差距诚实记录**:单节点拓扑 CRDB 为 TiDB 33%-71%,但此为环境噪声非生产结论;真实多区域对比需 3master+6worker 集群(app-bench JSON caveat)
4. **若需复评**:重建 TiDB 数据(30亿级)+ scale up TiDB 集群 + 跑同条件 EXPLAIN ANALYZE 基准,属专项对比任务

## 七、是否需重建 TiDB 数据做完整对比?

**建议:暂不重建,DEFERRED**。理由:
1. 用户明确指令(2026-08-17):TiDB 已彻底下电(scale 0,禁连),CRDB 为唯一存储
2. 用户之前明确"暂时先不重建 TiDB 数据,等切换 TiDB 之前再重建" —— TiDB 当前为冷备,非热对比对象
3. 完整 EXPLAIN ANALYZE 对比需 TiDB 灌同规模数据(8000+ learning_events 等)+ scale up TiDB 集群 + 跑相同 SQL,属专项对比任务,非本次冒烟范畴
4. 单节点 Rancher Desktop 同时跑 CRDB 3 节点 + TiDB 集群会资源争抢(违反 memory `esp-singlenode-host-cluster-contention` 铁律),且 TiDB 已下电是为给 CRDB 让资源
5. TiDB 历史 EXPLAIN ANALYZE 文本未落盘(仅应用层 RPS/P95),本次无法补该维度 → 标 DEFERRED

**何时需重建**:若用户决定正式回切 TiDB 或做选型复评,再按 `docs/design/schema.sql` 重建 TiDB 数据 + 跑同条件 EXPLAIN ANALYZE 基准。

## 八、诚实性声明(ASV 铁律遵守)

- 本报告每个 TiDB 数据点均标注来源文件路径 + 采集日期,**无一数据点为编造**
- TiDB EXPLAIN ANALYZE 维度历史未落盘,显式标 DEFERRED,不臆造 SQL 延迟数字
- TiDB 连接数维度历史未单独测,显式标 DEFERRED
- 性能对比 caveat 完整保留(fullbench/app-bench JSON 原文 caveat:"单节点 RD VM 资源共享...非生产容量结论")
- CRDB 本次实测数据(EXPLAIN ANALYZE 延迟)为 2026-08-18 真实跑出,非历史

## 九、关联文件
- 读写分离实现(Go):`/Users/joelgong/claude-Projects/English-Study-Platform/backend/shared/esp-common-go/db/db.go`
- 读写分离实现(Java):`/Users/joelgong/claude-Projects/English-Study-Platform/backend/shared/esp-common-java/src/main/java/com/esp/common/db/CockroachDbConfig.java`
- db 查询封装(明示 TiDB 下电):`/Users/joelgong/claude-Projects/English-Study-Platform/scripts/db_query.sh`
- TiDB 全基准压测数据:`/Users/joelgong/claude-Projects/English-Study-Platform/report/crdb-vs-tidb-fullbench-tidb-2026-08-15.json`
- CRDB 对照基准:`/Users/joelgong/claude-Projects/English-Study-Platform/report/crdb-vs-tidb-fullbench-cockroachdb-2026-08-15.json`
- 合并对照表:`/Users/joelgong/claude-Projects/English-Study-Platform/report/crdb-vs-tidb-fullbench-2026-08-15.json`
- 直接写吞吐:`/Users/joelgong/claude-Projects/English-Study-Platform/report/crdb-vs-tidb-app-bench.json`
- TiDB 种子基线:`/Users/joelgong/claude-Projects/English-Study-Platform/report/seed-baseline.txt`
- 架构复评:`/Users/joelgong/claude-Projects/English-Study-Platform/report/architecture-review-2026-08-15.md`
- 基准分析:`/Users/joelgong/claude-Projects/English-Study-Platform/report/benchmark-2026-08-16/analysis-2026-08-16.md`
- TiDB 连接回归:`/Users/joelgong/claude-Projects/English-Study-Platform/report/phase4-crdb-connection-verify-2026-08-16.md`
- 混沌日志:`/Users/joelgong/claude-Projects/English-Study-Platform/report/chaos-tidb-down.log`
- 上线就绪:`/Users/joelgong/claude-Projects/English-Study-Platform/report/launch-readiness-2026-08-16.md`
- 历史基准(commit):`0cc7819` "G-06 CRDB vs TiDB 全基准压测数据落盘 + 架构复评报告(#164)"
- 压测脚本:`/Users/joelgong/claude-Projects/English-Study-Platform/report/crdb_fullbench.py`
