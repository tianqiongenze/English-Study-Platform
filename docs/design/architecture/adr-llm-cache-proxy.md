# ADR-23: llm-cache-proxy 三层语义缓存代理(LCO 铁律工具侧落地)

> 日期:2026-08-20
> 状态:Accepted
> 关联:LCO 铁律(memory `llm-cache-optimization-ironrule`)、ADR-14(缓存 key 三端统一)、ADR-11(Redis Cluster 迁移)、TOGAF Phase G 实施治理

## 1. 背景

LCO(LLM 调用缓存优化)铁律要求:所有 LLM/AGI 链路必实施三层语义缓存(精确哈希 + 语义向量 + 知识图谱),禁止裸调外部 LLM API;缓存命中率 >30%,成本节省 >40%。

ESP 服务侧的三层缓存(L1 Redis / L2 Milvus / L3 Neo4j)在 conversation-go / writing-coach / orchestrator-rs 三个业务服务中落地。但**本机编码工具**(Claude Code / ZCode / Continue / Cline / Aider / Ollama 客户端)是更高频的 LLM 调用源(开发期反复生成/解释/重构代码),这些工具直连 LLM API 完全无缓存,违反 LCO 铁律。

需要一个**独立于业务服务的宿主代理**,横截所有本机工具的 LLM 调用,统一实施三层缓存,而不侵入各工具的 SDK。

## 2. 决策

采纳方案:独立宿主代理 `llm-cache-proxy`(Go,监听 `:15720`),作为本机工具 → LLM API 之间的反向代理,在转发链上插入三层缓存。工具把 `base_url` 指向 `http://127.0.0.1:15720`,代理按协议(Anthropic / OpenAI / Ollama)归一化缓存键并依次查 L1/L2/L3,命中即返回(带 `X-Lco-Cache-Hit` 头),未命中转发上游(CC Switch `:15721`)并回写各层。

### 2.1 三层语义缓存

| 层 | 存储 | 命中条件 | 命中收益 | key |
|---|---|---|---|---|
| L1 精确缓存 | Redis Cluster(经 port-forward 接 6 pod) | `SHA256(system_prompt+user_prompt)` 严格相等 | 跳过 LLM API,省 100% | `esp:cache:llm:exact:{hash}` |
| L2 语义缓存 | Milvus(REST `/v2/vectordb/entities`) | prompt 向量 cosine 相似度 > `L2_THRESHOLD`(默认 0.92) | 省约 80-95% | 向量 + body/protocol/model payload |
| L3 知识图谱 | Neo4j(REST `/db/neo4j/tx/commit`) | 非阻断:记 Prompt→Response 关系,作 L2 未命中的图增强(模糊 text contains 查相关历史) | 上下文增强,非直接命中 | `(:Prompt{hash,text,protocol})-[:ANSWERED_BY]->(:Response{body})` |

L3 设计为**非阻断**知识沉淀层:查询失败=无增强直穿,不影响主链路;L2 跨协议命中当前降级直穿(重生成 DEFERRED)。

### 2.2 调用方各自声明 embedding model(解 FORCE_MODEL 冲突)

宿主 embedding 服务(`tools/llm-cache-proxy/embedding/app.py`,`:15730`)多模型支持:
- `qwen` → Qwen3-Embedding-4B(2560 维,中文语义强)
- `minilm` → all-MiniLM-L6-v2(384 维,英文轻量)

原设计服务端 `EMBED_MODEL` 强制单一模型(FORCE_MODEL),会与"业务 LCO 用 384 维 minilm + 工具 LCO 用 2560 维 Qwen 共用同一 embedding 服务"冲突。

**决策**:服务端 `FORCE_MODEL` 仅作整体强制开关(为空时按语言自动路由);`/embed` 请求体显式传 `model` 字段,优先级:请求 `model`(auto/qwen/minilm)> 服务端 FORCE_MODEL > 语言路由。llm-cache-proxy 的 L2 调用显式传 `model=qwen`(见 `internal/cache/l2.go:embed` 注释),与 2560 维 collection 对齐。

