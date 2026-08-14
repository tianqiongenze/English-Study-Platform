// redisx —— 统一 Redis Cluster 异步连接(会话22 债1+2)。
// 消除 pronunciation-rs/assessment-rs/llm-orchestrator-rs 三处 ClusterClient 构建重复。
// ESP 用真 Redis Cluster(3 主分片),必须开 cluster-async 处理 MOVED/ASK,否则写静默失败。

use redis::cluster_async::ClusterConnection;

/// 默认 Redis Cluster seed 节点(单节点 RD 环境,任一节点即可引导发现拓扑)。
pub const DEFAULT_ADDR: &str = "redis://redis-cluster:6379";

/// 创建 Redis Cluster 异步连接。env_key 为空时用 DEFAULT_ADDR。service_name 仅用于日志。
/// 对齐 pronunciation-rs/assessment-rs 原 REDIS_CLUSTER env + ClusterClient::builder 行为。
pub async fn connect(env_key: &str, service_name: &str) -> Option<ClusterConnection> {
    let addr = std::env::var(env_key).unwrap_or_else(|_| DEFAULT_ADDR.to_string());
    match redis::cluster::ClusterClient::builder(vec![addr.as_str()]).build() {
        Ok(client) => match client.get_async_connection().await {
            Ok(conn) => {
                tracing::info!("[{}] Redis Cluster client ready", service_name);
                Some(conn)
            }
            Err(e) => {
                tracing::warn!("[{}] redis cluster connect failed: {}", service_name, e);
                None
            }
        },
        Err(e) => {
            tracing::warn!("[{}] redis cluster client build failed: {}", service_name, e);
            None
        }
    }
}

/// 创建 Redis Cluster 异步连接(必需,失败则 panic)。供 llm-orchestrator-rs 等强依赖 Redis 的服务用。
/// addr 直接传入(llm-orchestrator 原用 REDIS_ADDR + redis:// 前缀)。
pub async fn connect_required(addr: &str, service_name: &str) -> ClusterConnection {
    let client = redis::cluster::ClusterClient::new(vec![addr])
        .unwrap_or_else(|e| panic!("[{}] redis cluster client: {}", service_name, e));
    client
        .get_async_connection()
        .await
        .unwrap_or_else(|e| panic!("[{}] redis cluster connection: {}", service_name, e))
}
