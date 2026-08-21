#!/usr/bin/env bash
# p1-d10-hpa-watch.sh — P1-D10 阶梯压测期间每 10s 采 HPA + top pod 快照。
#
# 用途:与 deploy/locust/p1_d10_hpa_staircase.py 时间轴对齐,记录每档 CCU 下 HPA
# replicas 变化,用于算"扩容触发延迟"(从 CPU 超 target 到 replicas+1 的秒数)。
#
# 运行(主线程串行控制,与 locust 同时起、压测停后 Ctrl-C 本脚本):
#   bash scripts/p1-d10-hpa-watch.sh [esp-prod] [report/loadtest/hpa-snapshots.log]
#
# 输出列(每 10s 一块):
#   == ts=<epoch> stage_mark=<CCU档> ==  (stage_mark 由环境变量 STAGE_MARK 注入,可选)
#   kubectl get hpa(所有 HPA 的 REFERENCE/TARGETS/MINPODS/MAXPODS/REPLICAS/AGE)
#   kubectl top pod(CPU millicores/内存 Mi,看哪些 pod 触发扩容)
#
# 依赖:kubectl context=rancher-desktop, metrics-server 已装(top pod 需要)。
set -euo pipefail

NS="${1:-esp-prod}"
OUT="${2:-report/loadtest/hpa-snapshots.log}"
INTERVAL="${HPA_WATCH_INTERVAL:-10}"

mkdir -p "$(dirname "$OUT")"

echo ">>> HPA watch start ns=$NS interval=${INTERVAL}s out=$OUT" | tee "$OUT"
echo ">>> 配合 locust 脚本: deploy/locust/p1_d10_hpa_staircase.py" | tee -a "$OUT"
echo ">>> 阶梯档: 100→500→1000→2000→5000 CCU, 每档 ramp30s+hold120s" | tee -a "$OUT"
echo ">>> 看扩容触发延迟: 同档内 REPLICAS 从 N→N+1 的时间差" | tee -a "$OUT"
echo "" | tee -a "$OUT"

while true; do
  ts=$(date +%s)
  mark="${STAGE_MARK:-}"
  {
    echo "============================================================"
    echo "== ts=${ts} $(date '+%H:%M:%S') stage_mark=${mark} =="
    echo "--- kubectl get hpa -n ${NS} ---"
    kubectl -n "${NS}" get hpa 2>&1 || echo "(get hpa failed)"
    echo "--- kubectl top pod -n ${NS} (sorted by cpu) ---"
    # top pod 按需;metrics-server 不可用时降级跳过不中断
    kubectl -n "${NS}" top pod --sort-by=cpu 2>&1 | head -30 || echo "(top pod failed, metrics-server 未就绪?)"
    echo ""
  } >> "$OUT" 2>&1
  sleep "${INTERVAL}"
done
