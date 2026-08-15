# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。

## 当前(2026-08-16 会话22-39 P1-8 幂等 / 多区域 SQL 预备✅ / P0 端到端验证✅)

### 恢复锚点(会话22-39)
- **P1-8 learning_events 幂等✅(2026-08-16 端到端验证通过)**:代码 7 文件全改(LearningEvent/Service/Disruptor 加 messageId;DbLearningEventConsumer/OutboxRelay INSERT 加 message_id + ON CONFLICT DO NOTHING;BatchEventHandler/EventBatchQueue 深拷贝透传;LearningEventConsumer handleEventReport 透传 ev.messageId + handleSessionEnded 确定性 messageId;crdb-schema.sql 加列+UNIQUE)。**CRDB DDL 已全闭合**:旧卡死 job 1201742016749142017 CANCEL+回滚干净(重启动协调 pod crdb-0 解卡)→ 降副本 num_replicas=1 + checkpoint 10s → 拆句重跑成功:`learning_events`/`learning_event_outbox` 均加 message_id 列;**UNIQUE INDEX learning_events_message_id_key backfill 成功(job 1201752931390455809)→ index 公开(visible=t)** → num_replicas=3+constraints+lease_preferences 已恢复。**镜像**:learning-java:1.11.3 构建+双部署 rollout(20-learning-java 入口 / 21-learning-worker 各 1 副本)。**镜像源坑**:rd-vm docker 镜像源全挂(1ms.run/daocloud/docker.io 全 403)→ 派生构建(FROM 本地 1.11.2 + COPY 新 jar),见 memory esp-registry-mirror-outage-2026-08-16。**E2E 证据**(scripts/e2e-p1-8-idempotency.sh,messageId=p18-e2e-1786821276):同 messageId 投 2 次 → `cnt=1`(60s)、`cnt2=1`(+30s)、`obx=0`(outbox 无残留)= **PASS**。E2E 生产方式:pulsar-broker pod 内 python3+pulsar 3.4.0(避开 640MiB cgroup 内 pulsar-client JVM OOM)+ 重试 6x8s。**已改文件**:7 Java + scripts/crdb-schema.sql + deploy/k8s/20-learning-java.yaml + 21-learning-worker.yaml(→1.11.3)+ 新增 scripts/apply-p1-8-messageid-ddl.sh + scripts/e2e-p1-8-idempotency.sh。
- **#161 多区域 SQL 预备✅(2026-08-16 完成)**:产出 `scripts/crdb-multi-region-migrate.sql`(Phase1 库级 PRIMARY REGION/ADD REGION/SURVIVE REGION FAILURE ×19 库;Phase2 逐表 GLOBAL/REGIONAL BY TABLE/RBR+region 列;Phase3 验证)+ `scripts/crdb-multi-region-migrate-README.md`(**Phase0 单节点显式 DEFERRED**:当前节点 locality=zone=z1/z2/z3 无 region 层,需重启加 region= 层才能跑)。实测:19 库全 primary_region=NULL 非多区域默认态;库内表清单已盘点(esp_learning 6 表/esp_teacher 4/esp_writing 6/... 全列)。
- **#164 P0 第一波端到端验证✅(2026-08-16 完成)**:`report/p0-security-redline-verification-2026-08-16.md`。6/6 PASS 实证:P0-1 私钥仅 bff(learning/gateway PRIV 空)+旧 secret 已删;P0-2 learning 无 token 401/带 token 200;P0-3 gateway history `?user_id=` 覆写忽略 + learner→teacher 403;P0-4 role=admin→强制 learner;P0-5 业务服务全 ClusterIP;P0-6 NetworkPolicy 精确白名单(same-ns pod + kube-system traefik 80/8090/9090 + ipBlock NodePort 端口 80/8080/8090/9090/8848/8858/7091/8091)。回归:gateway scenarios 200 / learning review 200。
- **Redis 锚点**:`esp:ctx:hot:session-39`(本更新)。P1-8 DDL 卡点详情:job 1201742016749142017 已 CANCEL。

### 进行中🔄
- **#160 P1-8 幂等 ✅(本会话四轨全部完成)**:E2E cnt=1/obx=0 验证通过(见上)。四线全闭环:P1-8 / #161 多区域 SQL / #164 P0 验证 / #155 基线核阅。

## 当前(2026-08-16 会话22-38 #140 全闭合 + 新一轮四线推进:P0 安全红线第一波 / P1-8 幂等 / 多区域 SQL 预备)

### 恢复锚点(会话22-38)
- **当前子步骤**:#140/#144/#154/#131 全部闭合并双轨落盘✅(#140 全服务切 CRDB 7/7 env 指 crdb-public;TiDB 三组件 scale 0 + PVC 保留作回切底座;4 Java 服务 SEATA_ENABLED=false;worker 3 副本 Shared 无重复消费)。**用户选择四条线全推进**:① **P0 安全红线第一波**(6 项:私钥收敛/learning 鉴权/IDOR/register 锁 role/NodePort 收敛/NetworkPolicy 收窄,依 `~/.claude/plans/indexed-honking-spring.md`);② **P1-8 learning_events 幂等**(UNIQUE 约束 + 消费端去重);③ **多区域演进 SQL 预备**(region label + LOCALITY REGIONAL 迁移脚本,单节点显式 DEFERRED 应用,产出可移植脚本);④ **基线核阅**(architecture-review-2026-08-14.md 已读:综合 5.9/10,安全 3.5 致命,红线一=私钥扩散已核实)。
- **已确认**:P0 六项安全缺口核实属实(report/architecture-review 第 5 节);当前 deploy yaml 部分已用 `esp-rsa-jwt-sign`(bff L51)/`esp-rsa-jwt-verify`(worker L58/teacher L82)引用,secret 拆分现状待侦察核实(gateway 10-go/learning 20 的 envFrom 引用待查)。
- **下一步**:侦察 subagent 返回 P0/P1-8/多区域文件定位 → 逐项执行 → 每里程碑双轨落盘。
- **Redis 锚点**:`esp:ctx:hot:session-26`(本次更新)、`esp:ctx:hot:session-25`(22-35)。

