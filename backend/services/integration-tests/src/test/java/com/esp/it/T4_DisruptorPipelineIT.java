package com.esp.it;

import com.esp.disruptor.DisruptorPipeline;
import com.esp.disruptor.DisruptorStats;
import com.esp.disruptor.PublishResult;
import com.esp.disruptor.WaitStrategyType;
import com.lmax.disruptor.EventHandler;
import org.junit.jupiter.api.Test;

import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * T4:Disruptor 多级流水线代码级断言(进程内,不需集群)。
 *
 * <p>2-stage pipeline:ingest → sink。AtomicInteger 计数,CopyOnWriteArrayList 记顺序。
 *
 * <p>断言点:
 * <ol>
 *   <li>publish N 个事件,shutdown 后全部消费(consumed == N)。</li>
 *   <li>FIFO 顺序保持(sink stage 记录的序号列表 == 1..N 升序)。</li>
 *   <li>背压:无消费阻塞,publish 返回 PUBLISHED(小 buffer 可观测 DROPPED,此处用大 buffer 确保 PUBLISHED)。</li>
 *   <li>每级 stats:published/consumed 计数正确。</li>
 * </ol>
 */
class T4_DisruptorPipelineIT {

    /** 流水线事件槽(可复用)。 */
    static class PipeEvent {
        long seq;
        void set(long s) { this.seq = s; }
    }

    @Test
    void pipeline_twoStages_allConsumedInFifoOrder() throws Exception {
        int N = 500;
        CopyOnWriteArrayList<Long> sinkOrder = new CopyOnWriteArrayList<>();
        AtomicInteger ingestCount = new AtomicInteger();

        // stage 0:ingest —— 计数并原样转发到 stage 1
        // stage 1:sink —— 记录序号(验证 FIFO)
        // 级间衔接:在 handler 内调 pipeline.forward(0, event) 把派生事件推到 stage 1
        DisruptorPipeline[] holder = new DisruptorPipeline[1];

        EventHandler<PipeEvent> ingestHandler = (event, sequence, endOfBatch) -> {
            ingestCount.incrementAndGet();
            // 转发到下一级(派生事件用原 event)
            holder[0].forward(0, event);
        };
        EventHandler<PipeEvent> sinkHandler = (event, sequence, endOfBatch) -> {
            sinkOrder.add(event.seq);
        };

        DisruptorPipeline pipe = DisruptorPipeline.named("it-pipeline")
                .stage("ingest", 1024,
                        com.lmax.disruptor.dsl.ProducerType.SINGLE,
                        PipeEvent::new,
                        (slot, payload) -> slot.set(payload.seq),
                        ingestHandler,
                        WaitStrategyType.BLOCKING)
                .stage("sink", 1024,
                        com.lmax.disruptor.dsl.ProducerType.MULTI,
                        PipeEvent::new,
                        (slot, payload) -> slot.set(payload.seq),
                        sinkHandler,
                        WaitStrategyType.BLOCKING)
                .build();
        holder[0] = pipe;

        pipe.start();

        // publish N 个事件到 stage 0
        int publishedOk = 0;
        for (long i = 1; i <= N; i++) {
            PipeEvent p = new PipeEvent();
            p.set(i);
            PublishResult r = pipe.publish(p);
            if (r == PublishResult.PUBLISHED) publishedOk++;
        }

        // 断言 3:背压 —— 全部成功入队(大 buffer)
        assertThat(publishedOk).as("all N events should be PUBLISHED (no backpressure drop)").isEqualTo(N);

        // 优雅停机:阻塞至消费者追上
        pipe.shutdown();

        // 断言 1:全经过两级(ingest + sink 各 N)
        assertThat(ingestCount.get()).as("ingest stage should consume all N").isEqualTo(N);
        assertThat(sinkOrder).as("sink stage should consume all N").hasSize(N);

        // 断言 2:FIFO 顺序保持(sink 记录的序号 == 1..N 升序)
        for (int i = 0; i < N; i++) {
            assertThat(sinkOrder.get(i))
                    .as("FIFO order at index %d", i)
                    .isEqualTo((long) (i + 1));
        }

        // 断言 4:每级 stats 计数正确
        DisruptorStats ingestStats = pipe.stats(0);
        DisruptorStats sinkStats = pipe.stats(1);
        assertThat(ingestStats.published()).as("ingest published == N").isEqualTo(N);
        assertThat(ingestStats.consumed()).as("ingest consumed == N").isEqualTo(N);
        assertThat(ingestStats.dropped()).as("ingest dropped == 0").isZero();
        assertThat(sinkStats.consumed()).as("sink consumed == N").isEqualTo(N);
    }
}
