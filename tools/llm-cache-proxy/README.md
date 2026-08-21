# LCO 代理(LCO = LLM Cache Optimization)

LLM 多级缓存代理 —— 给本机编码工具(Claude Code / ZCode / Continue / Cline / Aider / Ollama 客户端)接入三层语义缓存,落地 LCO 铁律。

## 快速开始

### 1. 启动 port-forward 三通道(集群存储 → 宿主)

```bash
# Redis Cluster(6 pod)
for i in 0 1 2 3 4 5; do
  kubectl -n ekb-m2-aws-us-east-1 port-forward redis-cluster-new-$i $((6380+i)):6379 &
done
# Milvus + Neo4j
kubectl -n ekb-m2-aws-us-east-1 port-forward svc/milvus 19531:19530 &
kubectl -n ekb-m2-aws-us-east-1 port-forward svc/neo4j 7688:7687 &
```

### 2. 启动 embedding 服务(宿主 python venv,MPS 加速)

```bash
source ~/lco-embedding-venv/bin/activate
HF_ENDPOINT=https://hf-mirror.com \
  python3 tools/llm-cache-proxy/llm-cache-embedding/app.py
# 默认:中文→Qwen3-Embedding-4B, 英文→all-MiniLM-L6-v2 自动路由
```

### 3. 构建 + 启动 LCO 代理

```bash
cd tools/llm-cache-proxy
go build -o /tmp/llm-cache-proxy ./cmd/llm-cache-proxy/
LCO_REDIS_ADDR="127.0.0.1:6380,127.0.0.1:6381,..." /tmp/llm-cache-proxy
```

一键脚本:`scripts/start-lco.sh start`

### 4. 配置工具接入

完整接入指南(含 Claude Code / ZCode / Codex / Continue / Cline / Aider / Ollama 的 env、配置文件、验证命令、排错):**`docs/design/llm-cache-proxy-tool-integration.md`**。

快速接入(Claude Code 示例):

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:15720
export ANTHROPIC_API_KEY=PROXY_MANAGED   # 代理透传,真实凭据由 CC Switch 运行时注入,禁入 git
claude   # 启动,LLM 调用即走 LCO 三层缓存
```

验证缓存命中(发两次相同请求,第二次应返回 `X-Lco-Cache-Hit: L1`):

```bash
curl -s -D - -X POST http://127.0.0.1:15720/v1/messages \
  -H "Content-Type: application/json" -H "x-api-key: PROXY_MANAGED" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"claude-sonnet-4-6","max_tokens":50,"messages":[{"role":"user","content":"hi"}]}' \
  | grep -i X-Lco-Cache-Hit
```

## 架构

```
工具 → LCO 代理(:15720) → [L1 Redis / L2 Milvus / L3 Neo4j] → CC Switch(:15721) → LLM
```

## 配置项

| env | 默认 | 说明 |
|---|---|---|
| LCO_LISTEN | 127.0.0.1:15720 | 监听地址 |
| LCO_UPSTREAM_URL | http://127.0.0.1:15721 | 上游 |
| LCO_REDIS_ADDR | 127.0.0.1:6380(逗号分隔多 pod) | L1 Redis(L1Pool MOVED 重路由) |
| LCO_EMBEDDING_URL | http://127.0.0.1:15730/embed | embedding(zh→Qwen3-Embedding-4B / en→all-MiniLM) |
| LCO_MILVUS_ADDR | 127.0.0.1:19531 | Milvus L2 |
| LCO_NEO4J_URL | bolt://127.0.0.1:7688 | Neo4j L3 |
| LCO_L2_THRESHOLD | 0.92 | cosine 阈值 |
| LCO_L2_ENABLED | true | L2 语义缓存开关 |
| LCO_L3_ENABLED | true | L3 知识图谱开关 |

## 已知限制

- L1 Redis Cluster 经 port-forward 接入,L1Pool 维护 6 单节点 Client + MOVED 重路由。**2026-08-19 实测暴露 bug**:原 `withMovedRetry` 的 `podIdx != startIdx` 条件在 MOVED 指向 startIdx 自身时误跳过重试直接返回错误,真实命中率仅 ~50%(非声称的 100%)。已修(去掉误跳过 + 改为多 pod 遍历容错),补 3 个路径测试(TestL1PoolMovedRerouteSelfHost/AllMoved/NonMovedErrorContinues)全过,`go vet` 干净。**运行期命中率实测待集群恢复后补**(本次实测因 VM 雪崩打断)。
- L2 embedding(Qwen3-Embedding-4B 8GB)在 MPS 上单次推理 8-34s,`LCO_L2_TIMEOUT` 默认 20s;首请求可能超时降级直穿,常驻 MPS 后 <100ms。
- **雪崩坑**:embedding 推理 + ESP 编译并行会压爆 9 核单节点 VM(load 飙至 543 致 K8s API TLS 超时),见 memory `esp-lco-embedding-vm-snowball`。embedding 必须限流/离峰/不与编译并行。
- L2 跨协议命中当前降级直穿(重生成 DEFERRED,需 LLM 二次调用)
- ESP 服务侧 Phase2(L2 Milvus)/Phase3(L3 Neo4j)在 conversation-go/writing-coach/orchestrator-rs 三服务落地中,共用 `esp_llm_semantic` collection(orchestrator-rs 部署已 apply,运行期日志验证待集群恢复)
- 详见 `docs/design/llm-cache-proxy-sdd.md`
