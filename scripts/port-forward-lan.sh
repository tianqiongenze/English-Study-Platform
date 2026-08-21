#!/usr/bin/env bash
# ESP 局域网 port-forward 持久化(自动重连)。RD NodePort 不自动转发 Mac,需 port-forward --address 0.0.0.0。
# 用法:bash scripts/port-forward-lan.sh start|stop|status
set -e
ACTION="${1:-start}"
# 端口:svc:targetPort
PORTS="30070:frontend-portal-np:80 30080:gateway-go-np:80 30090:frontend-react-np:80 30091:frontend-h5-np:80 30009:bff-java-np:8090 30099:esp-admin-np:80"

start() {
  for item in $PORTS; do
    port=${item%%:*}; rest=${item#*:}; svc=${rest%%:*}; target=${rest##*:}
    pkill -f "port-forward svc/$svc " 2>/dev/null || true
    # 后台自动重连循环
    nohup bash -c "while true; do kubectl -n esp-prod port-forward svc/$svc $port:$target --address 0.0.0.0 > /tmp/pf-lan-$port.log 2>&1; sleep 2; done" > /dev/null 2>&1 &
    echo "  :$port → svc/$svc:$target (后台自动重连)"
  done
}

stop() {
  pkill -f "port-forward svc/" 2>/dev/null || true
  # 杀重连循环
  for item in $PORTS; do pkill -f "port-forward svc/${item#*:} " 2>/dev/null || true; done
  echo "port-forward 全停"
}

status() {
  for item in $PORTS; do
    port=${item%%:*}; echo -n "  :$port "; lsof -iTCP:$port -sTCP:LISTEN >/dev/null 2>&1 && echo "UP" || echo "DOWN"
  done
}

case "$ACTION" in start) start;; stop) stop;; status) status;; *) echo "用法: $0 start|stop|status"; exit 1;; esac
