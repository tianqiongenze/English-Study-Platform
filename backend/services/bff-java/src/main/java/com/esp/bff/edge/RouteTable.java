package com.esp.bff.edge;

import com.alibaba.nacos.api.NacosFactory;
import com.alibaba.nacos.api.config.ConfigService;
import com.alibaba.nacos.api.config.listener.Listener;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Properties;
import java.util.concurrent.Executor;

/**
 * 路由表(ADR-08/ADR-13)。Netty 边缘层按 path 前缀转发到下游服务短名(ClusterIP:80)。
 *
 * <p>v2:从 Nacos 配置中心动态读取路由表(dataId={@value DATA_ID}, group={@value GROUP})，
 * 本地缓存 fallback(Nacos 不可用时用静态默认值)，支持配置变更自动刷新。</p>
 *
 * <ul>
 *   <li>WS 旁路 BFF:前端 /ws/** 直连 gateway-go,不经此网关。</li>
 *   <li>REST 经 BFF:Netty 解码 → 鉴权(AuthFilter)→ 注入 X-User-Id/X-Role → 转发下游。</li>
 *   <li>/api/auth/** 回环到 BFF 自身的 Spring MVC 控制器(不转发)。</li>
 * </ul>
 */
public final class RouteTable {

    private static final Logger log = LoggerFactory.getLogger(RouteTable.class);

    private static final String DATA_ID = "esp-bff-routes";
    private static final String GROUP = "DEFAULT_GROUP";
    private static final long TIMEOUT_MS = 3000L;

    private static final ObjectMapper MAPPER = new ObjectMapper();

    /** 顺序敏感:更具体的前缀必须先于通配前缀。writing-coach 在 writing 之前。 */
    private static final Map<String, String> DEFAULT_ROUTES = new LinkedHashMap<>();
    static {
        DEFAULT_ROUTES.put("/api/scenarios",     "gateway-go:80");
        DEFAULT_ROUTES.put("/api/sessions",      "gateway-go:80");
        DEFAULT_ROUTES.put("/api/learning",      "learning-java:80");
        DEFAULT_ROUTES.put("/api/vocab",         "vocab-srs-go:80");
        DEFAULT_ROUTES.put("/api/writing-coach", "writing-coach:80");
        DEFAULT_ROUTES.put("/api/writing",       "writing-py:80");
        DEFAULT_ROUTES.put("/api/teacher",       "teacher-java:80");
        DEFAULT_ROUTES.put("/api/emotion",       "emotion-svc:80");
        DEFAULT_ROUTES.put("/api/content",       "content-go:80");
        DEFAULT_ROUTES.put("/api/community",     "community-go:80");
        DEFAULT_ROUTES.put("/api/gamification",  "gamification-go:80");
    }

    /** 当前路由快照(volatile + 引用替换,读无锁) */
    private static volatile Map<String, String> routes = new LinkedHashMap<>(DEFAULT_ROUTES);

    private static volatile ConfigService configService;

    private RouteTable() {}

    /**
     * 从 Nacos 初始化路由表。由 BffApplication 在 ApplicationReadyEvent 时调用。
     *
     * @param serverAddr Nacos server 地址(如 nacos:8848)
     * @param namespace  Nacos namespace(如 public, 或空串)
     */
    public static void init(String serverAddr, String namespace) {
        try {
            Properties props = new Properties();
            props.put("serverAddr", serverAddr);
            if (namespace != null && !namespace.isEmpty()) {
                props.put("namespace", namespace);
            }

            ConfigService cs = NacosFactory.createConfigService(props);
            configService = cs;

            // 1. 拉取初始配置
            String config = cs.getConfig(DATA_ID, GROUP, TIMEOUT_MS);
            if (config != null && !config.isBlank()) {
                Map<String, String> parsed = parseJson(config);
                if (parsed != null && !parsed.isEmpty()) {
                    routes = new LinkedHashMap<>(parsed);
                    log.info("[RouteTable] Loaded {} routes from Nacos", routes.size());
                } else {
                    log.warn("[RouteTable] Nacos config empty/invalid, using default routes");
                }
            } else {
                log.warn("[RouteTable] No Nacos config for {}/{}, using default routes", DATA_ID, GROUP);
            }

            // 2. 注册监听器:配置变更时自动刷新
            cs.addListener(DATA_ID, GROUP, new Listener() {
                @Override
                public Executor getExecutor() {
                    return null; // 在 Nacos 通知线程执行,避免额外线程池
                }

                @Override
                public void receiveConfigInfo(String configInfo) {
                    if (configInfo == null || configInfo.isBlank()) {
                        log.warn("[RouteTable] Nacos config cleared, keeping current routes");
                        return;
                    }
                    Map<String, String> parsed = parseJson(configInfo);
                    if (parsed != null && !parsed.isEmpty()) {
                        routes = new LinkedHashMap<>(parsed);
                        log.info("[RouteTable] Routes refreshed from Nacos: {} entries", routes.size());
                    } else {
                        log.error("[RouteTable] Failed to parse Nacos config, keeping current routes");
                    }
                }
            });
            log.info("[RouteTable] Nacos listener registered for {}/{}", DATA_ID, GROUP);

        } catch (Exception e) {
            log.error("[RouteTable] Failed to init from Nacos ({}), using default routes", e.getMessage());
            // fallback: routes 已在静态初始化中设为 DEFAULT_ROUTES
        }
    }

    /** 解析 JSON 路由配置，保持插入顺序。 */
    private static Map<String, String> parseJson(String json) {
        try {
            return MAPPER.readValue(json, new TypeReference<LinkedHashMap<String, String>>() {});
        } catch (Exception e) {
            log.error("[RouteTable] JSON parse error: {}", e.getMessage());
            return null;
        }
    }

    /** 返回下游 host:port,无匹配返回 null(由 BFF 自身处理或 404)。 */
    public static String resolve(String path) {
        if (path == null) return null;
        Map<String, String> snapshot = routes; // volatile read, 引用快照
        for (Map.Entry<String, String> e : snapshot.entrySet()) {
            if (path.startsWith(e.getKey())) return e.getValue();
        }
        return null;
    }

    /**
     * 是否为 BFF 自身处理的路径(/api/auth、/actuator、API 门户)。
     * 注意:/healthz 已移除——BFF 无 /healthz Controller，E2E/探针应使用 /actuator/health/readiness:8090。
     * 保留 /healthz 的 isLocal=true 会导致 Netty 边缘层把请求路由到本地(无 handler)，返回 404/超时。
     */
    public static boolean isLocal(String path) {
        return path != null && (path.startsWith("/api/auth")
                || path.startsWith("/actuator")
                || path.startsWith("/v3/api-docs") || path.startsWith("/docs")
                || path.startsWith("/swagger-ui") || path.startsWith("/openapi-go-rust.yaml"));
    }

    /** 获取当前路由表快照(只读,用于监控/调试)。 */
    public static Map<String, String> snapshot() {
        return Map.copyOf(routes);
    }
}
