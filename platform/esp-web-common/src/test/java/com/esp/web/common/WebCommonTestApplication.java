package com.esp.web.common;

import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.client.RestClientException;

/**
 * WebCommonAutoConfigurationTest 用最小应用:同包控制器触发异常,验证
 * GlobalExceptionHandler 拦截 + envelope + actuator 路径白名单 + AppLogClient 装配。
 *
 * <p>包 com.esp.web.common 与测试配置的 base-packages 匹配,控制器异常进入 advice。
 */
@SpringBootConfiguration
@EnableAutoConfiguration
@RestController
public class WebCommonTestApplication {

    @GetMapping("/api/boom")
    public String boom() {
        throw new IllegalStateException("boom");
    }

    @GetMapping("/api/rpc")
    public String rpc() {
        throw new RestClientException("upstream down");
    }

    /** 模拟 actuator 端点:位于被守卫路径,异常应放行(非 envelope)。 */
    @GetMapping("/actuator/health")
    public String health() {
        throw new IllegalStateException("actuator boom");
    }

    @GetMapping("/api/ok")
    public String ok() {
        return "OK";
    }
}
