package com.esp.disruptor;

/**
 * Disruptor 等待策略选型(LMAX 官方推荐,按 CPU-延迟权衡)。
 *
 * 场景选型:
 * - {@link #BLOCKING}    —— 通用默认。线程在 LockSupport.park 上阻塞,唤醒有 syscall 开销,
 *                          但空闲时几乎不耗 CPU。适合吞吐优先、可接受微秒级唤醒延迟的业务队列(学情落库、批改)。
 * - {@link #SLEEPING}    —— 空闲时 Thread.sleep(0)/yield 轮询,无锁无 park。延迟低于 BLOCKING,
 *                          CPU 略高。适合中低延迟且不能容忍 park 唤醒抖动的场景。
 * - {@link #YIELDING}    —— 空闲时先 spin 若干轮再 yield,延迟亚微秒。CPU 占用较高,适合
 *                          低延迟但非极致的网关/转发路径。
 * - {@link #BUSY_SPIN}   —— 纯自旋,延迟最低(纳秒级),但独占一核 100% CPU。仅用于
 *                          极致低延迟且 CPU 充裕的专线场景,一般不推荐。
 * - {@link #LITE_BLOCKING} —— BLOCKING 的简化版,少一把 lock,代码路径更短;多生产者下与 BLOCKING 接近。
 *
 * ESP 默认:BFF 边缘入口用 YIELDING(接入层要低延迟),业务落库用 BLOCKING(吞吐优先)。
 */
public enum WaitStrategyType {
    BLOCKING,
    SLEEPING,
    YIELDING,
    BUSY_SPIN,
    LITE_BLOCKING
}
