package com.esp.cache;

import org.junit.jupiter.api.Test;
import org.mockito.Mockito;
import org.springframework.cache.Cache;
import org.springframework.cache.CacheManager;

import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class CacheBroadcastDeepTest {

    @Test
    void consumerEnabledWithNullClientDoesNotThrowAndLogs() throws Exception {
        // consumerEnabled=true 时构造函数会异步 startConsumer;若 client 连不上,consumer 线程降级退出。
        // 注入 mock client,其 newConsumer 抛异常,使 consumer 线程快速失败(避免真实 broker 30s 连接)。
        PulsarClientHolder holder = new PulsarClientHolder();
        org.apache.pulsar.client.api.PulsarClient mockClient = mock(org.apache.pulsar.client.api.PulsarClient.class);
        when(mockClient.newConsumer(any())).thenThrow(new RuntimeException("no broker"));
        holder.set(mockClient);
        CacheManager cm = mock(CacheManager.class);
        assertDoesNotThrow(() -> new CacheBroadcast(holder, "pulsar://x:6650", "t", true, cm));
        // 给异步 consumer 线程一点时间触发 ensureClient/newConsumer 失败路径。
        Thread.sleep(300);
    }

    @Test
    void closeClosesProducerWhenPresent() throws Exception {
        // 用真实可连接的本地?不可行。改为反射注入一个 mock producer 验证 close 被调
        PulsarClientHolder holder = new PulsarClientHolder();
        CacheManager cm = mock(CacheManager.class);
        CacheBroadcast bc = new CacheBroadcast(holder, "pulsar://localhost:6650", "t", false, cm);
        org.apache.pulsar.client.api.Producer<InvalidateMsg> prod = mock(org.apache.pulsar.client.api.Producer.class);
        java.lang.reflect.Field f = CacheBroadcast.class.getDeclaredField("producer");
        f.setAccessible(true);
        f.set(bc, prod);
        bc.close();
        verify(prod).close();
    }

    @Test
    void closeClosesConsumerWhenPresent() throws Exception {
        PulsarClientHolder holder = new PulsarClientHolder();
        CacheManager cm = mock(CacheManager.class);
        CacheBroadcast bc = new CacheBroadcast(holder, "pulsar://localhost:6650", "t", false, cm);
        org.apache.pulsar.client.api.Consumer<InvalidateMsg> cons = mock(org.apache.pulsar.client.api.Consumer.class);
        java.lang.reflect.Field f = CacheBroadcast.class.getDeclaredField("consumer");
        f.setAccessible(true);
        f.set(bc, cons);
        bc.close();
        verify(cons).close();
    }

    @Test
    void pulsarClientHolderGetSet() {
        PulsarClientHolder h = new PulsarClientHolder();
        assertNull(h.get());
        org.apache.pulsar.client.api.PulsarClient c = mock(org.apache.pulsar.client.api.PulsarClient.class);
        h.set(c);
        assertSame(c, h.get());
    }
}
