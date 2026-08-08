package com.esp.cache.event;

import com.esp.cache.PulsarClientHolder;
import org.apache.pulsar.client.api.PulsarClient;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class EventPublisherTest {

    @Test
    void publishSilentWhenProducerNewProducerThrows() {
        // 注入 mock client,newProducer 抛异常 → ensureProducer 走 catch 返 null,publish 静默返回。
        // 避免真实 broker 连接(PulsarClient.builder().build() 重试 ~30s)。
        PulsarClientHolder holder = new PulsarClientHolder();
        PulsarClient mockClient = mock(PulsarClient.class);
        when(mockClient.newProducer(any())).thenThrow(new RuntimeException("boom"));
        holder.set(mockClient);
        EventPublisher pub = new EventPublisher(holder, "pulsar://x:6650");
        assertDoesNotThrow(() -> pub.publish("user.registered", new UserRegisteredEvent(1L, "a@b.c", "s")));
    }

    @Test
    void closeDoesNotThrowWhenNoProducers() {
        PulsarClientHolder holder = new PulsarClientHolder();
        EventPublisher pub = new EventPublisher(holder, "pulsar://localhost:6650");
        assertDoesNotThrow(pub::close);
    }
}
