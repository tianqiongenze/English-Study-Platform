package com.esp.bff.config;

import jakarta.servlet.FilterChain;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * RateLimitFilter 单测(ADR-08/ADR-12 限流)。
 * 验证:资源映射正确、正常请求放行、超限返 429。
 * 注:Sentinel 单线程内连续触发 5000+ QPS 阈值不现实,本测用极低阈值规则临时验证 429 路径。
 */
class RateLimitFilterTest {

    @Test
    void normalRequestPassesThrough() throws Exception {
        RateLimitFilter filter = new RateLimitFilter();
        filter.init();
        MockHttpServletRequest req = new MockHttpServletRequest("GET", "/api/learning/due");
        MockHttpServletResponse res = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);
        filter.doFilter(req, res, chain);
        assertEquals(200, res.getStatus());
        verify(chain).doFilter(req, res);
    }

    @Test
    void writingPathMappedToWritingResource() throws Exception {
        RateLimitFilter filter = new RateLimitFilter();
        filter.init();
        MockHttpServletRequest req = new MockHttpServletRequest("POST", "/api/writing/grade");
        MockHttpServletResponse res = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);
        filter.doFilter(req, res, chain);
        assertEquals(200, res.getStatus());
        verify(chain).doFilter(req, res);
    }

    @Test
    void overThresholdReturns429() throws Exception {
        RateLimitFilter filter = new RateLimitFilter();
        filter.init();
        // 阈值设为 0,所有对该资源的请求必然被限流,稳定触发 catch(429)分支。
        com.alibaba.csp.sentinel.slots.block.flow.FlowRule r =
                new com.alibaba.csp.sentinel.slots.block.flow.FlowRule("api-writing-grade");
        r.setGrade(com.alibaba.csp.sentinel.slots.block.RuleConstant.FLOW_GRADE_QPS);
        r.setCount(0);
        com.alibaba.csp.sentinel.slots.block.flow.FlowRuleManager.loadRules(java.util.List.of(r));

        MockHttpServletResponse res = new MockHttpServletResponse();
        filter.doFilter(new MockHttpServletRequest("POST", "/api/writing/grade"), res, mock(FilterChain.class));
        assertEquals(429, res.getStatus(), "阈值 0 时请求应被限流返 429");
        assertTrue(res.getContentAsString().contains("rate_limited"));
    }
}
