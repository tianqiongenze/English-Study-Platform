package com.esp.bff.edge;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/**
 * RouteTable 路由解析测试:验证前缀匹配顺序敏感性(writing-coach 必须先于 writing)与本地路径判定。
 */
class RouteTableTest {

    @Test
    void resolve_routesByPrefix() {
        assertEquals("gateway-go:80", RouteTable.resolve("/api/scenarios/123"));
        assertEquals("learning-java:80", RouteTable.resolve("/api/learning/review"));
        assertEquals("teacher-java:80", RouteTable.resolve("/api/teacher/1/kp-stats"));
        assertEquals("emotion-svc:80", RouteTable.resolve("/api/emotion/sense"));
    }

    @Test
    void resolve_writingCoachTakesPrecedenceOverWriting() {
        // 坑:/api/writing 是 /api/writing-coach 的前缀,顺序错误会误路由
        assertEquals("writing-coach:80", RouteTable.resolve("/api/writing-coach/assess"));
        assertEquals("writing-py:80", RouteTable.resolve("/api/writing/grade"));
    }

    @Test
    void resolve_returnsNullForUnknownOrLocal() {
        assertNull(RouteTable.resolve("/api/auth/register"));
        assertNull(RouteTable.resolve("/unknown"));
        assertNull(RouteTable.resolve(null));
    }

    @Test
    void isLocal_detectsBffOwnedPaths() {
        assertTrue(RouteTable.isLocal("/api/auth/register"));
        assertTrue(RouteTable.isLocal("/healthz"));
        assertTrue(RouteTable.isLocal("/actuator/health"));
        assertTrue(RouteTable.isLocal("/v3/api-docs"));
        assertTrue(RouteTable.isLocal("/swagger-ui/index.html"));
    }

    @Test
    void isLocal_falseForDownstreamPaths() {
        assertFalse(RouteTable.isLocal("/api/learning/review"));
        assertFalse(RouteTable.isLocal("/api/scenarios"));
        assertFalse(RouteTable.isLocal(null));
    }
}
