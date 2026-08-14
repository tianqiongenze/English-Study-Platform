package com.esp.rw;

/**
 * 数据源类型:读写分离。
 * WRITE -> TiDB 主库(tidb-primary);READ -> TiDB 副本(tidb-follower)。
 * READ + consistent=true -> 强制走主库(写后强一致读,避免主从延迟读旧值)。
 */
public enum DataSourceType {
    WRITE,
    READ
}
