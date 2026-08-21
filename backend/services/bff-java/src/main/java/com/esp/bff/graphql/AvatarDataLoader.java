package com.esp.bff.graphql;

import org.springframework.graphql.data.method.annotation.BatchMapping;
import org.springframework.stereotype.Controller;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * 排行榜头像批量加载器(Phase1 Task C,2026-08-21)——演示 N+1 防护机制。
 *
 * <p>问题:GraphQL 查询排行榜 entries 时,若每个 RankingEntry.avatar 单独 fetch,会触发
 * N 次 REST(N+1 问题:1 次取 list + N 次取每个用户头像)。
 *
 * <p>解法:{@code @BatchMapping} 把同层多个 RankingEntry 的 avatar 请求批量化 ——
 * graphql-java DataLoader 把一帧内对 avatar 的 N 次请求合并成 1 次 batch 调用,
 * 返回 Map<父对象, avatar>。本试点用单次批量计算首字母占位头像(无真实头像源时降级)。
 *
 * <p>ASV(2026-08-21 spring-graphql 2.0.4 sources 核实):{@code @BatchMapping} 方法签名
 * {@code Map<Parent, Value> method(List<Parent> parents)} —— Parent 是 GraphQL 父对象
 * (此处 RankingEntry,运行期为 LinkedHashMap,其 equals/hashCode 按内容稳定,可作 Map key)。
 * 返回 Map<父对象, 头像URL>,DataLoader 按父对象查回填 avatar 字段。
 *
 * <p>诚实降级(RWT/诚实优先铁律):当前 users 表无 avatar 列、无 user-service 头像端点,
 * 故返回 name 首字母占位头像(DiceBear HTTP API,前端常用方案),证明 DataLoader 批量机制可用。
 * 真实头像源接入后,此方法改为批量 REST 调用即可,签名不变。
 */
@Controller
public class AvatarDataLoader {

    /** DiceBear 占位头像 API(前端常用,name 首字母生成 SVG,无需鉴权)。 */
    private static final String AVATAR_API = "https://api.dicebear.com/7.x/initials/svg?seed=";

    /**
     * 批量取 RankingEntry.avatar。
     *
     * @param entries 一帧内所有 RankingEntry(父对象 Map,含 user_id/name)
     * @return 父对象 → avatarUrl 映射(spring-graphql DataLoader 按父对象回填)
     */
    @BatchMapping(typeName = "RankingEntry", field = "avatar")
    public Map<Map<String, Object>, String> avatar(List<Map<String, Object>> entries) {
        Map<Map<String, Object>, String> out = new HashMap<>();
        if (entries == null) return out;
        for (Map<String, Object> e : entries) {
            if (e == null) continue;
            // 用 name 生成首字母头像;无 name 用 user_id 兜底(CLC nil-safe)
            Object name = e.get("name");
            Object uid = e.get("user_id");
            String seed = (name != null && !name.toString().isBlank())
                    ? name.toString()
                    : ("user" + (uid != null ? uid.toString() : ""));
            out.put(e, AVATAR_API + urlEncode(seed));
        }
        return out;
    }

    private static String urlEncode(String s) {
        try {
            return java.net.URLEncoder.encode(s, java.nio.charset.StandardCharsets.UTF_8);
        } catch (Exception e) {
            return s;
        }
    }
}
