#!/usr/bin/env bash
# 跨会话运行态:用 Redis 做外部缓存,而非对话历史。
# Redis 在 esp-prod 内以 ExternalName "redis-cluster" 暴露;gateway-go 容器自带 redis-cli。
# 用法: scripts/ctx_state.sh set <key> <value> | get <key> | incr <key> | del <key>
set -e
OP="${1:?op: set|get|incr|del}"; KEY="${2:?key}"; shift 2
# 规范化:key 统一以 esp:ctx: 为前缀。调用方传裸 key(如 task:bff),脚本自动拼前缀;
# 若调用方误传已带前缀的 key,去重避免 esp:ctx:esp:ctx:* 双重前缀。
RAW="${KEY#esp:ctx:}"
FULL="esp:ctx:${RAW}"
case "$OP" in
  set) VAL="$1"; kubectl -n esp-prod exec deploy/gateway-go -- redis-cli -c -h redis-cluster SET "$FULL" "$VAL" 2>/dev/null | tail -1 ;;
  get) kubectl -n esp-prod exec deploy/gateway-go -- redis-cli -c -h redis-cluster GET "$FULL" 2>/dev/null | tail -1 ;;
  incr) kubectl -n esp-prod exec deploy/gateway-go -- redis-cli -c -h redis-cluster INCR "$FULL" 2>/dev/null | tail -1 ;;
  del) kubectl -n esp-prod exec deploy/gateway-go -- redis-cli -c -h redis-cluster DEL "$FULL" 2>/dev/null | tail -1 ;;
  *) echo "unknown op $OP" >&2; exit 1 ;;
esac
