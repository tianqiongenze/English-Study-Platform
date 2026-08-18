#!/usr/bin/env bash
# 依赖级联故障注入
# 验证: 目标服务不可达时调用方降级行为
# 策略: 将目标服务容器端口改为不可达,观察调用方超时/降级/快速失败
# 用法: scripts/chaos/dep_cascade.sh <target_deploy> [caller_deploy] [duration_sec]
# 示例: scripts/chaos/dep_cascade.sh emotion-svc learning-java 60
set -euo pipefail
TARGET="${1:-emotion-svc}"
CALLER="${2:-learning-java}"
DURATION="${3:-60}"
NS="${NS:-esp-prod}"

echo "[chaos] === 依赖级联: $TARGET 不可达 ${DURATION}s, caller=$CALLER at $(date +%T) ==="

# 记录原始 PORT
ORIG_PORT=$(kubectl -n "$NS" get deploy "$TARGET" -o jsonpath='{.spec.template.spec.containers[0].ports[0].containerPort}' 2>/dev/null || echo "8084")
echo "原始 $TARGET PORT: $ORIG_PORT"

# 注入不可达端口
kubectl -n "$NS" set env "deploy/$TARGET" PORT=1 --containers='*' 2>&1 | tail -1
echo "$TARGET 端口已改为不可达,观察 $CALLER 降级行为..."
sleep "$DURATION"

# 恢复: undo 最近一次 rollout
kubectl -n "$NS" rollout undo "deploy/$TARGET" 2>&1 | tail -1
echo "=== 恢复 $TARGET ==="
kubectl -n "$NS" rollout status "deploy/$TARGET" --timeout=120s 2>&1 | tail -2
echo "[chaos] === 级联恢复完成 at $(date +%T) ==="
