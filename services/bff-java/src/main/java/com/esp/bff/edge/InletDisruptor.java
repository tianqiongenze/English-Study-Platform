package com.esp.bff.edge;

import com.esp.disruptor.DisruptorWorker;
import com.esp.disruptor.PublishResult;
import com.esp.disruptor.WaitStrategyType;
import com.lmax.disruptor.dsl.ProducerType;
import io.netty.handler.codec.http.FullHttpRequest;
import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.context.Context;
import io.opentelemetry.context.propagation.TextMapSetter;
import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.concurrent.Executors;

/**
 * Disruptor 高吞吐入口队列(ADR-09)。
 * - Netty edge worker 解码+鉴权后,把请求以引用计数(retained FullHttpRequest)投递到此环形队列。
 * - 单消费者线程池异步转发到下游服务(JDK HttpClient),Netty worker 不阻塞于下游 IO。
 * - 环大小 65536(2^16);BlockingWaitStrategy 平衡 CPU 与延迟;满则返回 DROPPED → 调用方快速 503。
 *
 * traceId 贯通(P1-5):
 * - InletEvent 携带 OTel Context(含上游 span),DownstreamForwarder 用
 *   W3C TextMapPropagator.inject() 将 traceparent 写入下游请求 header,
 *   确保 gateway→bff→learning/writing/teacher 链路 traceId 贯通。
 *
 * 实现复用共用工具包 {@code com.esp:esp-disruptor} 的 {@link DisruptorWorker} Builder,
 * 屏蔽 RingBuffer/序列号/事件工厂等底层细节;本类仅负责:
 * 1) 以 {@link InletEvent} 为事件类型构建多生产者 worker;
 * 2) 提供转发消费者(DownstreamForwarder)将请求异步发往 RouteTable 解析的下游服务;
 * 3) 暴露 {@link #publish(InletEvent)} 给 EdgeAuthHandler 投递(背压语义)。
 */
@Component
public class InletDisruptor {

    private static final Logger log = LoggerFactory.getLogger(InletDisruptor.class);
    private static final int RING_SIZE = 1 << 16; // 65536

    /** W3C traceparent 注入到 HttpRequest.Builder 的 setter。 */
    private static final TextMapSetter<HttpRequest.Builder> HEADER_SETTER = (carrier, key, value) -> {
        if (carrier != null) {
            carrier.header(key, value);
        }
    };

    private DisruptorWorker<InletEvent> worker;
    private HttpClient httpClient;

    @PostConstruct
    public void init() {
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(2))
                .executor(Executors.newFixedThreadPool(16))
                .build();

        this.worker = DisruptorWorker.<InletEvent>builder()
                .name("bff-inlet")
                .bufferSize(RING_SIZE)
                .producerType(ProducerType.MULTI)        // 多个 Netty worker 并发投递
                .waitStrategy(WaitStrategyType.BLOCKING)
                .factory(InletEvent::new)
                // translator:把 payload 携带的字段拷进 Disruptor 预分配的复用槽
                .translator((slot, sequence, payload) -> slot.set(payload.channelId, payload.request, payload.otelContext))
                .handler(new DownstreamForwarder())
                .build();
        this.worker.start();
        log.info("InletDisruptor started, ringSize={}, worker={}", RING_SIZE, worker.name());
    }

    /**
     * 投递一个待转发请求;队列满返回 false(背压)。
     * 调用方(EdgeAuthHandler)负责 retain FullHttpRequest,消费者处理后 release。
     */
    public boolean publish(InletEvent payload) {
        PublishResult r = worker.publish(payload);
        return r == PublishResult.PUBLISHED;
    }

    @PreDestroy
    public void shutdown() {
        if (worker != null) worker.shutdown();
    }

    /** 消费者:转发请求到下游 RouteTable 解析的服务,异步化下游 IO。
     *  traceId 贯通:从 InletEvent.otelContext 提取 span context,W3C inject 到下游 header。 */
    private final class DownstreamForwarder implements com.lmax.disruptor.EventHandler<InletEvent> {
        @Override
        public void onEvent(InletEvent event, long sequence, boolean endOfBatch) {
            FullHttpRequest req = event.request;
            if (req == null) return;
            try {
                String path = URI.create(req.uri()).getPath();
                String downstream = RouteTable.resolve(path);
                if (downstream == null) {
                    // 非转发路径(应已被 EdgeAuthHandler 当本地处理),忽略
                    return;
                }
                String url = "http://" + downstream + req.uri();
                byte[] body = new byte[req.content().readableBytes()];
                req.content().readBytes(body);

                HttpRequest.BodyPublisher pub = body.length == 0
                        ? HttpRequest.BodyPublishers.noBody()
                        : HttpRequest.BodyPublishers.ofByteArray(body);

                HttpRequest.Builder rb = HttpRequest.newBuilder(URI.create(url))
                        .method(req.method().name(), pub)
                        .timeout(Duration.ofSeconds(10));
                req.headers().forEach(h -> {
                    String name = h.getKey();
                    // 跳过 hop-by-hop 头
                    if (!"content-length".equalsIgnoreCase(name)
                            && !"host".equalsIgnoreCase(name)
                            && !"connection".equalsIgnoreCase(name)
                            && !"transfer-encoding".equalsIgnoreCase(name)) {
                        rb.header(name, h.getValue());
                    }
                });

                // === traceId 贯通:W3C inject traceparent 到下游请求 ===
                Context otelCtx = event.otelContext;
                if (otelCtx != null) {
                    GlobalOpenTelemetry.getPropagators()
                            .getTextMapPropagator()
                            .inject(otelCtx, rb, HEADER_SETTER);
                }

                // 异步发送,不阻塞 Disruptor 消费线程太久
                httpClient.sendAsync(rb.build(), HttpResponse.BodyHandlers.ofByteArray())
                        .whenComplete((resp, ex) -> {
                            if (ex != null) {
                                log.debug("forward to {} failed: {}", downstream, ex.getMessage());
                            }
                            // 本期转发为 fire-and-forget 语义(响应由下游经 Ingress 直达前端,
                            // BFF 边缘转发主要用于旁路观测/治理;完整回写见 ForwardRespondHandler 扩展点)。
                        });
            } catch (Exception e) {
                log.warn("inlet forward error", e);
            } finally {
                req.release();
            }
        }
    }
}
