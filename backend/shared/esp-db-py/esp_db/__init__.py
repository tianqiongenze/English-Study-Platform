"""ESP 读写分离数据访问公共包(Python 侧,与 Java 侧 rw/ 语义对齐)。数据访问已迁 CockroachDB(#140)。

设计:
  - 两个连接池:write_pool -> CRDB 主端点(crdb-public),read_pool -> CRDB 读端点(单端点阶段与写同址)。
  - 调用方显式选择 execute_write / execute_read;读操作默认走从池,写后强一致读可强制走主池(forced_primary=True)。
  - 连接池基于 asyncpg.create_pool(异步,兼容 Litestar/asyncio)。
  - feature flag:DB_ENABLED=false 时 no-op(回退安全,供测试/无 DB 环境降级)。
  - 降级:从池不可达时自动 fallback 到主池读(只 warn 不阻断),与 Java 侧 RoutingDataSource 容错语义一致。
  - SQL 占位符:调用方仍写 MySQL 风格 %s;本层自动转 PG/CRDB 风格 $1,$2...(_pg_sql)。

env(CRDB 分库模型,每个服务部署必须把 WRITE_DB_NAME/READ_DB_NAME 指到本服务域库):
  WRITE_DB_HOST=crdb-public  WRITE_DB_PORT=26257  WRITE_DB_USER=root  WRITE_DB_PASS=  WRITE_DB_NAME=<域库>
  READ_DB_HOST=crdb-public   READ_DB_PORT=26257   READ_DB_USER=root   READ_DB_PASS=   READ_DB_NAME=<域库>
  DB_ENABLED=true
  DB_POOL_SIZE=10  DB_POOL_MIN=2  DB_CONNECT_TIMEOUT=10
"""
from __future__ import annotations

import json as _json
import logging
import os
from typing import Any, Sequence

try:
    import asyncpg
except ImportError:  # 降级:无 asyncpg 时 no-op(测试环境)
    asyncpg = None  # type: ignore

logger = logging.getLogger("esp_db")

_ENABLED = os.getenv("DB_ENABLED", "true").lower() != "false"
_POOL_SIZE = int(os.getenv("DB_POOL_SIZE", "10"))
_POOL_MIN = int(os.getenv("DB_POOL_MIN", "2"))
_CONNECT_TIMEOUT = int(os.getenv("DB_CONNECT_TIMEOUT", "10"))

_write_pool = None  # asyncpg.Pool | None
_read_pool = None   # asyncpg.Pool | None


def _jsonb_encoder(value: Any) -> str:
    """jsonb 绑定编码:str 视为已序列化 JSON 文本原样发送(dict/list 走 json.dumps)。

    保持与旧 aiomysql/MySQL 语义一致:调用方传 json.dumps 字符串即被 CRDB 解析为 JSON 对象,
    避免 asyncpg 默认对 str 二次编码成 JSON 字符串。
    """
    if value is None:
        return "null"
    if isinstance(value, (dict, list, tuple)):
        return _json.dumps(value, ensure_ascii=False)
    return str(value)


async def _init_conn(conn: "asyncpg.Connection") -> None:
    """每个池内连接初始化:覆盖 jsonb 编解码(encoder 见上;decoder 回 dict 与 PyMySQL 语义一致)。"""
    await conn.set_type_codec(
        "jsonb",
        encoder=_jsonb_encoder,
        decoder=_json.loads,
        format="text",
        schema="pg_catalog",
    )


def _write_cfg() -> dict:
    return {
        "host": os.getenv("WRITE_DB_HOST", "crdb-public"),
        "port": int(os.getenv("WRITE_DB_PORT", "26257")),
        "user": os.getenv("WRITE_DB_USER", "root"),
        "password": os.getenv("WRITE_DB_PASS", ""),
        "database": os.getenv("WRITE_DB_NAME", "esp"),
    }


def _read_cfg() -> dict:
    return {
        "host": os.getenv("READ_DB_HOST", "crdb-public"),
        "port": int(os.getenv("READ_DB_PORT", "26257")),
        "user": os.getenv("READ_DB_USER", "root"),
        "password": os.getenv("READ_DB_PASS", ""),
        "database": os.getenv("READ_DB_NAME", "esp"),
    }


