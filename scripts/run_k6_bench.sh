#!/usr/bin/env bash
# run_k6_bench.sh — 跑单个 k6 端点基准(k6 Job,集群内),提取 RPS/P95/fail 摘要到 report/。
# 用法: bash scripts/run_k6_bench.sh <EP> <VUS> [DUR]
#   EP  见 deploy/k6/scenarios-fullbench.js switch 清单
#   VUS 并发数(单节点勿超 200)
#   DUR 稳态时长,默认 45s
# 前置:configmap k6-scripts(scenarios-fullbench.js)+ esp-bench-tokens(LEARNER_TOKENS/TEACHER_TOKEN)已建。
set -uo pipefail
EP="${1:?EP required}"
VUS="${2:?VUS required}"
DUR="${3:-45s}"
NS=esp-prod
REPORT=report/benchmark-2026-08-16
mkdir -p "$REPORT"

echo "==> k6 $EP v$VUS dur=$DUR (start $(date +%H:%M:%S))"
# 坑㉟c:Job metadata.name 是 RFC1123,端点名含下划线(如 teacher_kpstats)→ apply 校验失败静默丢数据。
# Job 名 _ → -,文件/日志名保持原 EP(analyze 脚本按 EP 键)。
JOBID=$(echo "$EP" | tr '_' '-')
JOB="k6bench-$JOBID-$VUS"
# 坑㉙:固定 job 名 + delete 不 --wait → 旧 Error pod 残留污染新 job 的 condition=complete → 误报 FAIL。
# 改为每次唯一 job 名 + delete --wait 清干净。
kubectl -n $NS delete job $JOB --ignore-not-found --wait=true >/dev/null 2>&1 || true
sed -e "s/__EP__/$EP/g" -e "s/__VUS__/$VUS/g" -e "s/__JOB__/$JOB/g" deploy/k6/k6.yaml > /tmp/k6-$EP-$VUS.yaml
kubectl -n $NS apply -f /tmp/k6-$EP-$VUS.yaml >/dev/null 2>&1

if kubectl -n $NS wait --for=condition=complete job/$JOB --timeout=240s >/dev/null 2>&1; then
  kubectl -n $NS logs job/$JOB --tail=60 > "$REPORT/k6-$EP-v$VUS.log" 2>&1
  echo "==> completed $(date +%H:%M:%S)"
else
  # wait 超时也可能是节点 CPU 饥饿调度慢或 k6 exit99(阈值违例):从 Succeeded/Failed pod 兜底抓日志
  # (退出码≠0 的 pod 日志仍含完整 k6 summary——rps/p95/fail 同样可用,阈值违例如实标注)。
  POD=$(kubectl -n $NS get pods -l job-name=$JOB --field-selector=status.phase=Succeeded -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  if [ -z "$POD" ]; then
    POD=$(kubectl -n $NS get pods -l job-name=$JOB --field-selector=status.phase=Failed -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  fi
  if [ -n "$POD" ]; then
    kubectl -n $NS logs $POD --tail=60 > "$REPORT/k6-$EP-v$VUS.log" 2>&1
    echo "==> pod-log-captured(phase!=Running) $(date +%H:%M:%S)"
    exit 0
  fi
  kubectl -n $NS get pods -l job-name=$JOB 2>/dev/null | head -3
  echo "==> TIMEOUT/FAILED job (see kubectl describe job/$JOB)"
  exit 1
fi

# 提取摘要行(http_reqs / http_req_duration / http_req_failed)
grep -E "http_reqs|http_req_duration|http_req_failed|iteration_duration" "$REPORT/k6-$EP-v$VUS.log" | head -8
echo "==> log: $REPORT/k6-$EP-v$VUS.log"
