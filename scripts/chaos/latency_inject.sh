#!/usr/bin/env bash
# 混沌:网络延迟注入(给 learning-java 容器加 tc netem 延迟,验证超时降级)
# 需容器有 NET_ADMIN cap。若 pod 无特权则跳过并提示。
# 用法: scripts/chaos/latency_inject.sh <deployment> <latency_ms> <duration_sec>
set -euo pipefail
DEPLOY="${1:?usage: latency_inject.sh <deploy> <latency_ms> <duration_sec>}"
LAT="${2:-500}"
DUR="${3:-30}"
NS="${NS:-esp-prod}"
POD="$(kubectl -n "$NS" get pod -l app="$DEPLOY" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
[ -z "$POD" ] && { echo "[chaos] no pod for $DEPLOY"; exit 1; }
echo "[chaos] injecting ${LAT}ms latency on $POD for ${DUR}s at $(date +%T)"
kubectl -n "$NS" exec "$POD" -- sh -c "tc qdisc add dev eth0 root netem delay ${LAT}ms 2>/dev/null || (apk add -q iproute2 2>/dev/null; tc qdisc add dev eth0 root netem delay ${LAT}ms)" 2>&1 | tail -2 || \
  echo "[chaos] WARN: tc unavailable (no NET_ADMIN), skip"
sleep "$DUR"
echo "[chaos] removing latency at $(date +%T)"
kubectl -n "$NS" exec "$POD" -- sh -c "tc qdisc del dev eth0 root 2>/dev/null" 2>&1 | tail -1 || true
echo "[chaos] done at $(date +%T)"
