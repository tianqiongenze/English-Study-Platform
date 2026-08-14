package com.esp.bff.edge;

import com.esp.disruptor.DisruptorPipeline;
import com.esp.disruptor.DisruptorStats;
import com.esp.disruptor.WaitStrategyType;
import com.lmax.disruptor.dsl.ProducerType;
import io.netty.buffer.Unpooled;
import io.netty.handler.codec.http.DefaultFullHttpRequest;
import io.netty.handler.codec.http.FullHttpRequest;
import io.netty.handler.codec.http.HttpMethod;
import io.netty.handler.codec.http.HttpVersion;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.net.http.HttpRequest;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

import static org.awaitility.Awaitility.await;
import static java.time.Duration.ofSeconds;
import static org.junit.jupiter.api.Assertions.*;

/**
 * 验证 InletDisruptor 两级流水线拓扑(ADR-09 多级调度生产落地):
 *
 *   publish → Stage1(parse: InletNormalizer) → forward → Stage2(forward: DownstreamForwarder)
 *
 * 测试策略:Stage2 注入 mock AsyncSender 收集请求(不真发 HTTP),断言:
 * 1. 两级串联:发布的 Netty 请求经 Stage1 规整(路由解析/header 过滤/body 拷贝/
 *    traceparent 透传)后,Stage2 收到完整可发送的 HttpRequest;
 * 2. 引用计数:Stage1 拷贝完 body 即 release Netty 请求(refCnt 归零);
 * 3. 本地路径(/healthz 等非转发路径)在 Stage1 即被丢弃,不进 Stage2;
 * 4. 指标覆盖两级:各自 published/consumed 独立计数。
 */
class InletDisruptorPipelineTest {

    private static final String SAMPLE_TRACEPARENT =
            "00-4bf92f3577b3da6e3dfbe0a3442dc0b4-00f067aa0ba902b7-01";

    private DisruptorPipeline pipeline;
    private List<HttpRequest> sentRequests;
    private List<String> sentDownstreams;

    @BeforeEach
    void setUp() {
        sentRequests = new CopyOnWriteArrayList<>();
        sentDownstreams = new CopyOnWriteArrayList<>();

        InletNormalizer normalizer = new InletNormalizer();
        DownstreamForwarder forwarder = new DownstreamForwarder((req, ds) -> {
            sentRequests.add(req);
            sentDownstreams.add(ds);
        });

        pipeline = DisruptorPipeline.named("test-inlet")
                .stage("parse", 1024, ProducerType.MULTI,
                        InletEvent::new, InletEvent::translate, normalizer, WaitStrategyType.LITE_BLOCKING)
                .stage("forward", 256, ProducerType.MULTI,
                        InletEvent::new, InletEvent::translate, forwarder, WaitStrategyType.BLOCKING)
                .build();
        normalizer.bind(pipeline);
        pipeline.start();
    }

    @AfterEach
    void tearDown() {
        if (pipeline != null) pipeline.shutdown();
    }

    private FullHttpRequest nettyPost(String uri, String body) {
        FullHttpRequest req = new DefaultFullHttpRequest(HttpVersion.HTTP_1_1, HttpMethod.POST, uri,
                Unpooled.wrappedBuffer(body.getBytes(StandardCharsets.UTF_8)));
        req.headers().set("X-User-Id", "u1");
        req.headers().set("traceparent", SAMPLE_TRACEPARENT);
        // hop-by-hop 头:必须被 Stage1 过滤,否则 JDK HttpClient build() 直接抛异常
        req.headers().set("Host", "evil.example.com");
        req.headers().set("Connection", "keep-alive");
        return req;
    }

    @Test
    void twoStagePipelineNormalizesThenForwards() {
        FullHttpRequest nettyReq = nettyPost("/api/learning/quiz/answer", "hello");

        InletEvent payload = new InletEvent();
        payload.set("ch-1", nettyReq, null);
        assertTrue(pipeline.publish(payload) == com.esp.disruptor.PublishResult.PUBLISHED);

        // Stage2 收到规整后的请求
        await().atMost(ofSeconds(5)).until(() -> sentRequests.size() == 1);
        HttpRequest forwarded = sentRequests.get(0);
        assertEquals("http://learning-java:80/api/learning/quiz/answer", forwarded.uri().toString());
        assertEquals("POST", forwarded.method());
        // traceparent 透传(非 hop-by-hop header 原样保留)
        assertEquals(SAMPLE_TRACEPARENT, forwarded.headers().firstValue("traceparent").orElse(null));
        // 可信身份头保留
        assertEquals("u1", forwarded.headers().firstValue("X-User-Id").orElse(null));
        // hop-by-hop 头已剔除(build 未抛异常即证明;再显式断言 host 不存在)
        assertTrue(forwarded.headers().firstValue("Host").isEmpty());
        assertEquals("learning-java:80", sentDownstreams.get(0));

        // Stage1 已归还 Netty 引用计数(body 已拷贝出 ByteBuf)
        await().atMost(ofSeconds(5)).until(() -> nettyReq.refCnt() == 0);

        // 指标覆盖两级:各自独立计数
        List<DisruptorStats> stats = pipeline.stats();
        assertEquals(2, stats.size());
        await().atMost(ofSeconds(5)).until(() -> stats.get(0).consumed() >= 1);
        await().atMost(ofSeconds(5)).until(() -> stats.get(1).consumed() >= 1);
        assertTrue(stats.get(0).published() >= 1);
        assertTrue(stats.get(1).published() >= 1);
    }

    @Test
    void localPathDroppedAtStage1NotForwarded() {
        FullHttpRequest nettyReq = nettyPost("/healthz", "");

        InletEvent payload = new InletEvent();
        payload.set("ch-2", nettyReq, null);
        assertTrue(pipeline.publish(payload) == com.esp.disruptor.PublishResult.PUBLISHED);

        // 本地路径在 Stage1 即丢弃:Stage1 已消费(非转发路径 InletNormalizer 不 forward),
        //  Stage2 published 保持 0,Stage2 也消费了那个 forward 出来的空事件槽(若是 DROP 则 published 不增)
        await().atMost(ofSeconds(5)).until(() -> pipeline.stats(0).consumed() >= 1);
        assertEquals(0, sentRequests.size());
        assertEquals(0, pipeline.stats(1).published());
    }

    @Test
    void burstThroughBothStagesKeepsOrderPerRing() throws Exception {
        // 小环突发:全部按序通过两级(单消费者级内 FIFO),验证级间衔接无丢事件
        int n = 200;
        for (int i = 0; i < n; i++) {
            FullHttpRequest req = nettyPost("/api/teacher/plan?i=" + i, "b" + i);
            InletEvent p = new InletEvent();
            p.set("ch-" + i, req, null);
            assertEquals(com.esp.disruptor.PublishResult.PUBLISHED, pipeline.publish(p));
        }
        await().atMost(ofSeconds(10)).until(() -> sentRequests.size() == n);
        assertEquals("http://teacher-java:80/api/teacher/plan?i=0", sentRequests.get(0).uri().toString());
        assertEquals("http://teacher-java:80/api/teacher/plan?i=199", sentRequests.get(n - 1).uri().toString());
    }
}
