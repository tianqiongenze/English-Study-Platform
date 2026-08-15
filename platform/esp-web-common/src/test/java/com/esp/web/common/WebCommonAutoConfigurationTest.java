package com.esp.web.common;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.webmvc.test.autoconfigure.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.servlet.MockMvc;

import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

/**
 * 自动装配集成测试:验证 esp-web-common starter 在真实 Spring Boot 上下文中
 * (经 META-INF/spring/...AutoConfiguration.imports 加载)的端到端行为。
 *
 * <p>覆盖:basePackages 守卫命中→500 envelope、RestClientException→503、actuator 路径白名单放行、
 * AppLogClient/GlobalExceptionHandler Bean 装配。
 */
@SpringBootTest(classes = WebCommonTestApplication.class, properties = {
        "esp.web.common.base-packages=com.esp.web.common",
        "esp.web.common.exclude-path-prefixes=/actuator",
        "esp.web.common.app-log.service=test-svc",
        "spring.application.name=test-app",
        "spring.datasource.url=jdbc:h2:mem:espwc;DB_CLOSE_DELAY=-1",
        "spring.datasource.driver-class-name=org.h2.Driver"
})
@AutoConfigureMockMvc
class WebCommonAutoConfigurationTest {

    @Autowired
    private MockMvc mvc;

    @Autowired
    private GlobalExceptionHandler globalExceptionHandler;

    @Autowired
    private AppLogClient appLogClient;

    @Test
    void contextLoads_wiresBeans() {
        // 两个共享 Bean 由自动配置装配
        org.assertj.core.api.Assertions.assertThat(globalExceptionHandler).isNotNull();
        org.assertj.core.api.Assertions.assertThat(appLogClient).isNotNull();
    }

    @Test
    void genericException_returns500Envelope() throws Exception {
        mvc.perform(get("/api/boom"))
                .andExpect(status().isInternalServerError())
                .andExpect(jsonPath("$.code").value("INTERNAL_ERROR"))
                .andExpect(jsonPath("$.message").value("internal server error"));
    }

    @Test
    void restClientException_returns503Envelope() throws Exception {
        mvc.perform(get("/api/rpc"))
                .andExpect(status().isServiceUnavailable())
                .andExpect(jsonPath("$.code").value("UPSTREAM_UNAVAILABLE"))
                .andExpect(jsonPath("$.message").value("upstream unavailable"));
    }

    @Test
    void actuatorPath_whitelisted_notEnvelope() {
        // advice 对 /actuator 放行(重抛原异常)→ MOCK 容器以 ServletException 上抛,而非返回 envelope
        org.junit.jupiter.api.Assertions.assertThrows(
                jakarta.servlet.ServletException.class,
                () -> mvc.perform(get("/actuator/health")));
    }

    @Test
    void normalController_unaffected() throws Exception {
        mvc.perform(get("/api/ok"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$").value("OK"));
    }
}
