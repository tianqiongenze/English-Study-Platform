# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。
>
> 更新日期:2026-08-13 会话22-11(阶段B 图+向量库启用 ✅ + 阶段C 冒烟 7/8 PASS). **D 种子 v4 落盘**:learning_events 99.045M/100M(99%,pragmatic达标,其余 11 表全满标). **架构主线 4 文档 ✅**:togaf-assessment.md(7.34/10)+ optimization-plan.md + 09-人工测试操作指南. **TOGAF P0 9 文档 ✅**. **坑㉓ redis cluster 跨命名空间 hostname 阻断 ✅闭合**. **坑㉒ Neo4j 768Mi OOMKilled + 社区版无 CREATE DATABASE ✅闭合**(limit→1280Mi + label 隔离替代多 db). **阶段A 平台底座 ✅**(platform-infra ns + 05b 6 ExternalName + 多租户设计). **阶段B 图+向量库 ✅**:milvus+milvus-minio+neo4j 全 Ready=1,milvus:19530 pymilvus 连通 collections 空,neo4j:7474/7687 HTTP RETURN 1 ✅ + :Tenant esp 根节点已写,跨 ns 二跳 5 短名全可达. **阶段C 冒烟 7/8 PASS**(pulsar 待 VM 恢复重测). **Redis Cluster warm-up ✅**(23000 keys esp:cache:*). **Obsidian 同步 ✅**(MCP .mcp.json + 6 篇笔记). **当前⚠️**:RD VM 状态 Broken(QEMU 活/控制面断,等自愈,**不自行 rdctl start**). **Task#4**:gateway/learning/bff/learning-worker 已 1/1,余 ~14 deploy 待 VM 稳定后串行恢复. **Task#5 端到端人工测试待开始**. **双轨落盘 ✅**:PROGRESS.md 本节 + Redis 锚点(本轮刷新). host kubectl 坏(EOF 6443),必用 `rdctl shell sudo kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml`. git:多租户文档/05b/infra_smoke/PROGRESS/Obsidian 进度 未 commit.

## 会话22-11:阶段B 图+向量库启用 ✅ + 阶段C 冒烟 7/8 PASS + Neo4j 社区版坑(2026-08-13)

用户指令:"现在开始 B1 串行 scale up Milvus,并行推动后续所有任务"。

### 阶段 B 启用图库+向量库 ✅(串行 scale up,资源压低)
- **Milvus + milvus-minio**:milvus `ETCD_USE_EMBED=true`(内嵌 etcd,无独立 etcd deploy),依赖 milvus-minio(minio RELEASE.2024-05-28)。
  - 资源压低:milvus lim cpu1/mem1Gi(原 cpu3/6Gi),req 250m/512Mi;milvus-minio lim 500m/512Mi,req 100m/128Mi。
  - 串行 scale:minio→1 Ready✅ → milvus→1 Ready✅(QueryNode started + heartbeat 正常)。
  - 验证:`:19530` FQDN TCP 可达;pymilvus 连接成功,**collections 空**(业务未灌向量数据,预期)。
- **Neo4j 5.25.1-community**:
  - 资源:lim 1280Mi(原无 resources),req 512Mi;env `heap_max=512m` + `heap_initial=512m` + `pagecache=256m`;volume emptyDir。
  - **坑㉒(实测,已闭合)**:768Mi limit 下 Neo4j 被 **OOMKilled**(exit 137,09:32:53),且容器内 cypher-shell 起额外 JVM 叠加主进程触发 cgroup kill。**修复**:limit 768Mi→1280Mi;不在容器内起 JVM,改用外部 pod HTTP API 操作。
  - 验证:`:7474`(HTTP)+`:7687`(Bolt)FQDN 可达;HTTP `RETURN 1` 返回 1✅;`:Tenant {name:'esp'}` 根节点已写入(多租户隔离基准)。
- **跨 ns 二跳(platform-infra)✅**:ESP pod 经 `*.platform-infra.svc.cluster.local` 5 短名全可达(milvus:19530 / neo4j:7474+7687 / redis:6379 / tidb:4000)。

