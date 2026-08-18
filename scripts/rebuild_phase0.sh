#!/usr/bin/env bash
# Phase0 镜像统一 rebuild + deploy
# 包含: writing-coach mock修复 + writing-py/emotion-svc 代码更新
# 前置: rd-vm docker daemon 可达(DOCKER_CONTEXT=rd-vm docker info)
# 用法: bash scripts/rebuild_phase0.sh [--dry-run]
set -euo pipefail
DRY_RUN=${1:-}
TAG_SUFFIX="${2:-phase0}"

echo "=== Phase0 镜像统一 rebuild + deploy ==="
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo

# 前置检查
if ! DOCKER_CONTEXT=rd-vm docker info >/dev/null 2>&1; then
  echo "❌ rd-vm docker daemon 不可达"; exit 1
fi
if ! kubectl get nodes >/dev/null 2>&1; then
  echo "❌ k8s API 不可达"; exit 1
fi

# 需要 rebuild 的服务（Phase0 代码变更）
declare -A SVCS=(
  ["writing-coach"]="backend/services/writing-coach:esp/writing-coach"
  ["writing-py"]="backend/services/writing-py:esp/writing-py"
  ["emotion-svc"]="backend/services/emotion-svc:esp/emotion-svc"
)

ROOT=$(cd "$(dirname "$0")/.." && pwd)

for svc in "${!SVCS[@]}"; do
  IFS=':' read -r ctx img <<< "${SVCS[$svc]}"
  # 从 manifest 读当前 tag
  cur_tag=$(kubectl -n esp-prod get deploy "$svc" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null | cut -d: -f2)
  new_tag="${cur_tag}-${TAG_SUFFIX}"
  full_img="${img}:${new_tag}"

  echo "── $svc ──"
  echo "  当前: ${img}:${cur_tag}"
  echo "  新建: ${full_img}"

  if [ "$DRY_RUN" = "--dry-run" ]; then
    echo "  [DRY-RUN] 跳过 build + deploy"
    continue
  fi

  # Build
  echo "  构建中..."
  DOCKER_CONTEXT=rd-vm docker build -t "$full_img" -f "${ROOT}/${ctx}/Dockerfile" "${ROOT}/${ctx}" 2>&1 | tail -3
  if [ ${PIPESTATUS[0]} -ne 0 ]; then
    echo "  ❌ build 失败"; exit 1
  fi

  # Deploy
  echo "  部署中..."
  kubectl -n esp-prod set image "deployment/${svc}" "${svc}=${full_img}" 2>&1
  kubectl -n esp-prod rollout status "deployment/${svc}" --timeout=120s 2>&1 | tail -3
  echo "  ✅ $svc 已更新"
  echo
done

echo "=== Phase0 rebuild 完成 ==="
echo "验证: kubectl -n esp-prod get deploy -o wide | grep -E 'writing|emotion'"
