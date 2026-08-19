#!/usr/bin/env bash
# scripts/ccswitch-cluster-bridge.sh
# CC Switch 本地代理(127.0.0.1:15721)对 K8s 集群可达的桥接层。
# 集群 pod 经 host.lima.internal:15722 访问,本脚本起 socat 把 0.0.0.0:15722 -> 127.0.0.1:15721。
# launchd 每 30s 健康检查+自愈重连(模式同 nacos-crdb-channel-health.sh)。
# 关联铁律:写前核对目标系统 WTV(确认 CC Switch 代理真实可达后再切 env);上下文铁律层1根治。

set -euo pipefail

BRIDGE_PORT="${CCSWITCH_BRIDGE_PORT:-15722}"
CCSWITCH_PORT="${CCSWITCH_PORT:-15721}"
CCSWITCH_HOST="127.0.0.1"
LOG_FILE="${HOME}/Library/Logs/ccswitch-bridge.log"
PID_FILE="${HOME}/.ccswitch-bridge.pid"

mkdir -p "$(dirname "$LOG_FILE")"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >> "$LOG_FILE"; }

# 1. 检查 CC Switch 代理是否存活(15721 监听)
if ! lsof -nP -iTCP:"$CCSWITCH_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  log "WARN: CC Switch 代理 $CCSWITCH_HOST:$CCSWITCH_PORT 未监听,跳过桥接(等 CC Switch 启动)"
  exit 0
fi

# 2. 检查桥接是否已存活(幂等)
if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
  # 验证端口确实在听
  if lsof -nP -iTCP:"$BRIDGE_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    exit 0  # 已健康,无需操作
  fi
  # 端口没听但进程还在,杀掉重启
  kill "$(cat "$PID_FILE")" 2>/dev/null || true
  rm -f "$PID_FILE"
  log "INFO: 桥接进程僵死,重启"
fi

# 3. 启动 socat 桥接(后台,fork 处理多连接)
nohup /opt/homebrew/bin/socat "TCP-LISTEN:${BRIDGE_PORT},bind=0.0.0.0,reuseaddr,fork" "TCP:${CCSWITCH_HOST}:${CCSWITCH_PORT}" >> "$LOG_FILE" 2>&1 &
NEW_PID=$!
echo "$NEW_PID" > "$PID_FILE"
sleep 1

# 4. 验证启动
if lsof -nP -iTCP:"$BRIDGE_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  log "OK: socat 桥接启动 PID=$NEW_PID 0.0.0.0:${BRIDGE_PORT} -> ${CCSWITCH_HOST}:${CCSWITCH_PORT}"
else
  log "ERROR: socat 桥接启动失败 PID=$NEW_PID"
  rm -f "$PID_FILE"
  exit 1
fi
