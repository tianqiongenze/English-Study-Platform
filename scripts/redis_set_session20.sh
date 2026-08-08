#!/usr/bin/env bash
# 会话20 S4~S7 恢复锚点写入 Redis Cluster
# Key: esp:ctx:hot:2026-08-08-session20-s4-s7-batch2
# 用法: bash scripts/redis_set_session20.sh
set -e

KEY="esp:ctx:hot:2026-08-08-session20-s4-s7-batch2"
VAL_FILE="$(cd "$(dirname "$0")/.." && pwd)/tmp/esp-session20-redis-value.json"

if [[ ! -f "$VAL_FILE" ]]; then
  echo "ERR: value file not found: $VAL_FILE" >&2
  exit 1
fi

echo "=== SET $KEY ==="
cat "$VAL_FILE" | kubectl -n ekb-m2-aws-us-east-1 exec -i deploy/redis-cluster -- redis-cli -c -x SET "$KEY" 2>&1 | tail -3

echo "=== GET verify (first 200 chars) ==="
kubectl -n ekb-m2-aws-us-east-1 exec deploy/redis-cluster -- redis-cli -c GET "$KEY" 2>/dev/null | head -c 200
echo
echo "=== done ==="
