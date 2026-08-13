# 会话22-10:Obsidian 同步 ✅ + Redis warm-up ✅ + 双轨落盘刷新(2026-08-13)

> 从 `docs/PROGRESS.md` 归档(2026-08-14 会话22-15 瘦身)。

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
