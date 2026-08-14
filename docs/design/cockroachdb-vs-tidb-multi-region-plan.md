# CockroachDB vs TiDB 多区域方案对比(ESP)

> 更新日期:2026-08-15。**本文是纯方案文档,本阶段不实际部署 CRDB**;CRDB 验证环境落地清单见附录 A(供后续执行)。
> 读者视角假设:资深架构师,关注瓶颈/容灾/扩缩路径/成本权衡,不要营销话术。
> 事实基线:TiDB 现为共享基础设施(ns `ekb-m2-aws-us-east-1`,单副本 TiKV,单节点 RD 验证环境);历史上曾落地三区(aws/azure/gcp)3 副本 Raft 拓扑并实测强一致(`docs/design/多区域数据同步方案.md` §1.2/§1.2.1);ESP 全部 6 个应用服务已覆盖读写分离(tidb-primary 写 / tidb-follower 读);测试数据目标 30 亿(sessions 已达标 / session_turns 补灌中 / learning_events 排队,见 `docs/PROGRESS.md`)。

---

## 0. 结论速览(TL;DR)

1. **5000 CCU 尺度下,引入 CRDB 的收益不成立**:ESP 是单主体教育平台(非全球多活多写)、重度依赖 HTAP 行为分析(learning_events 聚合,TiFlash 路线已铺)、SQL 方言 MySQL 已渗透到 6 服务 3 语言驱动层;CRDB 在这三个维度上全是改造成本、没有对等收益。
2. **推荐路径:默认 TiDB 深化多区域**(恢复三区 3 副本 + Placement Rules 按用户区域 pin leader),**CRDB 仅保留为条件触发的双轨 PoC**(触发条件见 §6.2)。
3. CRDB 真正的差异化优势是 **Postgres 兼容 + GLOBAL 表读任意低延迟 + 声明式多区域语法**,若未来 ESP 出现"全球多写、用户跨区漫游写本地化"的产品形态,再启动 §4 压测对比与附录 A 验证环境。

---

## 1. CRDB 多区域拓扑方案

### 1.1 目标拓扑:3 区域 × 3 节点(9 节点)

对齐 ESP 既有三区命名(aws-us-east-1 / azure-eastasia / gcp-europe-west1):

| 节点 | region 标签 | zone 标签 | 职责 |
|---|---|---|---|
| crdb-0 | aws | a | SQL 网关 + range 副本/leaseholder |
| crdb-1 | aws | b | 同区第二副本(仅 ZONE 存活目标下) |
| crdb-2 | azure | a | SQL 网关 + 副本 |
| crdb-3 | azure | b | |
| crdb-4 | gcp | a | SQL 网关 + 副本 |
| crdb-5 | gcp | b | |
| (合计 9) | 每区 3 节点 | | 满足 REGION 存活目标的最小配置:每表 5 副本(voters)跨 3 区,任一区全挂仍有 3/5 多数派 |

- 9 节点是 **REGION SURVIVABILITY 的最小完整形态**(3 区 × 每区 3 节点,保证任一区挂掉后剩余两区各仍有足够 voting 副本与新 leaseholder 候选)。
- 每个 CRDB 节点同时是 SQL 网关与存储节点(无独立计算层,这是与 TiDB "无状态 TiDB 层 + TiKV 存储" 分层架构的本质差异):**应用连接串指向本区节点即可获得本区 leaseholder 读**,不需要 ESP 现有的 tidb-primary/tidf-follower 双端点读写分离层。

### 1.2 存活目标(Survival Goal)

CRDB 的存活目标是**数据库级声明**:

| 目标 | 副本布局 | 写路径代价 | 适用 |
|---|---|---|---|
| `SURVIVE ZONE FAILURE`(默认) | 副本可全在本 region | 每写本区 Raft 多数派,~ms 级 | 单区域部署 |
| `SURVIVE REGION FAILURE` | voters 跨 3 区(5 副本) | **每写至少 1 次跨区 RTT**(见 §1.5) | ESP 多区域目标形态 |

