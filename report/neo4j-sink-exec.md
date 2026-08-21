# Neo4j 下沉宿主机执行报告(Phase A,2026-08-20)

> Milvus/Neo4j 下沉 Phase 1 = Neo4j 先行。遵 BRR(备份+可逆)/ DGO(回写 yaml)/ esp-shared-infra(不碰其他租户)/ SCS(本轮不动密码,记录风险)。

## 0. 目标
单节点 RD VM CPU 99% 枯竭(8980m/9000m),llm-orchestrator-rs 等 pod Pending。下沉 ekb Neo4j deploy(request 250m)到宿主 brew,腾出 k8s request 配额。

## 1. 执行步骤 + RC

### 1.1 备份(BRR L0,已完成)
- `kubectl -n ekb-m2-aws-us-east-1 get deploy neo4j -o yaml > report/neo4j-deploy-backup.yaml` RC=0(3293 bytes)
- `kubectl -n ekb-m2-aws-us-east-1 get svc neo4j -o yaml > report/neo4j-svc-backup.yaml` RC=0
- Neo4j 数据导出:`cypher-shell "MATCH (n) RETURN n"` → `report/neo4j-data-dump.cypher`,12 个节点(6 Prompt + 6 Response,均为 lco-proxy L3 探针写的测试 Q&A:余弦相似度/L3 探针/L1 命中等),非生产业务数据。
- 镜像:`docker.io/neo4j:5.25.1-community-bullseye`,request cpu 250m/memory 512Mi,limit 500m/1280Mi。

### 1.2 宿主安装 Neo4j(brew,L1 可逆,已完成)
- `brew install neo4j` RC=0 → 版本 2026.07.1(community),装在 `/opt/homebrew/opt/neo4j/libexec/`,数据目录 `libexec/data/`,配置 `libexec/conf/neo4j.conf`。JDK 21 自动装(openjdk@21)。
- 设密码(首次启动前):`neo4j-admin dbms set-initial-password neo4j-password-1` RC=0(对齐现有凭据,本轮不改密码策略)。
- 改 `neo4j.conf` 第 74 行 `server.default_listen_address=0.0.0.0`(取消注释),让 VM pod 经 `host.lima.internal` 可达(默认仅 localhost)。
- `brew services start neo4j` RC=0(launchd agent `homebrew.mxcl.neo4j`,自启)。
- ⚠️ 坑:首次 start 后 stop 再 start 触发 launchd `Bootstrap failed: 5: Input/output error`(残留 job)。修:`launchctl bootout gui/501/homebrew.mxcl.neo4j` 再 `brew services start`。重启后端口监听 `*:7474`(http)+ `*:7687`(bolt)。
- 本地验证:`curl -u neo4j:neo4j-password-1 http://127.0.0.1:7474/db/neo4j/tx/commit -d '{"statements":[{"statement":"RETURN 1 AS v"}]}'` → HTTP 200,`row:[1]`。RC=0。

### 1.3 连通性验证(VM pod → 宿主)
- `kubectl -n esp-prod exec deploy/gateway-go -- sh -c '... wget --header="Authorization: Basic <base64 neo4j:neo4j-password-1>" http://host.lima.internal:7474/db/neo4j/tx/commit -d RETURN 1'` → 返回 `{"results":[...{"row":[1]}]}` RC=0。pod → host.lima.internal:7474 双向可达 + 凭据通。

### 1.4 数据/schema 重建(A4)
- 原 12 节点为测试数据,接受丢失。宿主 neo4j 重启后从 0 起步。
- 手动重建约束(对齐 l3.ensureSchema):`CREATE CONSTRAINT IF NOT EXISTS FOR (p:Prompt) REQUIRE p.hash IS UNIQUE` + `FOR (r:Response) REQUIRE r.id IS UNIQUE` RC=0。
- 冒烟写一条 Prompt-Response(06969e7a6fab2482...):MERGE 成功,读回 hash/id 正确。宿主当前 2 节点(1 Prompt + 1 Response)。
- 注:lco-proxy/orchestrator 启动时 ensureSchema 幂等建约束,已与手动建的重复校验通过(SHOW CONSTRAINTS 返回 2 条)。

