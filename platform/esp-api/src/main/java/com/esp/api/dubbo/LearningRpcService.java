package com.esp.api.dubbo;

import java.util.List;

/**
 * 学习域 Dubbo Triple 服务契约(ADR-08, API 契约 §7)。
 * - provider: learning-java
 * - consumer: teacher-java(学情分析时调用)
 * Triple 协议(gRPC over HTTP/2),注册到 Nacos。
 */
public interface LearningRpcService {

    /** 用户各知识点掌握统计(读副本聚合)。 */
    List<KpStat> userStats(Long userId);

    /** 用户薄弱知识点数量(供教师面板汇总)。 */
    int weakCount(Long userId);
}
