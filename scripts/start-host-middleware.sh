#!/bin/bash
set -euo pipefail

# ESP Host Middleware Start Script
# Starts dependencies in order: Nacos -> Prometheus -> Loki -> Alertmanager -> OTel Collector -> Tempo -> Grafana

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

OBS_DIR="$HOME/observability"
LOG_DIR="$OBS_DIR/logs"
PID_DIR="$OBS_DIR/pids"

mkdir -p "$LOG_DIR" "$PID_DIR"

# --- helpers ---

port_in_use() {
    lsof -i :"$1" -sTCP:LISTEN -t >/dev/null 2>&1
}

start_one() {
    local name="$1" port="$2"
    shift 2

    if port_in_use "$port"; then
        echo -e "${YELLOW}[SKIP]${NC} $name — port $port already in use"
        return 0
    fi

    echo -e "[START] $name ..."
    "$@" >>"$LOG_DIR/${name}.log" 2>&1 &
    local pid=$!
    echo "$pid" >"$PID_DIR/${name}.pid"

    sleep 2
    if kill -0 "$pid" 2>/dev/null; then
        if port_in_use "$port"; then
            echo -e "  ${GREEN}[OK]${NC} $name running (PID $pid, port $port)"
        else
            echo -e "  ${YELLOW}[WARN]${NC} $name process alive but port $port not yet listening"
        fi
    else
        echo -e "  ${RED}[FAIL]${NC} $name exited immediately. Check $LOG_DIR/${name}.log"
        return 1
    fi
}

# --- 1. Nacos ---
echo -e "\n=== [1/7] Nacos ==="
NACOS_SH="$HOME/nacos/nacos/bin/startup.sh"
if [[ -x "$NACOS_SH" ]]; then
    if port_in_use 8848; then
        echo -e "${YELLOW}[SKIP]${NC} Nacos — port 8848 already in use"
    else
        echo "[START] Nacos (standalone) ..."
        bash "$NACOS_SH" -m standalone >>"$LOG_DIR/nacos.log" 2>&1 &
        sleep 5
        if port_in_use 8848; then
            echo -e "  ${GREEN}[OK]${NC} Nacos running (port 8848)"
        else
            echo -e "  ${RED}[WARN]${NC} Nacos may still be starting (can take 10-30s)"
        fi
    fi
else
    echo -e "  ${RED}[SKIP]${NC} $NACOS_SH not found"
fi

# --- Nacos ↔ CRDB 通道自愈(launchd 托管,无需手动) ---
# 宿主 Nacos 写 CRDB 走本地 26257,由 ssh mux port-forward 经 lima-0 转发到
# esp-crdb/crdb-public ClusterIP。VM 重启会致 ssh mux 消失,转发断。
# 已由 launchd agent com.esp.nacos-crdb-channel 每 30s 健康检查 + 自愈重连,
# 无需本脚本额外处理。详见:
#   - scripts/nacos-crdb-channel-health.sh
#   - ~/Library/LaunchAgents/com.esp.nacos-crdb-channel.plist
# 局限(层2 多节点根治,DEFERRED): 单节点 Rancher Desktop 下 pod CIDR 不可从
# 宿主直连,必须经 lima ssh tunnel。多节点迁移后改宿主直连 ClusterIP 消除 tunnel。

# --- Fix Prometheus rule_files path ---
PROM_YML="$OBS_DIR/prometheus.yml"
RULES_FILE="$OBS_DIR/rules.yml"
if [[ -f "$PROM_YML" ]] && grep -q '/etc/prometheus/rules.yml' "$PROM_YML" 2>/dev/null; then
    echo -e "${YELLOW}[FIX]${NC} Updating rule_files path in prometheus.yml"
    sed -i '' "s|/etc/prometheus/rules.yml|$RULES_FILE|g" "$PROM_YML"
    echo -e "  ${GREEN}[OK]${NC} rule_files path updated"
fi

# --- 2. Prometheus ---
echo -e "\n=== [2/7] Prometheus ==="
PROM_BIN="$OBS_DIR/prometheus"
if [[ -x "$PROM_BIN" ]]; then
    start_one prometheus 9090 \
        "$PROM_BIN" \
        --config.file="$PROM_YML" \
        --storage.tsdb.path=$HOME/observability/data/prometheus \
        --storage.tsdb.retention.time=7d \
        --web.enable-lifecycle \
        --web.enable-rule-api
else
    echo -e "  ${RED}[SKIP]${NC} $PROM_BIN not found"
fi

# --- 3. Loki ---
echo -e "\n=== [3/7] Loki ==="
LOKI_BIN="$OBS_DIR/loki"
LOKI_CFG="$OBS_DIR/loki-fixed.yaml"
if [[ -x "$LOKI_BIN" ]] && [[ -f "$LOKI_CFG" ]]; then
    start_one loki 3100 \
        "$LOKI_BIN" \
        -config.file="$LOKI_CFG"
else
    echo -e "  ${RED}[SKIP]${NC} Loki binary or config missing"
fi

# --- 4. Alertmanager ---
echo -e "\n=== [4/7] Alertmanager ==="
AM_BIN="$OBS_DIR/alertmanager"
AM_CFG="$OBS_DIR/alertmanager.yml"
if [[ -x "$AM_BIN" ]] && [[ -f "$AM_CFG" ]]; then
    start_one alertmanager 9093 \
        "$AM_BIN" \
        --config.file="$AM_CFG" \
        --storage.path=$HOME/observability/data/alertmanager
else
    echo -e "  ${RED}[SKIP]${NC} Alertmanager binary or config missing"
