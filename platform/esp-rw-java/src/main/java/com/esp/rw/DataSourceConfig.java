package com.esp.rw;

import com.zaxxer.hikari.HikariDataSource;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.boot.jdbc.DataSourceBuilder;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.jdbc.datasource.LazyConnectionDataSourceProxy;

import javax.sql.DataSource;
import java.util.HashMap;
import java.util.Map;

/**
 * 读写分离数据源自动配置(esp-rw-java 公共包)。
 *
 * 约定 application.yml 配置:
 *   spring.datasource.write.jdbc-url / username / password / driver-class-name  -> 主库(tidb-primary)
 *   spring.datasource.read.jdbc-url  / username / password / driver-class-name  -> 从库(tidb-follower)
 *
 * Bean:
 *   writeDataSource  -> HikariCP 主库连接池
 *   readDataSource   -> HikariCP 从库连接池
 *   routingDataSource(@Primary) -> RoutingDataSource 包 LazyConnectionDataSourceProxy
 *     按 RoutingHolder(由 @DataSource AOP 切面设置)路由到主/从。
 *     默认目标 = writeDataSource(未标注 @DataSource 的方法走主库,安全默认)。
 *
 * feature flag:esp.rw.enabled=false 时不创建路由数据源(回退为单主库,使用方自行配 spring.datasource.url)。
 * 存储技术选择:esp.rw.storage.type=tidb(缺省,本配置生效)或 cockroachdb(见 CockroachDbDataSourceConfig);
 * 两路径互斥激活,type=tidb 时不装配任何 CRDB 类。
 *
 * 包一层 LazyConnectionDataSourceProxy:保证仅在真正获取连接时才路由,
 * 使 @DataSource 注解(Service 层)与事务(Repository 层)解耦。
 */
@Configuration
@ConditionalOnProperty(name = "esp.rw.enabled", havingValue = "true", matchIfMissing = true)
@ConditionalOnProperty(name = "esp.rw.storage.type", havingValue = "tidb", matchIfMissing = true)
public class DataSourceConfig {

    @Bean
    @ConfigurationProperties("spring.datasource.write")
    public DataSource writeDataSource() {
        return DataSourceBuilder.create().type(HikariDataSource.class).build();
    }

    @Bean
    @ConfigurationProperties("spring.datasource.read")
    public DataSource readDataSource() {
        return DataSourceBuilder.create().type(HikariDataSource.class).build();
    }

    @Bean
    @Primary
    public DataSource routingDataSource(@Qualifier("writeDataSource") DataSource writeDataSource,
                                         @Qualifier("readDataSource") DataSource readDataSource) {
        RoutingDataSource routing = new RoutingDataSource();
        Map<Object, Object> targets = new HashMap<>();
        targets.put(DataSourceType.WRITE, writeDataSource);
        targets.put(DataSourceType.READ, readDataSource);
        routing.setTargetDataSources(targets);
        routing.setDefaultTargetDataSource(writeDataSource);
        // 立即解析目标数据源,避免 Actuator dbHealthContributor 在
        // "DataSources not resolved yet" 阶段读取导致启动失败。
        routing.afterPropertiesSet();
        return new LazyConnectionDataSourceProxy(routing);
    }
}