### ⚠️ Neo4j 社区版多租户限制(架构级坑,已记入多租户文档)
- **现象**:HTTP API 执行 `CREATE DATABASE esp IF NOT EXISTS` 返回 `Neo.ClientError.Statement.UnsupportedAdministrationCommand`。
- **根因**:Neo4j **Community Edition 不支持 `CREATE DATABASE`**,仅默认 `neo4j`/`system` 两库;多 database + RBAC 是企业版特性。
- **影响**:多租户隔离矩阵 Neo4j 一行从"database 隔离"降级为 **label + 节点命名空间隔离**(默认库内 `:Tenant` 标签,Cypher `WHERE t.name=$tenant` 过滤)。
- **生产建议**:强隔离升企业版,或独立 Neo4j 实例/租户。
- **文档已改**:`docs/design/platform-infra-multitenant.md` 隔离矩阵 + SOP + 风险表三处同步更新。

### 阶段 C 冒烟测试(infra_smoke.sh)7/8 PASS
- `scripts/infra_smoke.sh` 增强:milvus 加 pymilvus 连通探测,neo4j 加 HTTP `RETURN 1` 查询探测(原仅 readyReplicas)。
- **结果**:tidb-ping✅ / redis-set-get✅ / redis-get✅ / **pulsar-broker-up❌** / milvus-pod-ready✅ / milvus-pymilvus-connect✅ / neo4j-pod-ready✅ / neo4j-http-query✅。
- **pulsar FAIL 待查**:冒烟跑完后 RD VM 状态变 **Broken**(QEMU 进程仍活,lima-guestagent 控制通道断,疑似持续 OOMKill 致 load 15+ 内存紧张)。pulsar readyReplicas 未能读到 —— **需 VM 恢复后重测 pulsar**。
- C1(TiDB 读写分离注释):05b 已含"伪分离:单节点同 svc;多节点改独立 follower svc"注释(line 10-11,30),✅ 无需改动。

### 恢复锚点(精确子步骤)— 阶段C 收尾 + Task#4/5
1. **已完成**:阶段A✅ / 阶段B✅(milvus+neo4j 启用+验证) / Redis warm-up✅ / Obsidian✅ / 阶段C 冒烟 7/8✅ / Neo4j 社区版坑记录✅。
2. **VM Broken 待恢复**:RD VM 状态 Broken(QEMU 活,控制面断)。等 guestagent 自愈或用户介入。⚠️ **铁律:不自行 rdctl start/Factory Reset**。
3. **VM 恢复后**:① 重测 pulsar readyReplicas + 补 `infra_smoke.sh pulsar`;② 确认 milvus/neo4j/milvus-minio 仍 Ready(若被 OOMKill 回退则重 patch+scale)。
4. **Task#4**:VM 稳定后串行 scale 余下 ~14 P0 deploy(assessment-rs/course-path-go/frontend-h5/frontend-react/gamification-go/listening-go/live-tutor-go/llm-orchestrator-rs/media-learn-rs/pronunciation-rs/reading-go/vocab-srs-go),每个等 Ready+load 回落。
5. **Task#5**:端到端人工测试(docs/test/09-人工测试操作指南.md)。
6. **Task#6 git commit**:本会话改动(05b 已 apply / 多租户文档 Neo4j 改 / infra_smoke 增强 / PROGRESS / Obsidian 进度笔记)待 commit。

### 关键命令速查(本会话)
- Neo4j HTTP 操作(避开容器内 JVM OOM):esp-prod 临时 python pod → `urllib.request` POST `http://neo4j.ekb-m2...:7474/db/neo4j/tx/commit`,Basic auth `neo4j:neo4j-password-1`。
- Milvus 验证:esp-prod 临时 python pod → `pip install pymilvus` → `connections.connect(host=milvus.ekb-m2..., port=19530)` + `utility.list_collections()`。
- VM Broken 诊断:`rdctl shell echo ok`(失败则 Broken);`ps aux | grep qemu`(进程在即 QEMU 活)。

