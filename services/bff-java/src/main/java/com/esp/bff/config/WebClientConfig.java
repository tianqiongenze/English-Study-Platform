package com.esp.bff.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.reactive.function.client.WebClient;

/**
 * 非 Java 服务(gateway-go/writing-py/emotion-svc)经 REST 聚合。
 * Java 服务(learning/teacher)经 Dubbo Triple(见各 RpcGateway)。
 */
@Configuration
public class WebClientConfig {
    @Bean
    public WebClient webClient() {
        return WebClient.builder().build();
    }
}
