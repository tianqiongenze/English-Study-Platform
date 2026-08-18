package com.esp.common.db;

import com.esp.rw.CockroachDbDataSourceProperties;
import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.sql.DataSource;

/**
 * CockroachDB 双池数据源配置(收编自 learning-java SharedDataSourceConfig,供多服务复用)。
 *
 * <p>CRDB 模式下,CockroachDB 使用 SERIALIZABLE 默认隔离级别 + Follower Read,
 * 读写连接池分离可避免写阻塞读(尤其在单 region 部署时)。
 *
 * <p>使用方注入 {@link CockroachDbDataSourceProperties} 后,
 * 调用 {@link #createWriteDataSource} / {@link #createReadDataSource} / {@link #createSharedDataSource}
 * 构建 HikariCP 池。
 *
 * <p>注意:本类不声明 @Configuration/@Bean,避免自动装配冲突。
 * 使用方在自己的 @Configuration 中调用,明确控制 bean 生命周期。
 *
 * <p>收编自 learning-java SharedDataSourceConfig.crdbSharedDataSource,消除服务间重复。
 *
 * @see com.esp.rw.CockroachDbDataSourceProperties
 */
public final class CockroachDbConfig {

    private static final Logger log = LoggerFactory.getLogger(CockroachDbConfig.class);
    private static final String PG_DRIVER = "org.postgresql.Driver";

    private CockroachDbConfig() {}

    /**
     * 创建主库(写)连接池。
     *
     * @param props     CRDB 连接属性(host/port/user/password/db)
     * @param poolName  HikariCP pool 名称(监控/日志区分)
     * @param maxPool   最大连接数
     * @param minIdle   最小空闲连接
     * @return HikariDataSource(需由使用方管理 close)
     */
    public static DataSource createWriteDataSource(CockroachDbDataSourceProperties props,
                                                   String poolName, int maxPool, int minIdle) {
        HikariConfig cfg = buildConfig(props, poolName, maxPool, minIdle);
        cfg.setJdbcUrl(props.buildWriteJdbcUrl());
        log.info("[CRDB] write pool: name={} url={} maxPool={}", poolName, cfg.getJdbcUrl(), maxPool);
        return new HikariDataSource(cfg);
    }

    /**
     * 创建只读(从)连接池。
     * Follower Read:SET default_transaction_read_only = true + AS OF SYSTEM TIME -3s。
     *
     * @param props     CRDB 连接属性(host/port/user/password/db)
     * @param poolName  HikariCP pool 名称
     * @param maxPool   最大连接数
     * @param minIdle   最小空闲连接
     * @return HikariDataSource(需由使用方管理 close)
     */
    public static DataSource createReadDataSource(CockroachDbDataSourceProperties props,
                                                  String poolName, int maxPool, int minIdle) {
        HikariConfig cfg = buildConfig(props, poolName, maxPool, minIdle);
        cfg.setJdbcUrl(props.buildReadJdbcUrl());
        // HikariCP connectionInitSql:每个新连接执行 SET default_transaction_read_only = true
        cfg.setConnectionInitSql("SET default_transaction_read_only = true");
        log.info("[CRDB] read pool: name={} url={} maxPool={}", poolName, cfg.getJdbcUrl(), maxPool);
        return new HikariDataSource(cfg);
    }

    /**
     * 创建共享 schema 数据源(如 esp_shared,跨 schema 读写 app_logs 等)。
     *
     * @param props     CRDB 连接属性(host/port/user/password)
     * @param poolName  HikariCP pool 名称
     * @param maxPool   最大连接数
     * @param minIdle   最小空闲连接
     * @return HikariDataSource
     */
    public static DataSource createSharedDataSource(CockroachDbDataSourceProperties props,
                                                    String poolName, int maxPool, int minIdle) {
        HikariConfig cfg = buildConfig(props, poolName, maxPool, minIdle);
        cfg.setJdbcUrl(props.buildSharedJdbcUrl());
        log.info("[CRDB] shared pool: name={} url={} maxPool={}", poolName, cfg.getJdbcUrl(), maxPool);
        return new HikariDataSource(cfg);
    }

    private static HikariConfig buildConfig(CockroachDbDataSourceProperties props,
                                            String poolName, int maxPool, int minIdle) {
        HikariConfig cfg = new HikariConfig();
        cfg.setPoolName(poolName);
        cfg.setDriverClassName(PG_DRIVER);
        cfg.setUsername(props.getUser());
        cfg.setPassword(props.getPassword());
        cfg.setMaximumPoolSize(maxPool);
        cfg.setMinimumIdle(minIdle);
        cfg.setConnectionTimeout(5_000);  // 5s 连接超时
        cfg.setIdleTimeout(600_000);      // 10min 空闲回收
        cfg.setMaxLifetime(1_800_000);    // 30min 最大生命周期
        return cfg;
    }
}
