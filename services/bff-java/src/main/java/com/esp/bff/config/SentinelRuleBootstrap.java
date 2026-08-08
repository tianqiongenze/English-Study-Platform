package com.esp.bff.config;

import com.alibaba.csp.sentinel.slots.block.degrade.DegradeRule;
import com.alibaba.csp.sentinel.slots.block.degrade.DegradeRuleManager;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRule;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRuleManager;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.context.event.EventListener;
import org.springframework.stereotype.Component;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;
import java.util.Properties;
import java.util.concurrent.TimeUnit;

/**
 * Sentinel 规则启动兜底重拉(SCA NacosDataSource 时序 bug workaround)。
 *
 * 根因:SCA 2025.1.0.0 + nacos client gRPC 模式下,NacosDataSource.loadInitialConfig()
 * 在 nacos gRPC 连接建立前执行(实测早 ~39s),拉到空 source → "converter can not convert
 * rules because source is empty" → FlowRule loaded: {}。SCA 不在连接建立后重拉,POST 推送
 * 也不触发 reload,getRules?type=flow 仍 []。
 *
 * 本监听器在 ApplicationReadyEvent(gRPC 必已连)主动从 nacos 拉一次 flow/degrade rules
 * 并 loadRules() 覆盖式加载。仅在当前规则为空时兜底,避免与 SCA listener 后续热更新冲突。
 */
@Component
public class SentinelRuleBootstrap {

    private static final Logger log = LoggerFactory.getLogger(SentinelRuleBootstrap.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final long FETCH_TIMEOUT_MS = 5000L;

    @Value("${NACOS_ADDR:nacos:8848}")
    private String nacosAddr;

    @Value("${NACOS_NS:public}")
    private String namespace;

    @Value("${spring.application.name}")
    private String appName;

    @EventListener(ApplicationReadyEvent.class)
    public void bootstrapRules() {
        log.info("[SentinelBootstrap] ApplicationReady, start fallback pull sentinel rules from nacos. addr={}, ns={}, app={}", nacosAddr, namespace, appName);
        // namespace=public 在 nacos 中实际是空 namespace;显式传 "public" 会被当作命名空间名 → 拉空。
        String ns = "public".equals(namespace) ? "" : namespace;
        loadFlowRules(ns);
        loadDegradeRules(ns);
    }

    private void loadFlowRules(String ns) {
        // 仅在当前 flow 规则为空时兜底(SCA 已加载则不覆盖)
        if (!FlowRuleManager.getRules().isEmpty()) {
            log.info("[SentinelBootstrap] Flow rules already loaded ({}), skip fallback.", FlowRuleManager.getRules().size());
            return;
        }
        String dataId = appName + "-flow-rules";
        List<FlowRule> rules = fetchRules(dataId, ns, new TypeReference<List<FlowRule>>() {});
        if (rules != null && !rules.isEmpty()) {
            FlowRuleManager.loadRules(rules);
            log.info("[SentinelBootstrap] Sentinel flow rules bootstrapped: {} 条 (dataId={})", rules.size(), dataId);
        } else {
            log.warn("[SentinelBootstrap] Fallback flow rules pull got empty/null (dataId={}), skip.", dataId);
        }
    }

    private void loadDegradeRules(String ns) {
        if (!DegradeRuleManager.getRules().isEmpty()) {
            log.info("[SentinelBootstrap] Degrade rules already loaded ({}), skip fallback.", DegradeRuleManager.getRules().size());
            return;
        }
        String dataId = appName + "-degrade-rules";
        List<DegradeRule> rules = fetchRules(dataId, ns, new TypeReference<List<DegradeRule>>() {});
        if (rules != null && !rules.isEmpty()) {
            DegradeRuleManager.loadRules(rules);
            log.info("[SentinelBootstrap] Sentinel degrade rules bootstrapped: {} 条 (dataId={})", rules.size(), dataId);
        } else {
            log.warn("[SentinelBootstrap] Fallback degrade rules pull got empty/null (dataId={}), skip.", dataId);
        }
    }

    /**
     * 从 nacos 拉 config 并反序列化。null=拉取失败,empty list=内容为空。
     * 含 3 次重试(每次间隔 2s),覆盖 gRPC 偶发未就绪窗口。
     *
     * 实现说明:nacos client gRPC 模式下 NacosFactory.createConfigService().getConfig()
     * 在 SCA 环境中偶发返回 null(新建 ConfigService 的 gRPC 通道在 5s timeout 内未完成握手)。
     * 改用 HTTP v1 API 直拉(services/nacos 已暴露 8848 HTTP),绕开 gRPC 时序问题,稳定可靠。
     * 注意:nacos "public" namespace 在 HTTP API 中对应 tenant=""(空串)。
     */
    private <T> List<T> fetchRules(String dataId, String ns, TypeReference<List<T>> typeRef) {
        String groupId = "SENTINEL_GROUP";
        log.info("[SentinelBootstrap] fetchRules via HTTP v1 API: dataId={}, groupId={}, ns=[{}] (raw namespace={})", dataId, groupId, ns, namespace);
        String url = String.format("http://%s/nacos/v1/cs/configs?dataId=%s&group=%s&tenant=%s",
                nacosAddr, dataId, groupId, ns);
        for (int attempt = 1; attempt <= 3; attempt++) {
            try {
                if (attempt > 1) {
                    TimeUnit.SECONDS.sleep(2);
                }
                HttpClient client = HttpClient.newBuilder()
                        .connectTimeout(Duration.ofSeconds(3))
                        .build();
                HttpRequest req = HttpRequest.newBuilder()
                        .uri(URI.create(url))
                        .timeout(Duration.ofSeconds(5))
                        .GET()
                        .build();
                HttpResponse<String> resp = client.send(req, HttpResponse.BodyHandlers.ofString());
                String content = resp.body();
                int code = resp.statusCode();
                log.info("[SentinelBootstrap] HTTP getConfig attempt={} dataId={} status={} len={}", attempt, dataId, code, content == null ? -1 : content.length());
                if (code == 200 && content != null && !content.trim().isEmpty()) {
                    log.info("[SentinelBootstrap] Fetched nacos config dataId={}, attempt={}, len={}", dataId, attempt, content.length());
                    return MAPPER.readValue(content, typeRef);
                }
                log.warn("[SentinelBootstrap] Empty/bad config dataId={}, attempt={}, status={}", dataId, attempt, code);
            } catch (Exception e) {
                log.warn("[SentinelBootstrap] Pull failed dataId={}, attempt={}: {}", dataId, attempt, e.toString());
            }
        }
        return null;
    }
}