---

## 会话22-10:Obsidian 同步 ✅ + Redis warm-up ✅ + 双轨落盘刷新(2026-08-13)

用户指令:① TiDB 灌完数据后 Redis Cluster 能否 warm up;② 本机有 Obsidian,把 Claude Code 所有项目信息/决策记录随时同步进 Obsidian,联网给最优方案并执行成功;③ 立即双轨落盘(PROGRESS+Redis)使上下文溢出后能完整恢复。

### Redis Cluster warm-up ✅(TiDB 热点数据 → Redis esp:cache:*)
- **Job**:`esp-redis-warmup`(esp-prod,python:3.12-slim,pip pymysql+redis 清华源),Completed。
- **ConfigMap**:`redis-warmup-script`(挂载 `scripts/redis-warmup.py`)。
- **连接**:TIDB_HOST=tidb-primary(经 05→platform-infra→ekb-m2 二跳)+ REDIS_STARTUP=redis-cluster-new-0 FQDN。
- **结果**:23000 keys 加载,esp:cache:* 前缀,3 master 均布(~7632-7693 dbsize 含 slave 镜像):
  - scenarios ~10000 → `esp:cache:scenario:{id}`
  - knowledge_points ~10000 → `esp:cache:kp:{id}`
  - sessions top1000 → `esp:cache:session:{id}`
  - user_mastery top1000 → `esp:cache:mastery:{user_id}:{kp_id}`
  - users top1000 → `esp:cache:user:{id}`
- **TTL**:3600s(1h),SSDictCursor + pipeline 批 500。
- **意义**:业务启动即命中缓存,验证 TiDB→Redis warm-up 通路 + Redis Cluster FQDN 跨 ns 读写稳定。

### Obsidian 同步 ✅(联网调研 + 执行)
- **选型**:对比 3 包(mcp-obsidian@1.0.0 只读 / obsidian-mcp-server@3.2.12 需 Local REST API 插件 / **@mauricio.wolff/mcp-obsidian@0.8.2** 直文件系统访问 14 工具)→ 选定 `@mauricio.wolff/mcp-obsidian`。
- **理由**:无需 Obsidian 运行/插件,直读写 vault,兼容 iCloud 路径(含空格),Claude Code 原生支持。
- **Vault**:`/Users/joelgong/Library/Mobile Documents/iCloud~md~obsidian/Documents/JoelGong`(iCloud 同步)。
- **MCP 注册**:项目级 `.mcp.json` 写入 `claude mcp add`(command=npx,args=`-y @mauricio.wolff/mcp-obsidian@latest <vault>`)。MCP 会话启动加载,**下次会话工具生效**。
- **首次同步**:vault 内 `人工智能项目/ESP平台/` 写入 6 篇 wikilink 互联笔记:
  - `00-ESP平台总览.md` — 一句话+评分7.34+五组件状态+单节点铁律+索引
  - `架构评分与重构方向.md` — 10维度+TOGAF+重构方向
  - `平台底座与多租户隔离.md` — 二跳解耦+隔离矩阵+SOP+三阶段
  - `关键坑记录.md` — 坑㉓+单节点铁律类坑
  - `进度同步.md` — 会话22-9/10 进度+恢复锚点
  - `如何持续同步到 Obsidian.md` — 选型+MCP配置+同步流程
- **同步策略**:仓库 docs/ 是 single source of truth,Obsidian 为镜像,不做反向同步;里程碑级同步(非每轮),用 patch_note 增量更新。

