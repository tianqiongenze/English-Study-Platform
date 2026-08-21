package com.esp.bff.graphql;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.graphql.data.method.annotation.Argument;
import org.springframework.graphql.data.method.annotation.QueryMapping;
import org.springframework.graphql.data.method.annotation.SchemaMapping;
import org.springframework.stereotype.Controller;
import org.springframework.web.reactive.function.client.WebClient;

import java.util.List;
import java.util.Map;

/**
 * 排行榜 GraphQL fetcher(Phase1 Task C,2026-08-21)。
 *
 * <p>聚合价值:前端排行榜页原先 3 串行 REST(rankings + rankingGap + rankingRegions)
 * → 1 GraphQL Query,减少 RTT,一次取全仪表盘数据。
 *
 * <p>下游 REST(已核实源码 rankings.go 真实返回形状,WTV 铁律):
 * <ul>
 *   <li>GET /api/gamification/rankings?type=&scope=&scope_id=&period=&limit=
 *       → {list:[{rank,user_id,name,score}], as_of, mine:{user_id,rank,score,gap_to_top}}</li>
 *   <li>GET /api/gamification/rankings/gap?type=&scope=&scope_id=&period=
 *       → {my_rank,my_score,total,gap_to_top,gap_to_prev,percentile,opted_out?}</li>
 *   <li>GET /api/gamification/rankings/regions → [{code,label}]</li>
 * </ul>
 *
 * <p>鉴权:/graphql 经 AuthFilter 校验 JWT 注入 X-User-Id,本 fetcher 透传该 header 到
 * gamification-go(其 rankings 取 X-User-Id 算 mine)。userId 不从 GraphQL 参数取,防越权伪造。
 *
 * <p>ASV(javap 核实 spring-graphql 2.0.4):
 * {@code @QueryMapping} 标注 Query 字段 fetcher(方法名默认=字段名),
 * {@code @Argument} 取 GraphQL 参数,
 * {@code @SchemaMapping(type=,field=)} 取子字段(avatar 批加载见 {@link AvatarDataLoader})。
 *
 * <p>RWT(韧性):下游调用复用 bff 共享 WebClient(4s 超时,见 WebClientConfig),
 * 失败抛异常由 GraphQL 错误传播(不静默返 null 掩盖)。单节点 5000 CCU 由 Netty edge 层消化并发。
 */
@Controller
public class RankingFetcher {

    private static final Logger log = LoggerFactory.getLogger(RankingFetcher.class);

    /** gamification-go REST 基址(K8s ClusterIP 短名,经 bff 内 WebClient 直连)。 */
    private static final String GAMIFICATION_BASE = "http://gamification-go:80";

    private final WebClient webClient;

    @Autowired
    public RankingFetcher(WebClient webClient) {
        // CLC nil-safe:WebClient 由 WebClientConfig 注入(已 .build());防御性兜底
        this.webClient = webClient != null ? webClient : WebClient.create();
    }

    // ===== rankings:主榜 =====

    /**
     * Query.rankings fetcher。聚合 gamification-go /api/gamification/rankings。
     * 返回原始 Map(GraphQL 按 schema 字段名自动映射;snake_case→camelCase 由 @SchemaMapping 兜底)。
     */
    @QueryMapping
    public Map<String, Object> rankings(
            @Argument String type,
            @Argument String scope,
            @Argument String scopeId,
            @Argument String period,
            @Argument Integer limit,
            graphql.schema.DataFetchingEnvironment env) {
        String userId = currentUserId(env);
        // 默认值对齐 gamification-go(xp/global/all/weekly/20)
        String t = type != null ? type : "xp";
        String sc = scope != null ? scope : "global";
        String sid = scopeId != null ? scopeId : "all";
        String p = period != null ? period : "weekly";
        int lim = limit != null ? limit : 20;

        String uri = java.net.URI.create(GAMIFICATION_BASE
                + "/api/gamification/rankings?type=" + enc(t)
                + "&scope=" + enc(sc) + "&scope_id=" + enc(sid)
                + "&period=" + enc(p) + "&limit=" + lim).toString();
        log.debug("[graphql rankings] userId={} uri={}", userId, uri);
        return webClient.get()
                .uri(uri)
                .header("X-User-Id", userId)   // 透传可信 userId 给下游算 mine
                .retrieve()
                .bodyToMono(new org.springframework.core.ParameterizedTypeReference<Map<String, Object>>() {})
                .block();   // 同步阻塞(GraphQL fetcher 同步语义;并发由 edge 层消化)
    }

    // 子字段映射:GraphQL RankingResult.entries ← REST list;asOf ← as_of
    @SchemaMapping(typeName = "RankingResult", field = "entries")
    public List<Map<String, Object>> entries(Map<String, Object> result) {
        Object list = result == null ? null : result.get("list");
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> rows = list instanceof List ? (List<Map<String, Object>>) list : List.of();
        return rows;
    }

    @SchemaMapping(typeName = "RankingResult", field = "mine")
    public Map<String, Object> mine(Map<String, Object> result) {
        Object m = result == null ? null : result.get("mine");
        @SuppressWarnings("unchecked")
        Map<String, Object> mine = m instanceof Map ? (Map<String, Object>) m : Map.of();
        return mine;
    }

    @SchemaMapping(typeName = "RankingResult", field = "asOf")
    public String asOf(Map<String, Object> result) {
        Object v = result == null ? null : result.get("as_of");
        return v == null ? null : v.toString();
    }

