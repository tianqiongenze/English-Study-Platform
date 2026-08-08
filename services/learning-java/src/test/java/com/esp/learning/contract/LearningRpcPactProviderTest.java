package com.esp.learning.contract;

import com.esp.api.dubbo.KpStat;
import com.esp.learning.domain.UserMastery;
import com.esp.learning.rpc.LearningRpcProvider;
import com.esp.learning.service.LearningService;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.File;
import java.io.InputStream;
import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Provider-side contract verification:读取 consumer(teacher-java)生成的 pact 文件,
 * 用真实 LearningRpcProvider(mock LearningService,复用 LearningRpcProviderTest 的 mock 方式)
 * 调用 userStats(42),把返回的 List&lt;KpStat&gt; 逐字段与契约期望比对。
 *
 * Dubbo Triple 无原生 Pact transport,provider 验证走 JVM 直接调用 + 手写字段断言。
 * 若 pact 文件路径不存在(如 CI 中 consumer 先跑),fallback 读 classpath 拷贝。
 */
class LearningRpcPactProviderTest {

    private static final String PACT_PATH = "../teacher-java/target/pacts/teacher-learningrpc-pact.json";
    private static final String PACT_FALLBACK = "pacts/teacher-learningrpc-pact.json";

    private LearningService learningService;
    private LearningRpcProvider provider;
    private JsonNode pactInteractions;

    @BeforeEach
    void setUp() throws Exception {
        // mock LearningService(复用 LearningRpcProviderTest 的 mock 方式)
        learningService = mock(LearningService.class);
        provider = new LearningRpcProvider(learningService);

        // 读取 pact 文件:先试文件系统路径(CI consumer 先跑),再试 classpath fallback
        ObjectMapper mapper = new ObjectMapper();
        File pactFile = new File(PACT_PATH);
        if (pactFile.exists()) {
            pactInteractions = mapper.readTree(pactFile).get("interactions");
        } else {
            try (InputStream is = getClass().getClassLoader().getResourceAsStream(PACT_FALLBACK)) {
                assertNotNull(is, "pact 文件既不在 " + PACT_PATH + " 也不在 classpath " + PACT_FALLBACK);
                pactInteractions = mapper.readTree(is).get("interactions");
            }
        }
    }

    /**
     * 验证 provider 的 userStats(42) 输出满足 consumer 契约:
     *  - 返回 List<KpStat> 的元素字段与 pact 期望体一一比对
     *  - 复用 LearningRpcProviderTest 的 UserMastery mock 方式构造 provider 输入
     */
    @Test
    void provider_userStats_matchesConsumerPact() {
        // 1. 准备 provider 输入:与 pact consumer 期望的 userId=42 对应
        UserMastery m1 = new UserMastery(42L, 10L);
        m1.setMasteryScore(BigDecimal.valueOf(3.5));
        m1.setReviewCount(2);
        m1.setNextReviewAt(Instant.parse("2026-08-01T00:00:00Z"));

        UserMastery m2 = new UserMastery(42L, 20L);
        m2.setMasteryScore(BigDecimal.ZERO);
        m2.setReviewCount(0);
        m2.setNextReviewAt(null);

        when(learningService.weakestPoints(42L)).thenReturn(List.of(m1, m2));

        // 2. 调用真实 provider
        List<KpStat> actual = provider.userStats(42L);

        // 3. 从 pact 文件读取 consumer 期望的响应体
        JsonNode interaction = pactInteractions.get(0);
        JsonNode expectedBody = interaction.get("response").get("body");
        JsonNode expectedStats = expectedBody.get("stats");

        // 4. 逐元素逐字段比对:provider 输出满足 consumer 契约
        assertEquals(expectedStats.size(), actual.size(),
                "provider 返回元素数应与契约期望一致");

        for (int i = 0; i < actual.size(); i++) {
            JsonNode expectedStat = expectedStats.get(i);
            KpStat actualStat = actual.get(i);

            // kpId:契约期望整数,provider 返回 long
            assertEquals(expectedStat.get("kpId").asLong(), actualStat.getKpId(),
                    "kpId 应与契约一致");

            // masteryScore:契约期望数值,provider 返回 double
            assertEquals(expectedStat.get("masteryScore").asDouble(), actualStat.getMasteryScore(), 1e-6,
                    "masteryScore 应与契约一致");

            // reviewCount:契约期望整数,provider 返回 int
            assertEquals(expectedStat.get("reviewCount").asInt(), actualStat.getReviewCount(),
                    "reviewCount 应与契约一致");

            // nextReviewAt:契约期望字符串或 null,provider 返回 String
            if (expectedStat.get("nextReviewAt").isNull()) {
                assertNull(actualStat.getNextReviewAt(), "nextReviewAt 契约为 null,provider 应返回 null");
            } else {
                assertEquals(expectedStat.get("nextReviewAt").asText(), actualStat.getNextReviewAt(),
                        "nextReviewAt 应与契约一致");
            }
        }
    }

    /**
     * 验证 pact 文件中 interaction 的描述与 provider 方法对应。
     */
    @Test
    void pactInteraction_describesUserStats() {
        JsonNode interaction = pactInteractions.get(0);
        String description = interaction.get("description").asText();
        assertTrue(description.contains("userStats"),
                "pact interaction 应描述 userStats 方法");

        JsonNode request = interaction.get("request");
        assertEquals("DUBBO", request.get("method").asText(),
                "请求方法应为 DUBBO");
        assertTrue(request.get("path").asText().contains("LearningRpcService.userStats"),
                "请求路径应含 LearningRpcService.userStats");
    }

    /**
     * 验证 provider 对空列表也满足契约(consumer 期望至少0个元素)。
     */
    @Test
    void provider_userStats_emptyList_stillConformsToContract() {
        when(learningService.weakestPoints(42L)).thenReturn(List.of());
        List<KpStat> actual = provider.userStats(42L);
        assertTrue(actual.isEmpty(), "空 userId 应返回空列表");
    }
}
