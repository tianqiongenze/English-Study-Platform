#!/usr/bin/env bash
# 混沌:TiDB 下线模拟(读副本不可达)
# 策略:临时把 learning-java 的读副本数据源指向不可达地址,持续 N 秒后还原。
# 用法: scripts/chaos/tidb_down.sh <duration_sec>
set -euo pipefail
DUR="${1:-30}"
NS="${NS:-esp-prod}"
echo "[chaos] simulating TiDB follower unavailability for ${DUR}s at $(date +%T)"
kubectl -n "$NS" set env deploy/learning-java READ_DB_URL=jdbc:mysql://127.0.0.1:1/esp --containers='*' 2>&1 | tail -1
sleep "$DUR"
echo "[chaos] restoring read db at $(date +%T)"
kubectl -n "$NS" set env deploy/learning-java READ_DB_URL- --containers='*' 2>&1 | tail -1
kubectl -n "$NS" rollout status deploy/learning-java --timeout=180s 2>&1 | tail -2
echo "[chaos] restored at $(date +%T)"
