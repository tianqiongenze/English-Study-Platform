# Milvus + Neo4j 下沉宿主机规划与第一步执行

> 2026-08-20 探查 + 规划。只读探查,集群未动。用户已授权"下沉+重建"。
> 背景:ekb-m2 ns 的 milvus(250m)+ milvus-minio(100m)+ neo4j(250m)= 600m CPU request,
> 单节点 CPU 已 8980m/9000m(99%)→ llm-orchestrator-rs 1.3.1 pod Pending(Insufficient cpu,无 preemption victim)。

## 0. 关键事实核实(只读结论)

### 0.1 邻居 education-platform 引用模式 —— 无需协调,零影响
- education-platform 当前 milvus/neo4j 全部 scale=0(edu-ai-platform/edu-knowledge-bff/edu-knowledge-service 均 0/0)。
- edu 已弃用 Neo4j:`deploy/k8s/base/nebulagraph.yaml:3` 明确"ADR-INF-02 / D-053 终止 Neo4j Community(GPL v3 协议冲突)",改用 NebulaGraph。
- edu Milvus 引用是 **configmap/env 值**(`base/configmap.yaml:37` `MILVUS_HOST=milvus.edu.svc` 自有短名;`local-real-direct/all-services.yaml:319` 直 FQDN `milvus.ekb-m2-aws-us-east-1.svc.cluster.local`),**不共享 ESP 的 ExternalName**,下沉改 ESP 的 extname 对 edu 无感。
- edu Milvus 调用走 pymilvus `MilvusClient(uri=f"http://{host}:{port}")`,代码自带 `create_collection`(dim 从 config.embedding_dim),重建由 edu 服务自启时幂等建,**无预置 collection 依赖**。
- 结论:**esp-shared-infra 合规**,下沉 ESP 侧不影响 edu;edu 即使将来 scale up,指向的是 ekb 真实 svc(`milvus.ekb-m2...`),只要 ESP 删的是 platform-infra/esp-prod 的 ExternalName + ekb 的 deploy,edu 的 FQDN 直连会断 —— ⚠️ 此点需在执行前确认 edu 是否真的不用了(当前 0/0 可接受,但删 ekb deploy 前需用户知晓 edu 也将失去 Milvus,见 §3 风险)。

### 0.2 ESP 实际用的 Milvus collection / Neo4j db
**Milvus 两个 collection(全代码路径确认):**
1. `lco_prompts` —— 宿主 lco-proxy 用(L2 工具缓存),2560 维 Qwen3-Embedding-4B。但**当前 EMBED_MODEL=minilm(start-lco-embedding.sh:21 强制)**,实际走 384 维;collection 由 lco-proxy 启动时**不自动建**(l2.go 只 search/insert,无 ensureCollection)。⚠️ 这是个已知缺口:若 collection 不存在,lco-proxy L2 search 返回非 200 降级(非致命)。
2. `esp_llm_semantic` —— ESP 三服务(conversation-go / llm-orchestrator-rs / writing-coach)共享,384 维 all-MiniLM-L6-v2,cosine≥0.92。**conversation-go 启动时 ensureCollection 幂等自动建**(`semantic.go:99/136/153/203`,POST `/v1/vector/collections/create`)。这是"创建者"服务。
   - 字段:prompt_sha16(VarChar PK)、response、model、scene、ts(Int64)、vector(FloatVector 384)。
   - llm-orchestrator-rs 复用该 collection(`mod.rs:33 const L2_COLLECTION="esp_llm_semantic"`),不自己建。
- 端点:MILVUS_ADDR 默认 `milvus:19530`(经 platform-infra/esp-prod ExternalName 两跳解析到 ekb svc);REST v1(`/v1/vector/*`,2.5 兼容)+ v2(`/v2/vectordb/*`)混用。

**Neo4j db:** 默认 db `neo4j`(REST `/db/neo4j/tx/commit`,见 l3.go:60 / orchestrator mod.rs:265)。
- 仅 llm-orchestrator-rs(mod.rs:263-300)+ conversation-go(knowledge.go KnowledgeGraph)+ lco-proxy(l3.go)三处用,全降级容错(失败 warn 不阻断)。
- 写 `MERGE (p:Prompt{sha16})-[:ANSWERED]->(r:Response{sha16})` + 启动建约束(l3.ensureSchema)。
- 凭据:`neo4j/neo4j-password-1`(ekb deploy env NEO4J_AUTH)。⚠️ SCS:明文弱密码已在集群 env,下沉宿主需设强密码 + ESP deploy env 同步改。
- 端点:NEO4J_URL 默认 `http://neo4j:7474`(orchestrator)/ `bolt://neo4j:7687`(gateway/30-other-services);lco-proxy 走 port-forward `bolt://127.0.0.1:7688` + REST 7475。

