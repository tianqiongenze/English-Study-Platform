package com.esp.learning.rpc;

import com.esp.api.dubbo.KpStat;
import com.esp.api.dubbo.LearningRpcService;
import com.esp.learning.domain.UserMastery;
import com.esp.rw.DataSource;
import com.esp.rw.DataSourceType;
import com.esp.learning.service.LearningService;
import org.apache.dubbo.config.annotation.DubboService;

import java.util.List;
import java.util.Map;

/**
 * 学习域 Dubbo Triple 服务实现(ADR-08, API 契约 §7)。
 * provider: learning-java;Triple 协议(gRPC over HTTP/2)注册到 Nacos。
 * userStats/weakCount 走读副本聚合。
 *
 * <p>§2.2 跨语言纯 JSON 契约(2026-08-15):CacheConfig 关闭 defaultTyping 后,
 * {@code weakestPoints()} 在 L2 命中时返回 {@code List<LinkedHashMap>} 而非 {@code List<UserMastery>}。
 * 本 provider 调 getter 组装 KpStat,需防御性类型还原。L1 命中时返回原对象不需还原。
 * {@link #toUserMastery} 统一适配两条路径。
 */
@DubboService(version = "1.0.0", group = "esp")
public class LearningRpcProvider implements LearningRpcService {

    private final LearningService learningService;

    public LearningRpcProvider(LearningService learningService) {
        this.learningService = learningService;
    }

    @Override
    @DataSource(value = DataSourceType.READ, consistent = true)
    public List<KpStat> userStats(Long userId) {
        List<?> list = learningService.weakestPoints(userId);
        return list.stream()
                .map(this::toUserMastery)
                .map(m -> new KpStat(
                        m.getKpId(),
                        m.getMasteryScore() == null ? 0d : m.getMasteryScore().doubleValue(),
                        m.getReviewCount() == null ? 0 : m.getReviewCount(),
                        m.getNextReviewAt() == null ? null : m.getNextReviewAt().toString()))
                .toList();
    }

    @Override
    @DataSource(value = DataSourceType.READ, consistent = true)
    public int weakCount(Long userId) {
        return learningService.weakestPoints(userId).size();
    }

    /**
     * §2.2 适配:L2 命中返回 LinkedHashMap,转回 UserMastery 调 getter;L1 命中直接强转。
     * 只在 RPC provider 需强类型 getter 时用;HTTP 入口(LearningController/BehaviorController)
     * 交 Spring Jackson 序列化响应体,不强类型,无需适配。
     */
    @SuppressWarnings("unchecked")
    private UserMastery toUserMastery(Object o) {
        if (o instanceof UserMastery m) return m;
        if (o instanceof Map<?, ?> map) {
            UserMastery m = new UserMastery();
            m.setUserId(asLong(map.get("userId")));
            m.setKpId(asLong(map.get("kpId")));
            Object score = map.get("masteryScore");
            if (score instanceof Number n) m.setMasteryScore(java.math.BigDecimal.valueOf(n.doubleValue()));
            Object rc = map.get("reviewCount");
            if (rc instanceof Number n) m.setReviewCount(n.intValue());
            Object nra = map.get("nextReviewAt");
            if (nra instanceof String s) m.setNextReviewAt(java.time.Instant.parse(s));
            else if (nra instanceof java.time.Instant inst) m.setNextReviewAt(inst);
            return m;
        }
        throw new ClassCastException("cannot adapt " + (o == null ? "null" : o.getClass().getName()) + " to UserMastery");
    }

    private static Long asLong(Object o) {
        if (o == null) return null;
        if (o instanceof Number n) return n.longValue();
        return Long.valueOf(o.toString());
    }
}