async def _ensure_pools() -> None:
    """延迟初始化主/从连接池。"""
    global _write_pool, _read_pool
    if not _ENABLED or asyncpg is None:
        return
    if _write_pool is None:
        cfg = _write_cfg()
        _write_pool = await asyncpg.create_pool(
            min_size=_POOL_MIN, max_size=_POOL_SIZE,
            timeout=_CONNECT_TIMEOUT, init=_init_conn, **cfg,
        )
        logger.info("esp_db write pool ready -> %s:%s/%s", cfg["host"], cfg["port"], cfg["database"])
    if _read_pool is None:
        cfg = _read_cfg()
        _read_pool = await asyncpg.create_pool(
            min_size=_POOL_MIN, max_size=_POOL_SIZE,
            timeout=_CONNECT_TIMEOUT, init=_init_conn, **cfg,
        )
        logger.info("esp_db read pool ready -> %s:%s/%s", cfg["host"], cfg["port"], cfg["database"])


def _pg_sql(sql: str, args: Sequence[Any]) -> tuple[str, Sequence[Any]]:
    """把 MySQL 风格 %s 占位符转 PG/CRDB 风格 $1,$2...。

    asyncpg 用 $n 位置参数,不支持 %s。调用方 SQL 保持 %s(改动面最小,service.py/db.py 零改动)。
    """
    n = len(args)
    if n == 0:
        return sql, args
    parts = sql.split("%s")
    if len(parts) - 1 != n:
        raise ValueError(f"esp_db: SQL %s placeholder count != args count: {sql!r}")
    out: list[str] = []
    for i, part in enumerate(parts[:-1]):
        out.append(part)
        out.append(f"${i + 1}")
    out.append(parts[-1])
    return "".join(out), args


def _rowcount_from_status(status: str) -> int:
    """asyncpg conn.execute 返回命令状态串('INSERT 0 1'),解析受影响行数;DDL 无数字则返回 0。"""
    try:
        return int(status.rsplit(" ", 1)[1])
    except (ValueError, IndexError):
        return 0


async def execute_write(sql: str, args: Sequence[Any] = ()) -> int:
    """写操作(INSERT/UPDATE/DELETE)走主池,返回 affected rows。DDL(CREATE/DROP/ALTER)也走主池。"""
    if not _ENABLED or asyncpg is None:
        logger.warning("esp_db disabled, skipping write: %s", sql[:60])
        return 0
    await _ensure_pools()
    assert _write_pool is not None
    pg_sql, pg_args = _pg_sql(sql, args)
    async with _write_pool.acquire() as conn:
        status = await conn.execute(pg_sql, *pg_args)
        return _rowcount_from_status(status)


async def execute_write_returning(sql: str, args: Sequence[Any] = ()) -> int:
    """写操作并返回新行主键 id(用于 INSERT ... 需要自增 id 回填的场景)。

    CRDB 不支持 MySQL lastrowid 语义;对 INSERT 自动追加 ' RETURNING id'(CRDB DDL 约定主键列名为 id,
    如 writing_plans/writing_progress 的 unique_rowid() id),经 fetchval 取回。无 DB / disabled 时返回 0。
    """
    if not _ENABLED or asyncpg is None:
        logger.warning("esp_db disabled, skipping write: %s", sql[:60])
        return 0
    await _ensure_pools()
    assert _write_pool is not None
    pg_sql, pg_args = _pg_sql(sql, args)
    upper = pg_sql.lstrip().upper()
    if upper.startswith("INSERT") and " RETURNING " not in upper:
        pg_sql += " RETURNING id"
        async with _write_pool.acquire() as conn:
            val = await conn.fetchval(pg_sql, *pg_args)
            return int(val) if val is not None else 0
    async with _write_pool.acquire() as conn:
        status = await conn.execute(pg_sql, *pg_args)
        return _rowcount_from_status(status)


async def execute_read(
    sql: str, args: Sequence[Any] = (), *, forced_primary: bool = False
) -> list[tuple]:
    """读操作(SELECT)走从池,返回行列表 list[tuple]。

    forced_primary=True:写后强一致读,强制走主池(与 Java 侧 @DataSource(consistent=true) 对齐)。
    从池不可达时自动 fallback 到主池读(容错,只 warn)。
    """
    if not _ENABLED or asyncpg is None:
        logger.warning("esp_db disabled, returning empty for read: %s", sql[:60])
        return []
    await _ensure_pools()
    pg_sql, pg_args = _pg_sql(sql, args)
    pool = _write_pool if forced_primary else _read_pool
    try:
        assert pool is not None
        async with pool.acquire() as conn:
            rows = await conn.fetch(pg_sql, *pg_args)
            return [tuple(r) for r in rows]
    except Exception as e:
        if forced_primary:
            raise
        logger.warning("esp_db read from follower failed, fallback to primary: %s", e)
        assert _write_pool is not None
        async with _write_pool.acquire() as conn:
            rows = await conn.fetch(pg_sql, *pg_args)
            return [tuple(r) for r in rows]


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
        await _write_pool.close()
        _write_pool = None
    if _read_pool is not None:
        await _read_pool.close()
        _read_pool = None
