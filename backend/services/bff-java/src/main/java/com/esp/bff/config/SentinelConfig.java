package com.esp.bff.config;

import com.alibaba.csp.sentinel.adapter.spring.webmvc_v6x.callback.BlockExceptionHandler;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.alibaba.csp.sentinel.slots.block.degrade.DegradeException;
import com.alibaba.csp.sentinel.slots.block.flow.FlowException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Sentinel Web MVC v6x 适配配置(ADR-08,Jakarta Servlet 6 / Spring Boot 3)。
 * 流控/熔断命中时返回标准化错误码:429(限流)/503(降级)。
 * 规则可从 Nacos 动态拉取(spring.cloud.sentinel.datasource)。
 */
@Configuration
public class SentinelConfig {

    @Bean
    public BlockExceptionHandler blockExceptionHandler() {
        return (HttpServletRequest req, HttpServletResponse resp, String resourceName, BlockException e) -> {
            int status = e instanceof FlowException ? 429 : e instanceof DegradeException ? 503 : 500;
            resp.setStatus(status);
            resp.setContentType("application/json;charset=UTF-8");
            resp.getWriter().write("{\"error\":\"" + (status == 429 ? "rate limited" : "degraded") + "\"}");
        };
    }
}
