// Package esp_common —— ESP Rust 服务公共库(会话22 债1+2)。
// 消除 4 个 Rust 服务(pronunciation-rs / assessment-rs / llm-orchestrator-rs / media-learn-rs)里
// 逐字复制的:tracing 初始化、TiDB 读写连接池、Redis Cluster 异步连接、axum 健康检查 handler。
//
// 设计原则:行为不变红线 —— 不改 HTTP body 契约、不改 env 命名、不改连接池默认值,
// 仅把重复代码上提。error enum 不抽(改 HTTP body 契约,单列评估)。

pub mod db;
pub mod health;
pub mod redisx;
pub mod tracing_init;
