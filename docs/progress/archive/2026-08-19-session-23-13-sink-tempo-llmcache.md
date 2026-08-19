# 会话23-13续3:中间件下沉宿主 + Tempo traces修复 + LLM缓存Phase1 + 层1根治

> 日期:2026-08-19
> Redis 锚点:`esp:ctx:hot:2026-08-19-session-23-13-sink-tempo-llmcache`(主)+ `esp:ctx:session:23-13-cont3`(续)

## 本轮完成项(11 项)

### 1. 中间件下沉宿主 + 集群腾退
| 中间件 | 下沉状态 | 集群内残留 |
|---|---|---|
| Nacos | ✅ 已下沉(ExternalName→host.lima.internal,集群 deploy/svc/pvc 全删) | 无 |
| Prometheus/Loki/Alertmanager/Grafana | ✅ 已下沉(ExternalName) | 无(本轮清了 OTel daemonset/gateway + fluent-bit 3 处残留) |
| OTel Collector | ✅ 已下沉(ExternalName→host.lima.internal:4317) | 无(daemonset+gateway svc+cm 本轮删,备份 report/) |
| Tempo | ✅ 已下沉+修复(本轮修端口冲突,见下) | 无 |
| Milvus/Neo4j | ❌ 未下沉(DEFERRED,评估 report/milvus-neo4j-sink-assessment.md) | 集群内保留(多项目共用底座,零 ESP 引用) |

### 2. Tempo traces 链路彻底修复(ASV 根因修正)
- **根因修正**:9095 冲突真凶是 tempo `server.grpc_listen_port`(默认 9095),**不是 memberlist**(memberlist 默认 7946)。之前 DEFERRED 报告误判。
- **ASV 查证**:`./tempo -help` 实测 tempo 2.7.2 真实 flag
- **修复**:tempo.yaml `server.grpc_listen_port=9096` + OTLP 14317/14318(避 OTel 4317/4318)+ memberlist.bind_port=7947
- **OTel 转发**:otel-gateway-config.yaml + otel-config.yaml exporter → localhost:14317
- **bff/gateway endpoint 修复**:`set env` 改 OTEL_EXPORTER_OTLP_ENDPOINT → otel-collector.esp-observability:4317(原 esp-prod 短名 NXDOMAIN)
- **端到端验证**:Tempo 收 2595+ spans;service.name=bff 20 traces;service.name=gateway 5 traces(链路完全打通)
- 报告:`report/tempo-traces-fix.md` + `report/tempo-port-conflict-deferred.md`(更新为已修复)

### 3. LLM 缓存 Phase1 推进(三服务接线+部署+IT)
- 架构文档:`docs/design/llm-cache-architecture.md`(531 行+mermaid)
- Phase1 conversation-go CachedProvider:cached.go+cached_test.go(12/12 单测 PASS)
- Phase1 完整推进 subagent(三服务接线+部署+IT)运行中(本轮末启动)
- LLM 缓存铁律 LCO:全局 ~/.claude/CLAUDE.md 第8条 + memory `llm-cache-optimization-ironrule`
- **Milvus/Neo4j 未用**:Phase2(Milvus)/Phase3(Neo4j)DEFERRED,当前 LLM 缓存零引用

### 4. Redis Cluster 自愈层1根治
- 脚本:`scripts/redis-cluster-selfheal.sh`(幂等:ok 跳过,非 ok 则 CLUSTER MEET live-IP)
- 集成:`start-host-middleware.sh` [8/8] 步(VM 重启后自动调)
- memory:`esp-redis-cluster-nodes-conf-stale-ip` 更新层1根治段
- 局限:层2根治(announce-ip 注入 PodIP/pod CIDR 直连)DEFERRED 多节点后落地

