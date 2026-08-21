package com.esp.bff.graphql;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.graphql.data.method.annotation.QueryMapping;
import org.springframework.graphql.data.method.annotation.SchemaMapping;
import org.springframework.stereotype.Controller;
import org.springframework.web.reactive.function.client.WebClient;

import java.util.List;
import java.util.Map;

/**
 * 学情 GraphQL fetcher(Phase1 Task C,2026-08-21)。
 *
 * <p>聚合价值:前端学情仪表盘原先 2 串行 REST(review + weak)→ 1 GraphQL Query,
 * 一次取全今日复习清单 + 薄弱知识点。
 *
 * <p>下游 REST(已核实 LearningService.java 真实返回形状,WTV 铁律):
 * <ul>
 *   <li>GET /api/learning/review → List<UserMastery>(Jackson 序列化为 [{userId,kpId,masteryScore,reviewCount,nextReviewAt,lastReviewAt}])</li>
 *   <li>GET /api/learning/mastery/weak → List<UserMastery>(同上,按 masteryScore 升序)</li>
 * </ul>
 * 注:learning-java L2 缓存命中时返回 List<LinkedHashMap>(字段同上,跨语言 JSON 契约)。
 *
 * <p>鉴权:learning-java review/weak 从 X-User-Id header 取 userId(已移除 query user_id 回落,
 * 防查他人)。本 fetcher 透传 AuthFilter 注入的可信 X-User-Id。
 */
@Controller
public class LearningFetcher {

    private static final Logger log = LoggerFactory.getLogger(LearningFetcher.class);

    private static final String LEARNING_BASE = "http://learning-java:8081";

    private final WebClient webClient;

    @Autowired
    public LearningFetcher(WebClient webClient) {
        this.webClient = webClient != null ? webClient : WebClient.create();
    }

    @QueryMapping
    public List<Map<String, Object>> reviewList(graphql.schema.DataFetchingEnvironment env) {
        String userId = currentUserId(env);
        log.debug("[graphql reviewList] userId={}", userId);
        return webClient.get()
                .uri(LEARNING_BASE + "/api/learning/review")
                .header("X-User-Id", userId)
                .retrieve()
                .bodyToMono(new org.springframework.core.ParameterizedTypeReference<List<Map<String, Object>>>() {})
                .block();
    }

    @QueryMapping
    public List<Map<String, Object>> weakPoints(graphql.schema.DataFetchingEnvironment env) {
        String userId = currentUserId(env);
        return webClient.get()
                .uri(LEARNING_BASE + "/api/learning/mastery/weak")
                .header("X-User-Id", userId)
                .retrieve()
                .bodyToMono(new org.springframework.core.ParameterizedTypeReference<List<Map<String, Object>>>() {})
                .block();
    }

    // MasteryItem 子字段映射:REST userId/kpId/masteryScore/nextReviewAt → GraphQL kpId/masteryScore/masteryInt/nextReviewAt
    // kpId/masteryScore/nextReviewAt 与 REST 同名 camelCase,GraphQL 自动映射;仅 masteryInt 需手取 reviewCount 或 score 取整。
    @SchemaMapping(typeName = "MasteryItem", field = "masteryInt")
    public Integer masteryInt(Map<String, Object> item) {
        // masteryInt:优先取 masteryScore 取整(schema 注释:整型掌握度)
        Object v = item == null ? null : item.get("masteryScore");
        if (v == null) return null;
        if (v instanceof Number n) return n.intValue();
        try { return Double.valueOf(v.toString()).intValue(); } catch (Exception e) { return null; }
    }

    private static String currentUserId(graphql.schema.DataFetchingEnvironment env) {
        Object uid = env.getGraphQlContext().get(GraphQLConfig.CTX_USER_ID);
        return uid == null ? "" : uid.toString();
    }
}