### 0.3 Milvus / Neo4j 宿主安装方式结论
**Milvus 2.5 standalone(macOS arm64):**
- 当前 ekb pod 即 `milvus run standalone` + `ETCD_USE_EMBED=true`(内嵌 etcd,无独立 etcd deploy,只依赖外部 milvus-minio)→ **无独立 etcd 容器**,只需 milvus + minio 两容器。
- macOS 无 brew 公式(社区无官方 milvus brew)。标准方式 = **docker compose**(官方 milvus standalone docker-compose.yml:etcd + minio + milvus 三容器;但 2.5 embed 模式可省 etcd)。
- 推荐:**在 VM(rancher-desktop docker context)docker run/compose 起 milvus standalone(embed etcd)+ minio**,而非宿主 brew。理由:① 与现有 Nacos(brew tarball)/OTel(二进制)下沉不同,Milvus 是多容器有状态服务,brew 无支持;② VM 内 docker run 与 k3s 共享同一 docker daemon(esp-k3s-containerd-image-isolation 记录:RD 用 moby/docker CRI,k3s pod 可见 docker 镜像),但**下沉要的是独立容器不是镜像**,docker run 独立容器对 k3s 不可见 = 正确隔离;③ VM 已有 30G/9核,跑 milvus 稳态低 CPU。
- 端口:19530(grpc)+ 9091(health/metrics)。集群经 `host.lima.internal:19530` 访问(RD lima usernet 已验证 host.lima.internal 双向可达,embedding 服务 15730 即此模式)。

**Neo4j 5.x community(macOS arm64):**
- 三选一:**brew install neo4j**(有官方 formula,tarball 模式,最简,与 Nacos 同模式)/ docker run `neo4j:5.25.1-community`/ tarball。
- 推荐 **brew install neo4j**(宿主原生,launchctl 自启,与 Nacos/observability 下沉一致)。端口 7474(http REST)+ 7687(bolt)。
- 风险:Neo4j 5.x brew formula 需 JDK17(brew 自动装);GPL v3 协议(edu 已因此弃用,但 ESP 是内部项目可接受)。

### 0.4 既有下沉模式参考(Nacos / OTel / Tempo)
- `~/observability/`:Nacos/Tempo/Loki/Alertmanager/OTel/Prometheus/Grafana 全是**宿主 brew tarball + 二进制**,由 `scripts/start-host-middleware.sh` 用 `start_one`(nohup 后台 + PID 文件 + 端口探活)启动,**无 launchd 自启**(除 lco-embedding/nacos-crdb-channel 等用 launchd)。
- Nacos:`~/nacos/nacos/bin/startup.sh -m standalone`,监听 8848(已确认 LISTEN,java PID 36014)。
- 但 Nacos 的 ExternalName 仍指 `nacos.ekb-m2...`(05b:88)**未改为 host.lima** —— 说明 Nacos 下沉是"宿主跑 + 集群 port-forward"模式(lco 用 port-forward 7688→7687 等),**非纯 ExternalName→host.lima**。⚠️ 这是关键:Milvus/Neo4j 下沉应统一为 **ExternalName→host.lima.internal:port**(更干净,ESP 服务直连无需 port-forward),而非照搬 Nacos 的 port-forward 模式。

## 1. 完整下沉计划(分阶段)

### 阶段 A:Neo4j 下沉(简单,单服务,先行)

**A1. 宿主安装(brew):**
```
brew install neo4j
# 设密码(首次):编辑 ~/opt/neo4j/conf/neo4j.conf 设 dbms.security.auth_enabled=true
# 或启动后 cypher: ALTER CURRENT USER SET PASSWORD FROM 'neo4j' TO '<strong>'
brew services start neo4j   # launchd 自启,7474/7687
# 验证:curl -u neo4j:<pw> http://127.0.0.1:7474/db/neo4j/tx/commit -d '{"statements":[{"statement":"RETURN 1"}]}'
```
- 数据目录:`~/opt/neo4j/data`(宿主持久化,不丢)。

