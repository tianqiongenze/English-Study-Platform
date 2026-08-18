package com.esp.common.health;

import java.util.List;

/**
 * 健康端点白名单常量(收编自 bff-java AuthFilter/EdgeAuthHandler + learning/teacher JwtAuthFilter)。
 * <p>
 * 使用方在各自的 Filter/Handler 中引用 {@link #COMMON_WHITELIST},按需追加服务自有白名单路径。
 * <p>
 * 三服务当前白名单交集:
 * <ul>
 *   <li>/actuator/ -- Spring Boot Actuator(health/prometheus/configenv)</li>
 *   <li>/healthz -- 简写健康探针(K8s liveness/readiness)</li>
 *   <li>/v3/api-docs -- springdoc OpenAPI JSON</li>
 *   <li>/swagger-ui -- Swagger UI 静态资源</li>
 *   <li>/docs -- API 门户(仅 bff 聚合,但不损害其他服务)</li>
 * </ul>
 *
 * <p>bff-java 特有(/.well-known/、/openapi-go-rust.yaml)由 bff 自行追加,不放此常量。
 */
public final class HealthConfig {

    private HealthConfig() {}

    /** 所有 Java 服务共用的鉴权白名单路径前缀。 */
    public static final List<String> COMMON_WHITELIST = List.of(
            "/actuator/",
            "/healthz",
            "/v3/api-docs",
            "/swagger-ui",
            "/docs"
    );
}
