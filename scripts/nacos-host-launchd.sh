#!/bin/bash
# nacos-host-launchd.sh — launchd 托管宿主 Nacos 前台阻塞式启动包装
# 设计对标 brew neo4j/minio:exec 替换进程让 launchd 直接持有 java PID,
# 进程挂了 launchd KeepAlive 立即重启。不用 startup.sh 的 nohup fork-disown(管不住)。
# 2026-08-20 会话23-17,nacos 下沉宿主机
set -euo pipefail

NACOS_HOME="${HOME}/nacos/nacos"
JAVA_HOME="/Users/joelgong/Library/Java/JavaVirtualMachines/azul-21.0.5/Contents/Home"
JAVA="${JAVA_HOME}/bin/java"
LOG_DIR="${HOME}/observability/logs/nacos-host-launchd"

mkdir -p "${LOG_DIR}"

# 若已有 nacos 进程在跑(裸 java 或前次 launchd 残留),先停掉避免端口冲突
EXISTING_PID=$(pgrep -f 'nacos-server.jar' | head -1 || true)
if [[ -n "${EXISTING_PID}" && "${EXISTING_PID}" != "$$" ]]; then
  echo "$(date '+%F %T') [launchd] 检测到既有 nacos PID=${EXISTING_PID},先优雅停止" >&2
  kill "${EXISTING_PID}" 2>/dev/null || true
  sleep 3
  if kill -0 "${EXISTING_PID}" 2>/dev/null; then
    echo "$(date '+%F %T') [launchd] 3s 后仍在,强制 kill" >&2
    kill -9 "${EXISTING_PID}" 2>/dev/null || true
    sleep 1
  fi
fi

echo "$(date '+%F %T') [launchd] 前台启动 nacos standalone(CRDB 持久化模式)" >&2

# exec 替换为 java 进程,launchd 直接持有此 PID。
# 参数取自 ~/nacos/nacos/bin/startup.sh 的 standalone 分支,
# 但改为前台阻塞(无 nohup/&),并显式指定 azul-21 JAVA_HOME。
# nacos 配置 application.properties 已配 CRDB(127.0.0.1:26257),由 nacos-crdb-channel launchd 保证通道。
exec "${JAVA}" \
  -Xms512m -Xmx512m -Xmn256m \
  -Dnacos.standalone=true \
  -Dnacos.member.list= \
  -Xlog:gc*:file=${NACOS_HOME}/logs/nacos_gc.log:time,tags:filecount=10,filesize=100m \
  -Dloader.path=${NACOS_HOME}/plugins,${NACOS_HOME}/plugins/health,${NACOS_HOME}/plugins/cmdb,${NACOS_HOME}/plugins/selector \
  -Dnacos.home=${NACOS_HOME} \
  -jar ${NACOS_HOME}/target/nacos-server.jar \
  --spring.config.additional-location=file:${NACOS_HOME}/conf/ \
  --logging.config=${NACOS_HOME}/conf/nacos-logback.xml \
  --server.max-http-header-size=524288 \
  nacos.nacos
