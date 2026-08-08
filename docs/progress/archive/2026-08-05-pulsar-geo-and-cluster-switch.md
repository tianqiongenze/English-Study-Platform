## Pulsar Geo 部署修复 + 应用切新集群(2026-08-05,恢复锚点)

> 上下文溢出后新会话**先读本段** + Redis `esp:ctx:session:recovery-2026-08-05` + TaskList(#1~#4),三处对齐续跑。
> **用户诉求**:① Pulsar MQ Geo 分区域部署完成后,把项目下所有用 Pulsar MQ 和 Redis Cluster 的服务/应用改到新集群;② 所有资料据新集群改到位;③ Redis 丢失会话/任务状态,以 PROGRESS.md + 指令恢复;④ 自动更新 PROGRESS.md + 存 Redis 双轨记忆。

### 任务清单(TaskList)
- #1 [已完成] 修复 Pulsar Geo 9-pod 部署 —— broker 1/1 Ready + geo-config Job Completed(根因 I 闭合;bk-2 cookie 遗留非阻塞)
- #2 [进行中] 应用切新 Pulsar + Redis Cluster 集群 —— 进度 70%(基础设施+源码改动已落盘,缺镜像重建+运行时验证)
- #3 [待办] 资料同步:所有文档/配置对齐新集群
- #4 [待办] PROGRESS.md 未决项恢复:灌装 + 07 文档补强
- #5 [进行中] 构建 harness 强制钩子(上下文铁律) —— context_guard.py 已建,待注册 settings.json + PreToolUse 守卫

### #1 完成总结(2026-08-05,broker Ready + geo-config Completed)

**最终状态**:pulsar-broker-0 1/1 Running restart 0,health endpoint HTTP 200;geo-config Job Complete 1/1(cluster/tenant esp/namespace esp/business+esp/cache 建好,replication clusters=aws,topics list 成功);3 bk 中 bk-0/bk-1 writable 注册(bk-2 cookie 遗留 CrashLoop,非阻塞)。

**真正根因 I(ZK 退化为 standalone,比预设根因 H 更深)**:`bin/pulsar zookeeper` 入口不像 broker/bookie 那样自动跑 `apply-config-from-env.py`,所以 `PULSAR_PREFIX_server.N` env 从未写入 `zookeeper.conf` → QuorumPeerMain 检测到 "no quorum defined" → 退化为 standalone(日志 `Either no config or no quorum defined in config, running in standalone mode`)→ ZK 抖动 + 无 leader election → broker BK 客户端 bookie-list watch 不稳 → `allBookies []` 间歇性空 → 创建 ledger 失败 → healthcheck 失败。根因 H(rack-aware + 探针)只是表象。

**改的 manifest(`deploy/k8s/06d-pulsar-geo.yaml`,均为可逆 Edit+apply)**:
1. **ZK sts**:command 改 `sh -c` 显式跑 `apply-config-from-env.py` + 写 `myid`;新增 `POD_NAME` fieldRef + `PULSAR_PREFIX_server.1/2/3` + `initLimit/syncLimit` env。
2. **Broker sts env**:新增 `PULSAR_PREFIX_bookkeeperMetadataServiceUri=zk+null://.../ledgers`(分号分隔);`bookkeeperClientRackawarePolicyEnabled=false`;`bookkeeperClientMinNumRacksPerWriteQuorum=1`;`enforceMinNumRacksPerWriteQuorum=false`;`managedLedgerDefaultEnsembleSize/WriteQuorum/AckQuorum` 2→1(单节点 2 副本无意义);`PULSAR_MEM -Xms384m→256m`;memory limit 512Mi→640Mi(原 OOMKilled)。
3. **Broker 探针**:readiness initialDelay 30→120、failureThreshold 12→24;新增 liveness(同参数)。
4. **geo-config Job**:`tenants create --cluster`→`-c`;`namespaces set-replication-clusters`→`set-clusters -c`;`get-replication-clusters`→`get-clusters`;新增 `clusters create aws`。

**healthcheck 成功关键日志**:`Update BookieInfoCache (writable bookie) pulsar-bk-0/1.pulsar-bk:3181`、`Created ledger 4`、`Successfully initialize managed ledger`、`Created topic .../healthcheck`。allBookies 已填 2 个 writable(bk-0/bk-1)。

**遗留(非阻塞)**:
- **bk-2 cookie instanceId 不一致**:本地 cookie 记旧 instanceId `f97edb1d...`,ZK 当前 `2311bbae...`,`allowStorageExpansion=true` 不覆盖已有 cookie 的 instanceId 校验。修法:删 bk-2 PVC 重建,或删本地 `VERSION` 文件。单副本 ensemble 已够用,暂不影响。后续要 3 bk 全 writable 再处理。
- 删了 stale ledger 0(原 IN_RECOVERY 卡住致 broker "Ledger not found: 0" 死循环),broker 自动建新 ledger 4。

**根因全集(A~I)**:A broker clusterName null / B bk 无 bookkeeper 子命令 / C bk NoNode /ledgers/LAYOUT / D broker $(POD_NAME) 未展开 / E 本地双 PVC cookie 不一致 / F broker healthcheck 级联(已修 E 后自愈) / G ZK 侧 cookie 残留(已修) / H rack-aware+探针(表象) / **I ZK 退化为 standalone(真正根因,已修)**。

### #1 进展(2026-08-05 bk-2 修复轮)

**当前状态(子代理 a998ef91 返回)**:
- ✅ **bk-2 已修复(根因 G 闭合)**:ZK cookie 删除成功(`bookkeeper shell cookie_delete pulsar-bk-2.pulsar-bk:3181`,日志 `Removed cookie from /ledgers/cookies`);bk-2 重建 1/1 Running;**3 个 bk 全 writable**(`listbookies -rw` 确认 pulsar-bk-0/1/2.pulsar-bk:3181);清掉残留 ledger 0(IN_RECOVERY 卡住,`deleteledger -l 0 -f`)。
- ❌ **broker 仍 CrashLoop(新根因 H,非 cookie 问题)**:broker-0 重建后短暂到过 1/1,随后 readiness 死循环。sts 已自动降副本到 1(broker-0 0/1,restart 2)。
- pulsar-geo-config Job:3 个历史 pod 全 Error,等 broker 稳定。

**根因 H(broker readiness 死循环,BK 客户端 placement policy 启动竞态)**:broker 的 BK 客户端启动时 `allBookies []` 为空(bookie-list watch 未及时填充)→ 无法创建 ledger(ensembleSize=2 找不到 bookie)→ healthcheck 失败 → readiness 探针(30s initialDelay + 12×10s=150s)杀 → 重启重置缓存 → 死循环。配置:`bookkeeperClientRackawarePolicyEnabled=true` + `bookkeeperClientMinNumRacksPerWriteQuorum=2` + `enforceMinNumRacksPerWriteQuorum=false`。单节点同 rack(`/default-rack`)下虽不强制,但 placement policy 初始化慢 + 探针过激进 + 启动竞态,组合致死循环。**bk 侧 3 个 writable,但 broker BK 客户端因被反复杀来不及填 bookie cache。**
- **修法(下一步执行)**:① 放宽 readiness 探针:initialDelay 30→120s、failureThreshold 12→24(给 BK 客户端 bookie cache 填充时间);② 关 rack-aware policy:`PULSAR_PREFIX_bookkeeperClientRackawarePolicyEnabled=false`(单节点同 rack 无意义);③ broker scale 0→3 全量重拉(避开单 pod 反复重启缓存死锁);④ 稳定后重跑 geo-config Job。
- **判断**:改 sts env/探针属可逆低风险运维(非不可逆),按用户"全程零确认"授权直接推进。

**根因 G(bk-2 `verifyDirsForStorageExpansion` 失败,ZK cookie 残留)**:bk-2 报错 `Either not all local directories have cookies or directories being added newly are not empty. Directories missing cookie file are: [data/bookkeeper/ledgers/current]`。子代理核查确认:**bk-2 的 PVC(bk-data-pulsar-bk-2/bk-journal-pulsar-bk-2)是本次重建的新空 PVC(6m30s),非旧数据残留**;真正问题是 ZK 侧 `/ledgers/cookies/` 残留 bk-2 旧 cookie 节点。**已修(cookie_delete + 删 pod 重建)**。
- **与根因 E 区别**:E 是本地双 PVC cookie 不一致(清 PVC VERSION 文件);G 是 ZK 侧 cookie 残留(清 ZK `/ledgers/cookies` 节点)。两者不同层。

**新 Redis 现状(本轮核查订正)**:`redis-cluster-new` 是 **StatefulSet**(6/6 Running 8h,无密码,Cluster 模式 3主3从),非 Deployment —— 故 `exec deploy/redis-cluster-new` 超时,改用 `exec redis-cluster-new-0`。旧 `deploy/redis-cluster` 已不存在。`esp:ctx:session:recovery-2026-08-05` 曾为空串,已补实(STRLEN=3288)。

### 上下文防溢出铁律(2026-08-05 固化,所有项目强制)
用户铁律:上下文达 10%(20k)起每里程碑双轨落盘;80%(160k)主动 /compact;90%(180k)强制止损落盘+compact/clear。三层防御 21 条(源头减量+状态外化+主动管控+subagent 纪律+ESP 特化)。三处落盘:① memory `context-overflow-ironrules.md`;② 全局 `~/.claude/CLAUDE.md` 第 6 条;③ Redis `esp:ctx:ironrules:context-overflow`(STRLEN=3001)。详见 memory。

### #1 Pulsar Geo 9-pod 修复(根因三连 + 解决方案)

**集群现状(2026-08-05,ekb-m2-aws-us-east-1)**:
- pulsar-zk-0/1/2 1/1 Running(ZK OOMKilled 已恢复,resources requests 提到 384Mi)
- pulsar-broker-0 / pulsar-bk-0/1/2 CrashLoopBackOff(修复中)
- pulsar-geo-config Job Error(等 broker ready)
- redis-cluster-new-0..5 6 pod 1/1 Running,cluster_state ok,3主3从,**无密码**,可读写
- TiDB(pd/tikv/tidb)Running;esp-prod 业务 deploy 多数 scaled 0,仅 writing-py/nacos 1/1

**根因 A(broker `Required clusterName is null`)**:manifest 给 sts 注入 env `clusterName=aws`,但 `bin/pulsar broker` 启动器**不自动调用 `apply-config-from-env.py`**,env 不会写入 `conf/broker.conf`,conf 中 `clusterName=` 仍空 → `PulsarConfigurationLoader.isComplete` 报 null。
- **修法**:broker command 改 `sh -c 'bin/apply-config-from-env.py --prefix PULSAR_PREFIX_ conf/broker.conf && exec bin/pulsar broker'`,env 全部加 `PULSAR_PREFIX_` 前缀(官方 Helm chart 同款机制)。

**根因 B(bk `Invalid command 'bookkeeper'`)**:`bin/pulsar` 3.3.0 无 bookkeeper 子命令。manifest 误用 `["bin/pulsar","bookkeeper","bookie"]`。
- **修法**:bk command 改 `["bin/bookkeeper","bookie"]`(同上先跑 apply-config-from-env.py),env 用 `PULSAR_PREFIX_` 前缀。

**根因 C(bk `NoNode for /ledgers/LAYOUT`)**:bookkeeper 首次启动需先 `metaformat` 初始化 ZK 元数据,否则 bookie 起不来。
- **修法**:bk command 加 `bin/bookkeeper shell metaformat -f || true` 再 exec bookie。

**根因 D(broker `Invalid named entity: $(POD_NAME).pulsar-broker`)**:K8s `$(VAR)` 只能引用**已定义**的 env;原 manifest `POD_NAME` 定义在 `advertisedAddress` 之后 → 未展开成字面量 `$(POD_NAME)` 传进 broker → NamedEntity 校验失败。
- **修法**:`POD_NAME` env 移到 `PULSAR_PREFIX_advertisedAddress` 之前(broker/bk 两处都改)。

**根因 E(bk `InvalidCookieException: instanceId not matching` / `Not all the new directories are empty` / `verifyDirsForStorageExpansion`)**:历史部署期 bk command 里曾带 `metaformat`,每个 bookie 各自 metaformat 各自生成 instanceId 并覆盖 ZK `/ledgers/INSTANCEID`,导致各 bookie 本地 cookie(VERSION 文件,存在 `journal/current/VERSION` 和 `ledgers/current/VERSION` 两处)记录的 instanceId 互不一致。即便开 `allowStorageExpansion=true`,BK 仍要求"两处 cookie 要么都有要么都没有",只清一处会报 `directories missing cookie / new directories not empty`。
- **修法(已落地,彻底版)**:① bk command 注释明确"不要在 bookie 启动时跑 metaformat",metaformat 仅由 `pulsar-bk-init` Job 单次完成;② **彻底重建 bk 数据**:`scale sts/pulsar-bk --replicas=0` → 删 bk-1/bk-2 的 bk-data+bk-journal 两套 PVC(空 PVC 重建)→ 用临时 pod 挂 bk-0 两套 PVC `find ... -name VERSION -delete` 清 bk-0 cookie → `scale --replicas=3` 重建。3 个 bk 全用空 PVC + `allowStorageExpansion=true`,启动后用当前 ZK instanceId 写新 cookie,3 个全 writable 注册。③ BK 4.17 conf key 是 `allowStorageExpansion`(非 `bookkeeperStorageExpansionEnabled`)。

**根因 F(broker healthcheck `Error while recovering ledger error code: -10` / `Not enough non-faulty bookies`)**:bk 未全部注册为 writable 时 broker 开 healthcheck 创建 `persistent://pulsar/aws/.../healthcheck` topic 失败(managedLedger 写 ledger 找不到足够 writable bookie)→ 探针 500 → liveness 杀 broker 循环重启。是 bk 注册问题的级联,根因 E 修后(3 个 bk 全 writable 注册)自愈。managedLedger 默认 ensemble=2/writeQuorum=2/ackQuorum=2,需 ≥2 个 writable bookie。

**已改文件**:`deploy/k8s/06d-pulsar-geo.yaml`(zk resources 提到 384Mi/768Mi;bk command+env 重写 + POD_NAME 前置 + `PULSAR_PREFIX_allowStorageExpansion=true`;broker command+env 重写,POD_NAME 前置)。已 `kubectl apply` + `rollout restart sts/pulsar-bk`,清 3 个 bk 的双 PVC cookie 后验证 bk 全 writable 注册 + broker healthcheck 200 中。

### #2/#3/#4/#5/#6 进展(2026-08-05 应用切新集群 + 启动 + SB4 虚拟线程/GraalVM)

**探查结论(子代理 a2cf1652,只读)**:
- ExternalName `redis-cluster`(`deploy/k8s/05-infra-reuse.yaml:24-32`)当前指旧主从 `redis-cluster.ekb-m2-aws-us-east-1`,需改指 `redis-cluster-new`。
- 各服务 Redis 当前**单 host 主从模式**,需改 **cluster 模式**:Java 三服务 yml `spring.data.redis.host`→`spring.data.redis.cluster.nodes`;Python `esp-cache-py/esp_cache/__init__.py:47` 改 `RedisCluster`;Go `gateway-go/src/cache/cache.go:113`+`main.go:60` 改 `redis.NewClusterClient`。
- Pulsar:各服务默认 `pulsar-broker:6650`,ExternalName 已指新集群,**零改**。
- **读写分离独立确认**:TiDB `tidb-primary` 写 / `tidb-follower` 读(`05-infra-reuse.yaml:9-22` + 各 deploy WRITE/READ_DB_* env),与 Redis/Pulsar 完全解耦,切集群不动。Redis Cluster 读写都走 slot 主(无读写分离),Pulsar 无读写分离。
- esp-prod scaled 0:bff/emotion/frontend/gateway/learning/seata/sentinel/teacher;在跑:writing-py 1/1、nacos 1/1。

**#2+#5 执行中(子代理 aa9841d8,零确认)**:ExternalName 重指向 + Java/Py/Go Redis 改 cluster 模式 + 串行 build 镜像 + 按依赖顺序启动(基础设施→seata→sentinel→bff→learning→teacher→gateway→emotion→writing→frontend,每起一个验证 Ready+连通性再起下一个)。Go 侧已遇编译坑:`session.NewManager` 参数 `*redis.Client` 不兼容 `*ClusterClient`,已改 `redis.UniversalClient`(兼容两者),go build 验证中。Python esp-cache-py/Java yml 改造进行中。

**#6 待办(用户指令,等 #2+#5 完成后接续,避免并发改 Java 冲突)**:Spring Boot 4 三服务(bff/learning/teacher)启用:
- ① **虚拟线程**:`spring.threads.virtual.enabled=true`(SB4 全局虚拟线程,I/O 密集收益,低风险,先做)。
- ② **GraalVM Native Image**(用户选"都开",自负风险):改 Dockerfile 用 GraalVM 镜像 + native-maven-plugin;加 AOT hints(Dubbo Triple/Seata/Hibernate/Jackson/Pulsar/Caffeine/SCA Nacos/Sentinel)。风险:构建慢~30min/服务 + 反射配置不全 + 单节点 VM 内存紧张;兜底 Native 失败回退 JVM(虚拟线程保留)。单节点铁律:一次只构建一个服务,与集群操作串行。

### 待办顺序
- 等 aa9841d8 完成 #2+#5(切集群 + 按依赖启动 + 端到端验证)
- #3 资料同步(docs/design、deploy/k8s manifest、CLAUDE.md、memory 旧集群地址对齐新集群)
- #4 esp-seed-3b 灌装恢复(2.42亿/30亿,suspend)+ 07 人工测试文档补强(21行→P0)
- #6 SB4 虚拟线程 + GraalVM



### 关键命令速查(恢复用)
- 查 Pulsar pod:`kubectl --context=rancher-desktop -n ekb-m2-aws-us-east-1 get pods | grep pulsar`
- broker 日志:`kubectl --context=rancher-desktop -n ekb-m2-aws-us-east-1 logs pulsar-broker-0 --tail=30`
- 新 Redis 写/读:`kubectl -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c SET/GET "esp:ctx:session:recovery-2026-08-05" "..."`
- 注:旧 `deploy/redis-cluster` 已不存在;新集群名 `redis-cluster-new`(6 pod,无密码)。
- 单节点铁律:重 mvn 构建与集群操作串行;不碰 k3s 配置。

---


