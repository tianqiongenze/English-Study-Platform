package com.esp.learning.config;

import com.esp.rw.CockroachDbDataSourceProperties;
import com.zaxxer.hikari.HikariDataSource;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.boot.jdbc.DataSourceBuilder;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.jdbc.core.JdbcTemplate;

import javax.sql.DataSource;

/**
 * app_logs 跨 schema 专用数据源(schema-per-service Phase1)。
 *
 * <p>app_logs 表在 esp_shared,而 learning 业务读写分离连接(WRITE_DB_URL/READ_DB_URL)指 esp_learning
 * (CRDB 模式指 esp.rw.cockroachdb.db=esp_learning)。直接用业务连接写 app_logs 需两段式表名;
 * 本配置另建独立 HikariCP 连接池绑 esp_shared,供:
 * <ul>
 *   <li>AppLogClient 写 app_logs(写主库)。</li>
 *   <li>BehaviorController 读 app_logs(behavior/top|funnel|heatmap,读副本可后续配)。</li>
 * </ul>
 * 与 esp-rw 路由数据源(@Primary)解耦,不污染业务表读写分离路径。
 *
 * <p>双模式(按 esp.rw.storage.type 互斥装配,两 bean 同名 sharedDataSource):
 * <ul>
 *   <li>tidb(缺省):绑定 spring.datasource.shared.*(mysql,SHARED_DB_URL 注入 esp_shared),行为不变。</li>
 *   <li>cockroachdb:复用 esp-rw 的 CockroachDbDataSourceProperties(host/user,默认 crdb-public:26257),
 *       driver=org.postgresql.Driver,url 指向 esp_shared(默认,可经 ESP_RW_COCKROACHDB_SHARED_DB 覆盖)。</li>
 * </ul>
 */
@Configuration
@EnableConfigurationProperties(CockroachDbDataSourceProperties.class)
public class SharedDataSourceConfig {

    private static final String PG_DRIVER = "org.postgresql.Driver";

    /** CRDB 模式:app_logs 池 -> crdb-public:26257/esp_shared(PG wire,esp.rw.cockroachdb.* 派生)。 */
    @Bean(name = "sharedDataSource")
    @ConditionalOnProperty(name = "esp.rw.storage.type", havingValue = "cockroachdb")
    public DataSource crdbSharedDataSource(CockroachDbDataSourceProperties crdbProps) {
        HikariDataSource ds = DataSourceBuilder.create()
                .type(HikariDataSource.class)
                .driverClassName(PG_DRIVER)
                .url(crdbProps.buildSharedJdbcUrl())
                .username(crdbProps.getUser())
                .password(crdbProps.getPassword())
                .build();
        ds.setMaximumPoolSize(10);
        ds.setMinimumIdle(2);
        return ds;
    }

    /** TiDB 模式(缺省):shared 池绑定 spring.datasource.shared.*(与既有行为一致)。 */
    @Bean(name = "sharedDataSource")
    @ConditionalOnProperty(name = "esp.rw.storage.type", havingValue = "tidb", matchIfMissing = true)
    @ConfigurationProperties("spring.datasource.shared")
    public DataSource tidbSharedDataSource() {
        return DataSourceBuilder.create().type(HikariDataSource.class).build();
    }

    /** 绑定 esp_shared 的 JdbcTemplate,专供 app_logs 读写。 */
    @Bean
    public JdbcTemplate sharedJdbcTemplate(@Qualifier("sharedDataSource") DataSource sharedDataSource) {
        return new JdbcTemplate(sharedDataSource);
    }
}
