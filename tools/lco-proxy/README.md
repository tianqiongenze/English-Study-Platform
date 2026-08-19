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
  python3 tools/lco-proxy/embedding/app.py
# 默认:中文→Qwen3-Embedding-4B, 英文→all-MiniLM-L6-v2 自动路由
```

### 3. 构建 + 启动 LCO 代理

```bash
cd tools/lco-proxy
go build -o /tmp/lco-proxy ./cmd/lco-proxy/
LCO_REDIS_ADDR="127.0.0.1:6380,127.0.0.1:6381,..." /tmp/lco-proxy
```

一键脚本:`scripts/start-lco.sh start`

### 4. 配置工具接入

**Claude Code**:`export ANTHROPIC_BASE_URL=http://127.0.0.1:15720`

**ZCode**:provider 设置 baseURL = `http://127.0.0.1:15720`

**Continue**:`models[].apiBase = "http://127.0.0.1:15720"`

**Ollama**:`export OLLAMA_BASE_URL=http://127.0.0.1:15720`

## 架构

```
工具 → LCO 代理(:15720) → [L1 Redis / L2 Milvus / L3 Neo4j] → CC Switch(:15721) → LLM
```

## 配置项

| env | 默认 | 说明 |
|---|---|---|
| LCO_LISTEN | 127.0.0.1:15720 | 监听地址 |
| LCO_UPSTREAM_URL | http://127.0.0.1:15721 | 上游 |
| LCO_REDIS_ADDR | 127.0.0.1:6380 | L1 Redis |
| LCO_EMBEDDING_URL | http://127.0.0.1:15730/embed | embedding |
| LCO_MILVUS_ADDR | 127.0.0.1:19531 | Milvus |
| LCO_NEO4J_URL | bolt://127.0.0.1:7688 | Neo4j |
| LCO_L2_THRESHOLD | 0.92 | cosine 阈值 |

## 已知限制

- Redis Cluster 经 port-forward 接入,MOVED 目标不可达时降级直穿(多节点迁移后改 ClusterClient)
- L2 跨协议命中降级直穿(重生成 DEFERRED)
- L3 Neo4j 实际写入 DEFERRED
- 详见 `docs/design/lco-proxy-sdd.md`
