#!/usr/bin/env bash
# ESP 宿主中间件混沌测试脚本
# 测试宿主机中间件故障对 K8s 内 ESP 服务的影响
# 用法: ./host-middleware-chaos.sh --target <service> --action <kill|pause|resume> [--yes]
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'

STATE_FILE="$HOME/observability/logs/chaos-state.json"
LOG_FILE="$HOME/observability/logs/chaos.log"
PIDS_DIR="$HOME/observability/pids"

mkdir -p ~/observability/logs

# 服务→端口+依赖映射
declare -A SVC_PORT=( [nacos]=8848 [prometheus]=9090 [loki]=3100 [alertmanager]=9093 [otel-collector]=4317 [grafana]=3000 [tempo]=3200 )
declare -A SVC_DEPS=(
    [nacos]="bff-java,learning-java,teacher-java,admin-go(governance),sentinel-rules"
    [prometheus]="grafana(dashboards),alertmanager(metrics),admin(/metrics/query)"
    [loki]="grafana(logs-panel)"
    [alertmanager]="prometheus(alerts→通知)"
    [otel-collector]="prometheus(otel-metrics),tempo(traces),loki(logs)"
    [grafana]="(可视化,无下游依赖)"
    [tempo]="grafana(traces-panel)"
)
declare -A SVC_PID_FILE=( [nacos]="$HOME/nacos/nacos/data/logs/nacos.pid" )

usage() {
    echo "用法: $0 --target <service> --action <kill|pause|resume> [--yes]"
    echo "  service: nacos prometheus loki alertmanager otel-collector grafana tempo"
    echo "  action:  kill(崩溃) pause(挂起) resume(恢复)"
    echo "  --yes:   跳过确认"
    exit 1
}

log_event() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $1"
    echo "$msg" >> "$LOG_FILE"
    echo -e "${CYAN}$msg${NC}"
}

save_state() {
    local svc=$1 action=$2 pid=$3 ts=$(date '+%Y-%m-%dT%H:%M:%S')
    python3 -c "
import json,os
f='$STATE_FILE'
state = json.load(open(f)) if os.path.exists(f) else {}
state['$svc'] = {'action':'$action','pid':'$pid','timestamp':'$ts'}
json.dump(state, open(f,'w'), indent=2)
"
}

# 参数解析
TARGET="" ACTION="" AUTO_YES=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --target) TARGET="$2"; shift 2 ;;
        --action) ACTION="$2"; shift 2 ;;
        --yes) AUTO_YES=true; shift ;;
        *) usage ;;
    esac
done

[[ -z "$TARGET" || -z "$ACTION" ]] && usage
[[ -z "${SVC_PORT[$TARGET]+x}" ]] && echo -e "${RED}未知服务: $TARGET${NC}" && usage

PORT=${SVC_PORT[$TARGET]}
DEPS=${SVC_DEPS[$TARGET]:-none}

echo -e "${YELLOW}═══════════════════════════════════════════════${NC}"
echo -e "${YELLOW}混沌测试: $TARGET → $ACTION${NC}"
echo -e "端口: $PORT"
echo -e "依赖此服务: $DEPS"
echo -e "${YELLOW}═══════════════════════════════════════════════${NC}"

if [[ "$AUTO_YES" != "true" ]]; then
    read -p "确认执行? (y/N) " confirm
    [[ "$confirm" != "y" && "$confirm" != "Y" ]] && echo "已取消" && exit 0
fi

case "$ACTION" in
    kill)
        # 找 PID
        PID=""
        if [[ -f "${SVC_PID_FILE[$TARGET]:-}" ]]; then
            PID=$(cat "${SVC_PID_FILE[$TARGET]}" 2>/dev/null || true)
        fi
        if [[ -z "$PID" ]]; then
            PID=$(lsof -iTCP:$PORT -sTCP:LISTEN -P -t 2>/dev/null | head -1 || true)
        fi
        if [[ -z "$PID" ]]; then
            echo -e "${RED}$TARGET 未运行,无需 kill${NC}"
            exit 1
        fi
        log_event "CHAOS: kill $TARGET (PID=$PID)"
        kill -9 $PID 2>/dev/null || true
        save_state "$TARGET" "killed" "$PID"
        echo -e "${RED}✗ $TARGET 已杀死 (PID=$PID)${NC}"
        ;;

    pause)
        PID=$(lsof -iTCP:$PORT -sTCP:LISTEN -P -t 2>/dev/null | head -1 || true)
        if [[ -z "$PID" ]]; then
            echo -e "${RED}$TARGET 未运行${NC}"
            exit 1
        fi
        log_event "CHAOS: pause $TARGET (PID=$PID)"
        kill -STOP $PID
        save_state "$TARGET" "paused" "$PID"
        echo -e "${YELLOW}⏸ $TARGET 已暂停 (PID=$PID)${NC}"
        ;;

    resume)
        # 读上次状态
        LAST_STATE=$(python3 -c "
import json,os
f='$STATE_FILE'
d=json.load(open(f)) if os.path.exists(f) else {}
s=d.get('$TARGET',{})
print(f\"{s.get('action','')} {s.get('pid','')}\")
" 2>/dev/null || echo "")
        LAST_ACTION=$(echo "$LAST_STATE" | awk '{print $1}')
        LAST_PID=$(echo "$LAST_STATE" | awk '{print $2}')

        if [[ "$LAST_ACTION" == "paused" ]]; then
            log_event "CHAOS: resume (SIGCONT) $TARGET (PID=$LAST_PID)"
            kill -CONT $LAST_PID 2>/dev/null || true
            echo -e "${GREEN}▶ $TARGET 已恢复 (PID=$LAST_PID)${NC}"
        elif [[ "$LAST_ACTION" == "killed" ]]; then
            echo -e "${YELLOW}$TARGET 之前被 kill,需要重启:${NC}"
            echo "  使用 scripts/start-host-middleware.sh 或手动启动"
        else
            echo -e "${YELLOW}$TARGET 没有待恢复的操作${NC}"
        fi
        ;;

    *)
        echo -e "${RED}未知 action: $ACTION${NC}"
        usage
        ;;
esac

# 等待影响传播
echo -e "\n${CYAN}等待 10s 让影响传播...${NC}"
sleep 10

# 健康检查
echo -e "\n${CYAN}═══ 影响检查 ═══${NC}"
echo -e "--- K8s Pod 状态 ---"
kubectl -n esp-prod get pods --no-headers 2>/dev/null | awk '{printf "%-40s %s\n",$1,$3}' | grep -E 'CrashLoop|Error|OOM|Pending|Unknown' | head -10 || echo "  全部正常"
echo ""
echo "--- 关键服务健康 ---"
GW_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:30080/healthz 2>/dev/null || echo "000")
BFF_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:30009/actuator/health/liveness 2>/dev/null || echo "000")
echo "  Gateway  :30080 → HTTP $GW_CODE"
echo "  BFF      :30009 → HTTP $BFF_CODE"

# 输出依赖影响
echo -e "\n${CYAN}═══ 依赖影响评估 ═══${NC}"
echo "  $TARGET 故障影响: $DEPS"
echo ""
log_event "CHAOS CHECK: gw=$GW_CODE bff=$BFF_CODE after $TARGET $ACTION"
