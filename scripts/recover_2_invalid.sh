#!/usr/bin/env bash
# recover_2_invalid.sh — 重跑 2 个 100%-fail 无效端点(bff_login v10 / teacher_kpstats v20)。
# 前置:等 loadavg<42 + 相关服务 HPA 收缩稳定(无 Pending/ContainerCreating/Terminating pod)。
# 用法: bash scripts/recover_2_invalid.sh  (后台跑,主线程只收 OK/FAIL 摘要)
set -uo pipefail
cd /Users/joelgong/claude-Projects/English-Study-Platform || exit 1
R=report/benchmark-2026-08-16
LOG="$R/recover-2-invalid.log"
exec > >(tee -a "$LOG") 2>&1

check_load() {
  local L
  L=$(timeout 22 ssh -o ConnectTimeout=10 rd-vm-127 uptime 2>/dev/null | grep -oE "load average: [0-9.]+" | grep -oE "[0-9.]+$")
  [ -n "$L" ] && [ "$(echo "$L > 42" | bc 2>/dev/null)" != "1" ]
}

# 服务 pod 全部 Ready 且无过渡态(Pending/ContainerCreating/Terminating)
pods_settled() {
  local bad
  bad=$(kubectl -n esp-prod get pods --no-headers 2>/dev/null | \
        grep -E "gateway-go|teacher-java|learning-java|bff-java" | \
        grep -vE "1/1 +Running" | head -3)
  [ -z "$bad" ]
}

gate() {
  local n=0
  until check_load && pods_settled; do
    n=$((n+1))
    [ $((n % 3)) -eq 0 ] && echo "$(date +%H:%M:%S) gate waiting (load/pods not settled)"
    sleep 20
  done
}

run_one() { # run_one <EP> <VUS> — 有 >=500 或 0-fail 数据即视为已恢复
  local ep=$1 v=$2
  local f="$R/locust-$ep-v$v.log"
  if grep -qE "Test stopped\. requests=([1-9][0-9]{2,})" "$f" 2>/dev/null && \
     ! grep -qE "fail_rate=1\.0000|fail_rate=0\.[1-9]" "$f" 2>/dev/null; then
    echo "$(date +%H:%M:%S) SKIP $ep v$v (已有非全败数据)"
    return
  fi
  gate
  echo "$(date +%H:%M:%S) ==> $ep v$v (start)"
  if bash "scripts/run_locust_bench.sh" "$ep" "$v" 35s >> "$LOG" 2>&1; then
    # 校验:请求数>=500 或 失败率<10%
    local req fail
    req=$(grep -oE "Test stopped\. requests=[0-9]+" "$f" 2>/dev/null | tail -1 | grep -oE "[0-9]+")
    fail=$(grep -oE "fail_rate=[0-9.]+" "$f" 2>/dev/null | tail -1 | sed 's/fail_rate=//')
    if [ -n "$req" ] && { [ "$req" -ge 500 ] || [ "$(echo "$fail < 0.10" | bc 2>/dev/null)" = "1" ]; }; then
      echo "$(date +%H:%M:%S) OK  $ep v$v (req=$req fail=$fail)"
    else
      echo "$(date +%H:%M:%S) INVALID $ep v$v (req=$req fail=$fail)"
    fi
  else
    echo "$(date +%H:%M:%S) FAIL2 $ep v$v"
  fi
}

run_one bff_login 10
run_one teacher_kpstats 20
echo "== RECOVER-2 DONE $(date +%H:%M:%S) =="