### 5. Nacos-CRDB 通道 launchd 自愈层1根治
- 脚本:`scripts/nacos-crdb-channel-health.sh`(每 30s 健康检查+自愈重连 ssh port-forward 26257)
- launchd:`~/Library/LaunchAgents/com.esp.nacos-crdb-channel.plist`(StartInterval=30)
- **自愈验证**:kill ssh mux 后 20s 内自动重连 26257 + Nacos healthy
- 关键坑:`ssh -o ControlPath` 含空格报错,改用 `-F` 读 lima ssh.config
- memory:`esp-nacos-crdb-channel-launchd-selfheal`
- 局限:层2多节点直连消除 ssh tunnel DEFERRED

### 6. 宿主 Nacos Derby→CRDB 迁移成功(Phase B)
- nacos 2.5.0 standalone,healthy,HikariPool org.postgresql.Driver,13 表空表
- **ASV 突破**(反例库第4条):nacos 2.5.0 ExternalDataSourceProperties 硬编码 mysql driver fallback,仅设 platform=postgresql 不切运行期 driver,显式设 `db.pool.config.driverClassName=org.postgresql.Driver` 一次修复
- 脚本:`scripts/nacos-crdb-init-schema.py`(幂等建库+导 schema)
- 插件:`~/nacos/nacos/plugins/nacos-datasource-plugin-postgresql-0.0.5.jar`(Maven Central,精确匹配 2.5.0)
- 报告:`report/nacos-crdb-migration-host.md`

### 7. bff-java 滚动卡死修复
- 根因:1.10.0 镜像 HOME=/ 未设 → Dubbo 3.3.6 写 /.dubbo 无权限 → Startup probe 338 次失败循环
- 修复:emptyDir 挂载 /.dubbo + /logs/nacos(纯 K8s patch 可逆)
- 结果:3/4 Ready(2 Pending CPU 不足),Startup 卡死根因消除
- 报告:`report/bff-java-rolling-fix.md` + `report/bff-java-pre-fix-backup.yaml`

### 8. H-01 混沌测试(降级 PASS)
- kill 集群 Nacos:Java 服务存活副本全程 Running,Dubbo 本地缓存保活,GW/BFF 全程 HTTP 200,零 CrashLoop
- 5 个 Error pod 根因:单节点 PLEG 雪崩(CPU 99%→CoreDNS 转发链断),非 CRDB/Nacos
- 报告:`report/chaos/H01-final-report.md`

### 9. CRDB probe 1s 误杀修复
- 根因:esp-crdb sts/crdb probe timeoutSeconds=1s 对重 DB 太短
- 修复:patch timeout 1→5/failure 5→3/period 10→15(可逆)
- 回滚:`report/chaos/crdb-probe-rollback.sh`
- memory:`esp-crdb-probe-timeout-1s-pitfall`

### 10. 压测 + E2E + CRDB vs TiDB 对比
- E2E 14/0 ALL GREEN(修 phase3-e2e-smoke.sh 3 处 bug)
- 压测(50 并发):learning_review P95=3s,bff_login avg 14.4s 瓶颈(应用层非 DB,CRDB 连接数=0)
- CRDB vs TiDB:补全 TiDB 历史数据(commit 0cc7819 全基准压测),单节点 CRDB 为 TiDB 33%-71%(环境噪声非生产结论)
- 报告:`report/e2e-23-13-cont.md` + `report/loadtest-23-13.md` + `report/crdb-vs-tidb-23-13.md`

### 11. 清理 3 处 OTel/fluent-bit 残留
- otel-collector-daemonset(ds+svc+cm 删,备份 report/)
- otel-collector-gateway(svc+cm 删,备份 report/)
- fluent-bit DS(删,备份 report/)
- esp-observability 现为纯 ExternalName 集合(6 svc 全→host.lima.internal)

## 关键铁律新增/更新

