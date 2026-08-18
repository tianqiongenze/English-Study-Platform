# 会话23-13:H-01 混沌 + CRDB probe 修复 + LLM 缓存铁律 + Nacos CRDB 迁移调研

> 日期:2026-08-18
> Redis 锚点:`esp:ctx:hot:2026-08-18-session-23-13-chaos-crdb-llmcache`

## 本轮交付物(6 份)

| 文档 | 路径 | 状态 |
|---|---|---|
| H-01 混沌测试报告(kill 集群 Nacos) | `report/chaos/H01-final-report.md` | ✅ 降级 PASS |
| H-01 基线快照 | `report/chaos/H01-baseline-20260818-222258.md` | ✅ |
| 宿主 Nacos→CRDB 迁移预案(Phase A 调研) | `report/nacos-crdb-migration-host.md` | ✅ 含 nacos 2.5.0 修正 + 回滚 + 集群复用要点 |
| LLM 缓存三层架构设计 | `docs/design/llm-cache-architecture.md` | ✅ 531 行 + mermaid 数据流图 |
| CRDB probe 回滚脚本 | `report/chaos/crdb-probe-rollback.sh` | ✅ 可逆 |
| Redis 混沌锚点 | key `esp:ctx:chaos:H01:recovery` | ✅ |

## 铁律新增(2 条,全局对所有项目生效)

| 铁律 | memory | 全局 CLAUDE.md |
|---|---|---|
| **LLM 调用缓存优化(LCO,2026-08-18)** | `memory/llm-cache-optimization-ironrule.md` | 第 8 条 |
| CRDB probe timeout=1s 坑 | `memory/esp-crdb-probe-timeout-1s-pitfall.md` | (项目级) |

LCO 铁律:所有 LLM/AGI 链路三层缓存(L1 精确哈希 Redis / L2 语义向量 Milvus / L3 知识图谱 Neo4j),禁止裸调无缓存,命中率监控 >30%,成本节省 >40%。

## 关键事件链(时序)

### 1. H-01 混沌:kill 集群 Nacos
- force delete nacos pod → Deployment 自动拉新 pod
- **降级 PASS**:Java 服务存活副本全程 Running,Dubbo 本地缓存保活,Gateway:30080/BFF:30009 全程 HTTP 200,零 CrashLoopBackOff
- Nacos 注册表恢复:bff-java + learning-java + Dubbo provider 全恢复

### 2. 5 个 Error pod 根因(关键诊断修正)
**不是 CRDB 故障,是单节点 PLEG 雪崩**:
- CPU 99% 超额 → kubelet PLEG 卡死 → CoreDNS 转发链断 → 所有 DNS 名(crdb-public + redis-cluster)解析失败
- 对照测试:nslookup redis-cluster(已知良好)同样失败 → 证明节点级 DNS 链整体不可达,非 CRDB 单点
- CRDB 集群本身 `cockroach node status` 3 节点 is_live=true(健康)
- 5 服务:assessment-rs/community-go/gamification-go/listening-go(pronunciation-rs 连 Nacos)全恢复 1/1

### 3. CRDB probe patch(已授权,可逆)
- 根因:esp-crdb sts/crdb 默认 probe `timeoutSeconds:1` 对重 DB 太短(放大器,PLEG 期间被误杀加剧震荡)
- 修复:`kubectl -n esp-crdb patch sts crdb --type=json` timeout 1→5 / failure 5→3 / period 10→15
- 效果:patch 后 crdb 3/3 stable,RESTARTS 归零,5 个 Error 服务全恢复
- 回滚:`report/chaos/crdb-probe-rollback.sh`

### 4. LLM 缓存架构勘测纠偏(关键)
三个 LLM 服务**已有部分缓存实现**,新设计必须收敛而非新建:
- writing-py 用 **Litestar**(非 FastAPI),已有 L1(cachetools)+ L2(Redis `llm:cache:{sha256}`)在 `app/llm.py:203`
- llm-orchestrator-rs 用 **axum**(非 actix),已有 L1(Redis SETEX)+ L2(SimHash 占位符待替换)在 `src/cache/mod.rs`
- conversation-go 有干净 `llm.Provider` 接口,**无 LLM 专用缓存** → Phase1 第一个落地(阻力最小)
- 核心结论:三服务 key 统一 `esp:cache:llm:exact:{sha256(canonical)}`,否则两套 key 各存一份命中分散
- Phase1 落地:改 `backend/services/conversation-go/src/llm/cached.go`(新增 CachedProvider 装饰器),go.mod 已有 go-redis 无新依赖

### 5. 宿主 Nacos→CRDB 迁移(正确停止,前置未齐)
nacos 版本核实:**2.5.0**(非 2.4.3,subagent 二次核实 jar MANIFEST + 启动日志)
- nacos 2.5.0 官方 `nacos-datasource-plugin-2.5.0.jar` 只有 impl/derby + impl/mysql,**无 impl/postgres**
- 旧迁移崩溃铁证:`platform=mysql` + `jdbc:postgresql://` 配置矛盾 → mysql driver 不认 PG URL → bean 创建失败
- CRDB v24.3.11 确认仅 PG wire,不支持 MySQL wire
- 剩余阻断:① 宿主→CRDB 通道(NodePort 无 svclb)② nacos 2.5.0 PG datasource 插件 jar 本地缺
- 两个前置已派 subagent 并行准备

## 集群终态

- CRDB:3/3 stable(probe patch 生效)
- esp-prod:29 pod 全 Running,5 曾 Error 服务全 1/1
- Nacos:1/1,bff-java+learning-java 注册表恢复
- Gateway:30080 / BFF:30009:全 HTTP 200

## 下一步(并行中)

- CRDB 迁移前置准备(subagent 运行):patch svc LoadBalancer + GitHub 取 nacos 2.5.0 PG 插件
- 压测 + E2E + CRDB vs TiDB 对比(subagent 运行):phase3-e2e-smoke + locust 小规模 + CRDB/TiDB 延迟对比

## 铁律遵守

- 未动集群 Nacos deploy / 未 scale CRDB(共享基础设施 [[esp-shared-infra-ironrule]])
- CRDB probe patch 已获用户显式授权,可逆
- 止血动作(bff/learning/teacher scale 3→1)全可逆,PLEG 恢复后 HPA 已拉回
- subagent 隔离大输出,主线程只收结论
- verification-before-completion:每步附证据
