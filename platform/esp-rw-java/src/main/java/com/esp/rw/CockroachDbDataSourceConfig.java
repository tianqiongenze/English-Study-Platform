package com.esp.rw;

import com.zaxxer.hikari.HikariDataSource;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.boot.jdbc.DataSourceBuilder;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.jdbc.datasource.LazyConnectionDataSourceProxy;

import javax.sql.DataSource;
import java.util.HashMap;
import java.util.Map;

/**
 * CockroachDB 读写分离数据源自动配置(esp-rw-java 公共包)。
 *
 * 与 TiDB 路径(DataSourceConfig)完全解耦、互斥激活:
 *   esp.rw.storage.type=cockroachdb   -> 本配置生效,装配 CRDB 读写分离;
 *   esp.rw.storage.type=tidb(缺省)   -> DataSourceConfig 生效,本配置不装配。
 *   master feature flag esp.rw.enabled=false 时同样整体禁用(回退安全)。
 *
 * CRDB 兼容 PostgreSQL wire(driver=org.postgresql.Driver),URL 形如
 *   jdbc:postgresql://crdb-public:26257/esp?sslmode=disable
 *
 * Bean:
 *   crdbWriteDataSource -> HikariCP 写池(CRDB primary)
 *   crdbReadDataSource  -> HikariCP 读池(CRDB read replica;readHost 缺省回退 host)
 *   routingDataSource(@Primary) -> RoutingDataSource 包 LazyConnectionDataSourceProxy
 *      按 RoutingHolder(由 @DataSource AOP 切面设置)路由到写/读池;
 *      默认目标 = 写池(未标注 @DataSource 的方法走主库,安全默认)。
 *
 * 路由语义与 TiDB 路径完全复用:RoutingHolder(ThreadLocal)+ RoutingDataSource +
 * DataSourceAspect(@DataSource 注解),@DataSource(READ)/@DataSource(value=READ, consistent=true)
 * 的强制主库逻辑对 CRDB 同样生效。
 */
@Configuration
@ConditionalOnProperty(name = "esp.rw.enabled", havingValue = "true", matchIfMissing = true)
@ConditionalOnProperty(name = "esp.rw.storage.type", havingValue = "cockroachdb")
@EnableConfigurationProperties(CockroachDbDataSourceProperties.class)
public class CockroachDbDataSourceConfig {

    private static final String PG_DRIVER = "org.postgresql.Driver";

    @Bean
    public DataSource crdbWriteDataSource(CockroachDbDataSourceProperties props) {
        return buildPool(props.buildWriteJdbcUrl(), props, props.getWritePoolSize());
    }

    @Bean
    public DataSource crdbReadDataSource(CockroachDbDataSourceProperties props) {
        return buildPool(props.buildReadJdbcUrl(), props, props.getReadPoolSize());
    }

    @Bean
    @Primary
    public DataSource routingDataSource(@Qualifier("crdbWriteDataSource") DataSource writeDataSource,
                                        @Qualifier("crdbReadDataSource") DataSource readDataSource) {
        RoutingDataSource routing = new RoutingDataSource();
        Map<Object, Object> targets = new HashMap<>();
        targets.put(DataSourceType.WRITE, writeDataSource);
        targets.put(DataSourceType.READ, readDataSource);
        routing.setTargetDataSources(targets);
        routing.setDefaultTargetDataSource(writeDataSource);
        // 立即解析目标数据源,避免 Actuator dbHealthContributor 在
        // "DataSources not resolved yet" 阶段读取导致启动失败(与 TiDB 路径同)。
        routing.afterPropertiesSet();
        return new LazyConnectionDataSourceProxy(routing);
    }

    private HikariDataSource buildPool(String jdbcUrl, CockroachDbDataSourceProperties props, int poolSize) {
        HikariDataSource ds = DataSourceBuilder.create()
                .type(HikariDataSource.class)
                .driverClassName(PG_DRIVER)
                .url(jdbcUrl)
                .username(props.getUser())
                .password(props.getPassword())
                .build();
        ds.setMaximumPoolSize(poolSize);
        ds.setMinimumIdle(Math.min(poolSize, 5));
        return ds;
    }
}
