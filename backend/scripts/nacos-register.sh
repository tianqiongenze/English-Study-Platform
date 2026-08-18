#!/bin/sh
# nacos-register.sh — Rust 服务 Nacos 注册/注销 entrypoint wrapper
# 用法: nacos-register.sh <binary> [args...]
# 环境变量: NACOS_ADDR(nacos:8848), SERVICE_NAME, POD_IP(127.0.0.1), PORT, HEALTH_URL(/actuator/health/liveness)
#
# 流程:
#   1. 启动后端服务(后台)
#   2. 等待 healthz 可达
#   3. POST 注册到 Nacos
#   4. 等待子进程退出
#   5. DELETE 注销(信号捕获 SIGTERM/SIGINT)

set -e

BINARY="$1"
shift
ARGS="$@"

NACOS_ADDR="${NACOS_ADDR:-nacos:8848}"
SERVICE_NAME="${SERVICE_NAME:-$(basename "$0")}"
POD_IP="${POD_IP:-127.0.0.1}"
PORT="${PORT:-8080}"
HEALTH_URL="${HEALTH_URL:-/actuator/health/liveness}"
HEALTH_RETRIES="${HEALTH_RETRIES:-30}"
HEALTH_INTERVAL="${HEALTH_INTERVAL:-2}"

# 日志辅助
log() { echo "[nacos-register] $*"; }

# 注册到 Nacos
register() {
  local url="http://${NACOS_ADDR}/nacos/v1/ns/instance"
  log "registering ${SERVICE_NAME} ${POD_IP}:${PORT} → ${url}"
  local resp
  resp=$(curl -s -w "\n%{http_code}" -X POST "$url" \
    -d "serviceName=${SERVICE_NAME}" \
    -d "ip=${POD_IP}" \
    -d "port=${PORT}" \
    -d "ephemeral=true" \
    -d "metadata=version%3D1.0.0" 2>&1) || true
  local http_code
  http_code=$(echo "$resp" | tail -1)
  if [ "$http_code" = "200" ]; then
    log "registered OK"
  else
    log "WARNING: registration returned HTTP $http_code (non-fatal)"
  fi
}

# 从 Nacos 注销
deregister() {
  local url="http://${NACOS_ADDR}/nacos/v1/ns/instance"
  log "deregistering ${SERVICE_NAME} ${POD_IP}:${PORT}"
  curl -s -X DELETE "$url" \
    -d "serviceName=${SERVICE_NAME}" \
    -d "ip=${POD_IP}" \
    -d "port=${PORT}" >/dev/null 2>&1 || true
  log "deregistered"
}

# 信号处理:子进程退出或收到 SIGTERM/SIGINT 时注销
cleanup() {
  deregister
  # 终止子进程
  if [ -n "$CHILD_PID" ] && kill -0 "$CHILD_PID" 2>/dev/null; then
    kill -TERM "$CHILD_PID" 2>/dev/null || true
    wait "$CHILD_PID" 2>/dev/null || true
  fi
  exit 0
}
trap cleanup TERM INT

# 启动后端服务(后台)
log "starting: $BINARY $ARGS"
$BINARY $ARGS &
CHILD_PID=$!

# 等待 healthz 可达(最多 HEALTH_RETRIES * HEALTH_INTERVAL 秒)
log "waiting for healthz on port ${PORT}..."
i=0
while [ $i -lt $HEALTH_RETRIES ]; do
  if curl -sf "http://127.0.0.1:${PORT}${HEALTH_URL}" >/dev/null 2>&1; then
    log "healthz OK after ${i} retries"
    break
  fi
  i=$((i + 1))
  sleep $HEALTH_INTERVAL
done

if [ $i -ge $HEALTH_RETRIES ]; then
  log "WARNING: healthz not ready after $((HEALTH_RETRIES * HEALTH_INTERVAL))s, registering anyway"
fi

# 注册到 Nacos
register

# 等待子进程(保持前台)
wait "$CHILD_PID" 2>/dev/null
EXIT_CODE=$?

# 子进程退出,注销
deregister
exit $EXIT_CODE
