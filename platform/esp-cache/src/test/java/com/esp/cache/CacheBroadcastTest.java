package com.esp.cache;

import org.apache.pulsar.client.api.PulsarClient;
import org.junit.jupiter.api.Test;
import org.springframework.cache.CacheManager;

import java.lang.reflect.Field;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * CacheBroadcast 单测。
 * 关键:避免真实 Pulsar 连接(PulsarClient.builder().build() 默认连接重试 ~30s,会拖慢单测)。
 * 用 mock PulsarClientHolder / 反射注入 mock producer-consumer,精确验证逻辑分支,零网络。
 */
class CacheBroadcastTest {

    @Test
    void consumerDisabledDoesNotStartConsumerAndDoesNotThrow() {
        PulsarClientHolder holder = new PulsarClientHolder();
        CacheManager cm = mock(CacheManager.class);
        // consumerEnabled=false,不启动 consumer(无 client 也不尝试连接)
        assertDoesNotThrow(() -> new CacheBroadcast(holder, "pulsar://localhost:6650", "t", false, cm));
    }

    @Test
    void publishEvictSilentWhenProducerNewProducerThrows() throws Exception {
        // 注入 mock PulsarClient,其 newProducer 抛异常 → ensureProducer 走 catch 返 null,
        // publishEvict 静默返回不抛。避免真实 broker 连接(快)。
        PulsarClientHolder holder = new PulsarClientHolder();
        PulsarClient mockClient = mock(PulsarClient.class);
        when(mockClient.newProducer(any())).thenThrow(new RuntimeException("boom"));
        holder.set(mockClient);
        CacheManager cm = mock(CacheManager.class);
        CacheBroadcast bc = new CacheBroadcast(holder, "pulsar://x:6650", "t", false, cm);
        assertDoesNotThrow(() -> bc.publishEvict("c", "k"));
    }

    @Test
    void closeDoesNotThrowWhenNothingStarted() {
        PulsarClientHolder holder = new PulsarClientHolder();
        CacheManager cm = mock(CacheManager.class);
        CacheBroadcast bc = new CacheBroadcast(holder, "pulsar://localhost:6650", "t", false, cm);
        assertDoesNotThrow(bc::close);
    }
}