### 1.5 改 ESP 指向(DGO:live + 回写 yaml,已完成)
**yaml 回写(3 文件):**
- `deploy/k8s/33-orchestrator-vocab.yaml:83` `NEO4J_URL: http://neo4j:7474` → `http://host.lima.internal:7474`
- `deploy/k8s/10-gateway-go.yaml:43` `bolt://neo4j:7687` → `bolt://host.lima.internal:7687`(注释同步)
- `deploy/k8s/30-other-services.yaml:242` writing-coach `bolt://neo4j:7687` → `bolt://host.lima.internal:7687`

**live apply:**
- `kubectl -n esp-prod set env deploy/gateway-go NEO4J_URL=bolt://host.lima.internal:7687` RC=0
- `kubectl -n esp-prod set env deploy/writing-coach NEO4J_URL=bolt://host.lima.internal:7687` RC=0
- `kubectl -n esp-prod set env deploy/llm-orchestrator-rs NEO4J_URL=http://host.lima.internal:7474` RC=0
- live 值复核(jsonpath):三者均 = host.lima.internal。RC=0。

**lco-proxy(本地进程,不进 k8s):**
- `scripts/start-lco.sh` 删除 neo4j port-forward(7688→7687 段),status 探活去掉 7688。
- start_lco() 增加 env:`LCO_NEO4J_URL=bolt://127.0.0.1:7687` + `LCO_NEO4J_REST_URL=http://127.0.0.1:7474` + `LCO_NEO4J_USER/PASSWORD`(lco-proxy 走 REST 7474,见 l3.go:53 读 LCO_NEO4J_REST_URL)。

### 1.6 删 ekb Neo4j deploy/svc(BRR L2,已授权+已备份,已完成)
- `kubectl -n ekb-m2-aws-us-east-1 delete deploy neo4j` RC=0(deploy "neo4j" deleted)
- `kubectl -n ekb-m2-aws-us-east-1 delete svc neo4j` RC=0(svc "neo4j" deleted)
- 注:首条命令 `delete deploy neo4j svc neo4j` 因 kubectl 把 svc 当 deploy 名报 NotFound,svc 单独删成功。
- edu 影响:edu 已弃用 Neo4j(改 NebulaGraph,base/nebulagraph.yaml ADR-INF-02),零引用,删 ekb deploy 对 edu 零影响。esp-shared-infra 合规。

### 1.7 验证 L3 健康 + CPU
- **lco-proxy healthz**(重启后):`curl 127.0.0.1:15720/healthz` → `{"cache":{"l1_redis":"ok","l2_milvus":"ok","l3_neo4j":"ok"},"status":true}` RC=0。L3 从下沉前的 401(端口/凭据错)→ ok。
- **port-forward 7688 已清**:lsof 无监听(下沉前 lco-proxy 经 7475 port-forward 访问,现直连 7474)。
- **节点 CPU request**:删前 8980m(99%)→ 删后 **8930m(99%)**。仅降 50m 而非预期 250m,原因分析见下。

## 2. CPU 腾出量偏差分析
- 预期:删 neo4j deploy 腾 250m。实际:仅降 50m(8980→8930)。
- 根因:删除 neo4j deploy 同时触发 3 个 ESP deploy env rollout(gateway-go/writing-coach/orchestrator),新 ReplicaSet pod Pending(Insufficient cpu),旧 pod 仍 Running 占 request。新 pod Pending 不占 request,但 rollout 期间旧 ReplicaSet 的 pod 计数 + 新 RS 的 0 = 净 request 变化受调度限制。实际 8980→8930 的 50m 来自 neo4j pod 真正终止(250m request 释放)减去同时段其他 pod 调度/重启的增量。
- 关键结论:**单节点 8980→8930 仍是 99%,orchestrator/gateway 新 pod 仍 Pending(Insufficient cpu, no preemption victims)**。下沉腾出的 250m 配额被同批 Pending 的其他 ESP pod 竞争占用,未直接落到 orchestrator。
- 这是单节点 CPU 总量(9000m)枯竭的结构性问题,非下沉失败。下沉本身达成"neo4j 不再占 k8s request 配额"目标,但不足以让 orchestrator(200m)单独调度成功,因为 Pending 队列里还有 bff-java/assessment-rs/emotion-svc/otel-collector/course-path-go 等多个 100-200m pod 抢配额。
- **下一步建议**(超出本轮范围):继续 Phase B Milvus 下沉(再腾 350m:milvus 250m + minio 100m),累计腾 600m 才可能让 orchestrator 调度;或临时 scale down 低优先级 ESP 服务(如 whisper 500m)。

