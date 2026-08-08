# 基础设施说明(TiDB / Redis Cluster / Pulsar 生产部署)
# 这些组件在生产环境通过各自 Operator/Helm 部署,此处给出指引。
#
# === TiDB(HTAP,含 TiFlash 列存,支撑读写分离 OLAP) ===
# 通过 TiDB Operator 部署:
#   helm repo add pingcap https://charts.pingcap.org/
#   helm install tidb-operator pingcap/tidb-operator --namespace tidb-admin --create-namespace
#   kubectl apply -f deploy/k8s/tidb-cluster.yaml   # TidbCluster CR
# 关键:
#   - pd.replicas: 3
#   - tikv.replicas: 3
#   - tidb.replicas: 2 (primary + follower,读写分离)
#   - tiflash.replicas: 2 (OLAP 列存,模块4学情分析)
# Service:
#   tidb-primary:    读写分离写主库
#   tidb-follower:   读写分离读副本
#
# === Redis Cluster(会话/缓存/限流) ===
# 通过 Redis Operator 或 helm:
#   helm install redis-cluster bitnami/redis-cluster \
#     --set persistence.enabled=true \
#     --set cluster.nodes=6
# Service: redis-cluster:6379
#
# === Apache Pulsar(事件总线) ===
# 通过官方 Helm chart:
#   helm repo add apache https://pulsar.apache.org/charts
#   helm install pulsar apache/pulsar \
#     --set components.broker=true --set components.bookkeeper=true
# Service: pulsar-broker:6650
#
# 详见 docs/design/02-系统架构设计文档-SAD.md 第 11 章部署架构。
