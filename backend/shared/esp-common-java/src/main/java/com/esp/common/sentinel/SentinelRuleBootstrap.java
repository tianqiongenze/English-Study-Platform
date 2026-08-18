package com.esp.common.sentinel;

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
import java.util.concurrent.TimeUnit;

/**
 * Sentinel 规则启动兜底重拉(SCA NacosDataSource 时序 bug workaround,共用版)。
 * <p>
 * 根因:SCA 2025.1.0.0 + nacos client gRPC 模式下,NacosDataSource.loadInitialConfig()
 * 在 nacos gRPC 连接建立前执行(实测早 ~39s),拉到空 source。
 * <p>
 * 本监听器在 ApplicationReadyEvent(gRPC 必已连)主动从 nacos HTTP v1 API 拉一次 flow/degrade rules
 * 并 loadRules() 覆盖式加载。仅在当前规则为空时兜底,避免与 SCA listener 后续热更新冲突。
 *
 * <p>收编自 bff-java config/SentinelRuleBootstrap,三服务共用。
 * 使用方在 Spring 上下文中引入本类(@ComponentScan 或 @Import)即可激活。
 */
@Component
public class SentinelRuleBootstrap {

    private static final Logger log = LoggerFactory.getLogger(SentinelRuleBootstrap.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Value("${NACOS_ADDR:nacos:8848}")
    private String nacosAddr;

    @Value("${NACOS_NS:public}")
    private String namespace;

    @Value("${spring.application.name}")
    private String appName;

    @EventListener(ApplicationReadyEvent.class)
    public void bootstrapRules() {
        log.info("[SentinelBootstrap] ApplicationReady, addr={}, ns={}, app={}", nacosAddr, namespace, appName);
        String ns = "public".equals(namespace) ? "" : namespace;
        loadFlowRules(ns);
        loadDegradeRules(ns);
    }

    private void loadFlowRules(String ns) {
        if (!FlowRuleManager.getRules().isEmpty()) {
            log.info("[SentinelBootstrap] Flow rules already loaded ({}), skip.", FlowRuleManager.getRules().size());
            return;
        }
        String dataId = appName + "-flow-rules";
        List<FlowRule> rules = fetchRules(dataId, ns, new TypeReference<List<FlowRule>>() {});
        if (rules != null && !rules.isEmpty()) {
            FlowRuleManager.loadRules(rules);
            log.info("[SentinelBootstrap] Flow rules bootstrapped: {} (dataId={})", rules.size(), dataId);
        }
    }

    private void loadDegradeRules(String ns) {
        if (!DegradeRuleManager.getRules().isEmpty()) {
            log.info("[SentinelBootstrap] Degrade rules already loaded ({}), skip.", DegradeRuleManager.getRules().size());
            return;
        }
        String dataId = appName + "-degrade-rules";
        List<DegradeRule> rules = fetchRules(dataId, ns, new TypeReference<List<DegradeRule>>() {});
        if (rules != null && !rules.isEmpty()) {
            DegradeRuleManager.loadRules(rules);
            log.info("[SentinelBootstrap] Degrade rules bootstrapped: {} (dataId={})", rules.size(), dataId);
        }
    }

    /**
     * 从 nacos HTTP v1 API 拉 config 并反序列化。null=拉取失败,empty list=内容为空。
     * 含 3 次重试(每次间隔 2s)。
     */
    private <T> List<T> fetchRules(String dataId, String ns, TypeReference<List<T>> typeRef) {
        String groupId = "SENTINEL_GROUP";
        String url = String.format("http://%s/nacos/v1/cs/configs?dataId=%s&group=%s&tenant=%s",
                nacosAddr, dataId, groupId, ns);
        for (int attempt = 1; attempt <= 3; attempt++) {
            try {
                if (attempt > 1) TimeUnit.SECONDS.sleep(2);
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
                log.info("[SentinelBootstrap] fetch attempt={} dataId={} status={} len={}",
                        attempt, dataId, code, content == null ? -1 : content.length());
                if (code == 200 && content != null && !content.trim().isEmpty()) {
                    return MAPPER.readValue(content, typeRef);
                }
            } catch (Exception e) {
                log.warn("[SentinelBootstrap] Pull failed dataId={}, attempt={}: {}", dataId, attempt, e.toString());
            }
        }
        return null;
    }
}
