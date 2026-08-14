package com.esp.bff.edge;

import java.util.Map;

/**
 * 路由表(ADR-08/ADR-13)。Netty 边缘层按 path 前缀转发到下游服务短名(ClusterIP:80)。
 * - WS 旁路 BFF:前端 /ws/** 直连 gateway-go,不经此网关。
 * - REST 经 BFF:Netty 解码 → 鉴权(AuthFilter)→ 注入 X-User-Id/X-Role → 转发下游。
 * - /api/auth/** 回环到 BFF 自身的 Spring MVC 控制器(不转发)。
 */
public final class RouteTable {

    /** 前缀 → 下游 host:port。顺序敏感,按插入顺序匹配前缀。 */
    private static final Map<String, String> ROUTES = new java.util.LinkedHashMap<>();
    static {
        // /api/auth/** 由 BFF 自身 AuthController 处理,不在此转发
        // 顺序敏感:更具体的前缀必须先于通配前缀。
        // /api/writing-coach 必须在 /api/writing 之前,否则 startsWith("/api/writing")
        // 会误吞 /api/writing-coach(/api/writing 是 /api/writing-coach 的前缀)。
        ROUTES.put("/api/scenarios",     "gateway-go:80");
        ROUTES.put("/api/sessions",      "gateway-go:80");
        ROUTES.put("/api/learning",      "learning-java:80");
        ROUTES.put("/api/writing-coach", "writing-coach:80");
        ROUTES.put("/api/writing",       "writing-py:80");
        ROUTES.put("/api/teacher",       "teacher-java:80");
        ROUTES.put("/api/emotion",       "emotion-svc:80");
    }

    private RouteTable() {}

    /** 返回下游 host:port,无匹配返回 null(由 BFF 自身处理或 404)。 */
    public static String resolve(String path) {
        if (path == null) return null;
        for (Map.Entry<String, String> e : ROUTES.entrySet()) {
            if (path.startsWith(e.getKey())) return e.getValue();
        }
        return null;
    }

    /** 是否为 BFF 自身处理的路径(/api/auth、/healthz、/actuator、API 门户)。 */
    public static boolean isLocal(String path) {
        return path != null && (path.startsWith("/api/auth")
                || path.startsWith("/healthz") || path.startsWith("/actuator")
                || path.startsWith("/v3/api-docs") || path.startsWith("/docs")
                || path.startsWith("/swagger-ui") || path.startsWith("/openapi-go-rust.yaml"));
    }
}