关键工程事实:**REGION 存活目标下没有"本地写"**——这是 CRDB 与"按用户分片 + 本地区独立提交"的 Cassandra 类架构的根本区别。CRDB 用全局一致性和声明式简单性,换掉了写路径的区域自治。ESP 若选 REGION 存活,必须接受写延迟下限 ≈ 跨区 RTT;这正是 §5 压测要量化的核心数字。

### 1.3 数据库/表级 locality 设置

CRDB 多区域三件套(均为声明式 SQL,无需理解物理调度):

1. `ALTER DATABASE esp SET PRIMARY REGION "aws-us-east-1";` + `ADD REGION` azure/gcp。
2. `ALTER DATABASE esp SURVIVE REGION FAILURE;`
3. 表级三种 locality:

| locality | 语义 | 读 | 写 | ESP 适配表 |
|---|---|---|---|---|
| `GLOBAL` | 每区各一全量副本 | **任意区本地读、零跨区** | 写需跨区共识(变更频率必须低) | scenarios / knowledge_points / 字典类 |
| `REGIONAL BY TABLE` | 副本集中 home region(PRIMARY 可切) | home 区本地读;远区读跨区 | home 区提交 | learning_event_outbox 等单写组件 |
| `REGIONAL BY ROW` | 每行有 `region` 列决定 home | 行 home 区本地读 | 行 home 区提交(但 REGION 存活下仍付跨区共识) | learning_events / session_turns / sessions / user_mastery / writing_tasks / users |

### 1.4 esp 业务表的 region key 设计(按 user_id 可行性)

CRDB 要求 REGIONAL BY ROW 表有显式 region 列(类型 `crdb_internal_region` enum),**该列不能由 user_id 哈希推导,必须是业务明确的用户归属地**。设计:

```
region 列来源 = users.home_region(注册时定,默认 = 注册入口网关所在区)
```

- **users 表**:加 `home_region` 列(注册网关写入,允许用户迁移时 `UPDATE ... SET home_region` → CRDB 产生跨区 move,后台完成)。
- **sessions / session_turns / learning_events / user_mastery / writing_tasks**:写入路径全部携带 user_id,但**必须冗余 region 列**(反范式),否则 CRDB 无法判断行归属。改造点:
  - 生产侧取 region 的两种方式:① JWT claim 增加 `region` 字段(登录时从 users 取,零额外查询,推荐);② 写前查 users(多一次点查,GLOBAL/同区命中时可接受,不推荐)。
  - 查询侧:同用户行集的查询加 `WHERE region = $1`(或 region 进 PK 前缀),CRDB 才能做 locality 剪枝;否则跨区扫描。
- **可行性结论**:
  - learning_events / session_turns(append-heavy,见 §3)适配良好——事件生产者(bff/learning-worker/gateway-go)在 JWT 里已有 user_id,加一个 claim 即可。
  - user_mastery(PK=(user_id,kp_id))适配良好,region 天然随 user 走。
  - **风险点 1:users.uk_email 唯一约束**——REGIONAL BY ROW 下跨区唯一性由全局约束保证,注册写延迟不再是本地操作(写共识 + 唯一性检查跨 home region),可接受但需压测确认。
  - **风险点 2:用户跨区漫游**(中国用户出差到欧洲网关):写仍回 home region(RBR 语义),漫游不获得本地写延迟;若改为"跟随网关"会引入行迁移风暴,不建议。
  - **风险点 3:现有 schema 迁移成本**——`AUTO_RANDOM`(TiDB 特有)→ CRDB 需改 `unique_rowid()` 或 UUID;`ON UPDATE CURRENT_TIMESTAMP`(user_mastery)→ 需触发器或应用层维护;`ENUM`/`JSON` CRDB 均支持,改动小。

### 1.5 跨区域网络延迟假设与写路径影响

假设:区内 RTT <2ms;区间 RTT aws↔azure 60ms、aws↔gcp 90ms、azure↔gcp 110ms(任务给定假设区间 50~100ms,下文按此档推演)。

REGION 存活目标下的延迟画像(CRDB 官方模型 + Raft 共识推演):

