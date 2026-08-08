package com.esp.rw;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

class RoutingHolderTest {

    @AfterEach
    void clear() {
        RoutingHolder.clear();
    }

    @Test
    void defaultsToWrite() {
        assertEquals(DataSourceType.WRITE, RoutingHolder.get());
    }

    @Test
    void setAndClear() {
        RoutingHolder.set(DataSourceType.READ);
        assertEquals(DataSourceType.READ, RoutingHolder.get());
        RoutingHolder.clear();
        assertEquals(DataSourceType.WRITE, RoutingHolder.get());
    }
}
