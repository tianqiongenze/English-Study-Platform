#!/usr/bin/env python3
"""
ESP esp_speaking 数据迁移:TiDB → CockroachDB (#140 全服务切 CRDB 的一部分)
=====================================================================
源: TiDB  esp_speaking.scenarios(~1万) / sessions(~5000万)
目标: CRDB esp_speaking.scenarios / sessions(同构,保留源 id)

迁移方式:
- scenarios: 小表,一次性全量,INSERT ... ON CONFLICT (id) DO NOTHING。
- sessions: 按 id 游标分页(WHERE id > last_id ORDER BY id LIMIT batch),
  psycopg2 execute_values 批量多行 INSERT,批后 sleep 节流。
  断点续迁:启动时取 CRDB max(id) 与 count 作为续迁点,批次原子提交。

进度:
- 本地文件 report/sessions_migrate_progress.txt(done/total/last_id/time/phase)
- Redis 锚点由外部 kubectl 写(key esp:ctx:seed:migrate:sessions)

用法:
  python3 migrate_esp_speaking.py --only scenarios     # 只迁 scenarios
  python3 migrate_esp_speaking.py --only sessions      # 只迁 sessions(续迁自动)
  python3 migrate_esp_speaking.py --batch 4000 --sleep 0.2 --limit 200000
环境变量: TIDB_HOST/TIDB_PORT/CRDB_HOST/CRDB_PORT
"""
import os, sys, time, argparse
from datetime import datetime

import pymysql
import psycopg2
from psycopg2.extras import execute_values

TIDB_HOST = os.environ.get("TIDB_HOST", "tidb-primary")
TIDB_PORT = int(os.environ.get("TIDB_PORT", "4000"))
CRDB_HOST = os.environ.get("CRDB_HOST", "crdb-public")
CRDB_PORT = int(os.environ.get("CRDB_PORT", "26257"))

REPORT = os.environ.get("MIGRATE_REPORT", "report/sessions_migrate_progress.txt")

# sessions 目标总量(用 TiDB information_schema 估算;迁移后精确 COUNT 核对)
SESSIONS_TOTAL = int(os.environ.get("SESSIONS_TOTAL", "50000000"))
SESSIONS_COLS = ["id", "user_id", "scenario_id", "status", "started_at", "ended_at"]

DEFAULT_BATCH = int(os.environ.get("MIGRATE_BATCH", "4000"))
DEFAULT_SLEEP = float(os.environ.get("MIGRATE_SLEEP", "0.2"))


def log(msg):
    line = "[%s] %s" % (datetime.now().strftime("%H:%M:%S"), msg)
    print(line, flush=True)


MAX_RETRY = int(os.environ.get("MIGRATE_MAX_RETRY", "10"))


class TiDBSession:
    """TiDB 游标读写包装:连接被 TiKV/网络断开时重连重试(坑㉑ 模式,同 seed_3b)。

    坑㉜修正:原实现在 except 块内调 connect_tidb(),连接失败抛出的 2003
    OperationalError 逃逸出 retry 循环,直接杀死脚本(TiDB OOM 重启后无法自愈)。
    现将 connect 移入 try 块内,连不上同样走 retry,最多 MAX_RETRY 次。
    """

    def __init__(self):
        self.conn = None

    def _connect(self):
        if self.conn is None:
            self.conn = connect_tidb()

    def _reset(self):
        try:
            self.conn.close()
        except Exception:
            pass
        self.conn = None

    def query(self, sql, params):
        last = None
        for i in range(MAX_RETRY):
            try:
                self._connect()
                with self.conn.cursor() as cur:
                    cur.execute(sql, params)
                    return cur.fetchall()
            except (pymysql.OperationalError, pymysql.InterfaceError) as e:
                last = e
                wait = 5 * (i + 1)
                log("TiDB 连接断开(%s),%ds 后重连 %d/%d" % (e, wait, i + 1, MAX_RETRY))
                time.sleep(wait)
                self._reset()
        raise last

    def close(self):
        try:
            self.conn.close()
        except Exception:
            pass


class CRDBSession:
    """CRDB 写入包装:批量 execute_values 断开时重连重试(坑㉑ 模式)。

    坑㉜修正:与 TiDBSession 同,connect 移入 try 内,连接失败也走 retry。
    """

    def __init__(self):
        self.conn = None

    def _connect(self):
        if self.conn is None:
            self.conn = connect_crdb()

    def _reset(self):
        try:
            self.conn.close()
        except Exception:
            pass
        self.conn = None

    def write(self, sql, rows, page_size):
        last = None
        for i in range(MAX_RETRY):
            try:
                self._connect()
                with self.conn.cursor() as cur:
                    execute_values(cur, sql, rows, page_size=page_size)
                return
            except psycopg2.OperationalError as e:
                last = e
                wait = 5 * (i + 1)
                log("CRDB 写入断开(%s),%ds 后重连 %d/%d" % (e, wait, i + 1, MAX_RETRY))
                time.sleep(wait)
                self._reset()
        raise last

    def close(self):
        try:
            self.conn.close()
        except Exception:
            pass


def write_progress(done, total, last_id, phase):
    os.makedirs(os.path.dirname(REPORT) or ".", exist_ok=True)
    ts = datetime.now().strftime("%Y-%m-%dT%H:%M:%S")
    content = "done=%d total=%d last_id=%s phase=%s time=%s" % (done, total, last_id, phase, ts)
    with open(REPORT, "w") as f:
        f.write(content + "\n")


