#!/usr/bin/env bash
# LCO 代理 + embedding 服务 + port-forward 三通道 一键启动(含 launchd 自愈)
# 复用 ESP Nacos-CRDB 通道的 launchd 自愈模式。
# 用法: ./start-lco.sh [start|stop|status]
set -e

ACTION="${1:-start}"
VENV="$HOME/llm-cache-embedding-venv"
LCO_BIN="/tmp/llm-cache-proxy-test"
PROJ="/Users/joelgong/claude-Projects/English-Study-Platform"

start_portforward() {
  # redis cluster 6 pod
  for i in 0 1 2 3 4 5; do
    local port=$((6380+i))
    if ! lsof -iTCP:$port -sTCP:LISTEN >/dev/null 2>&1; then
      kubectl -n ekb-m2-aws-us-east-1 port-forward redis-cluster-new-$i $port:6379 > /tmp/pf-redis-$i.log 2>&1 &
    fi
  done
  # milvus
  if ! lsof -iTCP:19531 -sTCP:LISTEN >/dev/null 2>&1; then
    kubectl -n ekb-m2-aws-us-east-1 port-forward svc/milvus 19531:19530 > /tmp/pf-milvus.log 2>&1 &
  fi
  # neo4j 已下沉宿主(brew services,7474 http / 7687 bolt 直连 127.0.0.1),删除 port-forward。
  # llm-cache-proxy 通过 LCO_NEO4J_REST_URL=http://127.0.0.1:7474 直连,无需 k8s svc。
}

start_embedding() {
  if ! lsof -iTCP:15730 -sTCP:LISTEN >/dev/null 2>&1; then
    source "$VENV/bin/activate"
    HF_ENDPOINT=https://hf-mirror.com \
    EMBED_MODEL=sentence-transformers/all-MiniLM-L6-v2 \
    LISTEN_PORT=15730 \
    nohup python3 "$PROJ/tools/llm-cache-proxy/llm-cache-embedding/app.py" > /tmp/llm-cache-embedding.log 2>&1 &
    deactivate 2>/dev/null || true
  fi
}

start_lco() {
  if ! lsof -iTCP:15720 -sTCP:LISTEN >/dev/null 2>&1; then
    # L2 语义阈值:2026-08-20 实测标定(会话23-19)。
    # qwen3-embedding-4b + Milvus COSINE + normalize_embeddings=True 下 distance 分布(大样本):
    #   完全相同 prompt          ~0.87(自匹配,因归一化压缩非 1.0)
    #   语义相近(同义改写)     0.79~0.90(均值 0.86)
    #   不相关                   0.44~0.69(均值 0.55)
    # 原默认 0.92 → 完全相同 prompt(0.87)都不命中 → L2 永远 miss(hits 不增长)。
    # 0.70 → 召回高但误命中 ~20%(返回错响应危害大)。
    # 0.80 → 误命中"科幻小说"(0.68)过滤,"闭包改写"(0.88)命中;精度优先于召回(LCO miss 仅多调 provider)。
    # 漏掉"函数调用自身"(0.79)等边缘改写——可接受(降级直穿 provider 非错误)。
    # PROGRESS 23-18 原方向 0.92→0.98 错误(更严更不可能命中)。
    LCO_L2_THRESHOLD="0.80" \
    LCO_L2_MODEL="qwen" \
    LCO_L2_COLLECTION="lco_prompts" \
    LCO_EMBED_PROTOCOL="grpc" \
    LCO_EMBED_GRPC_ADDR="127.0.0.1:15731" \
    LCO_EMBEDDING_URL="http://127.0.0.1:15730/embed" \
    LCO_REDIS_ADDR="127.0.0.1:6380,127.0.0.1:6381,127.0.0.1:6382,127.0.0.1:6383,127.0.0.1:6384,127.0.0.1:6385" \
    LCO_NEO4J_URL="bolt://127.0.0.1:7687" \
    LCO_NEO4J_REST_URL="http://127.0.0.1:7474" \
    LCO_NEO4J_USER="neo4j" \
    LCO_NEO4J_PASSWORD="neo4j-password-1" \
    nohup "$LCO_BIN" > /tmp/llm-cache-proxy.log 2>&1 &
  fi
}

stop_all() {
  pkill -f "llm-cache-proxy-test" 2>/dev/null || true
  pkill -f "llm-cache-proxy/llm-cache-embedding/app.py" 2>/dev/null || true
  pkill -f "port-forward redis-cluster-new" 2>/dev/null || true
  pkill -f "port-forward svc/milvus" 2>/dev/null || true
  # neo4j port-forward 已移除(下沉宿主直连)
}

status_all() {
  echo "=== port-forward ==="
  for p in 6380 6381 6382 6383 6384 6385 19531; do
    echo -n ":$p "; lsof -iTCP:$p -sTCP:LISTEN >/dev/null 2>&1 && echo "UP" || echo "DOWN"
  done
  echo "=== services ==="
  for p in 15720 15730; do
    echo -n ":$p "; lsof -iTCP:$p -sTCP:LISTEN >/dev/null 2>&1 && echo "UP" || echo "DOWN"
  done
}

case "$ACTION" in
  start) start_portforward; sleep 2; start_embedding; sleep 2; start_lco; echo "LCO 全栈启动完成";;
  stop) stop_all; echo "LCO 全栈已停";;
  status) status_all;;
  *) echo "用法: $0 [start|stop|status]"; exit 1;;
esac
