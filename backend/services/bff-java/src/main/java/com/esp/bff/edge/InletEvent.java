package com.esp.bff.edge;

import io.netty.handler.codec.http.FullHttpRequest;
import io.opentelemetry.context.Context;

/**
 * Disruptor 入口事件(高吞吐转发载体)。对象由 Disruptor 预分配并复用,
 * 生产者通过 EventTranslator 写入字段,消费者处理后字段会被下一次发布覆盖。
 */
public class InletEvent {
    /** 来源 channel 长 ID,用于异步回写响应(本期简化:转发后由下游连接回写)。 */
    public String channelId;
    /** 已 retain 的 Netty 请求对象,消费者处理后 release。 */
    public FullHttpRequest request;
    /** OTel Context(含提取的上游 span),Disruptor 消费者用此注入下游 traceparent。 */
    public Context otelContext;

    public InletEvent() {}

    public void set(String channelId, FullHttpRequest request, Context otelContext) {
        this.channelId = channelId;
        this.request = request;
        this.otelContext = otelContext;
    }
}
