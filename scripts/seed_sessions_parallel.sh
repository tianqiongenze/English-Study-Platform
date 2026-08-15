#!/bin/bash
# 批量灌 sessions STRING 数据到 CRDB
# 并发度=PARALLEL,每批 BATCH 行,总目标 TARGET 行
# id 区间不重叠:每进程领 [base + i*step, base + (i+1)*step)
set -u
PARALLEL=${PARALLEL:-4}
BATCH=${BATCH:-20000}
TARGET=${TARGET:-17000000}
BASE=${BASE:-1000000}   # 起始 id 数字,避免与测试段冲突
LOG=/tmp/seed_sessions_progress.log

echo "=== seed_sessions_parallel.sh START $(date) PARALLEL=$PARALLEL BATCH=$BATCH TARGET=$TARGET ===" | tee "$LOG"

total_done=0
batch_idx=0
while [ "$total_done" -lt "$TARGET" ]; do
  # 起 PARALLEL 个并行
  pids=""
  for p in $(seq 1 "$PARALLEL"); do
    batch_idx=$((batch_idx+1))
    S=$(( BASE + (batch_idx-1)*BATCH ))
    E=$(( BASE + batch_idx*BATCH - 1 ))
    if [ "$E" -gt $((BASE + 100000000)) ]; then
      echo "!! id 区间即将耗尽,BASE 需上调" | tee -a "$LOG"
      break 2
    fi
    kubectl -n esp-crdb exec crdb-0 -- timeout 180 cockroach sql --insecure --host crdb-public --execute "
INSERT INTO esp_speaking.sessions (id, user_id, scenario_id, status, started_at, ended_at)
SELECT 'sess_' || g::STRING,
  floor(random()*2000000+1)::INT8,
  floor(random()*15000+1)::INT8,
  CASE WHEN random() < 0.6 THEN 'active' ELSE 'ended' END,
  now() - (random() * interval '365 days'),
  CASE WHEN random() < 0.6 THEN NULL ELSE now() - (random() * interval '360 days') END
FROM generate_series(${S}, ${E}) AS g;" >/dev/null 2>&1 &
    pids="$pids $!"
  done
  # 等这批完成
  fail=0
  for pid in $pids; do
    wait "$pid" || fail=$((fail+1))
  done
  total_done=$(( total_done + PARALLEL*BATCH ))
  echo "[$(date +%H:%M:%S)] 批次组#$((batch_idx/PARALLEL)) 完成,累计约 $total_done / $TARGET (失败进程:$fail)" | tee -a "$LOG"
  if [ "$fail" -gt 0 ]; then
    echo "!! 有失败进程,继续(已提交的行已落盘)" | tee -a "$LOG"
  fi
done

echo "=== DONE $(date) 累计约 $total_done ===" | tee -a "$LOG"
