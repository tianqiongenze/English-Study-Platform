#!/usr/bin/env bash
# =============================================================================
# ESP Redis Cluster VM-Restart Self-Heal (Layer-1 mitigation)
# =============================================================================
# 背景(memory esp-redis-cluster-nodes-conf-stale-ip + esp-redis-cluster-hostname-announce-023):
#   redis-cluster-new(StatefulSet,6 节点 3 主 3 从,PVC 持久化 /data/nodes.conf)
#   在 RD VM 重启 / k3s 重调度后,Pod 拿到新 PodIP,但 nodes.conf 仍记旧 IP。
#   redis 7.2 CLUSTER MEET 不认 hostname(只认 IP),旧 IP 失效 → gossip 不建链
#   → cluster_state:fail → 全集群 CLUSTERDOWN 拒写(含 esp:ctx:* 锚点)。
#
# 本脚本(层1根治):VM 起来后自动用实时 PodIP 发 CLUSTER MEET,让节点互相发现
#   live IP,gossip 重建,cluster_state 转 ok。幂等:已 ok 则跳过,不破坏稳态。
#
# 安全(esp-shared-infra-ironrule):只读 + CLUSTER MEET,不 scale/重启/删 Redis pod,
#   不碰 k3s/CRDB 配置。CLUSTER MEET 对已知节点是 no-op,对健康集群无害。
#
# 局限(层2根治 DEFERRED):彻底消除 nodes.conf 旧 IP 需 sts 注入 cluster-announce-ip
#   = 实时 PodIP(initContainer 取 status.podIP)或 pod CIDR 直连,见 memory。
# =============================================================================
set -uo pipefail

NS="ekb-m2-aws-us-east-1"
APP="redis-cluster-new"
LOG_DIR="${HOME}/observability/logs"
LOG_FILE="${LOG_DIR}/redis-selfheal.log"
READY_TIMEOUT=90      # 等 Redis pod Ready 的最长时间(s)
CONVERGE_TIMEOUT=60   # 发完 MEET 后等 cluster_state 转 ok 的最长时间(s)
PORT=6379

mkdir -p "$LOG_DIR"

log()  { printf '[%s] %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$*" | tee -a "$LOG_FILE"; }
warn() { log "WARN: $*"; }
die()  { log "FAIL: $*"; exit 1; }

# --- helpers -----------------------------------------------------------------

# 打印 "<podname>\t<podIP>" 列表(只列 Ready 的 pod)
# 注:用 -o jsonpath(非 --template),后者 range .items 在本 kubectl 版本会回吐原始 JSON
pod_ip_list() {
    kubectl -n "$NS" get pod -l app="$APP" \
      -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.podIP}{"\t"}{range .status.containerStatuses[*]}{.ready}{end}{"\n"}{end}' \
      2>/dev/null | awk -F'\t' '$3=="true" && $2!="" && $2!="<nil>" {print $1"\t"$2}'
}

# 在指定 pod 上跑 redis-cli(容错)
rc_exec() {
    local pod="$1"; shift
    kubectl -n "$NS" exec "$pod" -- redis-cli -c --raw "$@" 2>/dev/null
}

cluster_info_field() {
    local pod="$1" field="$2"
    rc_exec "$pod" cluster info 2>/dev/null \
      | awk -F: -v k="$field" '$1==k {gsub(/\r/,"",$2); print $2; exit}' \
      | tr -d '[:space:]'
}

cluster_state()    { cluster_info_field "$1" "cluster_state"; }
cluster_slots_ok() { cluster_info_field "$1" "cluster_slots_ok"; }

# =============================================================================
# 主流程
# =============================================================================