### 恢复锚点(精确子步骤)— 阶段B 进行中,Obsidian/warm-up 已闭合
1. **已完成**:坑㉓✅ / 阶段A✅ / Redis warm-up✅ / Obsidian 同步✅ / 双轨落盘✅。
2. **B1 Milvus(下一步)**:apply `/tmp/patch-milvus-res.yaml`(req250m/512Mi,lim1/1Gi)+ `/tmp/patch-milvus-minio-res.yaml`(req100m/128Mi,lim500m/512Mi),串行 scale milvus-minio→1(等 Ready+load 回落)→ milvus→1,验证 `:19530` + list collections 空。
3. **B2 Neo4j**:压 limit req256/lim768 + env NEO4J_dbms_memory_heap_max__size=512m + 补 PVC(local-path),scale→1,验证 `:7474` + cypher `CREATE DATABASE esp`。
4. **B3**:apply 05b(已含 milvus/neo4j ExternalName),esp-prod 跨 ns 测短名解析。
5. **阶段C**:TiDB 读写分离注释(单节点伪分离)+ `scripts/infra_smoke.sh` 6 组件冒烟 + 落盘 commit。
6. **Task#4**:恢复余下 ~14 P0 deploy + 验证 healthz(单节点串行)。
7. **Task#5**:端到端人工测试(docs/test/09-人工测试操作指南.md)。
8. **Task#6 git commit**:PROGRESS/平台底座manifest/架构文档/seed/redis-warmup/Obsidian笔记 未 commit。

### 关键约束(本会话遵守)
- host kubectl 坏(EOF @ 127.0.0.1:6443),全用 `rdctl shell sudo kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml`。
- 单节点铁律:不碰 k3s 配置/restart/pkill/Factory Reset;主机重 mvn 与集群操作串行;VM 重启后必预置 pause 镜像。
- 文件传输到 VM 用 base64 内联。Bash 输出限流:`--tail=N` / `| head/grep`;大输出落 report/logs。
- Obsidian vault 路径含空格,bash 命令必须双引号包裹。

---

## 会话22-9:平台底座化(阶段A✅)+ redis 坑㉓闭合 + 阶段B/C(2026-08-13)

用户指令:① 自动从 PROGRESS+Redis 恢复完整完成所有任务;② Redis 重建是否丢数据 + 把 Redis/Pulsar/TiDB/图库/向量库共享为局域网通用基础设施;③(决策)全部启用图+向量库 + 升级为局域网通用平台底座;④ 立即双轨落盘(PROGRESS+Redis)使上下文溢出后能完整恢复。

