// Package redisx —— ESP Go 服务统一 Redis Cluster 客户端(会话22-3 债1+2)。
// 消除 gamification-go/course-path-go/gateway-go 三处 NewClusterClient 重复 + env 命名统一(REDIS_CLUSTER)。
package redisx

import (
	"log"

	"github.com/redis/go-redis/v9"
)

// DefaultAddr Redis Cluster 默认入口(单节点 RD 环境,任一节点即可引导)。
const DefaultAddr = "redis-cluster-new-0.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local:6379"

// MustNewCluster 创建 Redis Cluster 客户端,PoolSize 默认 16(对齐 gamification-go 原 NewClusterClient)。
// addr 为空时用 DefaultAddr。serviceName 仅用于启动日志。
func MustNewCluster(addr, serviceName string) *redis.ClusterClient {
	if addr == "" {
		addr = DefaultAddr
	}
	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    []string{addr},
		PoolSize: 16,
	})
	log.Printf("[%s] Redis Cluster client ready (%s)", serviceName, addr)
	return rdb
}
