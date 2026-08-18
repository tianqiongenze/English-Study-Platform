#!/usr/bin/env bash
# RedisInsight 集群连通辅助:把 redis-cluster-new 各节点 FQDN → pod IP 映射注入 RD-VM /etc/hosts。
# 背景:redis-cluster-new 宣告 cluster-announce-hostname=FQDN,mac 本机无法解析/直达,借 RedisInsight SSH 隧道经 VM 中转;
# VM 内可直达 pod IP 但解析不了 FQDN,注入 hosts 后 RedisInsight 全集群可浏览。
# 用法: scripts/redis-vm-hosts.sh     (pod IP 变化后重跑,自动替换旧块)
set -e
NS="${1:-ekb-m2-aws-us-east-1}"
IPS=$(kubectl -n "$NS" get pod -l app=redis-cluster-new \
  -o jsonpath='{range .items[*]}{.metadata.name} {.status.podIP}{"\n"}{end}')
[ -n "$IPS" ] || { echo "no redis-cluster-new pods"; exit 1; }

BLOCK=$'# BEGIN ESP-REDIS-GUI'
while read -r name ip; do
  [ -n "$name" ] && [ -n "$ip" ] || continue
  BLOCK="$BLOCK
$ip $name.redis-cluster-new.$NS.svc.cluster.local"
done <<< "$IPS"
BLOCK="$BLOCK
# END ESP-REDIS-GUI"

ssh rd-vm-127 "sudo sed -i.bak '/# BEGIN ESP-REDIS-GUI/,/# END ESP-REDIS-GUI/d' /etc/hosts
printf '%s\n' '$BLOCK' | sudo tee -a /etc/hosts >/dev/null"
echo "== verify from VM =="
ssh rd-vm-127 'getent hosts redis-cluster-new-0.redis-cluster-new.'"$NS"'.svc.cluster.local' 2>&1 | tail -2