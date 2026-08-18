#!/bin/bash
set -euo pipefail

# ESP Host Middleware Status Check Script
# Checks process, port, and health for all middleware services

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PID_DIR="$HOME/observability/pids"

port_in_use() {
    lsof -i :"$1" -sTCP:LISTEN -t 2>/dev/null | head -1
}

pid_alive() {
    local pid_file="$PID_DIR/$1.pid"
    if [[ -f "$pid_file" ]]; then
        local pid
        pid=$(cat "$pid_file")
        kill -0 "$pid" 2>/dev/null && echo "$pid" && return 0
    fi
    return 1
}

health_check() {
    local url="$1" timeout=3
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$timeout" "$url" 2>/dev/null || echo "000")
    echo "$code"
}

# Collect results
declare -a NAMES PORTS PROC PIDVals HEALTH HCODES STATUS

services=(
    "nacos|8848|http://localhost:8848/nacos/"
    "prometheus|9090|http://localhost:9090/-/healthy"
    "loki|3100|http://localhost:3100/ready"
    "alertmanager|9093|http://localhost:9093/-/healthy"
    "otelcol|4317|http://localhost:8889/metrics"
    "tempo|3200|http://localhost:3200/ready"
    "grafana|3000|http://localhost:3000/api/health"
)

echo "========================================"
echo "  ESP Host Middleware Status"
echo "========================================"
echo ""
printf "%-15s %-8s %-8s %-10s %-8s %s\n" "Service" "Port" "PID" "HTTP" "Health" "Status"
printf "%-15s %-8s %-8s %-10s %-8s %s\n" "--------" "----" "---" "----" "------" "------"

for svc in "${services[@]}"; do
    IFS='|' read -r name port url <<< "$svc"

    # Port
    port_pid=$(port_in_use "$port")
    port_ok="NO"
    [[ -n "$port_pid" ]] && port_ok="YES"

    # PID from PID file
    pid_val=""
    if pid_alive "$name"; then
        pid_val=$(pid_alive "$name")
    fi

    # HTTP health check
    hcode="N/A"
    health_str="N/A"
    if [[ "$port_ok" == "YES" ]]; then
        hcode=$(health_check "$url")
        if [[ "$hcode" =~ ^2[0-9]{2}$ ]]; then
            health_str="OK"
        elif [[ "$hcode" == "000" ]]; then
            health_str="TIMEOUT"
        else
            health_str="ERR $hcode"
        fi
    fi

    # Overall status
    overall="DOWN"
    color="$RED"
    if [[ "$port_ok" == "YES" && "$health_str" == "OK" ]]; then
        overall="UP"
        color="$GREEN"
    elif [[ "$port_ok" == "YES" ]]; then
        overall="DEGRADED"
        color="$YELLOW"
    elif [[ -n "$pid_val" ]]; then
        overall="STARTING"
        color="$YELLOW"
    fi

    printf "%-15s %-8s %-8s %-10s %-8s " "$name" "$port" "${pid_val:--}" "$hcode" "$health_str"
    echo -e "${color}${overall}${NC}"
done

echo ""
echo "--- Additional Checks ---"

# Tempo binary health
TEMPO_BIN="$HOME/observability/tempo"
if [[ -f "$TEMPO_BIN" ]]; then
    TEMPO_SIZE=$(wc -c <"$TEMPO_BIN" 2>/dev/null || echo 0)
    if [[ "$TEMPO_SIZE" -le 9 ]]; then
        echo -e "${RED}  Tempo: binary corrupted (${TEMPO_SIZE} bytes) — TEMPORARY UNAVAILABLE${NC}"
    else
        echo -e "  Tempo: binary OK (${TEMPO_SIZE} bytes)"
    fi
else
    echo -e "  Tempo: binary not found"
fi

# Disk usage for data dirs
echo ""
echo "--- Data Directory Usage ---"
for dir in /tmp/prometheus-data /tmp/loki-data /tmp/alertmanager-data /tmp/grafana; do
    if [[ -d "$dir" ]]; then
        size=$(du -sh "$dir" 2>/dev/null | cut -f1)
        printf "  %-25s %s\n" "$dir" "$size"
    else
        printf "  %-25s %s\n" "$dir" "(not created)"
    fi
done

echo ""
echo "PID dir: $PID_DIR"
echo "========================================"
