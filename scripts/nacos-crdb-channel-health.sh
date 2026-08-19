#!/usr/bin/env bash
# nacos-crdb-channel-health.sh
# 层1根治: 宿主 Nacos ↔ CRDB ssh port-forward 通道的健康检查 + 自愈守护
#
# 背景:
#   - 宿主 Nacos(localhost:8848)已切 CRDB 存储,写 CRDB 走本地 127.0.0.1:26257
#   - 26257 由 ssh mux port-forward 经 lima-0 ControlPath(ssh.sock)建立
#   - 硬伤: 无 launchd 托管时,VM 重启 → ssh mux 进程消失 → 转发断 → Nacos 写 CRDB 全挂无自愈
#
# 本脚本由 launchd(com.esp.nacos-crdb-channel.plist)每 30s 调用一次,实现:
#   1. 检测 26257 是否监听(nc -z)
#   2. 不通则重连 ssh port-forward(复用 lima-0 ControlPath mux,不新建连接)
#   3. 验证 Nacos liveness 健康
#   4. 全程日志落 ~/observability/logs/nacos-crdb-channel.log
#
# 铁律: 不碰 k3s/CRDB 共享基础设施(esp-shared-infra-ironrule);仅本机用户级 ssh 转发,可逆。
#
# 局限(层2 多节点根治,DEFERRED): 单节点 Rancher Desktop 下 pod CIDR 不可从宿主直连,
#   必须经 lima VM ssh tunnel 转发到 ClusterIP。多节点服务器迁移后,pod 网络可直连,
#   届时消除 ssh tunnel,改为宿主直连 ClusterIP:26257(需 svc 改 NodePort 或 CNI pod 可达)。

set -u
umask 022

# ---- 配置 ----
LOCAL_PORT="26257"
CRDB_SVC_IP="10.43.197.186"          # esp-crdb/crdb-public ClusterIP
CRDB_SVC_PORT="26257"
# lima ssh.config: 由 Lima 生成,内含正确 ControlPath(含空格路径)+Port 59195+IdentityFile。
# 关键: 不用 -o ControlPath=... 传参(路径含空格 "Application Support" 致 ssh 解析报
# "keyword controlpath extra arguments");改用 -F 读 config 文件,ControlPath 在文件内
# 正确引号化。注意:此文件 Lima 重启会重建,脚本每次按路径读取即可。
LIMA_SSH_CONFIG="/Users/joelgong/Library/Application Support/rancher-desktop/lima/0/ssh.config"
LIMA_HOST="lima-0"                    # ssh.config 中定义,ControlPath 复用 mux
NACOS_HEALTH_URL="http://localhost:8848/nacos/v1/console/health/liveness"
LOG_DIR="/Users/joelgong/observability/logs"
LOG_FILE="${LOG_DIR}/nacos-crdb-channel.log"
MAX_LOG_BYTES=$((1024 * 1024))        # 1MB 日志轮转阈值
LOG_KEEP=3

# ---- 工具 ----
ts() { date "+%Y-%m-%dT%H:%M:%S%z"; }
log() { echo "[$(ts)] $*" >> "${LOG_FILE}"; }

# 日志轮转(简单版,避免无限膨胀)
rotate_log() {
  if [[ -f "${LOG_FILE}" ]]; then
    local sz
    sz=$(wc -c < "${LOG_FILE}" 2>/dev/null || echo 0)
    if (( sz > MAX_LOG_BYTES )); then
      local i
      for ((i = LOG_KEEP - 1; i >= 1; i--)); do
        [[ -f "${LOG_FILE}.$i" ]] && mv "${LOG_FILE}.$i" "${LOG_FILE}.$((i+1))"
      done
      mv "${LOG_FILE}" "${LOG_FILE}.1"
    fi
  fi
}

# ---- 检测 26257 是否监听 ----
check_port_listen() {
  # nc -z 成功=0,失败=非0
  nc -z -w 3 127.0.0.1 "${LOCAL_PORT}" >/dev/null 2>&1
}

