package com.esp.rw;

/**
 * 基于 ThreadLocal 的路由上下文。
 * 默认 WRITE(主库),保证未标注 @DataSource 的方法走主库(安全默认)。
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
