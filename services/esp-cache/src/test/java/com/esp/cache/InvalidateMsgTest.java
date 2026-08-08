package com.esp.cache;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

class InvalidateMsgTest {

    @Test
    void noArgConstructorAndSetters() {
        InvalidateMsg m = new InvalidateMsg();
        m.setCacheName("learning:due");
        m.setKey("learning:due:1");
        assertEquals("learning:due", m.getCacheName());
        assertEquals("learning:due:1", m.getKey());
    }

    @Test
    void allArgConstructorAndGetters() {
        InvalidateMsg m = new InvalidateMsg("writing:score", "writing:score:9");
        assertEquals("writing:score", m.getCacheName());
        assertEquals("writing:score:9", m.getKey());
    }

    @Test
    void toStringContainsFields() {
        InvalidateMsg m = new InvalidateMsg("c", "k");
        String s = m.toString();
        assertTrue(s.contains("c"), s);
        assertTrue(s.contains("k"), s);
    }
}
