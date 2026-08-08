package com.esp.rw;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import java.lang.annotation.Annotation;

import static org.junit.jupiter.api.Assertions.*;

class RoutingDataSourceTest {

    @AfterEach
    void clear() { RoutingHolder.clear(); }

    @Test
    void determineCurrentLookupKeyReturnsRoutingHolder() {
        RoutingDataSource ds = new RoutingDataSource();
        RoutingHolder.set(DataSourceType.READ);
        assertEquals(DataSourceType.READ, ds.determineCurrentLookupKey());
        RoutingHolder.set(DataSourceType.WRITE);
        assertEquals(DataSourceType.WRITE, ds.determineCurrentLookupKey());
        RoutingHolder.clear();
        assertEquals(DataSourceType.WRITE, ds.determineCurrentLookupKey());
    }

    @Test
    void dataSourceAnnotationDefaults() throws Exception {
        DataSource ds = Dummy.class.getMethod("annotated").getAnnotation(DataSource.class);
        assertEquals(DataSourceType.WRITE, ds.value(), "default value=WRITE");
        assertFalse(ds.consistent(), "default consistent=false");
        assertFalse(ds.tiflash(), "default tiflash=false");
    }

    static class Dummy {
        @DataSource
        public void annotated() {}
    }
}
