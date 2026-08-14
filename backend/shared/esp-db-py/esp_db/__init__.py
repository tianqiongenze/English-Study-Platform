"""ESP 读写分离数据访问公共包(Python 侧,与 Java 侧 rw/ 语义对齐)。

设计:
  - 两个连接池:write_pool -> tidb-primary(主写),read_pool -> tidb-follower(从读)。
  - 调用方显式选择 execute_write / execute_read;读操作默认走从库,写后强一致读可强制走主库(forced_primary=True)。
  - 连接池基于 aiomysql.create_pool(异步,兼容 Litestar/asyncio)。
  - feature flag:DB_ENABLED=false 时 no-op(回退安全,供测试/无 DB 环境降级)。
  - 降级:从库不可达时自动 fallback 到主库读(只 warn 不阻断),与 Java 侧 RoutingDataSource 容错语义一致。

env:
  WRITE_DB_HOST=tidb-primary  WRITE_DB_PORT=4000  WRITE_DB_USER=root  WRITE_DB_PASS=  WRITE_DB_NAME=esp
  READ_DB_HOST=tidb-follower  READ_DB_PORT=4000   READ_DB_USER=root   READ_DB_PASS=   READ_DB_NAME=esp
  DB_ENABLED=true
  DB_POOL_SIZE=10  DB_POOL_MIN=2  DB_CONNECT_TIMEOUT=10  DB_READ_TIMEOUT=30
"""
from __future__ import annotations

import asyncio
import logging
import os
from typing import Any, Sequence

try:
    import aiomysql
except ImportError:  # 降级:无 aiomysql 时 no-op(测试环境)
    aiomysql = None  # type: ignore

logger = logging.getLogger("esp_db")

_ENABLED = os.getenv("DB_ENABLED", "true").lower() != "false"
_POOL_SIZE = int(os.getenv("DB_POOL_SIZE", "10"))
_POOL_MIN = int(os.getenv("DB_POOL_MIN", "2"))
_CONNECT_TIMEOUT = int(os.getenv("DB_CONNECT_TIMEOUT", "10"))
_READ_TIMEOUT = int(os.getenv("DB_READ_TIMEOUT", "30"))

_write_pool = None  # aiomysql.Pool | None
_read_pool = None   # aiomysql.Pool | None


def _write_cfg() -> dict:
    return {
        "host": os.getenv("WRITE_DB_HOST", "tidb-primary"),
        "port": int(os.getenv("WRITE_DB_PORT", "4000")),
        "user": os.getenv("WRITE_DB_USER", "root"),
        "password": os.getenv("WRITE_DB_PASS", ""),
        "db": os.getenv("WRITE_DB_NAME", "esp"),
        "connect_timeout": _CONNECT_TIMEOUT,
        # aiomysql 0.3.x 的 create_pool/connect 不支持 read_timeout kwarg
        # (只有 connect_timeout);读超时由 SQL 层 max_execution_time / 慢查监控兜底。
        "autocommit": True,
    }


def _read_cfg() -> dict:
    return {
        "host": os.getenv("READ_DB_HOST", "tidb-follower"),
        "port": int(os.getenv("READ_DB_PORT", "4000")),
        "user": os.getenv("READ_DB_USER", "root"),
        "password": os.getenv("READ_DB_PASS", ""),
        "db": os.getenv("READ_DB_NAME", "esp"),
        "connect_timeout": _CONNECT_TIMEOUT,
        "autocommit": True,
    }


async def _ensure_pools() -> None:
    """延迟初始化主/从连接池。"""
    global _write_pool, _read_pool
    if not _ENABLED or aiomysql is None:
        return
    if _write_pool is None:
        cfg = _write_cfg()
        _write_pool = await aiomysql.create_pool(
            minsize=_POOL_MIN, maxsize=_POOL_SIZE, **cfg
        )
        logger.info("esp_db write pool ready -> %s:%s/%s", cfg["host"], cfg["port"], cfg["db"])
    if _read_pool is None:
        cfg = _read_cfg()
        _read_pool = await aiomysql.create_pool(
            minsize=_POOL_MIN, maxsize=_POOL_SIZE, **cfg
        )
        logger.info("esp_db read pool ready -> %s:%s/%s", cfg["host"], cfg["port"], cfg["db"])


async def execute_write(sql: str, args: Sequence[Any] = ()) -> int:
    """写操作(INSERT/UPDATE/DELETE)走主库 tidb-primary,返回 affected rows。

    DDL(CREATE/DROP/ALTER)也走主库。
    """
    if not _ENABLED or aiomysql is None:
        logger.warning("esp_db disabled, skipping write: %s", sql[:60])
        return 0
    await _ensure_pools()
    assert _write_pool is not None
    async with _write_pool.acquire() as conn:
        async with conn.cursor() as cur:
            await cur.execute(sql, args)
            return cur.rowcount


async def execute_write_returning(sql: str, args: Sequence[Any] = ()) -> int:
    """写操作并返回新行主键 lastrowid(用于 INSERT ... 需要自增 id 回填的场景)。

    与 execute_write 区别:返回 cur.lastrowid 而非 rowcount。
    TiDB AUTO_RANDOM 主键:aiomysql 驱动支持 lastrowid(返回最新自增 id)。
    无 DB / disabled 时返回 0。
    """
    if not _ENABLED or aiomysql is None:
        logger.warning("esp_db disabled, skipping write: %s", sql[:60])
        return 0
    await _ensure_pools()
    assert _write_pool is not None
    async with _write_pool.acquire() as conn:
        async with conn.cursor() as cur:
            await cur.execute(sql, args)
            return cur.lastrowid


async def execute_read(
    sql: str, args: Sequence[Any] = (), *, forced_primary: bool = False
) -> list[tuple]:
    """读操作(SELECT)走从库 tidb-follower,返回行列表。

    forced_primary=True:写后强一致读,强制走主库(与 Java 侧 @DataSource(consistent=true) 对齐)。
    从库不可达时自动 fallback 到主库读(容错,只 warn)。
    """
    if not _ENABLED or aiomysql is None:
        logger.warning("esp_db disabled, returning empty for read: %s", sql[:60])
        return []
    await _ensure_pools()
    pool = _write_pool if forced_primary else _read_pool
    try:
        assert pool is not None
        async with pool.acquire() as conn:
            async with conn.cursor() as cur:
                await cur.execute(sql, args)
                return list(await cur.fetchall())
    except Exception as e:
        if forced_primary:
            raise
        logger.warning("esp_db read from follower failed, fallback to primary: %s", e)
        assert _write_pool is not None
        async with _write_pool.acquire() as conn:
            async with conn.cursor() as cur:
                await cur.execute(sql, args)
                return list(await cur.fetchall())


async def execute_read_one(
    sql: str, args: Sequence[Any] = (), *, forced_primary: bool = False
) -> tuple | None:
    """读单行,返回第一行或 None。"""
    rows = await execute_read(sql, args, forced_primary=forced_primary)
    return rows[0] if rows else None


async def close() -> None:
    """关闭连接池(应用 shutdown 时调用)。"""
    global _write_pool, _read_pool
    if _write_pool is not None:
        _write_pool.close()
        await _write_pool.wait_closed()
        _write_pool = None
    if _read_pool is not None:
        _read_pool.close()
        await _read_pool.wait_closed()
        _read_pool = None
