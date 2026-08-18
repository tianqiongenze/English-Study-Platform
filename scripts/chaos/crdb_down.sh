#!/usr/bin/env bash
# CRDB 下线混沌注入
# 替代原 tidb_down.sh(TiDB 已下电,CRDB 为唯一数据源)
# 策略: 将指定服务的 ESP_RW_COCKROACHDB_HOST 改为不可达地址,持续 N 秒后还原
# 用法: scripts/chaos/crdb_down.sh [duration_sec] [target_deploy]
set -euo pipefail
DURATION=${1:-60}
SVC="${2:-learning-java}"
NS="${NS:-esp-prod}"
ENV_NAME="ESP_RW_COCKROACHDB_HOST"

echo "[chaos] === CRDB 下线: ${DURATION}s, target=$SVC at $(date +%T) ==="

# 保存原始 env
ORIG=$(kubectl -n "$NS" get deploy "$SVC" -o jsonpath="{.spec.template.spec.containers[0].env[?(@.name==\"$ENV_NAME\")].value}" 2>/dev/null || true)
if [ -z "$ORIG" ]; then
  # 尝试所有容器
  ORIG=$(kubectl -n "$NS" get deploy "$SVC" -o jsonpath="{range .spec.template.spec.containers[*]}{.env[?(@.name==\"$ENV_NAME\")].value}{end}" 2>/dev/null | head -1 || true)
fi
echo "原始 $ENV_NAME: $ORIG"

# 注入不可达地址
kubectl -n "$NS" set env "deploy/$SVC" "$ENV_NAME=127.0.0.1:1" --containers='*' 2>&1 | tail -1
echo "已注入不可达地址,等待 ${DURATION}s..."
sleep "$DURATION"

# 恢复
if [ -n "$ORIG" ]; then
  kubectl -n "$NS" set env "deploy/$SVC" "$ENV_NAME=$ORIG" --containers='*' 2>&1 | tail -1
else
  # 原始 env 不存在,删除注入的值
  kubectl -n "$NS" set env "deploy/$SVC" "$ENV_NAME-" --containers='*' 2>&1 | tail -1
fi
echo "已恢复 $ENV_NAME=$ORIG"
kubectl -n "$NS" rollout status "deploy/$SVC" --timeout=120s 2>&1 | tail -2
echo "[chaos] === CRDB 恢复完成 at $(date +%T) ==="