**A2. ExternalName 改法(DGO:回写 yaml):**
- 文件:`deploy/k8s/05b-platform-infra-externalnames.yaml:76` `externalName: neo4j.ekb-m2-aws-us-east-1.svc.cluster.local`
- 改为:`externalName: host.lima.internal`(端口由 svc port 段保留 7474/7687 → 但 ExternalName 无 port 映射,需 ESP 服务 env NEO4J_URL 直接指 `http://host.lima.internal:7474`)。
- ⚠️ ExternalName 类型不做端口转换,所以**更干净的做法是删 ExternalName,改 ESP deploy env 直指 host.lima.internal**(见 A3)。
- ESP deploy env 改点(3 处,全回写 yaml):
  - `deploy/k8s/30-other-services.yaml:242` `NEO4J_URL: bolt://neo4j:7687` → `bolt://host.lima.internal:7687`
  - `deploy/k8s/10-gateway-go.yaml:43` 同上
  - `deploy/k8s/33-orchestrator-vocab.yaml:83` `NEO4J_URL: http://neo4j:7474` → `http://host.lima.internal:7474`
  - lco-proxy port-forward 7688 改直连 `bolt://host.lima.internal:7687` + REST 7474(`scripts/start-lco.sh:26` 删 neo4j port-forward)。

**A3. 集群清理(BRR:删前确认可逆 + 备份):**
- 备份:`kubectl -n ekb-m2-aws-us-east-1 get deploy neo4j -o yaml > report/neo4j-deploy-backup.yaml`(可逆,rollback=apply backup)。
- Neo4j 数据当前几乎空(仅 lco 写的 Prompt/Response 测试数据,非生产关键),可接受丢失;若要保留:`neo4j-admin dump` 从 pod 导出再 load 到宿主。
- 删:`kubectl -n ekb-m2-aws-us-east-1 delete deploy neo4j svc neo4j`(L2 可逆,deploy/svc 可随时从 ekb 仓库或 backup 重建)。
- ⚠️ edu 影响:edu 已弃用 Neo4j(0 引用),删 ekb neo4j deploy 对 edu 零影响。

**A4. 数据重建:**
- Neo4j db `neo4j` 默认存在(brew 启动自带),无需建 db。
- 约束/索引:lco-proxy(l3.ensureSchema)+ orchestrator(mod.rs ensureSchema)启动时幂等建,无需手动。
- 验证:lco-proxy `/debug` health 返回 L3 ok;orchestrator pod Ready 后写一条 Prompt-Response 查回。

**A5. 验证步骤:**
1. `lsof -iTCP:7474 -sTCP:LISTEN` 宿主 neo4j 在听。
2. `kubectl -n esp-prod exec deploy/llm-orchestrator-rs -- wget -qO- http://host.lima.internal:7474` 通。
3. lco-proxy debug `curl 127.0.0.1:15721/debug | jq .l3_neo4j` status ok。
4. 节点 CPU:删 neo4j deploy 后 `kubectl describe node | grep -A4 "Allocated"` request 降 250m。

### 阶段 B:Milvus 下沉(复杂,etcd+minio+milvus,后行)

**B1. 宿主安装(docker compose in VM):**
- 用官方 milvus standalone docker-compose v2.5(etcd + minio + milvus,或 embed 模式省 etcd)。
- 推荐 embed 模式(对齐 ekb 现状 `ETCD_USE_EMBED=true`):只需 milvus + minio 两容器。
- 在 VM:`docker --context rancher-desktop compose up -d`(compose 文件放 `~/milvus-standalone/`)。
- 端口:19530 + 9091 + minio 9000/9001。
- 数据卷:宿主 `~/milvus-standalone/data`(持久)。

**B2. ExternalName / env 改法(DGO):**
- `05b:64` milvus externalName → `host.lima.internal`(但端口 19530 需 ESP env 带)。
- ESP env(4 处):`MILVUS_ADDR: milvus:19530` → `host.lima.internal:19530`
  - `30-other-services.yaml:238`、`10-gateway-go.yaml:41`、`33-orchestrator-vocab.yaml:78`、(检查 31/32/34 是否还有)。
- lco-proxy port-forward 19531 删,改 `LCO_MILVUS_ADDR=host.lima.internal:19530`(`scripts/start-lco.sh:22`)。

**B3. 集群清理(BRR):**
- 备份:`kubectl get deploy milvus milvus-minio -o yaml > report/milvus-deploy-backup.yaml`。
- 数据:`esp_llm_semantic` 是运行时缓存(可重建,非持久业务数据),接受丢失;`lco_prompts` 同理。若要保留:pymilvus 导出 → 新实例导入。
- 删:`kubectl -n ekb-m2-aws-us-east-1 delete deploy milvus milvus-minio svc milvus milvus-minio`。
- ⚠️ edu 影响:**edu 仍引用 `milvus.ekb-m2-aws-us-east-1.svc.cluster.local`**(all-services.yaml:319),删 ekb milvus deploy 后 edu 若 scale up 会断。当前 edu 0/0 可接受,但**删前需用户确认 edu 确实不用 Milvus 或同步切到 host.lima**。这是 esp-shared-infra 红线点。

