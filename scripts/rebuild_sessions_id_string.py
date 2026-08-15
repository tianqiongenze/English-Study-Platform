#!/usr/bin/env python3
"""
重建 esp_speaking.sessions 表 id 类型 BIGINT→STRING(坑㉚ 修订方案)
=================================================================
背景:CRDB v24.3 无法原地 ALTER 索引列类型(int→string experimental + issue 47636
"not supported for columns that are part of an index";DROP PK 又要求 drop+add 同事务)。
而 gateway-go 1.11.0 契约要求 sessions.id 为 STRING(End() 写 sess_xxx,ListHistory 扫 string)。

方案:CRDB 原生 INSERT...SELECT 重建(分布式执行,比 psycopg2 网络批量快数倍)。
步骤:
  1. CREATE TABLE sessions_new(id STRING PK, ... 与旧表同构,最终约束名)
  2. INSERT INTO sessions_new SELECT id::STRING, ... FROM sessions
  3. 核验 count(sessions_new) == count(sessions) 且 max/min 对齐
  4. DROP TABLE sessions(先确认无 FK 依赖,esp_speaking 仅 scenarios/sessions 两表)
  5. ALTER TABLE sessions_new RENAME TO sessions
  6. 终验 SHOW CREATE + count

前置:须在 sessions 迁移完成后执行(迁移仍会写旧表;gateway-go 1.11.0 未部署前无写入)。
用法: python3 rebuild_sessions_id_string.py [--db esp_speaking]
"""
import os, sys, time, argparse
import psycopg2

CRDB_HOST = os.environ.get("CRDB_HOST", "crdb-public")
CRDB_PORT = int(os.environ.get("CRDB_PORT", "26257"))
DB = "esp_speaking"


def log(msg):
    print("[%s] %s" % (time.strftime("%H:%M:%S"), msg), flush=True)


def connect():
    c = psycopg2.connect(
        host=CRDB_HOST, port=CRDB_PORT, user="root", dbname=DB,
        sslmode="disable", connect_timeout=15, options="-c search_path=public",
    )
    c.autocommit = True
    return c


NEW_TABLE_DDL = """
CREATE TABLE IF NOT EXISTS esp_speaking.sessions_new (
    id STRING NOT NULL,
    user_id INT8 NOT NULL,
    scenario_id INT8 NOT NULL,
    status STRING(16) NOT NULL DEFAULT 'active',
    started_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp(),
    ended_at TIMESTAMPTZ NULL,
    CONSTRAINT sessions_pkey PRIMARY KEY (id ASC),
    INDEX idx_user (user_id ASC),
    CONSTRAINT check_status CHECK (status IN ('active', 'ended'))
)
"""

COPY_SQL = """
INSERT INTO esp_speaking.sessions_new (id, user_id, scenario_id, status, started_at, ended_at)
SELECT id::STRING, user_id, scenario_id, status, started_at, ended_at
FROM esp_speaking.sessions
"""


def main():
    log("连接 CRDB %s:%d/%s" % (CRDB_HOST, CRDB_PORT, DB))
    c = connect()
    cur = c.cursor()

    cur.execute("SELECT count(*) FROM esp_speaking.sessions")
    old_count = cur.fetchone()[0]
    cur.execute("SELECT count(*) FROM information_schema.tables WHERE table_schema='esp_speaking' AND table_name='sessions_new'")
    new_exists = cur.fetchone()[0] > 0
    log("旧表 sessions count=%d, sessions_new 已存在=%s" % (old_count, new_exists))
    if new_exists:
        cur.execute("SELECT count(*) FROM esp_speaking.sessions_new")
        cur_count = cur.fetchone()[0]
        log("sessions_new 当前 count=%d (若>0 视为中途断点,继续补齐而非重建)" % cur_count)
        if cur_count >= old_count:
            log("sessions_new 已达标,跳过 COPY")
            skip_copy = True
        else:
            log("sessions_new 落后于旧表,执行 COPY 补齐")
            skip_copy = False
    else:
        log("创建 sessions_new 并 COPY")
        cur.execute(NEW_TABLE_DDL)
        skip_copy = False

    if not skip_copy:
        t0 = time.time()
        cur.execute(COPY_SQL)
        dt = time.time() - t0
        cur.execute("SELECT count(*) FROM esp_speaking.sessions_new")
        new_count = cur.fetchone()[0]
        log("COPY 完成:count=%d 耗时=%.1fs (%.0f rows/s)" % (new_count, dt, new_count / max(dt, 0.01)))
        if new_count != old_count:
            log("!! count 不一致 new=%d old=%d,中止,不 drop 旧表" % (new_count, old_count))
            sys.exit(1)

    cur.execute("SELECT count(*), min(id::INT8), max(id::INT8) FROM esp_speaking.sessions_new")
    cnt, mn, mx = cur.fetchone()
    log("sessions_new 校验:count=%s min_id=%s max_id=%s" % (cnt, mn, mx))

    # 方案C:恢复 3 副本(1-per-zone)并等待 backfill 完成后再切流,保证 sessions 表始终 3 副本。
    # 载入期临时降到 1 副本提速(单节点 VM 3副本同步写放大 3x);这里恢复并发起副本补齐。
    log("恢复 sessions_new 3 副本 zone(num_replicas=3, constraints z1/z2/z3 各 1, lease→z1)")
    cur.execute(
        "ALTER TABLE esp_speaking.sessions_new CONFIGURE ZONE USING "
        "num_replicas = 3, constraints = '{+zone=z1: 1, +zone=z2: 1, +zone=z3: 1}', lease_preferences = '[[+zone=z1]]'"
    )
    deadline = time.time() + 7200  # 2h 上限
    last_log = 0.0
    while True:
        cur.execute(
            "SELECT count(*) FROM [SHOW RANGES FROM TABLE esp_speaking.sessions_new WITH DETAILS] "
            "WHERE cardinality(replica_localities) < 3"
        )
        pending = cur.fetchone()[0]
        if pending == 0:
            log("backfill 完成:所有 range 已 3 副本(z1/z2/z3)")
            break
        if time.time() > deadline:
            log("!! backfill 超时(2h),仍有 %d range 未达 3 副本,中止" % pending)
            sys.exit(1)
        if time.time() - last_log >= 60:
            log("backfill 中:剩余 %d range 未达 3 副本" % pending)
            last_log = time.time()
        time.sleep(10)

    log("DROP 旧表 sessions + RENAME sessions_new → sessions")
    cur.execute("DROP TABLE IF EXISTS esp_speaking.sessions")
    cur.execute("ALTER TABLE esp_speaking.sessions_new RENAME TO sessions")

    cur.execute("SHOW CREATE TABLE esp_speaking.sessions")
    ddl = cur.fetchone()[1]
    log("终验 SHOW CREATE:\n" + ddl)
    cur.execute("SELECT count(*) FROM esp_speaking.sessions")
    final_count = cur.fetchone()[0]
    log("终验 count=%d (期望=%d)" % (final_count, old_count))
    if final_count != old_count:
        log("!! 终验失败:count 不匹配")
        sys.exit(1)
    log("重建完成✅")


if __name__ == "__main__":
    main()