fi

# --- 5. OTel Collector ---
echo -e "\n=== [5/7] OTel Collector ==="
OTEL_BIN="$OBS_DIR/otelcol"
OTEL_CFG="$OBS_DIR/otel-gateway-config.yaml"
if [[ -x "$OTEL_BIN" ]] && [[ -f "$OTEL_CFG" ]]; then
    start_one otelcol 4317 \
        "$OTEL_BIN" \
        --config="$OTEL_CFG"
else
    echo -e "  ${RED}[SKIP]${NC} OTel Collector binary or config missing"
fi

# --- 6. Tempo ---
echo -e "\n=== [6/7] Tempo ==="
TEMPO_BIN="$OBS_DIR/tempo"
TEMPO_CFG="$OBS_DIR/tempo.yaml"
if [[ -f "$TEMPO_BIN" ]]; then
    TEMPO_SIZE=$(wc -c <"$TEMPO_BIN" 2>/dev/null || echo 0)
    if [[ "$TEMPO_SIZE" -le 9 ]]; then
        echo -e "  ${RED}[TEMPORARY UNAVAILABLE]${NC} Tempo binary is corrupted (${TEMPO_SIZE} bytes). Skipping."
        echo -e "  ${YELLOW}To fix: download tempo binary for darwin/arm64 from https://github.com/grafana/tempo/releases${NC}"
    elif [[ -x "$TEMPO_BIN" ]]; then
        start_one tempo 3200 \
            "$TEMPO_BIN" \
            -config.file="$TEMPO_CFG"
    else
        echo -e "  ${YELLOW}[FIX]${NC} Making Tempo binary executable"
        chmod +x "$TEMPO_BIN"
        start_one tempo 3200 \
            "$TEMPO_BIN" \
            -config.file="$TEMPO_CFG"
    fi
else
    echo -e "  ${RED}[SKIP]${NC} $TEMPO_BIN not found"
fi

# --- 7. Grafana ---
echo -e "\n=== [7/7] Grafana ==="
GRAFANA_BIN="$OBS_DIR/grafana/bin/grafana"
GRAFANA_HOME="$OBS_DIR/grafana"
GRAFANA_CFG="$OBS_DIR/grafana.ini"
if [[ -x "$GRAFANA_BIN" ]]; then
    # Ensure data dir exists
    mkdir -p $HOME/observability/data/grafana
    start_one grafana 3000 \
        "$GRAFANA_BIN" \
        -homepath="$GRAFANA_HOME" \
        -config="$GRAFANA_CFG"
else
    echo -e "  ${RED}[SKIP]${NC} $GRAFANA_BIN not found"
fi

# --- 8. Redis Cluster self-heal (VM 重启后 nodes.conf 旧 IP 自愈) ---
# 背景(memory esp-redis-cluster-nodes-conf-stale-ip):RD VM 重启后 redis-cluster-new
#   Pod 拿到新 PodIP,但 PVC 持久化的 nodes.conf 仍记旧 IP,redis 7.2 CLUSTER MEET 不认
#   hostname(只认 IP),gossip 不建链 → cluster_state:fail → 全集群 CLUSTERDOWN 拒写
#   (含 esp:ctx:* 跨会话锚点)。此步用实时 PodIP 发 CLUSTER MEET 重建 gossip。
#   幂等:cluster_state 已 ok 则跳过,不破坏稳态。只读+MEET,不 scale/重启 Redis pod
#   (esp-shared-infra-ironrule)。k3s 未起/pod 未 Ready 时脚本自跳过不阻断启动。
echo -e "\n=== [8/8] Redis Cluster self-heal ==="
SELFHEAL_SH="$HOME/claude-Projects/English-Study-Platform/scripts/redis-cluster-selfheal.sh"
if [[ -x "$SELFHEAL_SH" ]]; then
    bash "$SELFHEAL_SH"
else
    echo -e "  ${YELLOW}[SKIP]${NC} $SELFHEAL_SH not found/executable (Redis self-heal not applied)"
fi

# --- Summary ---
echo -e "\n========================================"
echo "  Middleware Startup Summary"
echo "========================================"
printf "%-18s %-8s %-12s\n" "Service" "Port" "Status"
printf "%-18s %-8s %-12s\n" "--------" "----" "------"

check_status() {
    local name="$1" port="$2"
    local pid_file="$PID_DIR/${name}.pid"
    local pid=""
    [[ -f "$pid_file" ]] && pid=$(cat "$pid_file")
    local proc_ok="no"
    [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null && proc_ok="yes"
    local port_ok="no"
    port_in_use "$port" && port_ok="yes"

    if [[ "$proc_ok" == "yes" && "$port_ok" == "yes" ]]; then
        echo -e "${GREEN}  %-18s %-8s RUNNING${NC}" "$name" "$port"
    elif [[ "$proc_ok" == "yes" ]]; then
        echo -e "${YELLOW}  %-18s %-8s STARTING${NC}" "$name" "$port"
    elif [[ "$port_ok" == "yes" ]]; then
        echo -e "${YELLOW}  %-18s %-8s PORT OK${NC}" "$name" "$port"
    else
        echo -e "${RED}  %-18s %-8s DOWN${NC}" "$name" "$port"
    fi
}

check_status "nacos"      8848
check_status "prometheus"  9090
check_status "loki"        3100
check_status "alertmanager" 9093
check_status "otelcol"     4317
check_status "tempo"       3200
check_status "grafana"     3000

echo -e "\nLogs: $LOG_DIR/"
echo -e "PIDs: $PID_DIR/"
