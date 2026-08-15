package com.esp.rw;

import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;

import java.util.Arrays;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 存储技术选择条件验证(esp.rw.storage.type=tidb|cockroachdb)。
 * 直接反射断言两个自动配置类的 @ConditionalOnProperty,不依赖 Spring 上下文:
 *   - type=cockroachdb 才激活 CockroachDbDataSourceConfig(不得 matchIfMissing,防缺省误激活);
 *   - type=tidb(缺省 matchIfMissing)激活 DataSourceConfig(向后兼容,现有服务零改动)。
 */
class StorageTypeConditionalTest {

    @Test
    void crdbConfigActivatesOnlyWhenStorageTypeIsCockroachdb() {
        ConditionalOnProperty[] anns = CockroachDbDataSourceConfig.class
                .getAnnotationsByType(ConditionalOnProperty.class);
        ConditionalOnProperty storage = find(anns, "esp.rw.storage.type");
        assertNotNull(storage, "CRDB 配置必须按 esp.rw.storage.type 条件化");
        assertEquals("cockroachdb", storage.havingValue());
        assertFalse(storage.matchIfMissing(), "type=cockroachdb 不得缺省激活,避免 type=tidb/未配置时装配 CRDB 类");
        assertTrue(hasMasterFlag(CockroachDbDataSourceConfig.class), "CRDB 路径应复用 esp.rw.enabled 主开关");
    }

    @Test
    void tidbConfigIsDefaultStorageType() {
        ConditionalOnProperty[] anns = DataSourceConfig.class
                .getAnnotationsByType(ConditionalOnProperty.class);
        ConditionalOnProperty storage = find(anns, "esp.rw.storage.type");
        assertNotNull(storage, "TiDB 配置必须按 esp.rw.storage.type 条件化");
        assertEquals("tidb", storage.havingValue());
        assertTrue(storage.matchIfMissing(), "缺省 storage.type 应回退 tidb(现有服务零改动)");
        assertTrue(hasMasterFlag(DataSourceConfig.class), "TiDB 路径应复用 esp.rw.enabled 主开关");
    }

    private ConditionalOnProperty find(ConditionalOnProperty[] anns, String name) {
        for (ConditionalOnProperty a : anns) {
            if (Arrays.asList(a.name()).contains(name) || name.equals(a.value())) {
                return a;
            }
        }
        return null;
    }

    private boolean hasMasterFlag(Class<?> clazz) {
        for (ConditionalOnProperty a : clazz.getAnnotationsByType(ConditionalOnProperty.class)) {
            if (Arrays.asList(a.name()).contains("esp.rw.enabled") || "esp.rw.enabled".equals(a.value())) {
                return true;
            }
        }
        return false;
    }
}
