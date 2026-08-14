package com.esp.cache;

import java.io.Serial;
import java.io.Serializable;

/**
 * Pulsar 跨实例失效广播消息体(ADR-多级缓存)。
 *
 * <p>当某实例写后失效 L1 时,把 {cacheName, key} 发到 topic esp-cache-invalidate,
 * 所有实例 consumer 收到后对各自 L1 Caffeine 执行 invalidate(key)。
 *
 * <p>JSON 序列化,字段精简,topic 单分区保序即可(失效幂等,无需全局序)。
 */
public class InvalidateMsg implements Serializable {

    @Serial
    private static final long serialVersionUID = 1L;

    /** L1 Caffeine 缓存名,如 "learning:due"。 */
    private String cacheName;
    /** 完整缓存 key,如 "learning:due:1"。 */
    private String key;

    public InvalidateMsg() {}

    public InvalidateMsg(String cacheName, String key) {
        this.cacheName = cacheName;
        this.key = key;
    }

    public String getCacheName() { return cacheName; }
    public void setCacheName(String cacheName) { this.cacheName = cacheName; }
    public String getKey() { return key; }
    public void setKey(String key) { this.key = key; }

    @Override
    public String toString() {
        return "InvalidateMsg{cacheName='" + cacheName + "', key='" + key + "'}";
    }
}
