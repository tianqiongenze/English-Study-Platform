package com.esp.learning.config;

import com.esp.learning.service.DifficultyAdjuster;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.jdbc.core.JdbcTemplate;

import javax.sql.DataSource;

/**
 * 业务组件装配:窗口大小等可配置参数在此注入。
 */
@Configuration
public class ServiceConfig {

    @Bean
    public DifficultyAdjuster difficultyAdjuster(@Value("${esp.difficulty.window-size:5}") int windowSize) {
        return new DifficultyAdjuster(windowSize);
    }

    /** JdbcTemplate 绑定路由 DataSource(默认走主库写),供 AppLogClient 等直接 SQL 使用。 */
    @Bean
    @Primary
    public JdbcTemplate jdbcTemplate(DataSource dataSource) {
        return new JdbcTemplate(dataSource);
    }
}
