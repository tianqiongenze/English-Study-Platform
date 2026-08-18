#!/usr/bin/env bash
# run_remaining_leg.sh — 补齐剩余基准(k6 3 个缺口 + locust 12 个 v10 + 5 个 v20 + 混用证明)。
# 负载门控:每轮先等 loadavg<42(单节点边沿,>50 会卡 pod 创建 坑㉟b),失败重试 1 次。
# 用法: bash scripts/run_remaining_leg.sh   (后台跑,主线程只收 OK/FAIL/DONE 摘要)
set -uo pipefail
cd /Users/joelgong/claude-Projects/English-Study-Platform || exit 1
R=report/benchmark-2026-08-16
mkdir -p "$R"
LOG="$R/remaining-leg.log"
# 可恢复:append 不截断(被打断后重跑可跳过已完成端点,monitor tail -f 持续可见)
exec > >(tee -a "$LOG") 2>&1

# has_data <tool> <EP> <VUS> — 0=该端点日志已有有效数据(可跳过);1=需跑
has_data() {
  local f
  case "$1" in
    k6)     f="$R/k6-$2-v$3.log"; grep -qE "http_reqs.*[0-9]+" "$f" 2>/dev/null ;;
    locust) f="$R/locust-$2-v$3.log"; grep -qE "Test stopped\. requests=(5[0-9]{2}|[6-9][0-9]{2}|[1-9][0-9]{3,})" "$f" 2>/dev/null ;;  # 有效数据要求 >=500 请求(排除被杀的 8~217 req 残留)
    *) return 1 ;;
  esac
}

check_load() {
  local L
  L=$(timeout 22 ssh -o ConnectTimeout=10 rd-vm-127 uptime 2>/dev/null | grep -oE "load average: [0-9.]+" | grep -oE "[0-9.]+$")
  [ -n "$L" ] && [ "$(echo "$L > 42" | bc 2>/dev/null)" != "1" ]
}
gate() {
  local n=0
  until check_load; do
    n=$((n+1))
    [ $((n % 3)) -eq 0 ] && echo "$(date +%H:%M:%S) load-gate waiting"
    sleep 20
  done
}
r() { # r <tool> <EP> <VUS> <DUR> — 已有有效数据则跳过(可恢复)
  local tool=$1 ep=$2 v=$3 dur=$4
  if has_data "$tool" "$ep" "$v"; then
    echo "$(date +%H:%M:%S) SKIP $tool $ep v$v (已有数据)"
    return
  fi
  gate
  if bash "scripts/run_${tool}_bench.sh" "$ep" "$v" "$dur" >> "$LOG" 2>&1; then
    echo "$(date +%H:%M:%S) OK  $tool $ep v$v"
  else
    echo "$(date +%H:%M:%S) RETRY $tool $ep v$v"
    sleep 40; gate
    if bash "scripts/run_${tool}_bench.sh" "$ep" "$v" "$dur" >> "$LOG" 2>&1; then
      echo "$(date +%H:%M:%S) OK(retry) $tool $ep v$v"
    else
      echo "$(date +%H:%M:%S) FAIL2 $tool $ep v$v"
    fi
  fi
}

# k6 缺口(3)
for ep in teacher_kpstats vocab_review writing_grade; do
  r k6 "$ep" 10 35s
done
# locust v10 缺 17(learning_event-v10 已有 3228 请求有效数据,不重复)
for ep in learning_review teacher_kpstats gamification_prof gamification_pts vocab_due vocab_review \
          emotion_recent writingcoach_tpl assessment_report llm_dialogue course_path writing_grade \
          bff_login gw_scenarios bff_scenarios listening_mat reading_passages; do
  r locust "$ep" 10 35s
done
# locust v20 全 5
for ep in learning_event gw_scenarios bff_scenarios learning_review teacher_kpstats; do
  r locust "$ep" 20 35s
done
# 混用证明(20s;已有 mixed-* 产物则跳过,不重复混跑)
if ls "$R"/mixed-* 2>/dev/null | grep -q .; then
  echo "$(date +%H:%M:%S) SKIP mixed (已有 mixed-* 产物)"
else
  gate
  if bash scripts/run_mixed_proof.sh 20s >> "$LOG" 2>&1; then
    echo "$(date +%H:%M:%S) OK  mixed-proof"
  else
    echo "$(date +%H:%M:%S) FAIL2 mixed-proof"
  fi
fi
echo "== REMAINING LEG DONE $(date +%H:%M:%S) =="