| 操作 | 延迟构成 | 估算 |
|---|---|---|
| RBR 表写(home 区网关) | 客户端→网关 0 + Raft 多数派(1 个远区 ack) | ≈ 1×跨区 RTT ≈ **50~100ms** |
| GLOBAL 表写 | 同上(全局序列化,更差) | 60~110ms+,**必须低频** |
| RBR/GLOBAL 本地读(leaseholder/全局副本在本区) | 本区 | **<5ms** |
| 跨 home 区读(如管理后台扫全量) | 跨区 | 50~100ms |
| 区故障切换 | 剩余区重选 leaseholder | 秒级,期间该 range 写入不可用 |

对照 TiDB 三区 3 副本(ESP 实测过的拓扑):写 aws 主库同样付 1×跨区 RTT(Raft 多数派含 1 远区),**两者写路径延迟下限相同**;差异在读:CRDB 的 RBR/GLOBAL 让"任意区本地读"成为一等公民,而 TiDB 需靠 Placement Rules 把各用户分片的 leader 固定到其 home 区才能等价(可行但 per-region 调度、非 per-row 语义,粒度是 region(96MB)而非行)。

**对 ESP 写路径的影响评估**:5000 CCU 下写路径主要是 learning_events 追加(目标 5000 ev/s,G-06)与 sessions/turns 追加。这些写入对单条延迟不敏感(202 异步化 + outbox 已落地,P1-8/P1-10),**50~100ms 共识延迟不构成吞吐瓶颈**(吞吐受 Raft 提交流水线而非单条 RTT 限制);真正敏感的是对话内强一致读(会话恢复、mastery 查询),RBR 本地读 <5ms 反而优于现状(全量走 tidb-follower 单点)。

### 1.6 写路径时序推演(RBR,REGION 存活,aws 用户经 aws 网关写)

```
客户端(aws) → CRDB 网关(aws 节点)
  1. SQL 层解析,定位 range 的 leaseholder(用户 home=aws → leaseholder 在 aws)≈ 本地 0ms
  2. 事务开始:HLC 取 ts,本地 ≈0ms(对照 TiDB:PD TSO 若 leader 在 aws 也 ≈0,在远区则 +RTT)
  3. 写提交:leaseholder 提议 Raft,5 voters(aws:azure:gcp 权重布局)多数派需 1 个远区 ack
     → +1×区间 RTT(50~100ms)  ← 唯一不可避免项
  4. ack 返回客户端,总延迟 ≈ 50~100ms
读(同用户行):leaseholder 在本区 → <5ms,无跨区
```

三条架构推论:
1. **追加型大表(learning_events)吞吐与共识 RTT 解耦**:Raft group 流水线化,单 range 上限受批处理/磁盘,5000 ev/s 经 range 分散(按 user_id 分片自然打散)后不是瓶颈;真正要压测的是 region 分裂/搬迁风暴期的尾延迟(§4.3 区故障演练覆盖)。
2. **user_mastery 热点用户复读**:SM2 复习驱动同一行高频更新,乐观并发 + SERIALIZABLE 下冲突重试率是 CRDB 特有风险(TiDB 悲观锁下排队)。§4 压测必须包含"单用户密集复习"微场景。
3. **GLOBAL 表写是全局串行化点**:scenarios/knowledge_points 若有管理后台批量改写,会形成全局写队列,须限频或改 REGIONAL。

---

## 2. CRDB vs TiDB 深度对比

### 2.1 一致性与共识(Raft 实现差异)

| 维度 | CRDB | TiDB |
|---|---|---|
| 共识 | Multi-Raft,range 分片(默认 512MiB,自动分裂) | Multi-Raft,region 分片(默认 96MiB,PD 调度) |
| 事务模型 | 乐观并发 + **默认 SERIALIZABLE**(v22.2+ 可降 READ COMMITTED);跨 range 事务经事务表 + uncertainty 窗口 | **Snapshot Isolation**,默认悲观锁模式(4.0+);冲突处理对 MySQL 迁移用户更友好 |
| 隔离级别语义 | 严格可串行化(工业级最强,代价是写冲突重试) | SI(写偏斜幻读窗口理论存在,MySQL 兼容层对外表现为 RR) |
| 时间戳 | HLC(混合逻辑时钟),跨区时钟不确定窗口影响读延迟 | TSO(PD 单点授时)——**PD 是全局授时单点,跨区部署时 TSO RTT 附加在事务首步** |
| 读写分离 | 任意节点可强一致读(读走 leaseholder 或 follower read 一致性前缀) | Stale Read / follower read 可用但需显式使用;ESP 现状是"primary 写 / follower 读"双端点应用层路由 |