# 1) 等待至少一个 Redis pod Ready(VM 刚起时 pod 可能还在 ContainerCreating)
log "=== Redis Cluster self-heal start (ns=$NS app=$APP) ==="
bootstrap=""
deadline=$(( $(date +%s) + READY_TIMEOUT ))
while :; do
    # 优先用 ordinal 0 作 bootstrap(redis-cluster-new-0),不存在则取第一个 Ready pod
    bootstrap=$(pod_ip_list | awk -F'\t' '$1 ~ /-0$/{print $1; exit}')
    [[ -z "$bootstrap" ]] && bootstrap=$(pod_ip_list | awk -F'\t' '{print $1; exit}')
    [[ -n "$bootstrap" ]] && break
    if [[ $(date +%s) -ge $deadline ]]; then
        warn "no Ready redis pod within ${READY_TIMEOUT}s — is RD/k3s up? skip self-heal"
        log "=== self-heal end (skipped: no ready pod) ==="
        exit 0   # 共享基础设施未就绪不阻断 host middleware 启动
    fi
    sleep 3
done
log "bootstrap pod = $bootstrap"

# 2) 读取稳态判定(cluster_state 与 slots_ok)
state=$(cluster_state "$bootstrap")
slots=$(cluster_slots_ok "$bootstrap")
log "initial cluster_state=$state cluster_slots_ok=$slots"

# 3) 幂等短路:cluster_state:ok 且 slots=16384 → 跳过,不动健康集群
if [[ "$state" == "ok" && "$slots" == "16384" ]]; then
    log "cluster_state:ok, slots 16384/16384 — 健康稳态,无需自愈,跳过"
    log "=== self-heal end (skipped: already ok) ==="
    exit 0
fi

# 4) 触发自愈:对每个非 bootstrap 节点用其实时 PodIP 发 CLUSTER MEET
#    CLUSTER MEET <ip> <port> — redis 7.2 只认 IP(不认 hostname,见 memory)
log "cluster_state=[$state] 非 ok 或 slots!=16384,触发自愈..."
mapfile -t NODES < <(pod_ip_list)
[[ ${#NODES[@]} -lt 2 ]] && die "less than 2 ready redis nodes, cannot heal"

meet_ok=0; meet_fail=0
for line in "${NODES[@]}"; do
    pod=$(echo "$line" | awk -F'\t' '{print $1}')
    ip=$(echo "$line"   | awk -F'\t' '{print $2}')
    [[ "$pod" == "$bootstrap" ]] && continue
    [[ -z "$ip" || "$ip" == "<nil>" ]] && { warn "$pod empty podIP, skip"; continue; }
    out=$(rc_exec "$bootstrap" CLUSTER MEET "$ip" "$PORT" 2>&1)
    if echo "$out" | grep -qi "OK"; then
        log "CLUSTER MEET $bootstrap -> $pod ($ip:$PORT) OK"
        meet_ok=$((meet_ok+1))
    else
        warn "CLUSTER MEET $bootstrap -> $pod ($ip:$PORT) => $out"
        meet_fail=$((meet_fail+1))
    fi
done
log "MEET done: ok=$meet_ok fail=$meet_fail"

# 5) 等待收敛 cluster_state:ok
deadline=$(( $(date +%s) + CONVERGE_TIMEOUT ))
final_state=""
while :; do
    final_state=$(cluster_state "$bootstrap")
    final_slots=$(cluster_slots_ok "$bootstrap")
    if [[ "$final_state" == "ok" && "$final_slots" == "16384" ]]; then
        break
    fi
    if [[ $(date +%s) -ge $deadline ]]; then
        die "cluster did not converge to ok within ${CONVERGE_TIMEOUT}s (state=$final_state slots=$final_slots) — 需人工介入:检查 redis pod 日志/nodes.conf"
    fi
    sleep 3
done

# 6) 全节点验证(不只 bootstrap,所有 Ready pod 都应 ok)
bad=0
for line in "${NODES[@]}"; do
    pod=$(echo "$line" | awk -F'\t' '{print $1}')
    s=$(cluster_state "$pod"); sl=$(cluster_slots_ok "$pod")
    if [[ "$s" != "ok" || "$sl" != "16384" ]]; then
        warn "$pod 验证不通过: state=$s slots_ok=$sl"
        bad=$((bad+1))
    fi
done
[[ $bad -gt 0 ]] && die "self-heal 后仍有 $bad 个节点 state/slots 异常"

log "自愈完成: cluster_state:ok, cluster_slots_ok:16384/16384, all ${#NODES[@]} nodes verified"
log "=== self-heal end (healed) ==="
exit 0