### 用户指令
① 完整完成所有任务(可并行则并行);② **#140 全服务数据访问切 CRDB + 关停 TiDB 及其所有组件(节省资源)**;③ **Go 统一数据访问层保留读写分离**(用户明确:不丢弃读写分离实现,esp-common-go 双池抽象 DSN 可配);④ **CRDB 多区域**:当前单节点无 Enterprise License→单 region 三副本 + 保留演进位(用户已确认);⑤ **esp_speaking.sessions 5000 万行迁 CRDB**(用户已确认迁移不丢);⑥ 双轨落盘铁律;⑦ **(新 2026-08-15)保留原 TiDB 读写分离实现,便于后续直接切回 TiDB**——Java 已具 `ESP_RW_STORAGE_TYPE=tidb|cockroachdb` 门控(#132);Go esp-common-go 待加 mysql+pgx 双驱动(DSN 前缀自动识别,双池保留),SQL 方言($N vs ?)为明确边界;Python esp-db-py 已保 API 形状(占位符自动翻译)。

### 已确认决策(本会话 AskUserQuestion)
- **Go 统一**:三套 Go DB 层(esp-common-go 单池 / conversation-go 读写分离 / vocab-srs-go 自造 dao)→ 统一到 esp-common-go 双池抽象(单池 MustConnectDB + 双池 MustConnectReadWrite + 包级 InitGlobal/Write/Read/Enabled 保留 forcedPrimary/fallback)。CRDB 单端点阶段 read/write DSN 同址,多区域阶段读池切 follower 代码零改动。
- **esp_speaking 50M**:迁移到 CRDB(非丢弃非重建)。
- **多区域**:当前单 region 三副本;多区域作为多节点服务器上线后的 DEFERRED 演进位。

