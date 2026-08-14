package com.esp.rw;

import java.lang.annotation.*;

/**
 * 标注方法的读写分离路由。
 * value=WRITE 走主库(tidb-primary);value=READ 走副本(tidb-follower)。
 * consistent=true 表示写后强一致读(强制走主库,避免主从延迟读旧值)。
 * tiflash=true 表示读走 TiFlash 列存(仅 OLAP 聚合查询,预留)。
 *
 * 用法:@DataSource(READ) 标注只读查询方法;@DataSource(WRITE) 标注写方法(默认)。
 *      @DataSource(value=READ, consistent=true) 写后立即读同一事务,强制走主库。
 */
@Target({ElementType.METHOD, ElementType.TYPE})
@Retention(RetentionPolicy.RUNTIME)
@Documented
public @interface DataSource {
    DataSourceType value() default DataSourceType.WRITE;
    boolean consistent() default false;
    boolean tiflash() default false;
}
