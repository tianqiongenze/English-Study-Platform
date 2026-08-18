#!/usr/bin/env bash
# run_mixed_proof.sh — 混用证明:k6+locust 同端点并发,证明两工具可协同。
# 保留单跑纯净数据:跑前快照 k6-<ep>-v10.log/locust-<ep>-v10.log,混用产物归档为 mixed-<tool>-<ep>-v10.log,再还原。
# 用法: bash scripts/run_mixed_proof.sh [DUR]
set -uo pipefail
cd /Users/joelgong/claude-Projects/English-Study-Platform || exit 1
DUR="${1:-25s}"
R=report/benchmark-2026-08-16
mkdir -p "$R"

for ep in learning_event gw_scenarios; do
  echo "== mixed $ep ($(date +%H:%M:%S)) =="
  for tool in k6 locust; do
    f="$R/$tool-$ep-v10.log"
    [ -f "$f" ] && cp "$f" "$f.single"
  done
  (bash scripts/run_k6_bench.sh "$ep" 10 "$DUR" >/dev/null 2>&1 && echo "OK   k6-mixed $ep" || echo "FAIL k6-mixed $ep") &
  (bash scripts/run_locust_bench.sh "$ep" 10 "$DUR" >/dev/null 2>&1 && echo "OK   loc-mixed $ep" || echo "FAIL loc-mixed $ep") &
  wait
  # 归档混用产物 + 还原单跑
  for tool in k6 locust; do
    f="$R/$tool-$ep-v10.log"
    [ -f "$f" ] && cp "$f" "$R/mixed-$tool-$ep-v10.log"
    [ -f "$f.single" ] && cp "$f.single" "$f" && rm -f "$f.single"
  done
done
echo "== mixed done $(date +%H:%M:%S) =="