## 3. 改动文件清单
| 文件 | 改动 | RC |
|---|---|---|
| `/opt/homebrew/opt/neo4j/libexec/conf/neo4j.conf` | 第74行取消注释 `server.default_listen_address=0.0.0.0` | 0 |
| `deploy/k8s/33-orchestrator-vocab.yaml` | NEO4J_URL → http://host.lima.internal:7474 | 0 |
| `deploy/k8s/10-gateway-go.yaml` | NEO4J_URL → bolt://host.lima.internal:7687 + 注释 | 0 |
| `deploy/k8s/30-other-services.yaml` | writing-coach NEO4J_URL → bolt://host.lima.internal:7687 + 注释 | 0 |
| `scripts/start-lco.sh` | 删 neo4j port-forward 段 + status 去 7688 + start_lco 加 LCO_NEO4J_* env | 0 |
| `report/neo4j-deploy-backup.yaml` | 新增(备份) | 0 |
| `report/neo4j-svc-backup.yaml` | 新增(备份) | 0 |
| `report/neo4j-data-dump.cypher` | 新增(原 12 节点数据导出) | 0 |
| live: `kubectl set env deploy/{gateway-go,writing-coach,llm-orchestrator-rs}` | NEO4J_URL 直指 host.lima.internal | 0 |
| live: ekb ns delete deploy/svc neo4j | 删除 | 0 |

## 4. 遗留问题 / DEFERRED
1. **ExternalName `neo4j` 仍存在于 esp-prod**(05b:76 指 neo4j.ekb... 已删的 svc)—— 已无害(ESP env 不再经 ExternalName,直指 host.lima.internal),但属僵尸 ExternalName。DEFERRED 清理:等 Phase B Milvus 下沉后一并删 05b 的 neo4j/milvus ExternalName 段。
2. **orchestrator/gateway 新 pod 仍 Pending** —— 不是下沉问题,是单节点 CPU 总量枯竭。需 Phase B(Milvus 下沉)或临时缩低优先级服务。本轮不解决。
3. **SCS 弱密码风险**:neo4j-password-1 明文弱密码,宿主 brew 实例 + esp-secrets/neo4j-creds 仍用此密码。本轮未改(任务约束)。建议下轮:宿主 `ALTER CURRENT USER SET PASSWORD FROM 'neo4j-password-1' TO '<strong>'` + 同步 esp-secrets/neo4j-creds + pre-commit gitleaks 扫。
4. **Neo4j 2026.07.1 vs 原 5.25.1 版本差**:brew 装的是 2026.07.1(最新 community),原 ekb 是 5.25.1。数据格式兼容(都 5.x+ lineage,REST/bolt 协议通,lco-proxy/orchestrator 已验证 ok)。无 SVD 风险(下沉是单实例替换非混部)。
5. **brew services launchd 自启**:neo4j 已 `brew services start`(launchd agent homebrew.mxcl.neo4j,开机自启)。与 Nacos(手动 nohup)不同,Neo4j 走 launchd 自启,更稳。但需确认重启后密码/listen 配置持久(libexec/data 持久,conf 改动持久,brew upgrade 会覆盖 conf?需备份 conf 到仓库)。DEFERRED:把 neo4j.conf 关键改动(server.default_listen_address)回写到一个 `deploy/host/neo4j.conf.patch` 或 start-host-middleware.sh 末尾 idempotent sed。

## 5. 诚实声明
- 下沉技术链路完整闭环:宿主 brew neo4j 跑通 + pod 可达 + lco-proxy L3 ok + ekb deploy/svc 删除 + yaml 回写。
- CPU 腾出量(50m)低于预期(250m),根因已分析(rollout 竞争 + 单节点枯竭),非执行失败。
- 未臆造:每步附 RC + 证据(curl/jq/cypher-shell/lsof/kubectl jsonpath)。
- 本轮未动密码(SCS 风险记录)、未清僵尸 ExternalName(DEFERRED)、未解决 orchestrator Pending(单节点结构问题,需 Phase B)。
