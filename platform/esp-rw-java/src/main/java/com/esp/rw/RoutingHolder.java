package com.esp.rw;

/**
 * 基于 ThreadLocal 的路由上下文。
 * 默认 WRITE(主库),保证未标注 @DataSource 的方法走主库(安全默认)。
 *
 * 虚拟线程语义验证(2026-08-15 会话22-27,Spring Boot 4 全局虚拟线程已开启):
 * 每个虚拟线程是独立 Thread,ThreadLocal 读写作用于该虚拟线程自身;请求全链路
 * (Jetty accept → 虚拟线程处理 → @DataSource AOP set/proceed/clear)在同一虚拟线程内
 * 完成,无跨线程传递,路由正确。已实证(P2-15):无 @Async/线程池异步链,Pulsar consumer
 * 线程作为独立入口自身闭环,亦正确。
 * TODO(不强改):若未来引入异步链(@Async/CompletableFuture),需显式传递路由上下文;
 * 届时评估迁移 ScopedValue(JDK25 stable,不可变+有界共享,虚拟线程原生支持)。
 */
public final class RoutingHolder {
    private static final ThreadLocal<DataSourceType> CTX = new ThreadLocal<>();

    private RoutingHolder() {}

    public static void set(DataSourceType type) {
        CTX.set(type);
    }

    public static DataSourceType get() {
        DataSourceType t = CTX.get();
        return t == null ? DataSourceType.WRITE : t;
    }

    public static void clear() {
        CTX.remove();
    }
}