### 2.3 双 collection 独立(2560 Qwen vs 384 minilm)

两个调用方维度不同,**禁止共用 collection**(Milvus collection schema 维度固定,维度不匹配导致 insert/search 静默失败):

| collection | 维度 | 使用方 | embedding model |
|---|---|---|---|
| `lco_prompts` | 2560 | llm-cache-proxy(本机工具) | qwen(Qwen3-Embedding-4B) |
| `esp_llm_semantic` | 384 | ESP 业务服务(conversation-go/writing-coach/orchestrator-rs) | minilm(all-MiniLM-L6-v2) |

L2 阈值 `LCO_L2_THRESHOLD` 默认 0.92,两 collection 独立配置。

### 2.4 Neo4j 下沉宿主

Neo4j(L3)作为本机知识图谱存储,下沉宿主进程(llm-cache-proxy 直接经 `bolt://127.0.0.1:7688` / REST `:7475` 访问),不进 K8s 集群(单节点验证期避免有状态集群运维负担,多节点迁移时 Neo4j 可独立部署)。`ensureSchema` 启动幂等建约束/索引(`Prompt.hash UNIQUE` / `Prompt.text INDEX` / `Response.id UNIQUE`),失败仅 warn 降级为无图增强。

## 3. 方案抉择

### 方案 A:独立宿主代理(采纳)
- Go 实现,协议层归一化(Anthropic/OpenAI/Ollama 统一 NormalizeKey),三层缓存解耦,工具零改造(只改 base_url)。
- **优点**:横截所有本机工具,缓存复用率最高;与业务服务 LCO 解耦(各自 collection 维度);Go 单二进制易部署。
- **缺点**:多一层代理(延迟 +本地回环 <1ms 可忽略);embedding 服务常驻吃 MPS(限流/离峰运行,见 memory `esp-lco-embedding-vm-snowball`)。

### 方案 B:各工具内置缓存
- **否决**:工具 SDK 各异(Claude Code 闭源 / Continue TS / Aider Python),逐个改造不可行;跨工具缓存无法复用。

### 方案 C:复用 ESP 业务服务 LCO
- **否决**:业务服务 LCO 是 384 维 collection,与工具场景(中文 prompt 为主,2560 维 Qwen 语义更准)维度需求冲突;且业务服务在 K8s 集群内,本机工具经 port-forward 访问链路脆弱。

## 4. 治理与供应链(本 ADR 同步补齐)

- **CLC 铁律**:`.golangci.yml` 启 `nilnesserr`/`nilerr`/`gosec`;L2/L3 代码入口 nil 防御(`if l == nil || l.cfg == nil { return }`)。
- **SCS 铁律**:CI 加 `govulncheck`(依赖漏洞前置阻断)+ 覆盖率 `coverage.out` artifact 落盘。
- **Dockerfile**:nonroot(`USER app`)+ `HEALTHCHECK`(wget 探 `/healthz`)。
- **RWT 铁律**:L2/L3 全链路降级(embedding 失败/Milvus 非 200/Neo4j 失败均 warn 后直穿,不阻断主链)。

## 5. 实施状态(2026-08-20)

- L1 Redis Cluster:L1Pool 6 单节点 Client + MOVED 重路由(2026-08-19 修 `withMovedRetry` 自指误跳过 bug,补 3 路径测试)。
- L2 Milvus:REST `/v2/vectordb/entities/search|insert`(2026-08-19 修端点 404 + 维度对齐 model=qwen)。
- L3 Neo4j:REST 写 Prompt→Response + 模糊查询增强(2026-08-19 实装)。
- 运行期命中率实测待 VM 雪崩恢复后补(见 memory `esp-lco-embedding-vm-snowball`)。

## 6. 后续(DEFERRED)

- 多节点迁移:Neo4j 独立部署 + Milvus 集群化 + embedding 服务容器化(MPS→GPU)。
- L2 跨协议命中重生成(当前降级直穿)。
- L2 embedding 限流/离峰调度(避免与 ESP 编译并行压爆单节点 VM)。
