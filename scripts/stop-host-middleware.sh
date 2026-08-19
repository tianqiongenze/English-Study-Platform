#!/usr/bin/env bash
set -euo pipefail

# ESP Host Middleware Stop Script
# Stops in reverse dependency order: Grafana -> Tempo -> OTel Collector -> Alertmanager -> Loki -> Prometheus -> Nacos

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

OBS_DIR="$HOME/observability"
PID_DIR="$OBS_DIR/pids"
LOG_DIR="$OBS_DIR/logs"
GRACE_TIMEOUT=5

stop_one() {
    local name="$1"
    local pid_file="$PID_DIR/${name}.pid"

    if [[ ! -f "$pid_file" ]]; then
        echo -e "${YELLOW}[SKIP]${NC} $name — no PID file"
        return 0
    fi

    local pid
    pid=$(cat "$pid_file")

    if ! kill -0 "$pid" 2>/dev/null; then
        echo -e "${YELLOW}[SKIP]${NC} $name — PID $pid not running"
        rm -f "$pid_file"
        return 0
    fi

    echo -e "[STOP] $name (PID $pid) ..."
    kill -TERM "$pid" 2>/dev/null || true

    local waited=0
    while kill -0 "$pid" 2>/dev/null && (( waited < GRACE_TIMEOUT )); do
        sleep 1
        waited=$((waited + 1))
    done

    if kill -0 "$pid" 2>/dev/null; then
        echo -e "  ${YELLOW}[FORCE]${NC} $name did not exit in ${GRACE_TIMEOUT}s, sending SIGKILL"
        kill -9 "$pid" 2>/dev/null || true
        sleep 1
    fi

    if ! kill -0 "$pid" 2>/dev/null; then
        echo -e "  ${GREEN}[OK]${NC} $name stopped"
    else
        echo -e "  ${RED}[FAIL]${NC} $name could not be killed"
    fi

    rm -f "$pid_file"
}

echo "=== Stopping host middleware (reverse order) ==="

# 7. Grafana
echo -e "\n--- [7] Grafana ---"
stop_one grafana

# 6. Tempo
echo -e "\n--- [6] Tempo ---"
stop_one tempo

# 5. OTel Collector
echo -e "\n--- [5] OTel Collector ---"
stop_one otelcol

# 4. Alertmanager
echo -e "\n--- [4] Alertmanager ---"
stop_one alertmanager

# 3. Loki
echo -e "\n--- [3] Loki ---"
stop_one loki

# 2. Prometheus
echo -e "\n--- [2] Prometheus ---"
stop_one prometheus

# 1. Nacos (use its own shutdown script)
echo -e "\n--- [1] Nacos ---"
NACOS_SH="$HOME/nacos/nacos/bin/shutdown.sh"
if [[ -x "$NACOS_SH" ]]; then
    echo "[STOP] Nacos (via shutdown.sh) ..."
    bash "$NACOS_SH" >>"$LOG_DIR/nacos.log" 2>&1 || true
    sleep 3
    # Verify port freed
    if lsof -i :8848 -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo -e "  ${YELLOW}[WARN]${NC} Nacos port 8848 still in use; trying to kill remaining processes"
        kill $(lsof -i :8848 -sTCP:LISTEN -t) 2>/dev/null || true
        sleep 2
    fi
    echo -e "  ${GREEN}[OK]${NC} Nacos shutdown complete"
else
    echo -e "  ${YELLOW}[SKIP]${NC} $NACOS_SH not found, trying PID-based stop"
    stop_one nacos
fi

# Cleanup any leftover PID files
rm -f "$PID_DIR"/*.pid 2>/dev/null || true

echo -e "\n========================================"
echo "  All middleware services stopped."
echo "========================================"
