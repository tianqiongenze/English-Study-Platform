package com.esp.rw;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;

class RoutingHolderTest {

    @AfterEach
    void clear() { RoutingHolder.clear(); }

    @Test
    void getDefaultsToWriteWhenNull() {
        assertEquals(DataSourceType.WRITE, RoutingHolder.get());
    }

    @Test
    void setReadThenGetReturnsRead() {
        RoutingHolder.set(DataSourceType.READ);
        assertEquals(DataSourceType.READ, RoutingHolder.get());
    }

    @Test
    void clearResetsToDefaultWrite() {
        RoutingHolder.set(DataSourceType.READ);
        RoutingHolder.clear();
        assertEquals(DataSourceType.WRITE, RoutingHolder.get());
    }

    @Test
    void threadIsolation() throws Exception {
        RoutingHolder.set(DataSourceType.READ);
        AtomicReference<DataSourceType> otherThread = new AtomicReference<>();
        Thread t = new Thread(() -> otherThread.set(RoutingHolder.get()));
        t.start();
        t.join();
        // 新线程无继承的 ThreadLocal 值,默认 WRITE
        assertEquals(DataSourceType.WRITE, otherThread.get(),
                "new thread should see default WRITE (no inherited ThreadLocal)");
        // 主线程仍是 READ
        assertEquals(DataSourceType.READ, RoutingHolder.get());
    }
}
