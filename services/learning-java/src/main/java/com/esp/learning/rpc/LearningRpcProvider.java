package com.esp.learning.rpc;

import com.esp.api.dubbo.KpStat;
import com.esp.api.dubbo.LearningRpcService;
import com.esp.learning.domain.UserMastery;
import com.esp.rw.DataSource;
import com.esp.rw.DataSourceType;
import com.esp.learning.service.LearningService;
import org.apache.dubbo.config.annotation.DubboService;

import java.util.List;

/**
 * 学习域 Dubbo Triple 服务实现(ADR-08, API 契约 §7)。
 * provider: learning-java;Triple 协议(gRPC over HTTP/2)注册到 Nacos。
 * userStats/weakCount 走读副本聚合。
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
        List<UserMastery> list = learningService.weakestPoints(userId);
        return list.stream()
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
}