**B4. 数据重建(collection):**
- `esp_llm_semantic`:conversation-go 启动 ensureCollection 自动建(幂等),无需手动。但要确保 conversation-go **先于** orchestrator 启动(orchestrator 复用不建)。
- `lco_prompts`:⚠️ lco-proxy 不自动建,需手动 POST `/v2/vectordb/collections/create` 建(2560 维 或 384 维 minilm,看实际 EMBED_MODEL)。补一个 init 脚本 `scripts/init-milvus-collections.sh`(DGO:落仓库)。
- 索引:cosine IVF_FLAT / HNSW(conversation-go ensureCollection 已含 indexParams)。

**B5. 验证:**
1. `lsof -iTCP:19530` 宿主/VM milvus 在听。
2. `kubectl -n esp-prod exec deploy/conversation-go -- wget -qO- http://host.lima.internal:19530/v2/vectordb/collections/list` 返回 esp_llm_semantic。
3. infra_smoke.sh milvus 探针改指向 host.lima.internal(`scripts/infra_smoke.sh:43-45` 更新)。
4. 节点 CPU 再降 350m(250+100)。

## 2. CPU 预期
- 腾出 request 配额:milvus 250m + milvus-minio 100m + neo4j 250m = **600m**。
- 节点 8980m → 8380m(93%),orchestrator pod(其 request 见 33-orchestrator-vocab.yaml,需核实是否 <600m,大概率可调度)。
- 实际 CPU 消耗:Milvus/Neo4j 稳态低,移出 k8s 调度但不移出 VM(CPU 总量不变),**好处是 k8s scheduler 不再被 request 配额卡住**。

## 3. 风险
| 风险 | 等级 | 缓解 |
|---|---|---|
| edu 删 ekb milvus 后断 | 中 | 删前确认 edu 0/0 弃用或同步切 host.lima;Neo4j 已确认 edu 弃用零影响 |
| lco_prompts collection 不自动建 | 中 | 补 init 脚本,下沉后手动建一次 |
| Neo4j 弱密码明文 | 中(SCS) | 下沉设强密码,ESP env 同步,pre-commit 扫 |
| ExternalName 不转端口 | 低 | 改 ESP env 直指 host.lima:port,不依赖 ExternalName |
| docker compose in VM 与 k3s 共存 | 低 | 独立容器对 k3s 不可见=正确隔离;端口不冲突(19530/7474 当前在 k3s,删 deploy 后腾出) |
| Milvus 2.5 REST v1/v2 端点混用 | 低 | 现有代码已验证 v1(conversation-go)+ v2(lco-proxy)均通,保持版本一致即可 |
| 数据丢失(esp_llm_semantic/lco_prompts) | 低 | 均为缓存可重建,非持久业务数据 |

## 4. 第一步执行状态(本会话)
- **未执行**任何集群变更(只读探查)。
- Neo4j 下沉第一步(brew install + 删 deploy)建议下一会话执行,因:
  1. brew install neo4j 是宿主副作用操作(L1 可逆,但需观察启动 + 设密码);
  2. 删 ekb deploy 属 L2 不可逆(虽可 backup 重建),按 BRR 需独立授权确认 edu 影响后执行;
  3. 本探查会话聚焦核实 + 规划落盘,已耗较多上下文于代码路径核实。
- **下一会话第一步动作清单**(按序):
  1. `brew install neo4j && brew services start neo4j`,设强密码;
  2. 备份 `kubectl -n ekb-m2-aws-us-east-1 get deploy neo4j -o yaml > report/neo4j-deploy-backup.yaml`;
  3. 改 ESP 3 处 deploy env NEO4J_URL → host.lima.internal(DGO 回写 yaml);
  4. 改 lco-proxy start-lco.sh 删 neo4j port-forward;
  5. 删 ekb neo4j deploy/svc;
  6. 验证 lco-proxy L3 + orchestrator pod Ready + 节点 CPU 降 250m。
  7. Milvus 下沉(阶段 B)留第三会话。

## 5. 诚实声明
- 本会话为只读探查 + 规划,**未动集群、未装宿主服务**。
- Milvus 部分因复杂(多容器 + collection init 脚本 + edu 影响确认)需下一会话执行。
- Neo4j 第一步 brew install 亦建议下一会话(避免本会话上下文已较满时引入宿主副作用操作)。
- 所有事实均经代码路径 + kubectl get 核实,无臆造。