架构师视角:**TSO vs HLC 是最深的分歧**。TiDB 的 PD TSO 在三区部署里 leader 在单区,远区事务取时间戳多付一次跨区 RTT(可与首次读写部分重叠);CRDB 无授时单点但付 uncertainty window(读可能需跨区确认)。两者都在 1×RTT 量级,无数量级差异,但要清楚"没有免费的跨区强一致"。

### 2.2 多区域数据放置

| 维度 | CRDB | TiDB |
|---|---|---|
| 抽象层级 | **声明式 SQL**(PRIMARY REGION / ADD REGION / SURVIVE / 表级 locality / 行级 region 列) | PD **Placement Rules**(JSON 规则,按 db/table/partition 指定副本数与标签约束) |
| 粒度 | 行级(RBR 的 region 列) | region 级(数据分片),表级约束可声明、行级需按表拆分区技巧 |
| 调度器 | 自动按 locality 目标搬迁(后台 rebalance) | PD scheduler(evict-leader / scatter / 按规则 place),成熟但配置面在运维侧非 SQL 侧 |
| 学习/审计成本 | DBA 用 SQL 即可表达,变更可进版本控制 | 规则 JSON 直改 PD,误配影响面大,需要专门 runbook |

结论:CRDB 的多区域表达力是产品级差异化优势;TiDB Placement Rules 能力等价(甚至副本拓扑控制更细),但表达层是运维接口而非 SQL,**人效与出错率上有真实差距**。

### 2.3 SQL 兼容性与 ESP 驱动改造量

ESP 现状:MySQL 协议(TiDB :4000),三语言栈:

| 栈 | 现驱动/框架 | 迁移到 CRDB(Postgres wire :26257)改造量 |
|---|---|---|
| Java(bff/learning/teacher) | Spring Data JPA + mysql-connector + esp-rw-java 读写分离包 | 驱动换 postgresql;Hibernate dialect 换 Postgres;JPA 实体大多中立,但**原生 SQL/JPQL 方言差异 + AUTO_RANDOM/ON UPDATE 改造**;esp-rw-java 双 DataSource 语义可删(CRDB 单端点) |
| Python(writing/emotion) | pymysql / aiomysql(esp-db-py) | 换 psycopg/asyncpg,改写占位符 `%s` 语法一致但 DDL/自增语义不同,量中 |
| Go(gateway-go/conversation-go 等) | go-sql-driver/mysql | 换 pgx,SQL 改写量中 |
| Schema | `AUTO_RANDOM` × 多表、`ON UPDATE CURRENT_TIMESTAMP`、TiDB `ENUM` | AUTO_RANDOM→unique_rowid()/UUID;ON UPDATE→触发器;ENUM JSON 兼容 |

**净评估:6 服务 3 语言 + 公共包(esp-rw-java/esp-db-py)+ 30 亿种子脚本(scripts/seed_3b.py,pymysql)全部要动**。这是数百人时的回归测试面,而收益侧没有用户可感知的功能增益——这是本方案不推荐现阶段切换的最大单一理由。

### 2.4 HTAP 能力

| | CRDB | TiDB |
|---|---|---|
| 列存引擎 | **无**。统一 Pebble LSM 行存,分析查询靠扫描并行度 | TiFlash:列存 + MPP 向量化,与 TiKV Raft 学习者同步 |
| ESP 影响 | learning_events 30 亿行为聚合(P2-16 BehaviorController top/funnel/heatmap 类负载)只能外推到数仓/ClickHouse | TiFlash 已在历史拓扑落地过(aws 区 1 副本),行为分析可库内闭环 |