    // RankingEntry 子字段:rank/userId/name/score ← REST rank/user_id/name/score
    @SchemaMapping(typeName = "RankingEntry", field = "userId")
    public String entryUserId(Map<String, Object> entry) {
        Object v = entry == null ? null : entry.get("user_id");
        return v == null ? null : v.toString();
    }
    @SchemaMapping(typeName = "RankingEntry", field = "rank")
    public Integer entryRank(Map<String, Object> entry) {
        return asInt(entry, "rank");
    }
    @SchemaMapping(typeName = "RankingEntry", field = "score")
    public Double entryScore(Map<String, Object> entry) {
        return asDouble(entry, "score");
    }
    @SchemaMapping(typeName = "RankingEntry", field = "name")
    public String entryName(Map<String, Object> entry) {
        Object v = entry == null ? null : entry.get("name");
        return v == null ? null : v.toString();
    }

    // RankingMine 子字段:gapToTop ← gap_to_top
    @SchemaMapping(typeName = "RankingMine", field = "gapToTop")
    public Double mineGapToTop(Map<String, Object> mine) {
        return asDouble(mine, "gap_to_top");
    }
    @SchemaMapping(typeName = "RankingMine", field = "userId")
    public String mineUserId(Map<String, Object> mine) {
        Object v = mine == null ? null : mine.get("user_id");
        return v == null ? null : v.toString();
    }
    @SchemaMapping(typeName = "RankingMine", field = "rank")
    public Integer mineRank(Map<String, Object> mine) {
        return asInt(mine, "rank");
    }
    @SchemaMapping(typeName = "RankingMine", field = "score")
    public Double mineScore(Map<String, Object> mine) {
        return asDouble(mine, "score");
    }

    // ===== rankingGap:我与榜首差距 =====

    @QueryMapping
    public Map<String, Object> rankingGap(
            @Argument String type,
            @Argument String scope,
            @Argument String scopeId,
            @Argument String period,
            graphql.schema.DataFetchingEnvironment env) {
        String userId = currentUserId(env);
        String t = type != null ? type : "xp";
        String sc = scope != null ? scope : "global";
        String sid = scopeId != null ? scopeId : "all";
        String p = period != null ? period : "weekly";
        String uri = GAMIFICATION_BASE + "/api/gamification/rankings/gap?type=" + enc(t)
                + "&scope=" + enc(sc) + "&scope_id=" + enc(sid) + "&period=" + enc(p);
        return webClient.get()
                .uri(uri)
                .header("X-User-Id", userId)
                .retrieve()
                .bodyToMono(new org.springframework.core.ParameterizedTypeReference<Map<String, Object>>() {})
                .block();
    }

    // RankingGap 子字段映射(my_rank/my_score/gap_to_top/gap_to_prev/opted_out)
    @SchemaMapping(typeName = "RankingGap", field = "myRank")
    public Integer gapMyRank(Map<String, Object> g) { return asInt(g, "my_rank"); }
    @SchemaMapping(typeName = "RankingGap", field = "myScore")
    public Double gapMyScore(Map<String, Object> g) { return asDouble(g, "my_score"); }
    @SchemaMapping(typeName = "RankingGap", field = "total")
    public Integer gapTotal(Map<String, Object> g) { return asInt(g, "total"); }
    @SchemaMapping(typeName = "RankingGap", field = "gapToTop")
    public Double gapGapToTop(Map<String, Object> g) { return asDouble(g, "gap_to_top"); }
    @SchemaMapping(typeName = "RankingGap", field = "gapToPrev")
    public Double gapGapToPrev(Map<String, Object> g) { return asDouble(g, "gap_to_prev"); }
    @SchemaMapping(typeName = "RankingGap", field = "percentile")
    public Double gapPercentile(Map<String, Object> g) { return asDouble(g, "percentile"); }
    @SchemaMapping(typeName = "RankingGap", field = "optedOut")
    public Boolean gapOptedOut(Map<String, Object> g) {
        Object v = g == null ? null : g.get("opted_out");
        return v instanceof Boolean ? (Boolean) v : null;
    }

    // ===== rankingRegions:可用地域 =====

    @QueryMapping
    public List<Map<String, Object>> rankingRegions() {
        String uri = GAMIFICATION_BASE + "/api/gamification/rankings/regions";
        return webClient.get()
                .uri(uri)
                .retrieve()
                .bodyToMono(new org.springframework.core.ParameterizedTypeReference<List<Map<String, Object>>>() {})
                .block();
    }
    // RegionOption code/label 与 REST 同名,GraphQL 自动映射,无需 @SchemaMapping。

    // ===== helpers =====

    /** 从 GraphQLContext 取当前登录用户 ID(@auth 已校验非空,此处 nil-safe 防御)。 */
    private static String currentUserId(graphql.schema.DataFetchingEnvironment env) {
        Object uid = env.getGraphQlContext().get(GraphQLConfig.CTX_USER_ID);
        return uid == null ? "" : uid.toString();
    }

    private static String enc(String s) {
        try {
            return java.net.URLEncoder.encode(s, java.nio.charset.StandardCharsets.UTF_8);
        } catch (Exception e) {
            return s;
        }
    }

    private static Integer asInt(Map<String, Object> m, String k) {
        if (m == null) return null;
        Object v = m.get(k);
        if (v == null) return null;
        if (v instanceof Number n) return n.intValue();
        try { return Integer.valueOf(v.toString()); } catch (Exception e) { return null; }
    }

    private static Double asDouble(Map<String, Object> m, String k) {
        if (m == null) return null;
        Object v = m.get(k);
        if (v == null) return null;
        if (v instanceof Number n) return n.doubleValue();
        try { return Double.valueOf(v.toString()); } catch (Exception e) { return null; }
    }
}
