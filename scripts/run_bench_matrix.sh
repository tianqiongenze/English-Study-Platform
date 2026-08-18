#!/usr/bin/env bash
# run_bench_matrix.sh — CRDB 全服务基准矩阵(顺序执行,容忍单跑失败,全程落 report/benchmark-2026-08-16/)。
# 覆盖:
#   k6    全18端点 v10 + 核心5 v20/v60(k6 阶梯)
#   locust全18端点 v10 + 核心5 v20
#   mixed 2端点(k6+locust 同端并发,证明两工具协同)
# 运行: bash scripts/run_bench_matrix.sh [DUR]   (DUR 默认 25s)
# 完成标志: report/benchmark-2026-08-16/matrix-run.log.done
set -uo pipefail
cd /Users/joelgong/claude-Projects/English-Study-Platform || exit 1
DUR="${1:-25s}"
REPORT=report/benchmark-2026-08-16
mkdir -p "$REPORT"
LOG="$REPORT/matrix-run.log"

full=(learning_event learning_review teacher_kpstats bff_scenarios gw_scenarios \
      bff_login reading_passages listening_mat gamification_prof gamification_pts \
      vocab_due vocab_review emotion_recent writingcoach_tpl assessment_report \
      llm_dialogue course_path writing_grade)
core5=(learning_event learning_review teacher_kpstats bff_scenarios gw_scenarios)

run_k6(){ bash scripts/run_k6_bench.sh "$1" "$2" "$DUR" >/dev/null 2>&1 && echo "OK   k6   $1 v$2" || echo "FAIL k6   $1 v$2"; }
run_loc(){ bash scripts/run_locust_bench.sh "$1" "$2" "$DUR" >/dev/null 2>&1 && echo "OK   loc  $1 v$2" || echo "FAIL loc  $1 v$2"; }

{
  echo "=== k6 full matrix v10 start $(date +%H:%M:%S) ==="
  for e in "${full[@]}"; do run_k6 "$e" 10; done
  echo "=== k6 core5 ladder v20/v60 $(date +%H:%M:%S) ==="
  for e in "${core5[@]}"; do run_k6 "$e" 20; run_k6 "$e" 60; done
  echo "=== locust full matrix v10 $(date +%H:%M:%S) ==="
  for e in "${full[@]}"; do run_loc "$e" 10; done
  echo "=== locust core5 v20 $(date +%H:%M:%S) ==="
  for e in "${core5[@]}"; do run_loc "$e" 20; done
  echo "=== mixed k6+locust concurrent (learning_event / gw_scenarios) $(date +%H:%M:%S) ==="
  (bash scripts/run_k6_bench.sh learning_event 10 "$DUR" >/dev/null 2>&1 && echo "OK   k6-mixed learning_event" || echo "FAIL k6-mixed learning_event") &
  (bash scripts/run_locust_bench.sh learning_event 10 "$DUR" >/dev/null 2>&1 && echo "OK   loc-mixed learning_event" || echo "FAIL loc-mixed learning_event") &
  wait
  (bash scripts/run_k6_bench.sh gw_scenarios 10 "$DUR" >/dev/null 2>&1 && echo "OK   k6-mixed gw_scenarios" || echo "FAIL k6-mixed gw_scenarios") &
  (bash scripts/run_locust_bench.sh gw_scenarios 10 "$DUR" >/dev/null 2>&1 && echo "OK   loc-mixed gw_scenarios" || echo "FAIL loc-mixed gw_scenarios") &
  wait
  echo "=== MATRIX DONE $(date +%H:%M:%S) ==="
} > "$LOG" 2>&1

echo "=== summary ==="
grep -E "^(OK|FAIL)" "$LOG"
echo "DONE $(date +%H:%M:%S)" > "$REPORT/matrix-run.log.done"