ESP 学情分析是产品核心闭环,HTAP 是 TiDB 的**结构性优势**。

### 2.5 生态与运维

| 维度 | CRDB | TiDB |
|---|---|---|
| K8s Operator | CockroachDB Operator(成熟度中等) | TiDB Operator(生产广泛验证,BR 备份/TiFlash 扩缩/滚动升级全托管) |
| 备份恢复 | `cockroach sql BACKUP` → S3;PITR 支持有限(read-from) | BR(快照 + PITR log backup,工业级) |
| CDC | changefeed → Kafka/S3(恰好可对位 ESP Pulsar 事件流) | TiCDC → Kafka/Pulsar(对 Pulsar 原生支持更好) |
| 监控 | Prometheus 端点内置 + CRDB Console | TiDB Dashboard + Prometheus 全家桶 |
| 中文社区/支持 | 国内生态弱 | PingCAP 国内一线支持(用户大厂背景下评估权重高) |

### 2.6 许可证与成本

| | CRDB Core | TiDB 社区版 |
|---|---|---|
| 许可 | **BSL 1.1**(每版本 3 年后转 Apache 2.0;生产使用限制:非竞争性) | **Apache 2.0** |
| 企业特性 | 多区域自治能力大部分在 Core;企业版卖备份调度/SSO/高级支持 | MPP/资源管控等在社区版可用;企业版卖 DM/SSE/支持 |
| 供应链风险 | BSL 对"再分发竞争产品"有约束,教育平台一般无触碰,但法务需过目 | 无 |

### 2.7 运维复杂度对比(扩展)

| 运维场景 | CRDB | TiDB |
|---|---|---|
| 扩容 | 加节点 + 自动 rebalance(自动迁移 range,可限速);SQL 侧零改动 | TiKV 加节点 + PD scatter(自动);TiDB 层扩容独立(无状态) |
| 滚动升级 | Operator 托管,跨版本需注意 deprecated 语法(多区域语法 v22→v23 有变更) | TiDB Operator 滚动,版本矩阵(TiDB/PD/TiKV/TiFlash)需对齐,组件多但流程极成熟 |
| 故障定位 | Console 单界面看 range/leaseholder/慢查询;`EXPLAIN ANALYZE (DISTSQL)` | TiDB Dashboard + `EXPLAIN ANALYZE`;PD/TiKV 两层日志,排障链路长但社区资料极多 |
| 副本/放置变更 | SQL 声明,变更历史可 git 化 | `pd-ctl` / API 改 Placement Rules,需 runbook 与双人复核 |
| 备份策略变更 | `CREATE SCHEDULE FOR BACKUP`(企业版增强,Core 用 cron 脚本) | BR cron + log backup;CRDB Core 的 PITR 能力弱于 BR log backup |
| 单节点 RD 验证难度 | 单二进制三副本即可起,门槛低 | 需 TiDB+PD+TiKV 三组件,ESP 已有成熟 manifest 可复用 |

### 2.8 对比总表

| 维度 | CRDB | TiDB | 对 ESP 权重 |
|---|---|---|---|
| 多区域声明式表达 | **胜**(SQL 级行级) | Placement Rules(等价能力,运维接口) | 中 |
| 任意区本地读 | **胜**(GLOBAL/RBR 原生) | 需 leader pinning per region | 中 |
| 写路径(跨区共识) | 平(同为 ~1×RTT 下限) | 平(TSO 附加可优化) | 中 |
| SQL/驱动迁移成本 | **大幅落后**(3 语言 6 服务+种子) | 现状零成本 | **高** |
| HTAP | **无** | TiFlash | **高** |
| 生态/Operator/备份 | 平 | 略胜(国内支持) | 中 |
| 许可 | BSL 1.1 | Apache 2.0 | 低 |

---

## 3. ESP 30 亿数据画像与逐表放置建议

### 3.1 数据画像(按 PROGRESS/种子进度)

