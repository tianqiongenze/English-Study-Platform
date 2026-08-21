package com.esp.bff.config;

import io.netty.channel.ChannelOption;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.http.client.reactive.ReactorClientHttpConnector;
import org.springframework.web.reactive.function.client.WebClient;
import reactor.netty.http.client.HttpClient;

import java.time.Duration;

/**
 * 非 Java 服务(gateway-go/writing-py/emotion-svc)经 REST 聚合。
 * Java 服务(learning/teacher)经 Dubbo Triple(见各 RpcGateway)。
 *
 * <p>P1-D11(2026-08-21 RWT 超时分级):原 {@code WebClient.builder().build()} 无 connect/response 超时,
 * 慢下游(gateway LLM 长尾/writing-py 批改 100s+)挂死 Netty event loop 致线程池耗尽。
 * 改:connectTimeout 2s(建连阶段,失败快);responseTimeout 90s(LLM 聚合链最长,writing 批改可达 100s,
 * 按 RWT LLM 30-60-90s 分级取上限 90s;若下游超 90s 视为故障该放而非挂死)。
 * per-request 更细分超时由调用方 {@code .timeout(Duration)} 覆盖(如普通下游 5s)。
 */
@Configuration
public class WebClientConfig {

    /** 连接建立超时(RWT 同 ns RPC 1s 量级,取 2s 留余量)。 */
    private static final long CONNECT_TIMEOUT_MS = 2_000L;
    /** 响应超时(LLM 聚合链上限,按 RWT LLM 90s 分级)。 */
    private static final long RESPONSE_TIMEOUT_SECS = 90L;

    @Bean
    public WebClient webClient() {
        HttpClient httpClient = HttpClient.create()
                .option(ChannelOption.CONNECT_TIMEOUT_MILLIS, (int) CONNECT_TIMEOUT_MS)
                .responseTimeout(Duration.ofSeconds(RESPONSE_TIMEOUT_SECS));
        return WebClient.builder()
                .clientConnector(new ReactorClientHttpConnector(httpClient))
                .build();
    }
}
