package com.esp.bff.edge;

import com.esp.disruptor.DisruptorPipeline;
import com.esp.disruptor.PublishResult;
import com.lmax.disruptor.EventHandler;
import io.netty.handler.codec.http.FullHttpRequest;
import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.context.Context;
import io.opentelemetry.context.propagation.TextMapSetter;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.net.URI;
import java.net.http.HttpRequest;
import java.time.Duration;

/**
 * Stage1(parse)消费者:轻量解析/规整(CPU 密集、微秒级,不碰网络 IO)。
 *
 * 职责(从原单级 DownstreamForwarder 前置的纯计算部分):
 * 1. RouteTable 解析下游 host:port(非转发路径直接丢弃并归还引用);
 * 2. body 拷贝出 Netty ByteBuf(此后即可 release,尽早归还堆外内存 —— 这是
 *    Stage2 只需小 RingBuffer 的关键:槽位持有 byte[] 拷贝而非 retained Netty buffer);
 * 3. header 白名单过滤(剔除 hop-by-hop:host/connection/transfer-encoding/content-length,
 *    JDK HttpClient 限制头,保留透传 traceparent 等);
 * 4. W3C traceparent inject(OTel Context → 下游 header,P1-5 traceId 贯通);
 * 5. 构建不可变 {@link HttpRequest},经 {@link DisruptorPipeline#forward} 投递 Stage2。
 *
 * 线程模型:本 handler 在 Stage1 独立消费线程执行,只做纯计算,不阻塞等待下游;
 * Stage2 满时 forward 返回 DROPPED(fire-and-forget 背压语义,丢弃计数记在 Stage2 stats)。
 */
public class InletNormalizer implements EventHandler<InletEvent> {

    private static final Logger log = LoggerFactory.getLogger(InletNormalizer.class);

    /** 下游转发超时(Stage1 构建 HttpRequest 时固化)。 */
    private static final Duration FORWARD_TIMEOUT = Duration.ofSeconds(10);

    /** W3C traceparent 注入到 HttpRequest.Builder 的 setter。 */
    private static final TextMapSetter<HttpRequest.Builder> HEADER_SETTER = (carrier, key, value) -> {
        if (carrier != null) {
            carrier.header(key, value);
        }
    };

    /** 流水线引用(build 后 bind;handler 需要它做级间 forward)。 */
    private volatile DisruptorPipeline pipeline;

    /** build() 之后、start() 之前绑定流水线实例。 */
    public void bind(DisruptorPipeline pipeline) {
        this.pipeline = pipeline;
    }

    @Override
    public void onEvent(InletEvent event, long sequence, boolean endOfBatch) {
        FullHttpRequest req = event.request;
        event.request = null; // 所有权移交本方法,finally 统一归还,防止异常路径泄漏
        if (req == null) return;
        try {
            String path = URI.create(req.uri()).getPath();
            String downstream = RouteTable.resolve(path);
            if (downstream == null) {
                // 非转发路径(应已被 EdgeAuthHandler 当本地处理),忽略
                return;
            }

            // body 拷贝:此后 Netty 引用计数即可归还,Stage2 槽位只持有轻量 byte[] 载荷
            byte[] body = new byte[req.content().readableBytes()];
            req.content().readBytes(body);

            HttpRequest.Builder rb = HttpRequest.newBuilder(URI.create("http://" + downstream + req.uri()))
                    .method(req.method().name(),
                            body.length == 0 ? HttpRequest.BodyPublishers.noBody()
                                    : HttpRequest.BodyPublishers.ofByteArray(body))
                    .timeout(FORWARD_TIMEOUT);
            req.headers().forEach(h -> {
                String name = h.getKey();
                // 跳过 hop-by-hop 头(JDK HttpClient 限制头:host/connection/transfer-encoding/content-length)
                if (!"content-length".equalsIgnoreCase(name)
                        && !"host".equalsIgnoreCase(name)
                        && !"connection".equalsIgnoreCase(name)
                        && !"transfer-encoding".equalsIgnoreCase(name)) {
                    rb.header(name, h.getValue());
                }
            });

            // === traceId 贯通:W3C inject traceparent 到下游请求(P1-5)===
            Context otelCtx = event.otelContext;
            if (otelCtx != null) {
                GlobalOpenTelemetry.getPropagators()
                        .getTextMapPropagator()
                        .inject(otelCtx, rb, HEADER_SETTER);
            }

            event.downstream = downstream;
            event.httpRequest = rb.build();
            PublishResult r = pipeline.forward(0, event);
            if (r == PublishResult.DROPPED) {
                // Stage2 环满:fire-and-forget 背压语义,丢弃(Stage2 worker 的 dropped 指标已 +1)
                event.httpRequest = null;
            }
        } catch (Exception e) {
            log.warn("inlet parse error uri={}", req.uri(), e);
        } finally {
            // 清空 Stage1 槽位全部引用(forward 已同步拷贝进 Stage2 槽,此处 null 安全):
            // 防止 byte[] body / OTel Context / Netty 引用在槽位复用间隙长期滞留
            event.otelContext = null;
            event.httpRequest = null;
            event.downstream = null;
            req.release();
        }
    }
}