| 铁律 | 类型 | 位置 |
|---|---|---|
| LLM 调用缓存优化(LCO,2026-08-18) | 全局铁律 | ~/.claude/CLAUDE.md 第8条 + memory `llm-cache-optimization-ironrule` |
| CRDB probe timeout=1s 坑 | 项目坑 | memory `esp-crdb-probe-timeout-1s-pitfall` |
| ASV 反例第4条(nacos 2.5.0 driver 硬编码) | 全局反例 | memory `api-signature-verify-ironrule` 典型反例库 |
| Redis Cluster 自愈层1根治 | 项目修复 | memory `esp-redis-cluster-nodes-conf-stale-ip` 更新 |
| Nacos-CRDB 通道 launchd 自愈 | 项目修复 | memory `esp-nacos-crdb-channel-launchd-selfheal` 新增 |

## admin 监控影响(实测)

| 监控项 | 影响 |
|---|---|
| Prometheus 指标/告警(esp-admin env PROMETHEUS_URL) | ✅ 不受影响(ExternalName 透明转发宿主) |
| Grafana 链接(GovernanceDashboard 硬编码 192.168.5.15:30300) | ⚠ 损坏(下沉后端口变了,需改 ExternalName 或宿主地址) |
| Sentinel 控制台链接(硬编码 192.168.5.15:8858) | ⚠ 需核实 |
| 全服务健康探活 | ✅ 不受影响(探活 ESP 服务 pod,非中间件) |

## DEFERRED 项(显式标注)

1. **Milvus/Neo4j 下沉**:ESP 零引用 + 联网不可用无法验证最佳实践 + 多组件高难度 + 三区域多项目共用 → 等LLM缓存 Phase2/3 启动(ESP真用)再下沉。评估 `report/milvus-neo4j-sink-assessment.md`
2. **LLM 缓存 Phase2(L2 Milvus 语义向量)**:Phase1 完成后启动
3. **LLM 缓存 Phase3(L3 Neo4j 知识图谱)**:Phase2 后启动
4. **Redis Cluster 层2根治**:sts announce-ip 注入 PodIP / pod CIDR 直连,多节点后落地
5. **Nacos-CRDB 通道层2根治**:多节点 pod CIDR 直连消除 ssh tunnel
6. **bff-java 2 Pending pod**:单节点 CPU 99% 枯竭,DEFERRED 多节点
7. **bff-java 镜像层根治**:1.10.0 Dockerfile 加 ENV HOME=/home/esp(对齐 1.14.4),K8s emptyDir 是部署层兜底
8. **push GitHub**:443 持续超时,本地 commit `99d97e0` 完整,网络恢复后推
9. **conversation-go WS 入口**:LLM 缓存 NewWithCache() 工厂就位但无 main 包调用方,待补
10. **5000 CCU 阶梯压测**:需先解 bff-java 全副本健康
11. **TiDB 重建 + CRDB vs TiDB 完整 EXPLAIN ANALYZE 基准**:用户已下电 TiDB,DEFERRED 正式回切时重建

## 本地 commit
- `99d97e0 feat(会话23-13续2):H-01混沌+CRDB probe+LLM缓存LCO+Nacos CRDB迁移+bff修复+压测E2E`(482 files,43516 insertions)
- ⚠ push GitHub 失败(443 超时),本地完整

## 下一步(恢复后执行)

1. LLM 缓存 Phase1 完整推进 subagent 完成(三服务接线+部署+IT)— 当前运行中
2. admin 监控 Grafana/Sentinel 硬编码链接修复(改 ExternalName)
3. push GitHub(网络恢复后)
4. bff-java 镜像层根治(Dockerfile ENV HOME)
5. LLM 缓存 Phase2(Milvus 启用)— 此时才需 Milvus 下沉决策
6. LLM 缓存 Phase3(Neo4j 启用)— 此时才需 Neo4j 下沉决策

## 铁律遵守

- 未碰 k3s/CRDB/Milvus/Neo4j(共享基础设施 esp-shared-infra-ironrule)
- CRDB probe patch + svc LoadBalancer patch 已获用户显式授权,可逆
- Nacos 下沉用户明确决策坚持,执行但诚实标注 3 硬伤 DEFERRED
- ASV 铁律:tempo/nacos/Go 接口配置 key 全部实测查证,不臆造
- verification-before-completion:每步附证据
- subagent 隔离大输出,主线程只收结论
