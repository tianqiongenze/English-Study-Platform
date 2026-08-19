#!/usr/bin/env bash
# CRDB probe 回滚脚本(还原为 timeout=1s)
kubectl -n esp-crdb patch sts crdb --type=json -p='[
  {"op":"replace","path":"/spec/template/spec/containers/0/livenessProbe/timeoutSeconds","value":1},
  {"op":"replace","path":"/spec/template/spec/containers/0/livenessProbe/failureThreshold","value":5},
  {"op":"replace","path":"/spec/template/spec/containers/0/livenessProbe/periodSeconds","value":10},
  {"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/timeoutSeconds","value":1},
  {"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/failureThreshold","value":3},
  {"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/periodSeconds","value":10}
]'
echo "CRDB probe 已回滚为原值(timeout=1s)"