### 现状实查(2026-08-13)
- **Redis Cluster**:`redis-cluster-new` 6 pod 全 1/1 Running(18m),`cluster_state:ok slots_ok:16384 cluster_size:3 known_nodes:6`,全 FQDN announce。esp-prod 跨 ns `SET/GET esp:ctx:plat-test` 成功(坑㉓ 闭合)。
- **平台底座(阶段A✅)**:`platform-infra` ns 6 ExternalName 入口全在(tidb/tidb-follower/redis-cluster/pulsar/milvus/neo4j);ESP `05-infra-reuse.yaml` 二跳指 platform-infra。
- **阶段B(⏳)**:milvus(0/0)+milvus-minio(0/0)+neo4j(0/0) 均 scale=0,122d 前创建。VM 9核/29.3G,可用 6.0G,负载 26%。
- **esp-prod 业务**:gateway-go/learning-java/bff-java/learning-worker 已 1/1;余 ~14 deploy 待 scale up(Task#4)。
- **双轨落盘(✅)**:PROGRESS.md 本节 + Redis 锚点 `esp:ctx:hot:2026-08-13-session22-9-platform-infra`(2504B,STRLEN 验证)。

### 坑㉓(Redis 跨命名空间 hostname 阻断)✅ 闭合
- **根因**:redis sts `--cluster-announce-hostname $(POD_NAME).redis-cluster-new`(无 ns 后缀),cross-ns 客户端 esp-prod 解析失败 → MOVED 路由到短名节点超时。
- **修复**:sts args 改 FQDN `$(POD_NAME).redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local` + `--cluster-preferred-endpoint-type hostname`。
- **重建难点**:nodes.conf/dump.rdb/appendonlydir 在 PVC 持久化旧短名元数据,patch args 后仍公告短名。需 6 pod 逐一 `FLUSHALL + CLUSTER RESET HARD + rm -f /data/nodes.conf /data/dump.rdb + rm -rf /data/appendonlydir`(AOF 残留恢复旧集群元数据),再 force-delete pod + re-apply manifest 重建 init Job。
- **数据丢失**:重建 FLUSHALL 清空。Redis 仅存跨会话运行态/缓存/进度(非业务数据,可重灌)。
- **关联**:[[esp-redis-cluster-hostname-announce-023]] memory 已更新。

### 平台底座多租户设计(阶段A 文档+manifest,纯文件不动运行态)
- 新增 `deploy/k8s/00-platform-infra-namespace.yaml` — platform-infra ns(shared-base 标签)。
- 新增 `deploy/k8s/05b-platform-infra-externalnames.yaml` — 6 组件统一 ExternalName 入口层(二跳解耦:组件迁移只改 05b,租户零改动)。
- 修改 `deploy/k8s/05-infra-reuse.yaml` — ESP 4→6 ExternalName 改指 `*.platform-infra.svc.cluster.local`(业务 env 短名不变,零改动)。
- 新增 `docs/design/platform-infra-multitenant.md` — 5 组件隔离矩阵(TiDB DB / Redis 前缀 / Pulsar tenant / Milvus collection / Neo4j database)+ 新租户 3 步接入 SOP + 单节点风险 + 多节点移植清单。

### 恢复锚点(精确子步骤)— 阶段B 进行中
1. **当前**:阶段B,3 组件 scale=0。VM 可用 6.0G。
2. **B1 Milvus**:查 deploy/milvus spec(内嵌 etcd 或 standalone),压 limit req256/lim512,串行 scale milvus-minio→1(等 Ready+load 回落)→ milvus→1,验证 `:19530` 可连 + list collections 空。
3. **B2 Neo4j**:压 limit req256/lim768 + env NEO4J_dbms_memory_heap_max__size=512m + 补 PVC(local-path),scale→1,验证 `:7474` + cypher `CREATE DATABASE esp`。
4. **B3**:apply 05b(已含 milvus/neo4j ExternalName),esp-prod 跨 ns 测短名解析。
5. **阶段C**:TiDB 读写分离注释(单节点伪分离,多节点 tidb=2 真分流)+ `scripts/infra_smoke.sh` 6 组件冒烟 + 落盘 commit。
6. **Task#4**:恢复余下 ~14 P0 deploy + 验证 healthz(单节点串行,每批等 load 回落)。
7. **Task#5**:端到端人工测试(docs/test/09-人工测试操作指南.md)。

### 关键约束(本会话遵守)
- host kubectl 坏(EOF @ 127.0.0.1:6443),全用 `rdctl shell sudo kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml`。
- 单节点铁律:不碰 k3s 配置/restart/pkill/Factory Reset;主机重 mvn 与集群操作串行;VM 重启后必预置 pause 镜像。
- 文件传输到 VM 用 base64 内联(`B64=...; rdctl shell sudo sh -c "echo '$B64'|base64 -d>..."`)。
- Bash 输出限流:`--tail=N` / `| head/grep`;大输出落 report/logs。

---

## 历史 milestone 归档索引(需要时取)

| 主题 | 文件 | Redis key |
|---|---|---|
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
- ArgoCD sync:`kubectl -n argocd patch application esp-platform --type=merge -p '{"operation":{"sync":{}}}'` 或 API POST。
- 压测:分布式 locust 见 `deploy/locust/README.md`。
- 单节点铁律:重 mvn/docker build 与集群操作串行;不碰 k3s 配置。

## 上下文工程铁律(摘要,详见 memory `context-overflow-ironrules`)

- 文件系统=长期记忆,对话=临时工作区。80%(160k)主动 /compact,90%(180k)强制止损落盘+compact。
- 三层防御:源头减量(subagent 隔离+输出限流)+ 状态外化(PROGRESS 热区+Redis 运行态)+ 主动管控。
- **本文件热区上限 ~300 行**,历史归 `docs/progress/archive/`,不全量读。完整铁律集见 memory。
