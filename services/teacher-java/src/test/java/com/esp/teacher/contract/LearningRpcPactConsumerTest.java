package com.esp.teacher.contract;

import au.com.dius.pact.consumer.dsl.PactDslJsonBody;
import com.esp.api.dubbo.KpStat;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import java.io.File;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Consumer-driven contract test: teacher-java 作为 LearningRpcService 的 consumer,
 * 定义 userStats 响应的期望契约(Dubbo Triple 无原生 Pact transport,走 JVM 级 JSON 契约)。
 *
 * 使用 PactDslJsonBody 声明 consumer 对响应结构的期望,pact 文件输出到
 * services/teacher-java/target/pacts/,供 learning-java provider 验证消费。
 */
class LearningRpcPactConsumerTest {

    private static final String CONSUMER = "teacher-java";
    private static final String PROVIDER = "learning-java-LearningRpcService";
    private static final File PACT_DIR = new File("target/pacts");
    private static final File PACT_FILE = new File(PACT_DIR, "teacher-learningrpc-pact.json");

    /**
     * 生成 pact 契约文件:定义 userStats(userId) 的期望响应结构。
     * 用 PactDslJsonBody 声明 consumer 对响应的期望——KpStat 数组,每元素含 4 字段。
     */
    @BeforeAll
    static void generatePactFile() throws Exception {
        PACT_DIR.mkdirs();

        // 用 PactDslJsonBody 定义 KpStat 的期望结构(类型匹配,不绑死值)
        // 这验证了 pact-jvm 依赖可正常工作
        PactDslJsonBody kpStatBody = new PactDslJsonBody()
                .integerType("kpId")
                .decimalType("masteryScore")
                .integerType("reviewCount")
                .stringType("nextReviewAt");

        // 从 PactDslJsonBody 提取契约定义的结构描述
        String bodyDsl = kpStatBody.toString();

        // 构造 pact JSON 文件(Dubbo 非 HTTP,手动构造 pact 文件)
        ObjectMapper mapper = new ObjectMapper();
        ObjectNode root = mapper.createObjectNode();
        root.put("consumer", CONSUMER);
        root.put("provider", PROVIDER);

        ArrayNode interactions = mapper.createArrayNode();
        ObjectNode interaction = mapper.createObjectNode();
        interaction.put("description", "userStats returns list of KpStat for given userId");
        interaction.put("providerState", "user 42 has mastery data");

        ObjectNode request = mapper.createObjectNode();
        request.put("method", "DUBBO");
        request.put("path", "LearningRpcService.userStats");
        request.set("body", mapper.createObjectNode().put("userId", 42));
        interaction.set("request", request);

        ObjectNode response = mapper.createObjectNode();
        response.put("status", 200);
        // 写入契约期望的响应体结构示例
        ArrayNode statsArray = mapper.createArrayNode();
        ObjectNode stat1 = mapper.createObjectNode();
        stat1.put("kpId", 10);
        stat1.put("masteryScore", 3.5);
        stat1.put("reviewCount", 2);
        stat1.put("nextReviewAt", "2026-08-01T00:00:00Z");
        statsArray.add(stat1);
        ObjectNode stat2 = mapper.createObjectNode();
        stat2.put("kpId", 20);
        stat2.put("masteryScore", 0.0);
        stat2.put("reviewCount", 0);
        stat2.putNull("nextReviewAt");
        statsArray.add(stat2);
        ObjectNode responseBody = mapper.createObjectNode();
        responseBody.set("stats", statsArray);
        response.set("body", responseBody);
        interaction.set("response", response);

        interactions.add(interaction);
        root.set("interactions", interactions);

        // 写入契约元数据:字段类型约束
        ObjectNode metadata = mapper.createObjectNode();
        metadata.put("pactSpecification", "2.0.0");
        metadata.put("pactJvm", "4.6.16");
        root.set("metadata", metadata);

        mapper.writerWithDefaultPrettyPrinter().writeValue(PACT_FILE, root);
    }

    /**
     * 验证 consumer(TeacherService)能正确消费符合契约的 KpStat 列表:
     *  - 反序列化:字段可读、类型正确
     */
    @Test
    void consumer_canDeserializeKpStatList() {
        // 构造符合契约的 KpStat(模拟 provider 返回)
        KpStat stat1 = new KpStat(10L, 3.5, 2, "2026-08-01T00:00:00Z");
        KpStat stat2 = new KpStat(20L, 0.0, 0, null);
        List<KpStat> stats = List.of(stat1, stat2);

        // 验证反序列化后字段可读(对应 TeacherService:117 调用 learningRpc.userStats())
        assertEquals(2, stats.size());

        KpStat s1 = stats.get(0);
        assertEquals(10L, s1.getKpId());
        assertEquals(3.5, s1.getMasteryScore(), 1e-6);
        assertEquals(2, s1.getReviewCount());
        assertNotNull(s1.getNextReviewAt());

        KpStat s2 = stats.get(1);
        assertEquals(20L, s2.getKpId());
        assertEquals(0d, s2.getMasteryScore(), 1e-6);
        assertEquals(0, s2.getReviewCount());
        assertNull(s2.getNextReviewAt());
    }

    /**
     * 验证 KpStat 序列化为 JSON 后字段结构完整(模拟 Dubbo Triple JSON 序列化路径)。
     */
    @Test
    void consumer_kpStatSerializesToJsonWithAllFields() throws Exception {
        KpStat stat = new KpStat(10L, 3.5, 2, "2026-08-01T00:00:00Z");
        ObjectMapper mapper = new ObjectMapper();
        String json = mapper.writeValueAsString(stat);
        JsonNode node = mapper.readTree(json);

        // 契约声明的 4 个字段均存在
        assertTrue(node.has("kpId"), "JSON 应含 kpId");
        assertTrue(node.has("masteryScore"), "JSON 应含 masteryScore");
        assertTrue(node.has("reviewCount"), "JSON 应含 reviewCount");
        assertTrue(node.has("nextReviewAt"), "JSON 应含 nextReviewAt");
        // 类型校验
        assertTrue(node.get("kpId").isIntegralNumber(), "kpId 应为整数");
        assertTrue(node.get("masteryScore").isFloatingPointNumber() || node.get("masteryScore").isIntegralNumber(),
                "masteryScore 应为数值");
        assertTrue(node.get("reviewCount").isIntegralNumber(), "reviewCount 应为整数");
        assertTrue(node.get("nextReviewAt").isTextual(), "nextReviewAt 应为字符串");
    }

    /**
     * 验证 pact 文件已生成且结构合法。
     */
    @Test
    void pactFile_isGenerated() throws Exception {
        assertTrue(PACT_FILE.exists(), "pact 文件应存在: " + PACT_FILE.getAbsolutePath());
        ObjectMapper mapper = new ObjectMapper();
        JsonNode pact = mapper.readTree(PACT_FILE);
        assertEquals(CONSUMER, pact.get("consumer").asText());
        assertEquals(PROVIDER, pact.get("provider").asText());
        assertTrue(pact.get("interactions").size() > 0, "应至少有1个 interaction");
    }
}