| 表 | 目标量级 | 访问模式 | 一致性需求 |
|---|---|---|---|
| learning_events | 最大(30 亿主体) | append-heavy 5000 ev/s;读为窗口聚合 | 写入吞吐优先,读可最终一致(缓存已覆盖) |
| session_turns | 次大(亿级) | append-heavy 对话轮;读=会话恢复顺序读 | 会话内顺序一致 |
| sessions | 千万级 | 写一条/会话 + 状态更新 | 写后读一致 |
| user_mastery | 用户×KP 笔记级 | 点读 + 点更新(SM2 复习驱动) | **强一致读写**(version 乐观锁在用) |
| users | 百万级 | 登录点读 + 注册写 | 强一致(uk_email 全局唯一) |
| scenarios / knowledge_points | 极小 | 读多写少 | 任意区本地读即可 |
| writing_tasks / outbox | 中小 | append + 少量点读 | outbox relay 单写者 |

### 3.2 逐表放置建议表(若走 CRDB)

| 表 | locality | region key | 理由 / 备注 |
|---|---|---|---|
| users | REGIONAL BY ROW | home_region(注册入口区) | 登录读本地化;uk_email 跨区唯一性由 CRDB 保证,注册延迟需压测 |
| scenarios | **GLOBAL** | — | 极小读多,零跨区读 |
| knowledge_points | **GLOBAL** | — | 同上 |
| sessions | REGIONAL BY ROW | 随 user home_region | JWT claim 带入,写读均本地语义 |
| session_turns | REGIONAL BY ROW | 随 session→user region | append-heavy 本地 leaseholder 吞吐最优 |
| user_mastery | REGIONAL BY ROW | 随 user home_region | 强一致点读点写,<5ms 本地;version 乐观锁语义在 SERIALIZABLE 下兼容 |
| learning_events | REGIONAL BY ROW | 随 user home_region | 本方案最大适配点:事件按用户归属本地化写入 + changefeed 替代部分 outbox |
| writing_tasks | REGIONAL BY ROW | 随 user home_region | 同 user 维度 |
| learning_event_outbox | REGIONAL BY TABLE(PRIMARY aws) | — | 单写者 relay,集中 home 区;若 CRDB changefeed 可整体替代则删表 |

对照 TiDB 等价物:GLOBAL ≈ Placement Rules "每区一 learner 副本 + 任意读";RBR ≈ "按 user_id 预分区 + 各分区 leader pin 到用户 home 区"——TiDB 可达,但分区数 × 区域映射的运维复杂度显著高于 RBR 声明。

---

## 4. 全链路压测对比方法论(双跑)

### 4.1 场景与负载

复用 `deploy/locust/` 既有场景,不新写剧本:

| 场景 | 文件 | 指标锚点 |
|---|---|---|
| 5000 CCU WS 口语陪练 | `ccu5000_ws.py` / `locustfile.py` | G-01 稳定在线 |
| 学情事件 | `locustfile.py` LearningEventUser | G-06 ≥5000 ev/s |
| 写作批改 | `locustfile.py` WritingUser | G-04 ≥50 req/s,P95<2s |
| 峰值/浸泡 | `spike.py` / `soak.py` | 恢复曲线 / 24h 稳态 |

双跑方式:同一套 locust 剧本,仅切换被测端——A 组指向现有 gateway-go(→TiDB),B 组指向同构部署的 gateway-go 副本(→CRDB 验证集群,见附录 A)。**两组除 DB 外一切变量固定**(镜像 tag、副本数、locust worker 数、网络路径)。

### 4.2 数据集

- 30 亿全量双跑不可行(时长与磁盘)。裁剪至 **1 亿行(≈1/30)**:users 50 万 / sessions 2000 万 / session_turns 5000 万 / learning_events 3000 万,由 `scripts/seed_3b.py` 参数化(速率参数已有 THROTTLE)生成 Postgres 版种子脚本。
- 分布要求:region 列按 40/30/30 三区分布注入(模拟真实用户地理分布),否则 RBR 本地化收益测不出来。

### 4.3 指标

