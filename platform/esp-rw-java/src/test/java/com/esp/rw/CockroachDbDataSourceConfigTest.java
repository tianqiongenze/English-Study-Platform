package com.esp.rw;

import com.zaxxer.hikari.HikariDataSource;
import org.aspectj.lang.ProceedingJoinPoint;
import org.aspectj.lang.reflect.MethodSignature;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.datasource.LazyConnectionDataSourceProxy;
import org.springframework.jdbc.datasource.lookup.AbstractRoutingDataSource;

import javax.sql.DataSource;
import java.lang.reflect.Method;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

/**
 * CockroachDB 读写分离装配测试。
 * 不依赖真实 CRDB 集群:直接实例化配置 + mock 数据源 + 反射调用
 * AbstractRoutingDataSource.determineTargetDataSource() 验证路由选择。
 */
class CockroachDbDataSourceConfigTest {

    @AfterEach
    void clear() {
        RoutingHolder.clear();
    }

    private CockroachDbDataSourceProperties props() {
        CockroachDbDataSourceProperties p = new CockroachDbDataSourceProperties();
        p.setHost("crdb-public");
        p.setPort(26257);
        p.setDb("esp");
        p.setUser("root");
        p.setPassword("");
        p.setWritePoolSize(10);
        p.setReadPoolSize(10);
        return p;
    }

    /** 反射调用 protected determineTargetDataSource(),验证当前 RoutingHolder 选中的池。 */
    private DataSource determineTarget(DataSource routingDataSource) throws Exception {
        DataSource target = ((LazyConnectionDataSourceProxy) routingDataSource).getTargetDataSource();
        assertInstanceOf(RoutingDataSource.class, target);
        Method m = AbstractRoutingDataSource.class.getDeclaredMethod("determineTargetDataSource");
        m.setAccessible(true);
        return (DataSource) m.invoke(target);
    }

    @Test
    void crdbWriteAndReadDataSourceAreHikariWithPgDriver() {
        CockroachDbDataSourceConfig config = new CockroachDbDataSourceConfig();
        HikariDataSource w = (HikariDataSource) config.crdbWriteDataSource(props());
        HikariDataSource r = (HikariDataSource) config.crdbReadDataSource(props());
        try {
            assertInstanceOf(HikariDataSource.class, w);
            assertInstanceOf(HikariDataSource.class, r);
            assertEquals("jdbc:postgresql://crdb-public:26257/esp?sslmode=disable", w.getJdbcUrl(),
                    "写池 URL 应指向 CRDB primary");
            assertEquals("jdbc:postgresql://crdb-public:26257/esp?sslmode=disable", r.getJdbcUrl(),
                    "readHost 缺省回退 host");
            assertEquals("org.postgresql.Driver", w.getDriverClassName(), "driver 应为 PostgreSQL wire");
            assertEquals("root", w.getUsername());
            assertEquals(10, w.getMaximumPoolSize(), "写池大小来自 props");
            assertEquals(10, r.getMaximumPoolSize(), "读池大小来自 props");
        } finally {
            w.close();
            r.close();
        }
    }

    @Test
    void readHostOverrideBuildsReadReplicaUrl() {
        CockroachDbDataSourceProperties p = props();
        p.setReadHost("crdb-public-ro");
        assertEquals("jdbc:postgresql://crdb-public-ro:26257/esp?sslmode=disable", p.buildReadJdbcUrl(),
                "读副本独立端点时读池应指向 readHost");
        assertEquals("jdbc:postgresql://crdb-public:26257/esp?sslmode=disable", p.buildWriteJdbcUrl(),
                "写池始终指向 host(primary)");
    }

    @Test
    void sharedDbBuildsAppLogsPoolUrlWithDefaultsAndOverride() {
        CockroachDbDataSourceProperties p = props();
        assertEquals("jdbc:postgresql://crdb-public:26257/esp_shared?sslmode=disable", p.buildSharedJdbcUrl(),
                "app_logs 共享池缺省指向 esp_shared");
        p.setSharedDb("esp_audit");
        assertEquals("jdbc:postgresql://crdb-public:26257/esp_audit?sslmode=disable", p.buildSharedJdbcUrl(),
                "sharedDb 可经 ESP_RW_COCKROACHDB_SHARED_DB 覆盖");
    }

    @Test
    void routingDataSourceWrapsProxyAndRoutesWriteReadAndDefault() throws Exception {
        CockroachDbDataSourceConfig config = new CockroachDbDataSourceConfig();
        DataSource write = mock(DataSource.class);
        DataSource read = mock(DataSource.class);
        DataSource routing = config.routingDataSource(write, read);

        assertInstanceOf(LazyConnectionDataSourceProxy.class, routing, "应包 LazyConnectionDataSourceProxy 延迟连接获取");

        RoutingHolder.set(DataSourceType.WRITE);
        assertSame(write, determineTarget(routing), "WRITE 应路由到写池(primary)");
        RoutingHolder.set(DataSourceType.READ);
        assertSame(read, determineTarget(routing), "READ 应路由到读池(replica)");
        RoutingHolder.clear();
        assertSame(write, determineTarget(routing), "未标注时缺省走写池(安全默认)");
    }

    @Test
    void readAnnotationThroughAspectSelectsReadPool() throws Throwable {
        CockroachDbDataSourceConfig config = new CockroachDbDataSourceConfig();
        DataSource write = mock(DataSource.class);
        DataSource read = mock(DataSource.class);
        DataSource routing = config.routingDataSource(write, read);

        // 模拟 @DataSource(READ) 方法被 DataSourceAspect 处理,全链路验证落到读池
        ProceedingJoinPoint pjp = mock(ProceedingJoinPoint.class);
        MethodSignature sig = mock(MethodSignature.class);
        Method readMethod = CrdbAnnotated.class.getDeclaredMethod("read");
        when(sig.getMethod()).thenReturn(readMethod);
        when(pjp.getSignature()).thenReturn(sig);
        AtomicReference<DataSource> selected = new AtomicReference<>();
        when(pjp.proceed()).thenAnswer(inv -> {
            selected.set(determineTarget(routing));
            return null;
        });

        new DataSourceAspect().around(pjp);
        assertSame(read, selected.get(), "@DataSource(READ) 注解 -> 切面 -> 路由数据源,应选中读池");
    }

    static class CrdbAnnotated {
        // 全限定名:本测试 import javax.sql.DataSource 会遮蔽同包 com.esp.rw.DataSource 注解
        @com.esp.rw.DataSource(DataSourceType.READ)
        public void read() {}
    }
}
