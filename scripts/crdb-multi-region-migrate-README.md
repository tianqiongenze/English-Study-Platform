# CRDB 多区域演进 — 迁移 README(Task #161)

> 更新日期:2026-08-16。配套脚本:`scripts/crdb-multi-region-migrate.sql`。
> 方案对比全文:`docs/design/cockroachdb-vs-tidb-multi-region-plan.md`(推荐 TiDB 深化多区域,CRDB 仅条件触发 PoC)。
> 本文档只回答一个工程问题:**当 CRDB 多区域真的需要落地时,SQL 怎么做、分几步、前置条件是什么。**

## 0. 现状基线(已实证)

- 19 个 ESP 数据库全在 `esp-crdb` StatefulSet(3 节点,`crdb-0/1/2`,ns `esp-crdb`)。
- `SHOW DATABASES` 实测:所有库 `primary_region=NULL`、`regions={}`、`survival_goal=NULL` —— 非多区域默认态,迁移是"从零建区",非"改区"。
- 节点 locality 为 `--locality=zone=z1/z2/z3`,**无 region 层**(建库时即如此,见部署 yaml)。

## 1. 执行前置条件(Phase 0 — 单节点显式 DEFERRED)

**本阶段是唯一硬阻塞,单节点 RD 环境无法满足,已在脚本头标注不得执行。**

CRDB 的 `SET PRIMARY REGION "aws"` 要求集群内至少一个节点的 locality 含 `region=aws`,否则报错。当前节点只有 zone 层:

| 节点 | 现 locality | 多区域所需 locality |
|---|---|---|
| crdb-0 | `zone=z1` | `region=aws,zone=a` |
| crdb-1 | `zone=z2` | `region=azure,zone=a` |
| crdb-2 | `zone=z3` | `region=gcp,zone=a` |

改造 = 修改 StatefulSet 的 `--locality` 参数并滚动重启 3 节点。**单节点 RD 上"3 个 pod 同 VM"只能逻辑模拟三区,无真实跨区 RTT;且滚动重启 CRDB 期间学习服务落库会短暂不可用** —— 因此本阶段明确 DEFERRED,等:
- 多节点服务器阶段(每区 ≥3 节点,真 RTT),或
- 单节点仅做"路由正确性"验证(接受无真实延迟语义)时再执行。

## 2. 迁移四阶段

| 阶段 | 内容 | 脚本位置 | 可回滚性 |
|---|---|---|---|
| 0 | 节点 locality 加 region 层(重启) | 运维操作,不在 SQL | 重启改回即可 |
| 1 | 每库 `SET PRIMARY REGION` + `ADD REGION` + `SURVIVE REGION FAILURE` | migrate.sql Phase 1 | `RESET DATABASE PLACEMENT` / 降级 |
| 2 | 逐表 locality:GLOBAL / REGIONAL BY TABLE / REGIONAL BY ROW(+region 列) | migrate.sql Phase 2 | `SET LOCALITY GLOBAL` 可撤;RBR region 列保留 |
| 3 | 验证 | migrate.sql Phase 3 | — |

## 3. 关键设计决策(脚本依据)

1. **REGIONAL BY ROW 必须业务 region,不可哈希推导**。`region` 列来源 = `users.home_region`(注册网关写;JWT claim 带 region)。脚本对所有 RBR 表加了 `crdb_internal_region` 列 + CHECK,默认 `'aws'`。
2. **SURVIVE REGION FAILURE 下无本地写**。写延迟下限 ≈ 1×跨区 RTT(50~100ms),吞吐受 Raft 提交流水线限制;对 ESP 追加型大表(learning_events 5000 ev/s)不构成瓶颈,但**对话内强一致读**(会话恢复/mastery)靠 RBR 本地读 <5ms 兑现。
3. **GLOBAL 只给字典类**(scenarios/knowledge_points/vocabulary 等):极小、读多、低频写,任意区本地读零跨区。
4. **REGIONAL BY TABLE 给单写者组件**(learning_event_outbox/undo_log/llm_semantic_cache):home=aws 集中写,outbox relay 是唯一写者。
5. **遗留 `esp` 库**(esp.users=202 / esp.learning_events=8000,主数据已在域库)迁移优先级最低,脚本默认先域库,esp 库可延迟或下线。

## 4. 与 TiDB 深化路线的取舍(简短)

本 SQL 预备是**能力储备**,不改变"默认 TiDB 深化多区域"的推荐(见对比文档 §5.2)。触发条件(满足其一)才启用:
- 产品出现"多区域就近接入 + 跨区漫游用户 >20%";
- TiDB Placement Rules 行级语义运维成本在多节点被证明不可接受;
- 教学/对标需要的 CRDB 能力验证。

## 5. 验证锚点(Phase 3 跑通后落盘)

- 数据库级:`primary_region='aws'`, `regions={aws,azure,gcp}`, `survival_goal='region'`。
- 表级:locality 声明生效(`crdb_internal.tables.locality`)。
- 行为级:UPDATE home_region 触发后台搬迁;JWT region claim 写本地化。
- 压测:对比文档 §4(双跑方法论,1 亿行数据集,region 40/30/30)。