| 指标 | 采集点 |
|---|---|
| 读写 P50/P99/P999 | locust CSV(`--csv=report/`) + 服务侧 OTel |
| 吞吐(ev/s、req/s、WS 并发稳定度) | locust 聚合 + Prometheus |
| **跨区域流量放大比** | TiDB:PD Metrics region 心跳流量;CRDB:`crdb_internal` 网络 metrics / Console——定义为"跨区网络字节 / 用户有效写入字节" |
| 区故障演练恢复时间 | 手动 kill 一区,DB 层 `SHOW ...` + 应用层错误率恢复点 |
| 成本 | 同负载下的节点资源消耗(CPU/IO)× 单价模型,换算每千 ev/s 成本 |

### 4.4 判定标准

1. **吞吐门槛(硬性)**:B 组(CRDB)G-06 5000 ev/s 达标且 P99 写路径 <200ms;不达标直接出局。
2. **读延迟优势验证(核心命题)**:三区本地读 P99 比"全量走 tidb-follower 单点"低 ≥50%,且不劣于 TiDB leader-pinning 理论值;否则 RBR 优势证伪。
3. **成本红线**:跨区流量放大比 >3× 或同吞吐资源成本 >TiDB 的 1.5× → 出局。
4. **迁移风险量化**:压测期间统计 SQL 兼容报错数(驱动改造回归面实测),>0 个 P0 语义差异(自增/唯一约束/事务重试)未闭环 → 出局。
5. 全部通过 → 进入 §5.2 决策门;任一不过 → 关闭 CRDB 线,结论落盘 PROGRESS。

### 4.5 执行顺序(双跑 runbook,后续执行参照)

1. **准备期**:附录 A 起验证集群 → 兼容冒烟(A.3)全绿 → 1 亿行种子注入(region 40/30/30)→ 建多区域配置(§1.3)。
2. **基线期**:A 组(TiDB)先跑全场景 3 轮取中位数(已有历史基线可对齐:PROGRESS 中 G-01/04/06 实测),排除环境漂移。
3. **对比期**:B 组(CRDB)同参数逐场景跑 3 轮;每轮间强制 10min 冷却(排除缓存/compaction 干扰)。
4. **专项期**:①单用户密集复习微场景(user_mastery 冲突重试率);②GLOBAL 表批量改写限频验证;③kill 一区演练(恢复时间 + 错误率曲线)。
5. **收尾**:全部原始数据落 `report/crdb-vs-tidb/`(locust CSV + Prometheus 快照 + 报错清单),本文档回填实测表格,结论写 PROGRESS。

> 注意(单节点铁律):双跑期间压测机与被测服务资源隔离参照既有教训(压测机不得与被测同节点争 CPU);CRDB 验证集群若与现有 TiDB 同 RD VM,禁止并行压测两组,必须串行。

---

## 5. 结论与建议

### 5.1 是否值得引入(ESP 5000 CCU 尺度)

**不值得现阶段引入**。理由三条(展开于 §2):

1. **无 HTAP**:learning_events 30 亿行为分析是产品闭环,TiFlash 路线已验证;CRDB 需另建数仓链路,架构净复杂度上升。
2. **迁移面不可控**:3 语言 6 服务 + 2 公共包 + 种子脚本 + AUTO_RANDOM/ON UPDATE 方言改造,换不来用户可感知收益;读写分离(TSDB 双端点)反而是 CRDB 可省的少数项,节省有限。
3. **写路径无优势**:跨区共识延迟下限两者同为 ~1×RTT;CRDB 的真优势(行级 locality + 任意区本地读)只有在"用户地理分布 × 多区网关接入"成为真负载时才兑现,ESP 当前单集群单入口,优势空转。

### 5.2 推荐路径

**默认线:TiDB 深化多区域**(对应多节点服务器阶段):
- 恢复三区 3 副本(已有实测蓝本,`多区域数据同步方案.md` §1.2)。
- 用 Placement Rules 按 user_id 分区把 leader pin 到用户 home 区,获得近似 RBR 的本地读;TiFlash 保持 aws 区。
- BR 快照 + PITR log backup 纳入运维基线。

**条件触发的双轨验证线(CRDB PoC)**,触发条件(满足其一):
- 产品形态出现"多区域就近接入 + 跨区漫游用户占比 >20%";
- TiDB Placement Rules 行级语义的运维成本在多节点实跑中被证明不可接受;
- 出于教学/对标需要的 CRDB 能力验证(此时按 §4 方法论 + 附录 A 执行,预算 1 亿行数据集双跑)。