def connect_tidb():
    c = pymysql.connect(
        host=TIDB_HOST, port=TIDB_PORT, user="root", database="esp_speaking",
        charset="utf8mb4", connect_timeout=15, read_timeout=600, write_timeout=120,
    )
    with c.cursor() as cur:
        cur.execute("SET time_zone = '+00:00'")
    return c


def connect_crdb():
    c = psycopg2.connect(
        host=CRDB_HOST, port=CRDB_PORT, user="root", dbname="esp_speaking",
        sslmode="disable", connect_timeout=15, options="-c search_path=public",
    )
    c.autocommit = True
    with c.cursor() as cur:
        cur.execute("SET TIME ZONE 'UTC'")
    return c


def migrate_scenarios(tidb: TiDBSession, crdb: CRDBSession):
    crdb._connect()
    with crdb.conn.cursor() as cur:
        cur.execute("SELECT count(*) FROM esp_speaking.scenarios")
        existing = cur.fetchone()[0]
    if existing > 0:
        log("scenarios 已存在 %d 行,跳过" % existing)
        return existing
    rows = tidb.query(
        "SELECT id, code, title, system_prompt, difficulty, lang FROM esp_speaking.scenarios ORDER BY id", ()
    )
    log("scenarios 读取 %d 行,写入 CRDB" % len(rows))
    crdb.write(
        "INSERT INTO esp_speaking.scenarios (id, code, title, system_prompt, difficulty, lang)"
        " VALUES %s ON CONFLICT (id) DO NOTHING",
        rows,
        2000,
    )
    with crdb.conn.cursor() as cur:
        cur.execute("SELECT count(*) FROM esp_speaking.scenarios")
        n = cur.fetchone()[0]
    log("scenarios 迁移完成:CRDB count=%d" % n)
    return n


def crdb_sessions_state(crdb: "CRDBSession"):
    crdb._connect()
    with crdb.conn.cursor() as cur:
        cur.execute("SELECT count(*), coalesce(max(id), 0) FROM esp_speaking.sessions")
        cnt, maxid = cur.fetchone()
    return int(cnt), int(maxid)


def migrate_sessions(tidb: TiDBSession, crdb: CRDBSession, batch, sleep_s, limit=None):
    done, last_id = crdb_sessions_state(crdb)
    log("sessions 续迁点:CRDB count=%d max_id=%s,total=%d" % (done, last_id, SESSIONS_TOTAL))
    if done >= SESSIONS_TOTAL:
        log("sessions 已达标,无需迁移")
        return done
    # 预取第一行最小 id,保证游标正确
    rows = tidb.query("SELECT min(id) FROM esp_speaking.sessions", ())
    min_id = rows[0][0] if rows else 0
    if last_id <= 0:
        last_id = min_id - 1
        log("首次迁移,min_id=%s,游标起点=%s" % (min_id, last_id))
    batch_no = 0
    t0 = time.time()
    while True:
        rows = tidb.query(
            "SELECT id, user_id, scenario_id, status, started_at, ended_at"
            " FROM esp_speaking.sessions WHERE id > %s ORDER BY id LIMIT %s",
            (last_id, batch),
        )
        if not rows:
            log("游标取空,迁移完成。CRDB count=%d" % done)
            write_progress(done, SESSIONS_TOTAL, last_id, "sessions-done")
            break
        crdb.write(
            "INSERT INTO esp_speaking.sessions (id, user_id, scenario_id, status, started_at, ended_at)"
            " VALUES %s ON CONFLICT (id) DO NOTHING",
            rows,
            batch,
        )
        last_id = rows[-1][0]
        done += len(rows)
        batch_no += 1
        if batch_no % 10 == 0:
            rate = done / max(time.time() - t0, 0.1)
            eta_s = (SESSIONS_TOTAL - done) / max(rate, 0.01)
            log("batch=%d done=%d (%.2f%%) last_id=%s rate=%.0f rows/s eta=%.1fh"
                % (batch_no, done, 100.0 * done / SESSIONS_TOTAL, last_id, rate, eta_s / 3600))
            write_progress(done, SESSIONS_TOTAL, last_id, "sessions")
        if limit and done >= limit:
            log("达 --limit %d,退出" % limit)
            write_progress(done, SESSIONS_TOTAL, last_id, "sessions-limited")
            break
        time.sleep(sleep_s)
    return done


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--only", choices=["scenarios", "sessions"], default=None)
    ap.add_argument("--batch", type=int, default=DEFAULT_BATCH)
    ap.add_argument("--sleep", type=float, default=DEFAULT_SLEEP)
    ap.add_argument("--limit", type=int, default=None)
    args = ap.parse_args()

    log("连接 TiDB %s:%d / CRDB %s:%d" % (TIDB_HOST, TIDB_PORT, CRDB_HOST, CRDB_PORT))
    tidb = TiDBSession()
    crdb = CRDBSession()
    tidb._connect()
    crdb._connect()
    log("连接成功")

    if args.only in (None, "scenarios"):
        migrate_scenarios(tidb, crdb)
    if args.only in (None, "sessions"):
        migrate_sessions(tidb, crdb, args.batch, args.sleep, args.limit)

    tidb.close()
    crdb.close()
    log("退出")


if __name__ == "__main__":
    main()
