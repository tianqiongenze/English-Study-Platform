package com.esp.learning.config;

import com.zaxxer.hikari.HikariDataSource;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.boot.jdbc.DataSourceBuilder;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.jdbc.core.JdbcTemplate;

import javax.sql.DataSource;

/**
 * app_logs 跨 schema 专用数据源(schema-per-service Phase1)。
 *
 * <p>app_logs 表在 esp_shared,而 learning 业务读写分离连接(WRITE_DB_URL/READ_DB_URL)指 esp_learning。
 * 直接用业务连接 INSERT/SELECT app_logs 会 1146。本配置建独立 HikariCP 连接池绑 esp_shared,
 * 经 SHARED_DB_URL env 注入(默认 tidb-primary:4000/esp_shared),供:
 * <ul>
 *   <li>AppLogClient 写 app_logs(写主库 tidb-primary)。</li>
 *   <li>BehaviorController 读 app_logs(behavior/top|funnel|heatmap,读副本可后续配)。</li>
 * </ul>
 * 与 esp-rw 路由数据源(@Primary)解耦,不污染业务表读写分离路径。
 */
@Configuration
public class SharedDataSourceConfig {

    @Bean
    @ConfigurationProperties("spring.datasource.shared")
    public DataSource sharedDataSource() {
        return DataSourceBuilder.create().type(HikariDataSource.class).build();
    }

    /** 绑定 esp_shared 的 JdbcTemplate,专供 app_logs 读写。 */
    @Bean
    public JdbcTemplate sharedJdbcTemplate(@Qualifier("sharedDataSource") DataSource sharedDataSource) {
        return new JdbcTemplate(sharedDataSource);
    }
}
