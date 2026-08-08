#!/usr/bin/env bash
# 混沌实验脚本:Pod kill(随机杀服务 pod,验证自愈/重试)
# 用法: scripts/chaos/pod_kill.sh <deployment> [count]
# 例: scripts/chaos/pod_kill.sh learning-java 1
set -euo pipefail
DEPLOY="${1:?usage: pod_kill.sh <deployment> [count]}"
COUNT="${2:-1}"
NS="${NS:-esp-prod}"
echo "[chaos] killing $COUNT pod(s) of $DEPLOY in $NS at $(date +%T)"
kubectl -n "$NS" delete pod -l app="$DEPLOY" --limit="$COUNT" 2>/dev/null || \
  kubectl -n "$NS" delete pod -l app="$DEPLOY" 2>&1 | head -3
echo "[chaos] waiting for $DEPLOY to recover..."
kubectl -n "$NS" rollout status deploy/"$DEPLOY" --timeout=180s 2>&1 | tail -2
echo "[chaos] done at $(date +%T)"
