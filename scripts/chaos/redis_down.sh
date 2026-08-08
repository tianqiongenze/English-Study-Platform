#!/usr/bin/env bash
# 混沌:Redis Cluster 下线模拟(给 gateway-go 注入 Redis 不可达)
# 策略:临时把 redis-cluster ExternalName 指向一个不存在地址,或用 NetworkPolicy 阻断。
# 简化版:用 kubectl 临时改 gateway-go 的 REDIS_HOST 环境变量为不可达地址,持续 N 秒后还原。
# 用法: scripts/chaos/redis_down.sh <duration_sec>
set -euo pipefail
DUR="${1:-30}"
NS="${NS:-esp-prod}"
echo "[chaos] simulating redis unavailability for ${DUR}s at $(date +%T)"
# 备份当前副本数,用 network policy 不可行(无 CNI 插件),改用临时 patch readinessProbe 失败的 env 注入
kubectl -n "$NS" set env deploy/gateway-go REDIS_HOST=127.0.0.1:1 --containers='*' 2>&1 | tail -1
echo "[chaos] waiting ${DUR}s while requests should fail/degrade..."
sleep "$DUR"
echo "[chaos] restoring REDIS_HOST=redis-cluster at $(date +%T)"
kubectl -n "$NS" set env deploy/gateway-go REDIS_HOST=redis-cluster --containers='*' 2>&1 | tail -1
kubectl -n "$NS" rollout status deploy/gateway-go --timeout=120s 2>&1 | tail -2
echo "[chaos] restored at $(date +%T)"
