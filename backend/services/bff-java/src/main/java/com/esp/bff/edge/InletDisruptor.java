package com.esp.bff.edge;

import com.esp.disruptor.DisruptorPipeline;
import com.esp.disruptor.DisruptorStats;
import com.esp.disruptor.PublishResult;
import com.esp.disruptor.WaitStrategyType;
import com.lmax.disruptor.dsl.ProducerType;
import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.Executors;

/**
 * Disruptor 两级流水线入口(ADR-09 多级调度,生产落地)。
 *
 * <pre>
 *   Netty edge worker ──tryPublish──▶ Stage1 parse(65536, LITE_BLOCKING)
 *                                        │ InletNormalizer:路由解析+body 拷贝+header 过滤
 *                                        │ +traceparent inject+构建 HttpRequest
 *                                        ▼ forward(满则 DROPPED)
 *                                      Stage2 forward(16384, BLOCKING)
 *                                        │ DownstreamForwarder:JDK HttpClient sendAsync
 *                                        ▼
 *                                      下游服务(learning/writing/teacher/...)
 * </pre>
 *
 * - Stage1(parse)65536 + LITE_BLOCKING:多个 Netty worker 并发投递(MULTI 生产者);
 *   纯计算微秒级任务,LITE_BLOCKING 先自旋后挂起,突发下低延迟且闲时不烧 CPU
 *   (YIELDING 持续让出也会占满核,BUSY_SPIN 单节点环境不可接受)。
 * - Stage2(forward)16384 + BLOCKING:转发(IO)速率低于解析是常态,小环天然限制
 *   在途 body 拷贝总量(16384 × 平均 body)防内存放大;IO 密集级对唤醒延迟不敏感,
 *   BLOCKING 锁语义最省 CPU。环满 forward 返回 DROPPED,fire-and-forget 背压。
 * - 对外 API 不变:publish(InletEvent) 满则返回 false → EdgeAuthHandler 快速 503;
 *   stats() 覆盖两级(每级独立 published/dropped/consumed/backlog)。
 *
 * 事件复用语义:两级共用 InletEvent 类型;Stage1 消费完拷贝出 Netty 引用并清槽,
 * Stage2 消费完清 httpRequest/downstream,避免 body 引用滞留复用槽。
 */
@Component
public class InletDisruptor {

    private static final Logger log = LoggerFactory.getLogger(InletDisruptor.class);

    /** Stage1(parse)环大小:保持原单级 65536(2^16),接住 Netty 侧突发。 */
    private static final int RING_PARSE = 1 << 16;
    /** Stage2(forward)环大小:16384(2^14)。转发慢于解析,小环限制在途 body 总量防内存放大;
     *  实测压测(5000 CCU 目标)转发速率上限约 2×10^3 QPS × 10s 超时窗口,16384 已留 3 倍余量。 */
    private static final int RING_FORWARD = 1 << 14;

    private DisruptorPipeline pipeline;
    private HttpClient httpClient;

    @PostConstruct
    public void init() {
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(2))
                .executor(Executors.newFixedThreadPool(16))
                .build();

        InletNormalizer normalizer = new InletNormalizer();
        DownstreamForwarder forwarder = new DownstreamForwarder(this::sendAsync);

        this.pipeline = DisruptorPipeline.named("bff-inlet")
                .stage("parse", RING_PARSE, ProducerType.MULTI,
                        InletEvent::new, InletEvent::translate, normalizer, WaitStrategyType.LITE_BLOCKING)
                .stage("forward", RING_FORWARD, ProducerType.MULTI,
                        InletEvent::new, InletEvent::translate, forwarder, WaitStrategyType.BLOCKING)
                .build();
        normalizer.bind(pipeline);
        this.pipeline.start();
        log.info("InletDisruptor pipeline started: stage1(parse) ringSize={} liteBlocking, stage2(forward) ringSize={} blocking",
                RING_PARSE, RING_FORWARD);
    }

    /**
     * 投递一个待转发请求到 Stage1;队列满返回 false(背压,调用方快速 503)。
     * 调用方(EdgeAuthHandler)负责 retain FullHttpRequest,Stage1 消费后 release。
     */
    public boolean publish(InletEvent payload) {
        PublishResult r = pipeline.publish(payload);
        return r == PublishResult.PUBLISHED;
    }

    /** 两级指标快照(Actuator/监控用):每级独立 published/dropped/consumed/backlog。 */
    public List<DisruptorStats> stats() {
        return pipeline.stats();
    }

    /** 生产 AsyncSender:JDK HttpClient 异步发送(fire-and-forget,不阻塞 Stage2 消费线程)。 */
    private void sendAsync(HttpRequest request, String downstream) {
        httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofByteArray())
                .whenComplete((resp, ex) -> {
                    if (ex != null) {
                        log.debug("forward to {} failed: {}", downstream, ex.getMessage());
                    }
                    // 本期转发为 fire-and-forget 语义(响应由下游经 Ingress 直达前端,
                    // BFF 边缘转发主要用于旁路观测/治理;完整回写见 ForwardRespondHandler 扩展点)。
                });
    }

    @PreDestroy
    public void shutdown() {
        if (pipeline != null) pipeline.shutdown(); // 逆序停机:先停上游再 drain 下游
    }
}
