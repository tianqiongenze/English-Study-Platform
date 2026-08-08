package com.esp.cache;

import org.apache.pulsar.client.api.PulsarClient;

/**
 * PulsarClient 持有者:延迟初始化(首次 producer/consumer 创建时才连 broker)。
 * 由 {@link CacheBroadcast} 按需调用,连接失败时上层降级为不发广播 + TTL 兜底。
 */
public class PulsarClientHolder {

    private volatile PulsarClient client;

    public PulsarClient get() { return client; }

    public void set(PulsarClient client) { this.client = client; }
}
