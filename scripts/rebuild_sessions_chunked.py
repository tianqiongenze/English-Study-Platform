#!/usr/bin/env python3
"""
重建 esp_speaking.sessions 表 id 类型 BIGINT→STRING(坑㉚ 修订方案 v2:分块幂等)
=========================================================================
v1 单语句 INSERT...SELECT 失败:CRDB "remote wall time too far ahead" 在执行 ~1h35m 后
回滚整个事务(sessions_new 确认 0 行,2490MB 为回滚垃圾 intent)。

v2 策略:按 source id keyset 分块,每块 200K 行独立提交(独立 txn)。
  - 任一块遇 wall-time error → 该块重试(最多 5 次),不污染已提交块。
  - 幂等:每块 INSERT 前先 DELETE 该 id 区间已存在的行(防重)。
  - 完成后 count 核验(分块累加,避免单 count 超时)。
  - 恢复 3 副本 zone + 等 backfill → DROP 旧表 + RENAME。
"""
import os, sys, time
import psycopg2

CRDB_HOST = os.environ.get("CRDB_HOST", "crdb-public")
CRDB_PORT = int(os.environ.get("CRDB_PORT", "26257"))
DB = "esp_speaking"
CHUNK = 200000
MAX_RETRY = 5

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


def log(msg):
    print("[%s] %s" % (time.strftime("%H:%M:%S"), msg), flush=True)


def connect():
    c = psycopg2.connect(
        host=CRDB_HOST, port=CRDB_PORT, user="root", dbname=DB,
        sslmode="disable", connect_timeout=15,
    )
    c.autocommit = True
    return c


def main():
    log("连接 CRDB %s:%d/%s" % (CRDB_HOST, CRDB_PORT, DB))
    c = connect()
    cur = c.cursor()

    cur.execute("SELECT count(*) FROM esp_speaking.sessions")
    old_count = cur.fetchone()[0]
    log("源表 sessions count=%d" % old_count)

    cur.execute(NEW_TABLE_DDL)
    log("sessions_new 建表就绪(IF NOT EXISTS)")

    # 载入期 1 副本提速
    cur.execute(
        "ALTER TABLE esp_speaking.sessions_new CONFIGURE ZONE USING "
        "num_replicas = 1, constraints = '[]', lease_preferences = '[]'"
    )
    log("sessions_new 降到 1 副本(载入期提速)")

    # keyset 分块 COPY
    last_id = 0
    copied = 0
    t0 = time.time()
    batch = 0
    while True:
        # 取一块源 id 边界
        cur.execute(
            "SELECT id FROM esp_speaking.sessions WHERE id > %s ORDER BY id LIMIT 1",
            (last_id,),
        )
        row = cur.fetchone()
        if row is None:
            log("源已扫尽(无更多 id > %s)" % last_id)
            break
        chunk_start = last_id
        cur.execute(
            "SELECT max(id) FROM (SELECT id FROM esp_speaking.sessions WHERE id > %s ORDER BY id LIMIT %s) t",
            (last_id, CHUNK),
        )
        chunk_end = cur.fetchone()[0]

        # 幂等:删该区间已存(STRING id 比较需 cast)
        cur.execute(
            "DELETE FROM esp_speaking.sessions_new "
            "WHERE id::INT8 > %s AND id::INT8 <= %s",
            (chunk_start, chunk_end),
        )

        # 插入该块(独立 txn, autocommit)
        sql = (
            "INSERT INTO esp_speaking.sessions_new (id, user_id, scenario_id, status, started_at, ended_at) "
            "SELECT id::STRING, user_id, scenario_id, status, started_at, ended_at "
            "FROM esp_speaking.sessions WHERE id > %s AND id <= %s"
        )
        ok = False
        for attempt in range(1, MAX_RETRY + 1):
            try:
                cur.execute(sql, (chunk_start, chunk_end))
                ok = True
                break
            except Exception as e:
                log("  块[%s,%s] 第%d次失败: %s" % (chunk_start, chunk_end, attempt, str(e)[:120]))
                if attempt == MAX_RETRY:
                    log("!! 块[%s,%s] 达最大重试,中止" % (chunk_start, chunk_end))
                    sys.exit(1)
                time.sleep(5)
                # 重连
                try:
                    c.close()
                except Exception:
                    pass
                c = connect()
                cur = c.cursor()

        cur.execute(
            "SELECT count(*) FROM esp_speaking.sessions WHERE id > %s AND id <= %s",
            (chunk_start, chunk_end),
        )
        chunk_rows = cur.fetchone()[0]
        copied += chunk_rows
        batch += 1
        last_id = chunk_end
        dt = time.time() - t0
        rate = copied / max(dt, 0.01)
        log("块#%d [%s,%s] +%d 行,累计 %d/%d (%.1f%%) %.0f rows/s" % (
            batch, chunk_start, chunk_end, chunk_rows, copied, old_count,
            100.0 * copied / old_count, rate))

    log("COPY 全部完成:累计 %d / 源 %d" % (copied, old_count))
    if copied != old_count:
        log("!! count 不一致,中止,不 drop 旧表")
        sys.exit(1)

    # 恢复 3 副本 zone + 等 backfill
    log("恢复 sessions_new 3 副本 zone")
    cur.execute(
        "ALTER TABLE esp_speaking.sessions_new CONFIGURE ZONE USING "
        "num_replicas = 3, constraints = '{+zone=z1: 1, +zone=z2: 1, +zone=z3: 1}', "
        "lease_preferences = '[[+zone=z1]]'"
    )
    deadline = time.time() + 7200
    last_log = 0.0
    while True:
        cur.execute(
            "SELECT count(*) FROM [SHOW RANGES FROM TABLE esp_speaking.sessions_new WITH DETAILS] "
            "WHERE cardinality(replica_localities) < 3"
        )
        pending = cur.fetchone()[0]
        if pending == 0:
            log("backfill 完成:所有 range 已 3 副本")
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

    # 终验 count:用 keyset 分块累加(单 count 在 17M 行会超时)
    log("终验 count(keyset 分块累加,期望=%d)..." % old_count)
    final_count = 0
    vc_id = 0
    while True:
        cur.execute(
            "SELECT count(*) FROM (SELECT id FROM esp_speaking.sessions WHERE id > %s ORDER BY id LIMIT %s) t",
            (vc_id, CHUNK),
        )
        n = cur.fetchone()[0]
        if n == 0:
            break
        final_count += n
        cur.execute(
            "SELECT max(id) FROM (SELECT id FROM esp_speaking.sessions WHERE id > %s ORDER BY id LIMIT %s) t",
            (vc_id, CHUNK),
        )
        vc_id = cur.fetchone()[0]
    log("终验 count=%d (期望=%d)" % (final_count, old_count))
    if final_count != old_count:
        log("!! 终验失败:count 不匹配")
        sys.exit(1)
    log("重建完成✅")


if __name__ == "__main__":
    main()
