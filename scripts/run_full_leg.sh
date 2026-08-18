#!/usr/bin/env bash
# run_full_leg.sh — 同窗口(同一负载条件)全矩阵双工具重跑,保证 CRDB 对比/全语言对比/k6-locust 相关的诚实可比。
# 1) 快照旧 k6 日志(07:20 低负载段)到 .aug16-0730 保留
# 2) k6 全 18×v10(重跑,与 locust 同条件)
# 3) locust 全 18×v10(坑㉝ 修复后)
# 4) core5(learning_event/gw_scenarios/bff_scenarios/learning_review/teacher_kpstats)×v20 双工具
# 5) 混用证明(learning_event/gw_scenarios,k6+locust 并发)
# 产物全落 report/benchmark-2026-08-16/;逐端点 fail 才打印 FAIL(不中断)。
# 用法: bash scripts/run_full_leg.sh   (后台跑,主线程只收 FAIL/DONE 摘要)
set -uo pipefail
cd /Users/joelgong/claude-Projects/English-Study-Platform || exit 1
R=report/benchmark-2026-08-16
mkdir -p "$R"
LEG_LOG="$R/leg-run.log"
: > "$LEG_LOG"
exec > >(tee "$LEG_LOG") 2>&1

FULL=(learning_event learning_review teacher_kpstats bff_scenarios gw_scenarios bff_login
      reading_passages listening_mat gamification_prof gamification_pts vocab_due vocab_review
      emotion_recent writingcoach_tpl assessment_report llm_dialogue course_path writing_grade)
CORE5=(learning_event gw_scenarios bff_scenarios learning_review teacher_kpstats)

# 0) 快照旧 k6 低负载段(不覆盖)
for f in "$R"/k6-*.log; do
  [ -f "$f" ] && cp "$f" "${f%.log}.aug16-0730" 2>/dev/null || true
done
echo "$(date +%H:%M:%S) snapshot done: $(ls "$R"/*.aug16-0730 2>/dev/null | wc -l | tr -d ' ') k6 files"

# 1) k6 全 18 × v10 重跑
for ep in "${FULL[@]}"; do
  if bash scripts/run_k6_bench.sh "$ep" 10 45s >> "$LEG_LOG" 2>&1; then
    echo "$(date +%H:%M:%S) OK  k6 $ep v10"
  else
    echo "$(date +%H:%M:%S) FAIL k6 $ep v10"
  fi
done
# 2) locust 全 18 × v10 重跑
for ep in "${FULL[@]}"; do
  if bash scripts/run_locust_bench.sh "$ep" 10 45s >> "$LEG_LOG" 2>&1; then
    echo "$(date +%H:%M:%S) OK  locust $ep v10"
  else
    echo "$(date +%H:%M:%S) FAIL locust $ep v10"
  fi
done
# 3) core5 × v20 双工具
for ep in "${CORE5[@]}"; do
  if bash scripts/run_k6_bench.sh "$ep" 20 45s >> "$LEG_LOG" 2>&1; then
    echo "$(date +%H:%M:%S) OK  k6 $ep v20"
  else
    echo "$(date +%H:%M:%S) FAIL k6 $ep v20"
  fi
done
for ep in "${CORE5[@]}"; do
  if bash scripts/run_locust_bench.sh "$ep" 20 45s >> "$LEG_LOG" 2>&1; then
    echo "$(date +%H:%M:%S) OK  locust $ep v20"
  else
    echo "$(date +%H:%M:%S) FAIL locust $ep v20"
  fi
done
# 4) 混用证明(learning_event/gw_scenarios,k6+locust 同端点并发)
bash scripts/run_mixed_proof.sh 25s >> "$LEG_LOG" 2>&1 \
  && echo "$(date +%H:%M:%S) OK  mixed-proof" \
  || echo "$(date +%H:%M:%S) FAIL mixed-proof"
echo "== FULL LEG DONE $(date +%H:%M:%S) =="