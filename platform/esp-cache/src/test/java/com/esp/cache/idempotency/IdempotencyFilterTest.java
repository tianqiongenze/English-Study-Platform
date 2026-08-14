package com.esp.cache.idempotency;

import com.esp.cache.CacheService;
import jakarta.servlet.FilterChain;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;
import org.springframework.test.util.ReflectionTestUtils;

import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * IdempotencyFilter 单元测试(P1-8 幂等机制)。
 * 用 Spring 的 MockHttpServletRequest/Response,避免引入 servlet 容器。
 * CacheService 用 Mockito stub 控制 enabled / tryIdempotent / getOrLoad / put / evict。
 */
class IdempotencyFilterTest {

    private CacheService cacheService;
    private IdempotencyFilter filter;

    @BeforeEach
    void setup() {
        cacheService = mock(CacheService.class);
        filter = new IdempotencyFilter(cacheService);
    }

    private MockHttpServletRequest req(String method, String uri, String idemKey) {
        MockHttpServletRequest r = new MockHttpServletRequest(method, uri);
        if (idemKey != null) r.addHeader("Idempotency-Key", idemKey);
        return r;
    }

    @Test
    void nonPostPassesThrough() throws Exception {
        FilterChain chain = mock(FilterChain.class);
        filter.doFilterInternal(req("GET", "/api/x", "k1"), new MockHttpServletResponse(), chain);
        verify(chain).doFilter(any(), any());
        verifyNoInteractions(cacheService);
    }

    @Test
    void postWithoutKeyPassesThrough() throws Exception {
        FilterChain chain = mock(FilterChain.class);
        filter.doFilterInternal(req("POST", "/api/x", null), new MockHttpServletResponse(), chain);
        verify(chain).doFilter(any(), any());
        verifyNoInteractions(cacheService);
    }

    @Test
    void cacheDisabledPassesThrough() throws Exception {
        when(cacheService.isEnabled()).thenReturn(false);
        FilterChain chain = mock(FilterChain.class);
        filter.doFilterInternal(req("POST", "/api/x", "k1"), new MockHttpServletResponse(), chain);
        verify(chain).doFilter(any(), any());
        verify(cacheService, never()).tryIdempotent(anyString(), anyLong());
    }

    @Test
    void firstAcquireExecutesAndWritesBackOn2xx() throws Exception {
        when(cacheService.isEnabled()).thenReturn(true);
        when(cacheService.tryIdempotent(anyString(), anyLong())).thenReturn(true);
        AtomicInteger chainCalls = new AtomicInteger();
        FilterChain chain = (request, response) -> {
            chainCalls.incrementAndGet();
            HttpServletResponse hr = (HttpServletResponse) response;
            hr.setStatus(201);
            hr.getWriter().write("{\"ok\":true}");
        };
        MockHttpServletResponse res = new MockHttpServletResponse();
        filter.doFilterInternal(req("POST", "/api/grade", "k1"), res, chain);
        assertEquals(1, chainCalls.get());
        verify(cacheService).put(eq("esp_idem"), anyString(), anyString(), eq(300L));
        verify(cacheService, never()).evict(anyString(), anyString());
    }

    @Test
    void failedBusinessEvictsPlaceholder() throws Exception {
        when(cacheService.isEnabled()).thenReturn(true);
        when(cacheService.tryIdempotent(anyString(), anyLong())).thenReturn(true);
        FilterChain chain = (request, response) -> {
            HttpServletResponse hr = (HttpServletResponse) response;
            hr.setStatus(500);
        };
        filter.doFilterInternal(req("POST", "/api/x", "k1"), new MockHttpServletResponse(), chain);
        verify(cacheService).evict(eq("esp_idem"), anyString());
        verify(cacheService, never()).put(anyString(), anyString(), anyString(), anyLong());
    }

    @Test
    void replayReturnsCachedBody() throws Exception {
        when(cacheService.isEnabled()).thenReturn(true);
        when(cacheService.tryIdempotent(anyString(), anyLong())).thenReturn(false);
        when(cacheService.getOrLoad(eq("esp_idem"), anyString(), any(), anyLong(), anyBoolean()))
                .thenReturn("{\"ok\":true}");
        FilterChain chain = mock(FilterChain.class);
        MockHttpServletResponse res = new MockHttpServletResponse();
        filter.doFilterInternal(req("POST", "/api/x", "k1"), res, chain);
        verify(chain, never()).doFilter(any(), any());
        assertEquals(200, res.getStatus());
        assertTrue(res.getContentAsString().contains("\"ok\":true"));
    }

    @Test
    void replayInProgressReturns409() throws Exception {
        when(cacheService.isEnabled()).thenReturn(true);
        when(cacheService.tryIdempotent(anyString(), anyLong())).thenReturn(false);
        when(cacheService.getOrLoad(eq("esp_idem"), anyString(), any(), anyLong(), anyBoolean()))
                .thenReturn(null);
        FilterChain chain = mock(FilterChain.class);
        MockHttpServletResponse res = new MockHttpServletResponse();
        filter.doFilterInternal(req("POST", "/api/x", "k1"), res, chain);
        verify(chain, never()).doFilter(any(), any());
        assertEquals(409, res.getStatus());
    }

    @Test
    void bizTagFromPathStripsApiPrefix() {
        // 通过反射访问私有静态方法验证路径转换逻辑
        String tag = (String) ReflectionTestUtils.invokeMethod(IdempotencyFilter.class,
                "bizTagFromPath", "/api/writing/grade");
        assertEquals("writing-grade", tag);
    }
}
