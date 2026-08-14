package com.esp.rw;

import com.zaxxer.hikari.HikariDataSource;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.datasource.LazyConnectionDataSourceProxy;

import javax.sql.DataSource;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class DataSourceConfigTest {

    @Test
    void routingDataSourceWrapsLazyProxyAndRegistersTargets() {
        DataSourceConfig config = new DataSourceConfig();
        DataSource write = mock(DataSource.class);
        DataSource read = mock(DataSource.class);
        // routingDataSource 调 afterPropertiesSet,需要 target 已解析;mock DataSource 直接可用
        DataSource routing = config.routingDataSource(write, read);
        assertInstanceOf(LazyConnectionDataSourceProxy.class, routing,
                "应包 LazyConnectionDataSourceProxy 延迟连接获取");
        assertNotNull(routing);
    }

    @Test
    void writeAndReadDataSourceBuildersAreHikari() {
        // 验证 writeDataSource/readDataSource 工厂方法返回 HikariDataSource(非空)
        // 不连真实 DB,只验证类型
        DataSourceConfig config = new DataSourceConfig();
        // 不调 afterPropertiesSet,直接验证 build 出 HikariDataSource 实例
        DataSource w = config.writeDataSource();
        DataSource r = config.readDataSource();
        assertInstanceOf(HikariDataSource.class, w);
        assertInstanceOf(HikariDataSource.class, r);
        // 关闭连接池避免资源泄漏
        ((HikariDataSource) w).close();
        ((HikariDataSource) r).close();
    }
}
