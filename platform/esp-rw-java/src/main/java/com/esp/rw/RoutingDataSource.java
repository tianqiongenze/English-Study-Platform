package com.esp.rw;

import org.springframework.jdbc.datasource.lookup.AbstractRoutingDataSource;

/**
 * 读写分离路由数据源:按 RoutingHolder 当前 Key(DataSourceType)选择主库或副本。
 * 继承 Spring AbstractRoutingDataSource,与 JdbcTemplate/JPA 透明兼容。
 */
public class RoutingDataSource extends AbstractRoutingDataSource {
    @Override
    protected Object determineCurrentLookupKey() {
        return RoutingHolder.get();
    }
}
