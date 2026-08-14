package com.esp.bff.edge;

import com.lmax.disruptor.EventHandler;

import java.net.http.HttpRequest;

/**
 * Stage2(forward)消费者:纯 IO 派发(IO 密集,不占 Stage1 的低延迟预算)。
 *
 * 输入是 Stage1(InletNormalizer)构建好的不可变 {@link HttpRequest},本类不再做任何解析,
 * 只把请求交给可注入的 {@link AsyncSender}(生产实现 = JDK HttpClient sendAsync fire-and-forget,
 * 响应由下游经 Ingress 直达前端,BFF 边缘转发用于旁路观测/治理;单测注入 mock 即可闭环验证两级拓扑)。
 *
 * Disruptor 复用语义:消费完立即把派生字段置 null —— httpRequest 的 BodyPublisher 持有
 * body byte[] 引用,若滞留在复用槽里,最坏情况下 16384 个槽各挂一份 body 直到下次复用,
 * 显式清引用让 GC 在环覆盖前即可回收。
 */
public class DownstreamForwarder implements EventHandler<InletEvent> {

    /** 异步发送器抽象:生产 = JDK HttpClient;测试 = mock/收集。 */
    @FunctionalInterface
    public interface AsyncSender {
        void send(HttpRequest request, String downstream);
    }

    private final AsyncSender sender;

    public DownstreamForwarder(AsyncSender sender) {
        this.sender = sender;
    }

    @Override
    public void onEvent(InletEvent event, long sequence, boolean endOfBatch) {
        HttpRequest httpRequest = event.httpRequest;
        String downstream = event.downstream;
        if (httpRequest == null) return; // Stage1 丢弃的空槽(本地路径/背压 DROP)
        try {
            sender.send(httpRequest, downstream);
        } finally {
            event.httpRequest = null;
            event.downstream = null;
        }
    }
}