### 已完成✅(2026-08-15 四轨并行启动)
- **Python 三服务切 CRDB✅(Track D 完成)**:esp-db-py aiomysql→asyncpg(API 形状保持,`%s`→`$N` 自动翻译、lastrowid→RETURNING id、jsonb codec);emotion-svc/writing-py logging_client pymysql→psycopg3(DSN crdb-public/esp_shared,NOW(3)→now(),CAST AS JSON→::jsonb);py_compile+import 全过。**部署前置已落**:30 yaml 三服务 WRITE/READ_DB_HOST→crdb-public:26257 + 新增 WRITE_DB_NAME/READ_DB_NAME(writing-py/coach→esp_writing,emotion→esp_learning)+ CRDB_HOST/CRDB_PORT;**遗留** asyncpg×CRDB 运行时实连未 smoke(待部署验证);seed_3b.py 未迁移(非服务,单独决策)。
- **k8s env 迁移 10/30/32/33 完成✅(待 Go 编译后 apply)**:10-gateway(CRDB_HOST 替换 WRITE/READ_DB_HOST+TIDB_HOST)、30-other(Python 三服务如上)、32-batch2(listening/reading/gamification READ/WRITE_DB_URL→postgres crdb)、33-orchestrator(vocab READ/WRITE_DB_URL 替换 TIDB_*)。**发现** deploy/k8s/esp-prod/ 有 2 个过期副本(vocab-srs-go/llm-orchestrator-rs TiDB env,live 实际用 33)→ Phase4 清理;esp-redis-warmup-job 连 tidb-primary 一次性 Job→TiDB 关停后失效,Phase4 处置。
- **esp_writing 4 小表数据迁移✅**:TiDB writing_plans=2/progress=2/templates=8/essays=4 共 16 行真实数据 → CRDB(显式保留源 id,脚本 `scripts/migrate_writing_to_crdb.sql`),CRDB 计数核验一致。writing_tasks 已知差异:TiDB 1.1M vs CRDB 2M(#136 合成 seed,CRDB ⊇ 无丢失风险,如实记录)。
- **Rust 三服务部署+CRDB smoke✅(Track A 完成)**:pronunciation-rs 0.3.0 / assessment-rs 0.3.0 / llm-orchestrator-rs 1.1.0 rollout 全 1/1 Running,CRDB 三库写删 smoke 通过(pronunciation_attempts/assessment_answers/llm_call_logs INSERT+DELETE),readiness 探针 `{"db_read":true,"status":"UP"}`。**坑㉗**:live 集群 Rust 探针是旧 tcpSocket+TiDB env(文件 CRDB env 从未 apply),kubectl apply 合并双 handler Invalid → 非破坏性 patch 探针为文件 httpGet 后 apply 成功。**遗留**:32 yaml 内 listening/reading/gamification 3 Go 服务同样探针漂移(tcpSocket),待 #142 Go 迁移时一并修。
- **CRDB 辅助服务建库建表✅**:写 `docs/design/crdb-schema-aux-services.sql`(TiDB DDL→CRDB 方言:AUTO_RANDOM→unique_rowid()、DATETIME→TIMESTAMPTZ、JSON→JSONB、去 ENGINE/ON UPDATE)。9 库 30 表全部建齐(esp_speaking 新建库+scenarios/sessions;pronunciation/listening/vocab/assessment/gamification/reading/llm 从 0 表补齐;writing 补 4 表)。CRDB 核验 `SHOW TABLES` 全过。
- **esp-common-go/db/db.go 统一层重写✅**:mysql→pgx(v5.10.0,go.mod 已加)、DSN→postgres、单池+双池+全局包级 API(保留读写分离/forcedPrimary/fallback)。
- **四轨并行子代理已派发**:A=Rust 三服务部署+smoke(esp-builder);B=TiDB→CRDB 数据迁移含 50M sessions(esp-data,长任务断点续迁);C=Go 服务方言迁移+编译(esp-builder);D=Python 服务迁移(esp-builder)。
- **Go/Python 镜像构建推进✅(2026-08-15,第2个落盘点)**:**坑㉘ gateway-go 镜像构建失败根因=go.mod/go.sum 缺 mysql 传递依赖**(esp-common-go db.go 新增 `_ "github.com/go-sql-driver/mysql"` 双驱动,但 gateway-go go.mod 仅间接依赖 pgx;本地 build 过是 GOMODCACHE 已缓存,容器内 `go mod download` 缺 go.sum 条目失败)→ `go mod tidy`×5 服务(gateway/vocab/listening/reading/gamification)补 `go-sql-driver/mysql v1.10.0 // indirect`+go.sum,gateway-go:1.11.0 重建成功(66.8MB)。**yaml 标签已对齐**:10-gateway=1.11.0、32(reading/gamification=0.3.0,listening=0.3.0,pronunciation/assessment=0.3.0 未动)、33(vocab=1.1.0)、30(writing-py=1.6.0/emotion-svc=1.7.0/writing-coach=1.2.0);**防降级**:30 yaml teacher-java 原 1.9.0 但活体 1.10.0→已改 1.10.0(否则 apply 会把 teacher 打回旧镜像)。**CRDB 目标库/表前置核验✅**:esp_vocab(user_vocabulary/vocabulary)、esp_reading(reading_attempts/passages)、esp_gamification(user_streaks)、esp_listening(listening_attempts/exercises)、esp_writing 5 表、esp_learning.learning_events(payload=JSONB) 全在;esp_shared.app_logs 在(logging_client 目标)。writing-py:1.6.0 镜像已实证 asyncpg 0.31.0+esp_db。**会话归档写库前置**:esp_speaking.sessions 现 BIGINT,gateway-go 新代码写 STRING id→gateway-go 部署 DEFERRED 至迁移完成 + `ALTER TABLE esp_speaking.sessions ALTER COLUMN id TYPE STRING;`。
- **Go 服务方言迁移+编译✅(Track C 完成)**:7 Go 模块全 build RC=0;esp-common-go 统一双池(Write/Read/forcedPrimary/fallback,DSN 前缀自动识别 pgx/mysql);conversation-go/src/db/db.go 旧 MySQL 层删除;vocab-srs-go DAO 重写 ON CONFLICT/RETURNING;gateway-go ListHistory 修 `COALESCE(ended_at::STRING,'')`(原 `COALESCE(ended_at,'')` CRDB SQLSTATE 22007)。**esp-common-go 双驱动(mysql+pgx)已实现✅(#148)**:`driverFor()` 按 DSN 前缀选驱动,`ReadWriteDSN()` 支持 `DB_DRIVER=mysql`→tidb-primary/tidb-follower 双池回切,CRDB 阶段 `CRDB_READ_HOST` 切读池。course-path-go/live-tutor-go 无 DB 访问(scaffold,免迁移);conversation-go 为库内嵌 gateway-go 构建。
- **Go+Python 全量部署 CRDB+运行时 smoke✅(2026-08-15,第3落盘点)**:7 镜像全建(Go5+Python2),apply 32/33/30,**坑㉙ 复用坑㉗ 探针漂移**:live listening/reading/gamification 探针是旧 tcpSocket,yaml 是 httpGet,merge patch Invalid → 先 patch 三服务探针为 httpGet 再 apply 成功。8 部署全部 rollout 1/1(含 teacher-java 防降级 1.10.0、llm-orchestrator-rs、vocab-srs-go unchanged=早前 Track C 已 CRDB)。**运行时验证**:listening/reading/gamification readyz `{"db_read":true}`+gamification `redis:true`,vocab readyz 探 DB ping UP(4 Go 全 CRDB 通);**Python 三服务 DB 路径实证**——emotion-svc `POST /api/emotion/sense` → CRDB esp_learning.learning_events 落行 `{"emotion":"neutral","text_snippet":"crdb smoke"}`(JSONB),writing-coach `GET /api/writing-coach/templates` 200 返 8 模板(CRDB 读)+`plan/{uid}` 可信头查询正常,writing-py `POST /api/writing/grade` → esp_writing.writing_tasks 新行 id=1201653028465213443 score=24.0(读历史+写主库双路径全通)。**剩余**:gateway-go 1.11.0 待 sessions 迁移完成后 `ALTER TABLE esp_speaking.sessions ALTER COLUMN id TYPE STRING;` 再部署(现 BIGINT 列,新代码写 STRING id)。
- **Phase4 TiDB 连接审计✅(#144 前置,2026-08-15)**:TiDB 部署形态=非 StatefulSet,三 deploy 于 ekb-m2-aws-us-east-1(共享 ns):`tidb-cluster-tidb`(单 pod,svc :4000)+`pd-aws`+`tikv-aws`;esp-prod ExternalName 链:`tidb-primary`→platform-infra `tidb`→`tidb-cluster-tidb`;**`tidb-follower`→同一 `tidb-cluster-tidb`(伪分离实证,与 #132 双池回切设计不冲突)**。`information_schema.processlist` 实测:**仅 2 连接**——migrate-runner(esp_speaking 迁移源,idle,须保留至迁移完成)+ 审计查询自身;**任何 ESP 服务已无 TiDB 连接**(Java 的 WRITE_DB_URL=jdbc:mysql://tidb-* 为惰性回切残留,Python/Rust TIDB_HOST 为惰性残留)。→ 关停 TiDB 前仅剩:①sessions 迁移完成;②关停确认(共享 ns 邻居项目影响评估,见 #144 待办)。
- **sessions 迁移加速✅(2026-08-15 09:33)**:8.6M/50M(17.2%),rate≈41K rows/s(重试改造后不再被 TiKV 单节点 region 压力拖死),**ETA≈0.3h**;续迁锚点 last_id=1441151880781348190。迁移完成后续:ALTER sessions.id→STRING→部署 gateway-go 1.11.0。
- **坑㉚ CRDB v24.3 无法原地 ALTER id→STRING(2026-08-15,会话22-37)**:**原计划 `ALTER COLUMN id TYPE STRING` 被 CRDB 两重硬限制否定**——①int→string 属 general path,需 `SET enable_experimental_alter_column_type_general=true`(experimental);②即使开 experimental,**"ALTER COLUMN TYPE requiring rewrite of on-disk data is currently not supported for columns that are part of an index"**(issue 47636),而 sessions.id 是 PK + idx_user 隐式后缀列,无处可避;`DROP CONSTRAINT pkey` 又被要求"drop+add PK 同事务"(issue 48026)。→**原地 ALTER 不可行**。**修订方案(已定)**:迁移完成后用 CRDB 原生 `INSERT INTO sessions_new (id::STRING...) SELECT ... FROM sessions` 重建 sessions 表为 STRING id + drop+rename(原生 SQL 走分布式执行,比 psycopg2 网络批量快数倍;TiDB 源数据在关停前仍是安全网)。**Why 必须 STRING**:gateway-go 1.11.0 End() 写 `s.ID`(sess_xxx STRING)入 sessions.id + ListHistory 扫 id 到 Go string,API 契约=DB id==WS id==客户端可见 id。重建脚本待命 `scripts/rebuild_sessions_id_string.py`(迁移完成后执行,再部署 gateway-go 1.11.0)。
- **坑㉜ 迁移崩溃根因=TiDB/TiKV 双 OOM(2026-08-15,会话22-37,用户定案暂停+关停 TiDB)**:batch=20000 读压下**TiDB 1Gi 与 TiKV 3Gi 先后 OOMKilled**(TiDB 5 次重启、TiKV 18 次,末次 10:53:54),TiKV 单节点 region 选举风暴("Region is unavailable")→ 迁移进程死在 done≈16.4M(CRDB 实存 17.01M)。**脚本重连 bug**:`connect_tidb()` 在 except 块内重连失败时抛 2003 逃逸出 retry 循环(非重试语义),已修——connect 移入 try 内、失败即走 retry(最多 MAX_RETRY=10),`migrate_esp_speaking.py` 编译通过。**用户定案(2026-08-15 11:00)**:无法稳定迁移 → **暂停迁移 + 关停 TiDB 再执行后续**。已执行:①aws TiDB 三组件 scale 0(tidb-cluster-tidb/pd-aws/tikv-aws,共享 ns ekb-m2-aws-us-east-1),Service+2 PVC(5Gi/10Gi)保留,数据可回切;②azuer/gcp region 集群未动(ESP 不依赖);③迁移脚本保留断点续迁(resume-from-max(id)),migrate-runner pod 保留。**迁移现状如实记录:CRDB esp_speaking.sessions=17.01M/50M(34%),缺口 33M 留存 TiDB PVC**。
- **rebuild_sessions_id_string.py 执行中(2026-08-15 11:05→13:30)**:对 CRDB 现有 17.01M 行重建 sessions(id BIGINT→STRING + 重建 idx_user),复制入 migrate-runner pod 后台跑(脚本 /tmp/rebuild_sessions_id_string.py,日志 /tmp/rebuild_sessions_run2.log)。**COPY 进度(13:30 实测)**:sessions_new 3 range 合计 1255.4MB / 源 sessions 1508.5MB = **83%**,COPY 单语句执行 1h03m 仍在 executing(早前"120MB/min"为 rebalance/compaction 瞬时噪音,真实持续 ~20MB/min → 剩余 ~12-15min);已恢复轮询重建日志的持久 monitor(b2jf7pgli,抓 COPY 完成/backfill 完成/DROP 旧表/重建完成/Traceback)。脚本后续自动:count 核验→恢复 3 副本 zone(num_replicas=3,z1/z2/z3 各1,lease→z1)+ 等 backfill(replica_localities<3 轮询)→ DROP+RENAME。完成后 deploy gateway-go 1.11.0(10-gateway-go.yaml 已 CRDB env 就绪)→ verify_gateway_archive.py 验证 End() 写 STRING id 归档。
- **方案C locality+follower-read 落地(2026-08-15 #149/#150 完成)**:**用户架构质询**"locality 空能否同步/是否改 2 副本"→已答:locality 仅影响调度不影响 Raft 同步;2 副本 CRDB 可用性数学更差(任何单节点丢失=写停)→ 用户批准方案C(3 副本+locality+follower-read)。**#149 已落地**:`deploy/crdb/crdb-3node.yaml` 按 pod ordinal 注入 `--locality=zone=z{1,2,3}`,rolling restart(Parallel policy 全滚,2/3 保持 quorum),实证 `gossip_nodes` node1=z1/node2=z2/node3=z3;`ALTER DATABASE esp_speaking CONFIGURE ZONE USING num_replicas=3, constraints='{+zone=z1:1,+zone=z2:1,+zone=z3:1}', lease_preferences='[[+zone=z1]]'`,实证 `SHOW RANGES ... WITH DETAILS` 全 range replica_localities={z1,z2,z3}+lease_holder=1,intent_bytes=0。**#150 已落地(代码+单测)**:`esp-common-go/db/db.go` 读池 DSN 注入 `default_transaction_use_follower_reads=on`(startup 参数,psycopg2 实证 CRDB 接受 + pgx v5 pgconn RuntimeParams 转发),写池不注入保持强一致;`db_test.go` 3 测试通过(CRDB 读注入/写不注入/mysql 路径不变)。**重建提速**:sessions_new 载入期临时 `num_replicas=1`(单节点 3 副本同步写放大 3x 是重建慢根因,~12%/38min→预计 3x 提速),COPY 完成 count 核验后恢复 3 副本 zone + 等 backfill(卡点数 replica_localities<3 轮询)再 DROP+RENAME;脚本已加该逻辑。**进行中**:sessions_new COPY(1 副本)后台跑 + gateway-go:1.11.0 重建镜像(带 follower-read)并行。

### 已完成✅(含证据)
- **#133 重构落地✅(按审查 Top5 ROI)**:learning-java 拆独立 DB writer 线程池——新增 `disruptor/EventBatchQueue.java`(有界 BlockingQueue+深拷贝入队+队列满 DROP 不阻塞 consumer+独立 writers=4);`BatchEventHandler` flush 不再同步调 consumer;`DbLearningEventConsumer` batchUpdate 失败整批写 outbox(status=failed)由 OutboxRelay 重试替代仅 log 丢数据;`OutboxRelay` 逐行 N+1→batchUpdate(200 行 400 往返→~3);disruptor 配置显式加 consumers/writers/writer-queue。gateway-go WS 加固——新增 `conversation-go/src/ws/SafeConn`(per-conn writeMu+WriteDeadline 10s),hub conns 改 *ws.SafeConn 解并发写(gorilla 非并发安全),`SetReadDeadline(60s)`+SetPongHandler+30s ping 协程,`newID()`→`sess_<unixms>_<8位rand hex>`(零新依赖,注释说明),`session.Manager.End` 归档有界 semaphore(256)。GlobalExceptionHandler 三服务各一(统一 envelope{code,message,traceId},actuator 白名单)。**测试全绿**:gateway-go/conversation-go go test ok(含 newID 并发 1000 无重复)、learning-java 59、bff 33、teacher 20。**部署:esp/gateway-go:1.10.0 + esp/learning-java:1.10.0 rollout 成功**(pod 日志确认 consumers=4/writers=4/writerQueue=100),冒烟 /livez /readyz 200、/api/scenarios 200、WS 收到 ai_reply 帧(TTS+emotion+visemes)、session ID 新格式。**遗留:bff/teacher 的 handler 代码已写但未重建镜像部署**(并入 #135 统一做)。
- **#132 CRDB DAL✅**:esp-rw-java 新增 `CockroachDbDataSourceProperties/Config`,`esp.rw.storage.type=tidb|cockroachdb`(默认 tidb 互斥门控 @ConditionalOnProperty),读写分离复用 RoutingHolder/@DataSource,PG driver postgresql:42.7.8(provided)。19 测试全过(4 新 CRDB 配置+2 条件装配+13 回归),mvn install 成功。
- **#134 README 细化✅**:README.md + README.zh-CN.md 同步新增架构速览数字表、Disruptor 多级调度(.stage 链)+分片(sequence%shardCount)mermaid、读写分离 RoutingDataSource 图、并发模型评分表(6.7/5.7/4.0/3.8)、G-04/G-06 压测数字表+重构后预期(标待复测)、覆盖率矩阵、5.2/10 工程化评分卡。**两处待校正**:bff 实际 2-stage(非 3)、esp.rw.storage.type 已实现但 README 标 "reserved"(#132 已实现)。
- **工程铁律 10 条✅(所有项目)**:权威副本 `docs/design/architecture/engineering-ironrules.md`(根因 R1-R6 + 方案 S1-S6 + 铁律 1-10:①公共能力共享包优先②设计三态追踪③契约测试④无产物不声称完成⑤单节点显式 DEFERRED⑥CI门禁⑦双轨落盘⑧共享设施红线⑨上下文工程⑩诚实优先);全局 memory `engineering-ironrules` + 全局 ~/.claude/CLAUDE.md 第 7 条;Redis `esp:ctx:ironrules:engineering`。
- **共享代码审计✅**:`docs/design/architecture/shared-code-audit-2026-08-15.md`。发现:GlobalExceptionHandler×3、JwtValidator/JwtAuthFilter×2(逐字节同仅 package)、AppLogClient×3、jjwt+bc 依赖块×3、gateway 自写 tracing.go/healthz、conversation/vocab 自写 DB 池(20/5 vs 默认 10/2)、esp-common-rs health.rs 零服务使用、emotion/writing 手写 logging_client.py、**bff Dubbo 配置全空转(0 @DubboReference,全走同步 RestClient)**、IdempotencyFilter 无 @Component、跨语言 traceId 键名不统一(traceId/trace_id)。Top5 抽共享包路线(esp-web-common/JWT starter/common-go 收编/rs 健康/app_logs 契约)。
- **#124 归档✅**:PROGRESS 623→82 行,归档 `docs/progress/archive/session22-27-to-30-archive.md`;Redis `esp:ctx:hot:session-24`。
- **#139 CRDB 驱动兼容修复✅(2026-08-15)**:根因=PG JDBC 42.7.8 `setObject(int, java.time.Instant)` 抛 SQLState 07006(最小探针实证,TiDB/MySQL 驱动支持 Instant 故 TiDB 模式正常);CRDB 模式下 learning_events 批量写**全转 outbox(~4 往返/事件)** + app_logs 遥测降级丢弃。修=`DbLearningEventConsumer`/`AppLogClient` 时间参数绑定 Instant→Timestamp,重建部署 **learning 1.11.1 / bff 1.10.0 / teacher 1.10.0**。验证:behavior POST→learning_events 直写 +1 零 outbox 转发;app_logs 实落 CRDB `esp_shared.app_logs`(`behavior.answer 999`)。
- **#137 全服务 TiDB vs CRDB 压测✅(2026-08-15)**:同一 harness(`report/crdb_fullbench.py`,集群内 ClusterIP 直连 RS256,5 端点×并发 20/60)。TiDB 基线 vs CRDB 腿:**全部 DB 路径 CRDB 落后**(learning_events 写 0.41-0.56×、review 读 0.42-0.62×、kp-stats 读 0.54-0.69×、bff scenarios 读 0.33-0.71×;gateway 非 DB 参考 0.80-1.00× 示单节点争用噪声),与微基准 0.54× 一致。对比落盘 `report/crdb-vs-tidb-fullbench-2026-08-15.json`(+`{tidb,cockroachdb}-2026-08-15.json`);README 双版已更新;服务已恢复 TiDB 模式(生产缺省)。
- **运维清理**:17 个卡 ContainerCreating 孤儿 pod(st-seeder-*×16 + esp-schema-init,无 owner)强删;HPA learning-java/teacher-java 钉 1/1 释放 ~1000m CPU;esp-crdb 恢复 3 副本(3/3 Running,v24.3.11)。

### 进行中🔄
- **#136 CRDB 分库+灌数✅(2026-08-15 完成)**:6 域库 13 表全部灌入,seed pod Succeeded(elapsed=7639s≈2.1h,断点续跑:users/scenarios/kp 已满跳过、sessions 补 1.14M→5M)。**最终计数(seed 脚本 final_counts == 独立复核)**:users 1M / scenarios 10k / knowledge_points 10k / sessions 5M / session_turns 5M / **user_mastery 4,998,737(差 1,263 行,0.025%)** / learning_events 10M / writing_tasks 2M / lesson_plans 1M / classes 500k / assignments 2M / submissions 2M / app_logs 85k。合计 33,603,737 行。**唯一偏差**:user_mastery 差 1,263(两源一致,如实记录;对压测无影响)。Redis `esp:ctx:seed-crdb-progress` 100%。
- **#135 esp-web-common 共享模块✅(2026-08-15 部署+冒烟完成)**:mvn package×3 RC=0 → docker build×3 RC=0 → apply learning 1.11.0 / bff 1.9.0 + set image teacher 1.9.0,全部 rollout ready=1/1。**TiDB 模式冒烟全绿**:learning readiness UP / behavior POST `{"accepted":true}` **且 esp_shared.app_logs 实落一行** `('learning','behavior.smoke135',12345)`(#135 两段式 INSERT 验证通过)/ recommend 返回复习数据 / **bff scenarios(8090 mvc 端口)200** / **teacher kp-stats(teacher 角色 token)200** 返回真实 mastery 数据。**附带修复**:live teacher envFrom=null(JWT 公钥未注入→filter 禁用→无 role 全 403)→ patch 对齐 git yaml(esp-rsa-jwt-verify),pubkey 注入后鉴权正常。
- **#131 learning-worker 分片(待执行)**:worker yaml image 已升 1.11.0。入口 ESP_DISRUPTOR_ENABLED=false→Pulsar,worker PULSAR_CONSUMER_ENABLED=true,验证 1→3→5 消费。

### 问题与解决方案(本会话)
| 问题 | 解决 |
|---|---|
| 声称完成但无产物(#131/#132/#133 前期核验) | 核验发现未做→实际执行;触发铁律四"无产物不声称完成" |
| 单节点 CPU 99%(HPA 扩 learning 4/teacher 3) | 钉 HPA min/max=1,释放 ~1000m CPU |
| worker 会落错库(esp 100M 老库而非 esp_learning) | 21-learning-worker.yaml WRITE/READ→esp_learning + SHARED_DB_URL |
| CRDB 单库 5 冒烟表(用户要求分库) | 6 域库拆分 + 等量灌装脚本 |
| 16 seeder pod 卡 ContainerCreating 173min | 孤儿 pod(无 ownerReference)强删 |
| TiDB Region unavailable(大表 count 不稳) | 大表行数记近似值,压测前 TiKV 健康检查 |
| GlobalExceptionHandler 散落 3 服务 | 建 esp-web-common 共享包(#135,已部署验证) |
| live teacher envFrom=null→JWT 禁用→全 403 | patch envFrom esp-rsa-jwt-verify 对齐 git yaml,冒烟 200 |
| bff 冒烟 8080 Connection refused | bff 端口=mvc 8090/edge 9090/dubbo 50090,非 8080;改用 8090 验证通过 |
| CRDB 模式 learning 批量写全转 outbox + app_logs 降级(#139) | 根因=PG JDBC 42.7.8 setObject(Instant) SQLState 07006(探针实证;MySQL 驱动支持 Instant 故 TiDB 正常);改 Timestamp 绑定 + 重建,验证直写零 outbox + app_logs 落 CRDB |
| bff/teacher 增量 mvn package 假成功(内嵌旧 esp-web-common) | 源码未变→repackage 跳过;改 `mvn clean package` 强制重打,解包核验修复字节码 |

### 恢复锚点(会话22-35)
- **当前子步骤**:#137 全服务 TiDB vs CRDB 压测**已闭环**(证据见"已完成"#139/#137),服务已恢复 TiDB 模式(production 缺省),README 双版已更新。**当前任务 = #131 learning-worker 1→3→5 副本水平分片验证**:① worker yaml `21-learning-worker.yaml` image 现为 esp/learning-java:1.11.0 → 需升 **1.11.1**(#139 重建后镜像);② 入口 learning-java 设 ESP_DISRUPTOR_ENABLED=false + PULSAR_CONSUMER_ENABLED=false(防消费循环),worker 设 ESP_ROLE=worker + PULSAR_CONSUMER_ENABLED=true;③ replicas 1→3→5 观察 Pulsar 消费分片 + learning_events 落库(worker 写 esp_learning)。
- **已确认根因**:G-06=Disruptor consumer 同步 batchUpdate 反压(已拆独立 writer 池);bff Dubbo 配置未接线(0 @DubboReference);teacher 403=live envFrom null 无公钥(filter 禁用);**CRDB 模式批量写全转 outbox=PG JDBC 42.7.8 setObject(Instant) SQLState 07006(已改 Timestamp 修复,#139)**。
- **部署核实(2026-08-15 更新)**:活体=learning **1.11.1** / bff **1.10.0** / teacher **1.10.0**(TiDB 模式);CRDB 6 域库 seed 33.6M 行保留(待多区域复测);crdb-public 26257 通。
- **已改文件**:`DbLearningEventConsumer.java`+`AppLogClient.java`(Instant→Timestamp)、`README.md`/`README.zh-CN.md`(#137 全服务对比 3 处)、`report/crdb-vs-tidb-fullbench-*.json`×3、`21-learning-worker.yaml`(待升 image→1.11.1)。
- **下一步**:#131 worker 1→3→5 分片验证 → #131 完成双轨落盘收尾。
- **Redis 锚点**:`esp:ctx:hot:session-26`(本更新)、`esp:ctx:session:session-22-35`、`esp:ctx:seed-crdb-progress`。

---

## 当前✅(2026-08-15 会话22-33 全服务切 CRDB 完成 + sessions STRING 重建 + 用户指令停灌)

### 任务
#140 全服务数据访问切 CRDB + 关停 TiDB(用户根指令);#141~#145 Phase1~2 Java/Go 切 CRDB env;#146 清理 Seata AT 死配置;#147~#150 Go 统一+CRDB DDL+3副本+follower-read;#151 重建 sessions STRING + 灌随机数据;#152 Java 服务切 CRDB env apply。

### 落地内容(CRDB 切库全闭合✅)
- **#140 全服务切 CRDB✅**:Java 4 服务(bff/teacher/learning/learning-worker)env `ESP_RW_STORAGE_TYPE=cockroachdb` + `ESP_RW_COCKROACHDB_HOST=crdb-public`,均已 PgConnection 实连验证;Python 服务(writing-py/emotion-svc/writing-coach)env `WRITE_DB_HOST/READ_DB_HOST=crdb-public:26257`;Go 统一 esp-common-go 双驱动(pgx+mysql)可回切。**TiDB 已 scale 0**(svc+PVC 保留可回切)。
- **#152 Java CRDB env apply 4/4 Ready✅**:`15-bff-java.yaml` L26 `SEATA_ENABLED=true→false`(yaml 漂移修正:原 true 致新 pod 连不上已 scale 0 的 Seata TC→CrashLoopBackOff;根因非 CRDB)。新 pod `bff-java-7bb9b48f54-27znm` 1/1 Running,PgConnection 实连,env 确认 `ESP_RW_STORAGE_TYPE=cockroachdb`/`SEATA_ENABLED=false`。learning/worker/teacher 前序已 Ready。
- **#151 sessions STRING 重建✅(用户指令停灌)**:DDL `id STRING NOT NULL PRIMARY KEY`(gateway-go 1.11.0 硬要求)。灌入 **2,020,000 行**(id=sess_<n>,无重复,类型=STRING)后**用户明确指令停止灌装**("已有的数据可以直接使用了")。灌数期间 `num_replicas=1`(负载阶段),**已恢复 3 副本 zone**(`CONFIGURE ZONE USING num_replicas=3`,SHOW CREATE 实证)。用户决策:2.02M 即最终数据量,不再追 17M 目标。

### 待执行
- **多区域演进**(DEFERRED):加 `region=` label + `ALTER DATABASE ... LOCALITY REGIONAL`,应用代码零改动,多节点服务器上线后启用。
- **learning_events 幂等缺口**(P1-8 候选):`DbLearningEventConsumer` 纯 INSERT,无 UNIQUE 约束(messageId 去重),Shared 重投递有重复风险。当前 10,001,206 行稳态无增量。

### 恢复锚点(会话22-33)
- **当前子步骤**:CRDB 切库全闭合✅;sessions STRING 2.02M✅(用户停灌);3 副本 zone 已恢复✅(z1/z2/z3 各 2,020,000,under-replicated=0,range 84/84/84 均衡);**#131 worker 3 副本水平分片闭合✅**(用户定不再扩 5);**#154 Phase4 CRDB 连接回归闭合✅**(7/7 env 指 crdb-public,6 直连 PgConnection 实连,0/7 TiDB 回退);**#144 TiDB 清理闭合✅**(用户决策:共享底座 scale 0 + PVC 保留作回切网,符合 esp-shared-infra-ironrule;ESP deploy yaml 的 TiDB 惰性回切 env 保留作占位,#132 双池门控下非活路径;邻居 education-platform 5 deploy env 仍指 TiDB 但全 scale 0,互不干扰)。
- **已确认根因**:bff CrashLoopBackOff = Seata yaml 漂移(SEATA_ENABLED=true vs TC scale 0),非 CRDB;新 worker 启动期 redis-cluster DNS 瞬态 NXDOMAIN(Netty DNS resolver 与 pod 网络就绪竞态),startupProbe 900s 窗口内自愈转 1/1。
- **已改文件**:`15-bff-java.yaml`(L26 Seata fix)、`20/21/30-other` yamls(CRDB env)、`scripts/seed_sessions_parallel.sh`、`report/phase4-crdb-connection-verify-2026-08-16.md`。
- **TiDB 清理最终态**:tidb-cluster-tidb/pd-aws/tikv-aws deploy 全 0/0 scale 0;PVC pd-aws-data(5Gi)/tikv-aws-data(10Gi) Bound 保留;esp-prod ExternalName tidb-primary/tidb-follower 保留(回切链);4 Java 服务 SEATA_ENABLED=false 实证。
- **下一步**:#140/#144 双轨落盘收尾 → 等待用户下一指令(候选:P1-8 幂等 / 多区域演进 / 计划文件 P0 安全红线重构)。
- **Redis 锚点**:`esp:ctx:hot:session-25`、`esp:ctx:hot:session-26`(会话22-35)。

---

## 上一节(2026-08-15 会话22-32 架构师级全服务审查 + Disruptor 公共多级调度队列审查)

### 任务
#129 Disruptor 公共多级调度队列审查 + #130 架构师级全服务审查(架构评分 + Top5 ROI 重构项)。

### 落地内容(审查闭合项)
- **#129 Disruptor 公共多级调度队列审查✅**:`platform/esp-disruptor` 的 DisruptorPipeline 支持任意层级自定义(`.stage()` 链)——bff-java 3 级 ingest→normalize→forward;learning-java 单级 DisruptorWorker;**teacher-java 未接入 = 唯一缺口**。整体流程测试通过:`LearningEventDisruptorTest` 2/2(批量消费 + ≥5000ev/s 发布吞吐)。
- **#130 架构师级全服务审查✅**:报告 `report/architecture-review-2026-08-15.md`,综合 **5.2/10**(栈选型7/落地4/测试4/根因7/质量4)。**Top5 ROI**:① learning Disruptor consumer 拆独立 DB writer 线程池(现 4 consumer 线程同步 batchUpdate 反压 RingBuffer);② gateway-go WS ping/pong+deadline+UUIDv7 session ID+Hub 写互斥(现 `sess_<unix_ms>` 碰撞 + 无 deadline 头线阻塞);③ bff-java 接线 Dubbo Triple(配置已存在但零 `@DubboReference`,全走同步 RestClient=G-06 根因之一);④ 共享模块(esp-common-go/rust/esp-cache-py/esp-db-py)零单测补齐;⑤ 全局异常处理+DB 池 env 化+日志统一。**新发现**:OutboxRelay 逐行 INSERT N+1、DbLearningEventConsumer batch 失败仅 log 丢数据。
- **learning-java 1.9.0 稳定部署**:rev25 单 pod 1/1 Running;`learning_event_outbox` 表已在 esp_learning schema 落地。

### 待执行
- **#131** learning-worker 1→3→5 分片验证(部署 0 副本未启);**#132** esp-rw-java 加 CockroachDB 读写分离+可配置选型(esp-crdb 已扩容 3 副本);**#133** 按审查 Top5 重构落地;**#134** README 细化。

### 恢复锚点(会话22-32)
- **当前子步骤**:#129/#130 审查闭合✅(本会话双轨落盘)。已改文件=none(纯审查)。
- **下一步**:#131 worker 分片 → #132 CRDB DAL → #133 重构落地 → #134 README。
- **Redis 锚点**:`esp:ctx:hot:session-24`。

---

## 当前✅(2026-08-15 会话22-31 C/D 类批量推进:全 7 项闭合✅)

### 任务
兑现 C 类(Loki/ArgoCD)+ D 类(73条人工测试/5000CCU压测复测/探针三分离/数字人TTS/个性化推荐/全方位README)。用户指令"完整完成所有任务(可并行则并行)"+ 要求双轨落盘以便上下文溢出后恢复。

### 落地内容(7/7 全闭合✅,详细数字见对应 report/ 与 manifest)
- **#116 Loki 日志聚合✅**:otel-collector filelog receiver(零新镜像,fluent-bit 403 不可拉)。坑链=docker json 日志符号链接/root-only/json_parser+regex_parser/cid guard;6/7 pod Running,~150k-210k/2-3min 入 Loki。manifest `deploy/k8s/monitoring/10-observability.yaml`。
- **#120 探针三分离✅**:11 服务全量改 /healthz /readyz /livez(Go gin)+ /actuator/health/readiness/liveness(Java/Rust);tcpSocket→httpGet;preStop sleep 5~15 + terminationGracePeriodSeconds 30。gateway-go 1.9.0 重建(rollout clean,/readyz /livez 200)。
- **#121 数字人 TTS 情绪驱动✅**:`conversation-go/src/ws/frame.go` Frame 加 Emotion+Visemes;`src/tts/mock.go` Result{Audio,Emotion,Visemes}(Preston Blair 口型 60ms/viseme,cap 400);gateway-go handleUserText 接 TTS(仅 AIReply 帧附 emotion+visemes);mock_test 3/3;gateway-go 1.9.1 重建。
- **#122 个性化推荐 emotion_aware✅**:`BehaviorController` /recommend 加第 4 维(近 7 天 emotion 事件 neg/pos/unknown 三态);builder agent 旧 jar 坑→`mvn -DskipTests package`+`--no-cache` 重建 1.8.9;实测 keys=[review,weak_points,similar_scenarios,emotion_aware],emotion_aware={state:unknown}。
- **#118 73条人工测试✅**:report/manual-73-run-20260815-062028.json(PASS=57/FAIL=1/SKIP=22)。FAIL 仅 SEC-6(限流 429 未触发,Nacos flow 未发布非功能缺陷);placement 带 kp_ids 经 BFF 返 401 待 trace;WS 全会话 TC-E2E-7 PASS。
- **#123 全方位 README✅**:README.md + README.zh-CN.md(各 635 行,6+ mermaid 图);CRDB vs TiDB 实测(139.0 vs 259.3 insert/s,ratio 0.54,多区域 4 维度,拒选结论);四语言 12 维对比表+决策树。
- **#119 5000 CCU 压测复测✅(未达标,单节点瓶颈)**:report/ccu5000-g0{4,6}-*-2026-08-15.json。G-04 WS peak 955(FAILED 5000,根因=gateway-go 单 pod 512Mi+session ID 碰撞;建议多节点+3 副本 1Gi+UUIDv7);G-06 events peak 370ev/s(FAILED 5000,根因=同步 thread-per-request+Disruptor consumer 同步批量写+scale 2 反 CPU 争抢;建议独立 DB writer 线程池+修 OutboxRelay)。
- **#117 ArgoCD GitOps✅(控制平面已起 + 3 STS OOS 定性为结构性限制)**:quay 镜像 403→本地缓存 v3.5.0 起控制平面;3 StatefulSet OutOfSync=managedFields=0 击穿 SSA 归属判定,`kubectl diff git vs live` 空=非真 drift,业务零影响,留多节点剥离管辖。

### 恢复锚点(会话22-31)
- **当前子步骤**:C/D 类 7/7 全闭合✅(#116/#117/#118/#119/#120/#121/#122/#123)。
- **已确认根因**:① builder agent 陷阱——Dockerfile `COPY target/*.jar`,mvn package 未重跑则 COPY 旧 jar,修复=`mvn -DskipTests package`+`--no-cache`+升 tag;② quay/公开镜像 403→本地缓存;③ 3 STS OOS=managedFields=0 空 diff 非真 drift。
- **下一步**:① git commit 批次(C/D 全闭合 + #117/#119 报告);② 多节点移植时剥离 3 STS argocd 管辖 + 5000CCU 达标。
- **Redis 锚点**:`esp:ctx:hot:session-23`、`esp:ctx:hot:refactor-p0~p2`。

---

## 历史 milestone 归档索引(需要时取)

| 主题 | 文件 | Redis key |
|---|---|---|
| **会话22-14~22-30 + CI修绿 + P1-13 httpOnly(热区瘦身,2026-08-15 归档)** | `docs/progress/archive/session22-27-to-30-archive.md` | — |
| **会话22-21 k6+Locust 混合压测第二轮 + traefik 502 坑㉕ + 三根因闭合** | `docs/progress/archive/session22-21-loadtest-traefik502-launch.md` | `esp:ctx:hot:2026-08-14-session22-21-loadtest-launch` |
| **会话22-13 colima-stop DNS 雪崩** | `docs/progress/archive/session22-13-colima-dns.md` | — |
| **会话22-12 CrashLoop 诊断 + 双轨落盘** | `docs/progress/archive/session22-12-crashloop-diagnosis.md` | — |
| **会话22-11 图+向量库 + 冒烟7/8 + Neo4j坑 + 实测校正** | `docs/progress/archive/session22-11-graph-vector-smoke.md` | — |
| **会话22-10 Obsidian同步 + Redis warm-up** | `docs/progress/archive/session22-10-obsidian-redis-warmup.md` | — |
| **会话22-9 平台底座化 + redis坑㉓闭合** | `docs/progress/archive/session22-9-platform-infra-redis-023.md` | `esp:ctx:hot:2026-08-13-session22-9-platform-infra` |
| **会话22-3 ~ 22-8(08-09~0811)** | `docs/progress/archive/2026-08-09-to-0811-session22-3-to-22-8.md` | — |
| **会话18~22 冷区批次(08-07~09)** | `docs/progress/archive/2026-08-09-cold-archive-session18-22.md` | `esp:ctx:hot:2026-08-09-session22-abcd-batch-ab` |
| teacher readiness 根治 + 安全扫描四件套(08-07) | (见冷区归档) | `esp:ctx:hot:2026-08-07-teacher-readiness-and-security-scan` |
| teacher CrashLoop + 增量服务(08-07) | (见冷区归档) | `esp:ctx:hot:2026-08-07-teacher-crashloop-and-new-services` |
| Pulsar cookie + VM 网络中断(08-06) | `docs/progress/archive/2026-08-06-pulsar-cookie-fix-and-vm-down.md` | `esp:ctx:archive:2026-08-06-pulsar-cookie-fix-and-vm-down` |
| Pulsar Geo + 应用切新集群(08-05) | `docs/progress/archive/2026-08-05-pulsar-geo-and-cluster-switch.md` | `esp:ctx:archive:2026-08-05-pulsar-geo-and-cluster-switch` |
| 上线就绪 + 三层坑事故(08-04) | `docs/progress/archive/2026-08-04-readiness-and-three-layer-incident.md` | `esp:ctx:archive:2026-08-04-readiness-and-three-layer-incident` |
| 07-31~08-03 历史 | `docs/progress/archive/2026-08-{01,02,03}-*.md` | `esp:ctx:archive:2026-08-{01,02,03}-*` |

## 关键命令速查

- 查 TiDB 表:`scripts/db_query.sh "SQL"`(pymysql pod 封装)。
- 服务日志:`kubectl -n esp-prod logs deploy/<svc> --tail=20`。
- Redis 写/读:`kubectl -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c SET/GET "esp:ctx:*" "..."`。
- 压测:分布式 locust 见 `deploy/locust/README.md`。
- 单节点铁律:重 mvn/docker build 与集群操作串行;不碰 k3s 配置。

## 上下文工程铁律(摘要,详见 memory `context-overflow-ironrules`)

- 文件系统=长期记忆,对话=临时工作区。80%(160k)主动 /compact,90%(180k)强制止损落盘+compact。
- 三层防御:源头减量(subagent 隔离+输出限流)+ 状态外化(PROGRESS 热区+Redis 运行态)+ 主动管控。
- **本文件热区上限 ~300 行**,历史归 `docs/progress/archive/`,不全量读。完整铁律集见 memory。
