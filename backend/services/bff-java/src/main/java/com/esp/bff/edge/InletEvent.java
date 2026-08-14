package com.esp.bff.edge;

import io.netty.handler.codec.http.FullHttpRequest;
import io.opentelemetry.context.Context;

import java.net.http.HttpRequest;

import com.lmax.disruptor.EventTranslatorOneArg;

/**
 * Disruptor 入口事件(高吞吐转发载体,贯穿两级流水线)。对象由 Disruptor 预分配并复用,
 * 生产者通过 EventTranslator 写入字段,消费者处理后字段会被下一次发布覆盖。
 *
 * 两级流水线字段归属(ADR-09 多级调度):
 * - Stage1(parse)输入:channelId / request / otelContext(EdgeAuthHandler 投递);
 * - Stage1 输出(派生字段):downstream / httpRequest —— 解析+规整后的轻量载荷,
 *   Stage2(forward)槽位只持有这两个字段(Netty 引用计数已在 Stage1 归还),
 *   因此 Stage2 小 RingBuffer(16384)不会放大堆外内存占用。
 */
public class InletEvent {
    /** 来源 channel 长 ID,用于异步回写响应(本期简化:转发后由下游连接回写)。 */
    public String channelId;
    /** 已 retain 的 Netty 请求对象,Stage1(parse)拷贝完 body 后即 release 归还。 */
    public FullHttpRequest request;
    /** OTel Context(含提取的上游 span),Stage1 用此注入下游 traceparent。 */
    public Context otelContext;

    // === Stage1 派生字段(Stage2 消费,消费完由 Stage2 handler 置 null 防止 byte[] body 滞留槽位)===
    /** Stage1 解析出的下游 host:port(RouteTable)。 */
    public String downstream;
    /** Stage1 构建好的不可变 JDK HttpRequest(含过滤后 header + traceparent),Stage2 直接发送。 */
    public HttpRequest httpRequest;

    public InletEvent() {}

    public void set(String channelId, FullHttpRequest request, Context otelContext) {
        this.channelId = channelId;
        this.request = request;
        this.otelContext = otelContext;
    }

    /** 级间拷贝(Disruptor translator 用):把源事件字段拷进目标复用槽。
     *  两级流水线同一事件类型,未派生字段为 null 时原样拷 null。 */
    public void copyFrom(InletEvent o) {
        this.channelId = o.channelId;
        this.request = o.request;
        this.otelContext = o.otelContext;
        this.downstream = o.downstream;
        this.httpRequest = o.httpRequest;
    }

    /** Disruptor translator 静态方法引用目标(EventTranslatorOneArg 签名:translateTo(slot, sequence, payload))。
     *  本质就是把 {@link #copyFrom} 套成 translator 签名,方便 {@code InletEvent::translate} 方法引用。 */
    public static void translate(InletEvent slot, long sequence, InletEvent payload) {
        slot.copyFrom(payload);
    }
}
