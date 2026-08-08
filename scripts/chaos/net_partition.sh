#!/usr/bin/env bash
# 混沌:跨区断网模拟(模拟异地多活分区,部分服务互相不可达)
# 简化版:给两个指定 deployment 之间加 NetworkPolicy deny(需 CNI 支持),持续 N 秒后删除策略。
# 用法: scripts/chaos/net_partition.sh <deploy_a> <deploy_b> <duration_sec>
set -euo pipefail
A="${1:?usage: net_partition.sh <deploy_a> <deploy_b> <duration_sec>}"
B="${2:?deploy_b required}"
DUR="${3:-30}"
NS="${NS:-esp-prod}"
POLICY="chaos-deny-${A}-${B}"
echo "[chaos] partition ${A}<->${B} for ${DUR}s at $(date +%T)"
kubectl -n "$NS" apply -f - <<EOF 2>&1 | tail -1
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: ${POLICY}
spec:
  podSelector: {matchLabels: {app: ${A}}}
  policyTypes: [Ingress, Egress]
  ingress: []
  egress: []
EOF
sleep "$DUR"
echo "[chaos] removing partition at $(date +%T)"
kubectl -n "$NS" delete networkpolicy "$POLICY" 2>&1 | tail -1 || echo "[chaos] no policy to delete"
echo "[chaos] done at $(date +%T)"
