#!/usr/bin/env bash
# Pulsar 下线混沌注入
# 验证: gamification-go rankings feed 中断 + learning-worker 消费暂停
# 策略: scale pulsar-broker StatefulSet 到 0,持续 N 秒后恢复
# 用法: scripts/chaos/pulsar_down.sh [duration_sec]
set -euo pipefail
DURATION=${1:-60}
INFRA_NS="${INFRA_NS:-ekb-m2-aws-us-east-1}"

echo "[chaos] === Pulsar 下线: ${DURATION}s at $(date +%T) ==="

# 记录当前副本数
ORIG_REPLICAS=$(kubectl -n "$INFRA_NS" get sts/pulsar-broker -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "2")
echo "原始 pulsar-broker replicas: $ORIG_REPLICAS"

# Scale to 0
kubectl -n "$INFRA_NS" scale sts/pulsar-broker --replicas=0 2>&1 | tail -1
echo "Pulsar broker scaled to 0, 等待 ${DURATION}s..."
sleep "$DURATION"

# 恢复
kubectl -n "$INFRA_NS" scale sts/pulsar-broker --replicas="$ORIG_REPLICAS" 2>&1 | tail -1
echo "Pulsar broker scaled to $ORIG_REPLICAS"
kubectl -n "$INFRA_NS" rollout status sts/pulsar-broker --timeout=180s 2>&1 | tail -2
echo "[chaos] === Pulsar 恢复完成 at $(date +%T) ==="
