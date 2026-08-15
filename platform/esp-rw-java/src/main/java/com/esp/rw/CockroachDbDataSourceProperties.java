package com.esp.rw;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * CockroachDB 读写分离数据源属性(esp.rw.storage.type=cockroachdb 时生效)。
 *
 * 全部字段支持 env 驱动(Spring relaxed binding):
 *   ESP_RW_COCKROACHDB_HOST              -> esp.rw.cockroachdb.host
 *   ESP_RW_COCKROACHDB_READ_HOST         -> esp.rw.cockroachdb.read-host
 *   ESP_RW_COCKROACHDB_PORT              -> esp.rw.cockroachdb.port
 *   ESP_RW_COCKROACHDB_DB                -> esp.rw.cockroachdb.db
 *   ESP_RW_COCKROACHDB_SHARED_DB         -> esp.rw.cockroachdb.shared-db(默认 esp_shared,app_logs 共享池)
 *   ESP_RW_COCKROACHDB_USER              -> esp.rw.cockroachdb.user
 *   ESP_RW_COCKROACHDB_PASSWORD          -> esp.rw.cockroachdb.password
 *   ESP_RW_COCKROACHDB_WRITE_POOL_SIZE   -> esp.rw.cockroachdb.writePoolSize
 *   ESP_RW_COCKROACHDB_READ_POOL_SIZE    -> esp.rw.cockroachdb.readPoolSize
 *
 * 拓扑:
 *   host     = CRDB 主端点(写池),默认 crdb-public:26257(ns esp-crdb 的 ExternalName 短名)。
 *   readHost = 读副本端点(读池),可选;缺省回退到 host(单端点部署仍正确读写分池,
 *              但读副本独立端点时应显式配置 readHost 以真正实现 读->副本)。
 *   driver   = org.postgresql.Driver(CRDB 兼容 PostgreSQL wire),URL 形如
 *              jdbc:postgresql://crdb-public:26257/esp?sslmode=disable
 */
@ConfigurationProperties(prefix = "esp.rw.cockroachdb")
public class CockroachDbDataSourceProperties {

    /** CRDB 主端点 host(写池)。 */
    private String host = "crdb-public";

    /** CRDB 读副本端点 host(读池);缺省回退到 host。 */
    private String readHost;

    /** CRDB SQL 端口(兼容 PostgreSQL wire)。 */
    private int port = 26257;

    /** 数据库名。 */
    private String db = "esp";

    /** app_logs 共享库(所有服务 CRDB 模式共用;两段式 db.table 跨连接库可达)。 */
    private String sharedDb = "esp_shared";

    /** 用户名。 */
    private String user = "root";

    /** 密码。 */
    private String password = "";

    /** 写池大小。 */
    private int writePoolSize = 10;

    /** 读池大小。 */
    private int readPoolSize = 10;

    public String getHost() {
        return host;
    }

    public void setHost(String host) {
        this.host = host;
    }

    public String getReadHost() {
        return readHost;
    }

    public void setReadHost(String readHost) {
        this.readHost = readHost;
    }

    public int getPort() {
        return port;
    }

    public void setPort(int port) {
        this.port = port;
    }

    public String getDb() {
        return db;
    }

    public void setDb(String db) {
        this.db = db;
    }

    public String getSharedDb() {
        return sharedDb;
    }

    public void setSharedDb(String sharedDb) {
        this.sharedDb = sharedDb;
    }

    public String getUser() {
        return user;
    }

    public void setUser(String user) {
        this.user = user;
    }

    public String getPassword() {
        return password;
    }

    public void setPassword(String password) {
        this.password = password;
    }

    public int getWritePoolSize() {
        return writePoolSize;
    }

    public void setWritePoolSize(int writePoolSize) {
        this.writePoolSize = writePoolSize;
    }

    public int getReadPoolSize() {
        return readPoolSize;
    }

    public void setReadPoolSize(int readPoolSize) {
        this.readPoolSize = readPoolSize;
    }

    /** 写池 JDBC URL。 */
    public String buildWriteJdbcUrl() {
        return "jdbc:postgresql://" + host + ":" + port + "/" + db + "?sslmode=disable";
    }

    /** 读池 JDBC URL(readHost 未配置时回退到 host)。 */
    public String buildReadJdbcUrl() {
        return "jdbc:postgresql://" + effectiveReadHost() + ":" + port + "/" + db + "?sslmode=disable";
    }

    private String effectiveReadHost() {
        return readHost == null || readHost.isBlank() ? host : readHost;
    }

    /** app_logs 共享池 JDBC URL(CRDB 模式专用:各服务跨 schema 写 esp_shared.app_logs)。 */
    public String buildSharedJdbcUrl() {
        return "jdbc:postgresql://" + host + ":" + port + "/" + sharedDb + "?sslmode=disable";
    }
}