# ---- 重连 ssh port-forward ----
reconnect_forward() {
  log "ACTION: 26257 不通,重连 ssh port-forward via lima-0 mux..."

  # 先清理残留监听(若有僵尸进程占端口)
  local stale_pid
  stale_pid=$(lsof -tiTCP:"${LOCAL_PORT}" -sTCP:LISTEN -P 2>/dev/null)
  if [[ -n "${stale_pid:-}" ]]; then
    log "WARN: 26257 仍有残留进程 ${stale_pid} 占用,kill 清理"
    kill -9 "${stale_pid}" >/dev/null 2>&1 || true
    sleep 1
  fi

  # 用 -F 读 lima ssh.config 建立 -L 转发(后台 -fN 不分配 tty)
  # ControlPath 在 config 文件内正确引号化,复用 ssh.sock mux,不新建 SSH 连接。
  # ExitOnForwardFailure=yes: 端口转发失败时 ssh 退出而非挂空。
  ssh -fN \
    -F "${LIMA_SSH_CONFIG}" \
    -L "${LOCAL_PORT}:${CRDB_SVC_IP}:${CRDB_SVC_PORT}" \
    -o ServerAliveInterval=15 \
    -o ServerAliveCountMax=3 \
    -o ExitOnForwardFailure=yes \
    "${LIMA_HOST}" 2>>"${LOG_FILE}"

  local rc=$?
  if (( rc == 0 )); then
    log "OK: ssh port-forward 已建立(本地 ${LOCAL_PORT} → ${CRDB_SVC_IP}:${CRDB_SVC_PORT})"
  else
    log "ERROR: ssh port-forward 建立失败 rc=${rc},尝试 kubectl port-forward 兜底"
    # 兜底:直接 kubectl port-forward(不经 lima ssh,走 RD 的 kubeconfig 上下文)
    # 用 nohup + setsid 真正脱离父进程,否则 launchd 周期触发会把它当子进程回收。
    nohup kubectl -n esp-crdb port-forward --address=127.0.0.1 svc/crdb-public \
      "${LOCAL_PORT}:${CRDB_SVC_PORT}" >>"${LOG_FILE}" 2>&1 &
    local kpf_pid=$!
    disown "${kpf_pid}" 2>/dev/null || true
    sleep 2
    if kill -0 "${kpf_pid}" 2>/dev/null && lsof -iTCP:"${LOCAL_PORT}" -sTCP:LISTEN -P >/dev/null 2>&1; then
      log "OK: kubectl port-forward 兜底已存活(PID ${kpf_pid} 监听 ${LOCAL_PORT})"
    else
      log "ERROR: kubectl port-forward 兜底未存活或未监听,需人工介入(检查 kubeconfig/RD 状态)"
    fi
  fi
  return "${rc}"
}

# ---- 验证 Nacos 健康 ----
check_nacos_health() {
  local resp
  resp=$(curl -s -m 5 "${NACOS_HEALTH_URL}" 2>/dev/null)
  if [[ "${resp}" == "OK" || "${resp}" == *"OK"* ]]; then
    log "OK: Nacos liveness 健康 (${resp})"
    return 0
  else
    log "WARN: Nacos 健康异常 resp='${resp}'(可能 CRDB 通道断致写失败,但 liveness 未必立即反映)"
    return 1
  fi
}

# ---- 主流程 ----
main() {
  rotate_log
  log "---- health check cycle ----"

  if check_port_listen; then
    log "OK: 26257 监听正常,通道存活"
  else
    log "WARN: 26257 未监听(端口不通),触发自愈重连"
    reconnect_forward
    sleep 2
    # 重连后复检
    if check_port_listen; then
      log "OK: 自愈重连后 26257 已恢复监听"
    else
      log "ERROR: 自愈重连后 26257 仍不通,等待下个 30s 周期重试"
    fi
  fi

  # Nacos 健康验证(独立于端口检测,提供应用层信号)
  check_nacos_health || true
}

main "$@"
