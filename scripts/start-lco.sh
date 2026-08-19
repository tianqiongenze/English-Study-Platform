#!/usr/bin/env bash
# LCO 代理 + embedding 服务 + port-forward 三通道 一键启动(含 launchd 自愈)
# 复用 ESP Nacos-CRDB 通道的 launchd 自愈模式。
# 用法: ./start-lco.sh [start|stop|status]
set -e

ACTION="${1:-start}"
VENV="$HOME/lco-embedding-venv"
LCO_BIN="/tmp/lco-proxy-test"
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
  # neo4j
  if ! lsof -iTCP:7688 -sTCP:LISTEN >/dev/null 2>&1; then
    kubectl -n ekb-m2-aws-us-east-1 port-forward svc/neo4j 7688:7687 > /tmp/pf-neo4j.log 2>&1 &
  fi
}

start_embedding() {
  if ! lsof -iTCP:15730 -sTCP:LISTEN >/dev/null 2>&1; then
    source "$VENV/bin/activate"
    HF_ENDPOINT=https://hf-mirror.com \
    EMBED_MODEL=sentence-transformers/all-MiniLM-L6-v2 \
    LISTEN_PORT=15730 \
    nohup python3 "$PROJ/tools/lco-proxy/embedding/app.py" > /tmp/lco-embedding.log 2>&1 &
    deactivate 2>/dev/null || true
  fi
}

start_lco() {
  if ! lcof -iTCP:15720 -sTCP:LISTEN >/dev/null 2>&1; then
    LCO_REDIS_ADDR="127.0.0.1:6380,127.0.0.1:6381,127.0.0.1:6382,127.0.0.1:6383,127.0.0.1:6384,127.0.0.1:6385" \
    nohup "$LCO_BIN" > /tmp/lco-proxy.log 2>&1 &
  fi
}

stop_all() {
  pkill -f "lco-proxy-test" 2>/dev/null || true
  pkill -f "lco-proxy/embedding/app.py" 2>/dev/null || true
  pkill -f "port-forward redis-cluster-new" 2>/dev/null || true
  pkill -f "port-forward svc/milvus" 2>/dev/null || true
  pkill -f "port-forward svc/neo4j" 2>/dev/null || true
}

status_all() {
  echo "=== port-forward ==="
  for p in 6380 6381 6382 6383 6384 6385 19531 7688; do
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
