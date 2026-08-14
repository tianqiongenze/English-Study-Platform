package com.esp.it;

import com.esp.api.dubbo.KpStat;
import com.esp.api.dubbo.LearningRpcService;
import org.apache.dubbo.config.annotation.DubboReference;
import org.junit.jupiter.api.Test;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Import;
import org.springframework.jdbc.core.JdbcTemplate;

import javax.sql.DataSource;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * T1:teacher→learning Dubbo Triple 真实跨服务联调(代码级断言)。
 *
 * <p>consumer 直连 provider learning-java 的 Triple 端口(tri://learning-java:50091),
 * 绕开 Nacos(单节点环境 Nacos 不一定可用)。provider 侧 userStats/weakCount 标
 * {@code @DataSource(READ, consistent=true)} —— 写后强一致读走主库。
 *
 * <p>断言点(全代码级,非 HTTP 探活):
 * <ol>
 *   <li>从 TiDB user_mastery 取一个真实有数据的 userId。</li>
 *   <li>userStats(userId) 返回非空 List<KpStat>;每个 KpStat kpId>0、masteryScore∈[0,1]、reviewCount>=0。</li>
 *   <li>weakCount(userId) == userStats(userId).size()(同一查询语义)。</li>
 *   <li>不存在 userId(999999999L):userStats 返回空 List,weakCount 返回 0。</li>
 * </ol>
 *
 * <p>前置:learning-java provider 已部署在集群 esp-prod,DNS learning-java 可解析到其 Service。
 */
@SpringBootTest(classes = T1_CrossServiceDubboTripleIT.ITConfig.class,
        properties = "spring.profiles.active=it")
class T1_CrossServiceDubboTripleIT {

    @DubboReference(
            group = "esp",
            version = "1.0.0",
            // 直连 provider,绕开 Nacos:tri://learning-java:50091/<全限定接口名>?version=1.0.0&group=esp
            url = "tri://learning-java:50091/com.esp.api.dubbo.LearningRpcService?version=1.0.0&group=esp",
            check = false,
            timeout = 8000
    )
    private LearningRpcService learningRpc;

    @org.springframework.beans.factory.annotation.Autowired
    private DataSource dataSource;

    /** 精简配置:仅 Dubbo 自动配置 + JdbcTemplate,不拉起整个 web 上下文。 */
    @Configuration
    @Import(org.apache.dubbo.spring.boot.autoconfigure.DubboAutoConfiguration.class)
    static class ITConfig {
        @org.springframework.context.annotation.Bean
        JdbcTemplate jdbcTemplate(DataSource ds) {
            return new JdbcTemplate(ds);
        }
    }

    @Test
    void userStats_returnsValidKpStats_forExistingUser() {
        Long userId = pickExistingUserId();
        // 断言 1:provider 跨网络返回非空(真实 Dubbo Triple RPC 成功)
        List<KpStat> stats = learningRpc.userStats(userId);
        assertThat(stats).isNotNull();
        assertThat(stats).isNotEmpty();

        // 断言 2:每个 KpStat 字段合法性(代码级,非空探活)
        for (KpStat s : stats) {
            assertThat(s.getKpId()).as("kpId must be > 0").isGreaterThan(0);
            assertThat(s.getMasteryScore()).as("masteryScore must be in [0,1]")
                    .isBetween(0.0, 1.0);
            assertThat(s.getReviewCount()).as("reviewCount must be >= 0").isNotNegative();
        }
    }

    @Test
    void weakCount_matchesUserStatsSize() {
        Long userId = pickExistingUserId();
        List<KpStat> stats = learningRpc.userStats(userId);
        int weak = learningRpc.weakCount(userId);
        // 断言 3:同一查询语义,两个方法返回规模一致
        assertThat(weak).as("weakCount should equal userStats size").isEqualTo(stats.size());
    }

    @Test
    void userStats_returnsEmpty_forNonExistentUser() {
        long nonexistent = 999_999_999L;
        // 断言 4:不存在用户返回空 List(非 null)
        List<KpStat> stats = learningRpc.userStats(nonexistent);
        assertThat(stats).isNotNull().isEmpty();
        // 断言 4b:weakCount 返回 0
        int weak = learningRpc.weakCount(nonexistent);
        assertThat(weak).isZero();
    }

    /** 从 TiDB user_mastery 取一个真实有数据的 userId(种子 499 万行,必定命中)。 */
    private Long pickExistingUserId() {
        JdbcTemplate jdbc = new JdbcTemplate(dataSource);
        // 切读路由(本测试用例 provider 侧已标 @DataSource,这里只是取 seed)
        return jdbc.queryForObject(
                "SELECT user_id FROM user_mastery LIMIT 1",
                Long.class);
    }
}
