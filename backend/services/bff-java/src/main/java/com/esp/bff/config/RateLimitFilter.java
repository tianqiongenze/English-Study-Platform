package com.esp.bff.config;

import com.alibaba.csp.sentinel.Entry;
import com.alibaba.csp.sentinel.SphU;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRule;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRuleManager;
import jakarta.annotation.PostConstruct;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.nio.charset.StandardCharsets;

/**
 * Sentinel 限流(ADR-08/ADR-12,Web MVC 版,替代原 WebFlux WebFilter)。
 * - 仅对进入 Spring MVC 控制器的请求生效(Netty 边缘转发路径不经此 Filter)。
 * - 每资源 QPS 阈值,超限返 429(标准化 JSON 错误体)。
 * 规则:/api/learning/event、/api/writing/** 高频入口单独限流。
 *
 * 规则来源:Nacos 动态规则(application.yml 中 spring.cloud.sentinel.datasource.flow/degrade)。
 * 不再硬编码 loadRules —— Nacos 是规则单一来源,支持热更新;硬编码 loadRules 会覆盖 Nacos 推送。
 * 启动时若 Nacos 尚无 flow 规则,记录 WARN 以提示运维(不阻断启动)。
 */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE + 1) // 在 AuthFilter 之后
public class RateLimitFilter extends OncePerRequestFilter {

    private static final Logger log = LoggerFactory.getLogger(RateLimitFilter.class);

    private static final String BODY = "{\"error\":\"rate_limited\",\"message\":\"too many requests\"}";

    @PostConstruct
    public void init() {
        // 不硬编码 loadRules(Nacos 推送是规则单一来源)。仅作 fallback 检查并告警。
        // 注:datasource 在 Spring Bean 初始化时序中加载,此处检查可能早于 Nacos 首次拉取完成,
        // 因此空规则不一定是异常 —— 仅 WARN,由 Sentinel dashboard / Nacos 控制台人工核验。
        java.util.List<FlowRule> rules = FlowRuleManager.getRules();
        if (rules == null || rules.isEmpty()) {
            log.warn("[RateLimitFilter] Nacos flow 规则尚未加载(可能首次拉取时序);"
                    + "请到 Nacos 控制台确认 dataId=bff-java-flow-rules / groupId=SENTINEL_GROUP 已发布。"
                    + "在规则就绪前,所有资源默认不限流(由下游服务兜底)。");
        } else {
            log.info("[RateLimitFilter] Nacos flow 规则已加载,共 {} 条:{}", rules.size(),
                    rules.stream().map(FlowRule::getResource).toList());
        }
    }

    @Override
    protected void doFilterInternal(HttpServletRequest req, HttpServletResponse res, FilterChain chain)
            throws ServletException, IOException {
        String path = req.getRequestURI();
        String resource = path.startsWith("/api/learning/event") ? "api-learning-event"
                : path.startsWith("/api/writing/") ? "api-writing-grade" : "api-default";
        try (Entry ignored = SphU.entry(resource)) {
            chain.doFilter(req, res);
        } catch (BlockException e) {
            res.setStatus(HttpStatus.TOO_MANY_REQUESTS.value());
            res.setContentType(MediaType.APPLICATION_JSON_VALUE);
            res.setCharacterEncoding(StandardCharsets.UTF_8.name());
            res.getWriter().write(BODY);
        }
    }
}
