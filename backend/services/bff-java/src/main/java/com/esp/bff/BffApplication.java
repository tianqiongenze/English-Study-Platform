package com.esp.bff;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.client.discovery.EnableDiscoveryClient;

/**
 * ESP BFF(重构版,按用户要求:仅 Dubbo + Spring Cloud Alibaba,不再用 Spring Cloud Gateway)。
 *
 * 架构:
 * - 边缘接入:Spring Boot Web MVC(REST 聚合 + JWT 校验 + Sentinel 限流 + X-User-Id 注入),取代 SCG。
 * - 服务间通信:Dubbo Triple(gRPC over HTTP/2),Java 服务(learning/teacher)走强类型 RPC。
 * - 非 Java 服务(gateway-go/writing-py/emotion-svc):WebClient REST 聚合。
 * - 治理:Spring Cloud Alibaba 全套 —— Nacos(注册+配置)、Sentinel(流控熔断)、Seata(分布式事务)。
 * - 工业级目标:高可用(Nacos 注册+多副本)、高性能(Triple HTTP/2 + 连接池)、
 *   分布式事务合理(Seata AT/@GlobalTransactional 跨服务一致)。
 */
@EnableDiscoveryClient
@SpringBootApplication(scanBasePackages = {"com.esp.bff", "com.esp.cache"})
public class BffApplication {
    public static void main(String[] args) {
        SpringApplication.run(BffApplication.class, args);
    }
}
