#!/usr/bin/env bash
# run_locust_bench.sh — 跑单个端点的 Locust 全服务基准(locust Job,集群内 headless)。
# 与 k6 同端点同条件背对背,产出该端点 RPS/P95/P99 摘要到 report/benchmark-2026-08-16/。
# 坑㉟b:loadavg 高时 kubectl exec stream 升级超时(unable to upgrade connection)→ 改 Job 模式(API 轮询,同 k6)。
# 用法: bash scripts/run_locust_bench.sh <EP> <VUS> [DUR]
#   EP  见 deploy/locust/locustfile_fullbench.py switch 清单
#   VUS 并发数(单节点勿超 200)
# 前置:configmap bench-scripts(locustfile_fullbench.py)+ esp-bench-tokens + esp-rsa-jwt-sign/esp-secrets 已建。
set -uo pipefail
EP="${1:?EP required}"
VUS="${2:?VUS required}"
DUR="${3:-45s}"
NS=esp-prod
REPORT=report/benchmark-2026-08-16
mkdir -p "$REPORT"

echo "==> locust $EP v$VUS dur=$DUR (start $(date +%H:%M:%S))"
# 坑㉟c:Job metadata.name 是 RFC1123,端点名含下划线(如 gamification_pts)→ apply 校验失败静默丢数据。
# Job 名 _ → -,文件/日志名保持原 EP。
JOBID=$(echo "$EP" | tr '_' '-')
JOB="locbench-$JOBID-$VUS"
kubectl -n $NS delete job $JOB --ignore-not-found --wait=true >/dev/null 2>&1 || true
sed -e "s/__JOB__/$JOB/g" -e "s/__EP__/$EP/g" -e "s/__VUS__/$VUS/g" -e "s/__DUR__/$DUR/g" deploy/locust/locust-job.yaml > /tmp/locust-$EP-$VUS.yaml
kubectl -n $NS apply -f /tmp/locust-$EP-$VUS.yaml >/dev/null 2>&1

if kubectl -n $NS wait --for=condition=complete job/$JOB --timeout=240s >/dev/null 2>&1; then
  kubectl -n $NS logs job/$JOB --tail=80 > "$REPORT/locust-$EP-v$VUS.log" 2>&1
  echo "==> completed $(date +%H:%M:%S)"
else
  POD=$(kubectl -n $NS get pods -l job-name=$JOB --field-selector=status.phase=Succeeded -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  if [ -z "$POD" ]; then
    POD=$(kubectl -n $NS get pods -l job-name=$JOB --field-selector=status.phase=Failed -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  fi
  if [ -n "$POD" ]; then
    kubectl -n $NS logs $POD --tail=80 > "$REPORT/locust-$EP-v$VUS.log" 2>&1
    echo "==> pod-log-captured(phase!=Running) $(date +%H:%M:%S)"
    exit 0
  fi
  kubectl -n $NS get pods -l job-name=$JOB 2>/dev/null | head -3
  echo "==> TIMEOUT/FAILED job (see kubectl describe job/$JOB)"
  exit 1
fi

# 提取 locust 最终聚合(每端点 RPS/P50/P95/P99):Test stopped 汇总行 + 响应时间百分位表
grep -E "Test stopped|percentile|Requests per second|Median|Average response|Total requests|Request failures|Fatal" "$REPORT/locust-$EP-v$VUS.log" | tail -12
echo "==> log: $REPORT/locust-$EP-v$VUS.log"