---

## 6. 附录 A:CRDB 验证环境落地清单(单节点 RD,后续执行用)

> 目标:RD VM(30G/9 核,现状资源上限)内起 3 节点 CRDB,逻辑模拟三区(对齐既有 Redis/TiDB 三区逻辑模拟做法),仅做功能/兼容性/小规模延迟验证,不做吞吐结论(吞吐结论必须等 §4 多节点双跑)。

### A.1 部署清单

- 镜像:`cockroachdb/cockroach:v24.2.x`(固定 minor,BSL 版本)。
- StatefulSet `crdb-test`,3 副本,`--join=crdb-test-0.crdb-test:26257,...`,每副本 locality 逻辑标签:
  - crdb-test-0 `--locality=region=aws,zone=a`
  - crdb-test-1 `--locality=region=azure,zone=a`
  - crdb-test-2 `--locality=region=gcp,zone=a`
- 资源(单节点 RD 约束下的最小可行):每副本 requests 500m CPU / 1Gi mem,limits 1 CPU / 2Gi;PVC `storage:20Gi`(1 亿行裁剪集 + 预留)。
- Service `crdb-test`:26257(SQL + 节点间共用端口);另暴露 8080 Console(ClusterIP 即可,port-forward 访问)。
- ns 建议 `esp-prod`(与 ESP 验证组件同域,ExternalName `crdb-test`);**不动共享基础设施 ns `ekb-m2-aws-us-east-1`**(共享底座铁律)。

### A.2 初始化与多区域配置脚本(要点,非全文)

1. `kubectl exec crdb-test-0 -- ./cockroach init --host=crdb-test-0:26257`
2. psql 兼容连接:`kubectl run psql-runner --image=postgres:16 -- psql "postgresql://root@crdb-test:26257?sslmode=disable"`(或 `cockroach sql`)。
3. 多区域三件套:
   - `ALTER DATABASE esp SET PRIMARY REGION "aws";` `ADD REGION "azure"` / `"gcp"`(用短名模拟);
   - `ALTER DATABASE esp SURVIVE ZONE FAILURE;`(3 节点只能验 ZONE 目标;REGION 目标需 ≥5 副本,超出 RD 资源,**明确记为单节点验证边界**);
   - 建 3 张样本表分别 GLOBAL / REGIONAL BY TABLE / REGIONAL BY ROW(含 `region crdb_internal_region DEFAULT 'aws'`)。
4. 验证点:跨"区"读写延迟(单节点内无真 RTT,只验证路由正确性不验证延迟)、REGIONAL BY ROW 行搬迁(`UPDATE region`)、控制台副本可视化。

### A.3 兼容性冒烟清单

- pymysql→psycopg 跑 `seed_3b.py` 裁剪版(1 万行)验证 DDL 兼容;
- Java 侧单服务(建议 writing 或 emotion,代码量最小)换 postgresql 驱动跑通 insert/select;
- Go 侧 gateway-go `src/db` 包加 pgx 分支跑 scenarios 读;
- 记录全部 SQL 报错到 `report/crdb-compat-errors.txt`,作为 §4.4-4 的量化输入。

---

## 7. 引用

- 现状 schema:`docs/design/schema.sql`(11 表,AUTO_RANDOM/ON UPDATE 迁移点)
- TiDB 三区实测:`docs/design/多区域数据同步方案.md` §1.2/§1.2.1(Raft 强一致 + 点查证据)
- 读写分离覆盖:`docs/design/多区域数据同步方案.md` §1.2.2(6 服务全覆盖)
- Redis Cluster 三区蓝本(格式参照):`docs/design/redis-cluster-migration-plan.md`
- 压测场景:`deploy/locust/README.md`(G-01/G-04/G-06)、`ccu5000_ws.py`/`spike.py`/`soak.py`
- 种子:`scripts/seed_3b.py`、TiDB 访问封装 `scripts/db_query.sh`(CRDB 需另建 psql 封装)
