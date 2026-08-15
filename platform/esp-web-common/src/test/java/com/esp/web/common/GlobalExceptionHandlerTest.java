package com.esp.web.common;

import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.web.client.RestClientException;
import org.springframework.web.method.HandlerMethod;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;

/**
 * GlobalExceptionHandler 单测:envelope 字段 / RestClientException→503 / 包范围守卫 / actuator 路径白名单。
 *
 * <p>直接调用 handler 方法验证逻辑(不依赖 Spring 容器);HandlerMethod 参数由 Spring 在运行时注入
 * (ExceptionHandlerExceptionResolver 以 providedArg 传入,见其源码 doResolveHandlerMethodException),
 * 此处用真实 {@link HandlerMethod} 构造以贴近运行行为。
 */
class GlobalExceptionHandlerTest {

    private static final String MATCH_PACKAGE = "com.esp.web.common.test.controllers";

    private final GlobalExceptionHandler handler = new GlobalExceptionHandler(List.of(MATCH_PACKAGE), List.of("/actuator"));

    private HandlerMethod matchingHandler() throws NoSuchMethodException {
        return new HandlerMethod(new com.esp.web.common.test.controllers.SampleController(), "boom");
    }

    private HandlerMethod nonMatchingHandler() throws NoSuchMethodException {
        // OtherController 包 com.esp.web.common.test 不匹配 basePackages(MATCH_PACKAGE)
        return new HandlerMethod(new com.esp.web.common.test.OtherController(), "boom");
    }

    private MockHttpServletRequest request(String uri) {
        return new MockHttpServletRequest("GET", uri);
    }

    @Test
    void envelopeFields_onGenericException_returns500AndCodeMessageTraceId() throws Exception {
        MockHttpServletRequest req = request("/api/learning/behavior");
        ResponseEntity<ApiError> resp = handler.handleException(new IllegalStateException("boom"), req, matchingHandler());

        assertEquals(HttpStatus.INTERNAL_SERVER_ERROR, resp.getStatusCode());
        assertNotNull(resp.getBody());
        assertEquals("INTERNAL_ERROR", resp.getBody().getCode());
        assertEquals("internal server error", resp.getBody().getMessage());
        assertNull(resp.getBody().getTraceId()); // 无 OTel 上下文 → null
    }

    @Test
    void upstreamUnavailable_returns503Envelope() throws Exception {
        MockHttpServletRequest req = request("/api/auth/login");
        ResponseEntity<ApiError> resp = handler.handleUpstreamUnavailable(
                new RestClientException("connect timeout"), req, matchingHandler());

        assertEquals(HttpStatus.SERVICE_UNAVAILABLE, resp.getStatusCode());
        assertNotNull(resp.getBody());
        assertEquals("UPSTREAM_UNAVAILABLE", resp.getBody().getCode());
        assertEquals("upstream unavailable", resp.getBody().getMessage());
        assertNull(resp.getBody().getTraceId());
    }

    @Test
    void actuatorPath_rethrowsOriginalException_whitelist() throws Exception {
        MockHttpServletRequest req = request("/actuator/prometheus");
        IllegalStateException ex = new IllegalStateException("boom");
        assertThrows(IllegalStateException.class, () -> handler.handleException(ex, req, matchingHandler()));
    }

    @Test
    void nonMatchingBasePackage_rethrowsOriginalException() throws Exception {
        MockHttpServletRequest req = request("/other/api");
        IllegalStateException ex = new IllegalStateException("boom");
        assertThrows(IllegalStateException.class, () -> handler.handleException(ex, req, nonMatchingHandler()));
    }

    @Test
    void nonMatchingBasePackage_upstream_rethrowsOriginalException() throws Exception {
        MockHttpServletRequest req = request("/other/api");
        RestClientException ex = new RestClientException("boom");
        assertThrows(RestClientException.class, () -> handler.handleUpstreamUnavailable(ex, req, nonMatchingHandler()));
    }

    @Test
    void emptyBasePackages_handlesAllControllers() throws Exception {
        GlobalExceptionHandler global = new GlobalExceptionHandler(List.of(), List.of("/actuator"));
        MockHttpServletRequest req = request("/other/api");
        ResponseEntity<ApiError> resp = global.handleException(new IllegalStateException("boom"), req, nonMatchingHandler());

        assertEquals(HttpStatus.INTERNAL_SERVER_ERROR, resp.getStatusCode());
        assertNotNull(resp.getBody());
        assertEquals("INTERNAL_ERROR", resp.getBody().getCode());
    }

    @Test
    void nullHandlerMethod_withEmptyBasePackages_handled() throws Exception {
        // basePackages 为空且无控制器上下文 → 处理(而非重抛)
        GlobalExceptionHandler global = new GlobalExceptionHandler(List.of(), List.of());
        MockHttpServletRequest req = request("/api/x");
        ResponseEntity<ApiError> resp = global.handleException(new IllegalStateException("boom"), req, null);
        assertEquals(HttpStatus.INTERNAL_SERVER_ERROR, resp.getStatusCode());
    }

    @Test
    void nullHandlerMethod_withBasePackages_skipped() throws Exception {
        // 配置了包范围但无控制器上下文 → 重抛(与旧 basePackages=com.esp.* 行为一致:不适用于 null handler)
        MockHttpServletRequest req = request("/api/x");
        IllegalStateException ex = new IllegalStateException("boom");
        assertThrows(IllegalStateException.class, () -> handler.handleException(ex, req, null));
    }
}
