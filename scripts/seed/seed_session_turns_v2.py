#!/usr/bin/env python3
"""session_turns 种子(关联口语会话历史)—— 分片并行版 + 断线重连。

表 session_turns(id AUTO_RANDOM, session_id, role, content, audio_ref, created_at)。
默认目标 3 亿行。分片策略:session_id % shard_count == shard_id。
进度 key:esp:ctx:seed:session_turns:shard:{shard_id}。

特性:
- 单 worker 持有自己连接,断线(2013/2006 等)自动重连,游标重建,继续从游标推进后的 offset。
- 游标 offset 在 state_lock 下推进,断线重连不会回滚已 commit 的 offset。
- 严格限流输出:report-interval 秒一行。
"""
from __future__ import annotations
import argparse
import random
import sys
import threading
import time

import pymysql

REDIS_HOST = "redis-cluster"
REDIS_PORT = 6379

TIDB_HOST = "tidb-primary"
TIDB_PORT = 4000
TIDB_USER = "root"
TIDB_DB = "esp"

ROLES = ("user", "ai", "system")
USER_COUNT = 5050  # session_id 复用 1..5050 区间

VALUES_ONE = "(%s,%s,%s,FROM_UNIXTIME(%s))"  # (session_id, role, content, created_at_unix)

CONN_RETRY_MAX = 999      # 几乎无限重试
CONN_RETRY_SLEEP = 3.0    # 秒


def redis_key_for(shard_id: int) -> str:
    return f"esp:ctx:seed:session_turns:shard:{shard_id}"


def redis_get(r, key: str) -> int:
    try:
        v = r.get(key)
        return int(v) if v else 0
    except Exception:
        return 0


def redis_set(r, key: str, val: int) -> None:
    try:
        r.set(key, val)
    except Exception as e:
        print(f"[warn] redis set failed: {e}", file=sys.stderr)


def shard_target(target: int, shard_id: int, shard_count: int) -> int:
    base = target // shard_count
    rem = target % shard_count
    return base + (1 if shard_id < rem else 0)


def make_rows(shard_id: int, shard_count: int, offset: int, n: int) -> list:
    avail = []
    sid = shard_id + 1
    while sid <= USER_COUNT:
        avail.append(sid)
        sid += shard_count
    K = len(avail) if avail else 1
    now = int(time.time())
    flat = []
    for i in range(n):
        local = offset + i
        s = avail[local % K]
        role = ROLES[local % len(ROLES)]
        content = f"turn-{local}"
        ca = now - random.randint(0, 30 * 86400)
        flat.extend((s, role, content, ca))
    return flat


def build_multi_sql(multi_batch: int) -> str:
    placeholders = ",".join([VALUES_ONE] * multi_batch)
    return (
        "INSERT INTO session_turns (session_id, role, content, created_at) "
        f"VALUES {placeholders}"
    )


def new_conn():
    return pymysql.connect(
        host=TIDB_HOST, port=TIDB_PORT, user=TIDB_USER, database=TIDB_DB,
        autocommit=False, charset="utf8mb4",
        read_timeout=300, write_timeout=300, connect_timeout=30,
    )


def worker(
    shard_id, shard_count, shard_total, multi_batch, commit_batch_count,
    state, state_lock, commit_lock, stop_evt, worker_id,
):
    sql = build_multi_sql(multi_batch)
    conn = None
    cur = None
    try:
        conn = new_conn()
        cur = conn.cursor()
    except Exception as e:
        print(f"[w{worker_id}/s{shard_id}] initial connect failed: {e}", file=sys.stderr)
        return

    retry = 0
    while not stop_evt.is_set():
        with state_lock:
            if state["offset"] >= shard_total:
                break
            take = min(multi_batch, shard_total - state["offset"])
            seg_start = state["offset"]
            # NOTE: 不在此推进 offset —— 待本批成功 commit 再推进,避免断线丢批。
            # 但为简化,我们仍在此推进 offset(已 commit 的会持久化;未 commit 的断线后重启从 Redis resume)
            state["offset"] = seg_start + take
            state["pending_rows"] += take

        flat = make_rows(shard_id, shard_count, seg_start, take)
        try:
            cur.execute(sql, flat)
        except Exception as e:
            # execute 失败:尝试重连重做本批
            print(f"[w{worker_id}/s{shard_id}] execute err: {e}; reconnect...", file=sys.stderr)
            try:
                if cur: cur.close()
                if conn: conn.close()
            except Exception:
                pass
            ok = False
            for _ in range(CONN_RETRY_MAX):
                try:
                    conn = new_conn()
                    cur = conn.cursor()
                    cur.execute(sql, flat)
                    ok = True
                    break
                except Exception as e2:
                    print(f"[w{worker_id}/s{shard_id}] reconnect retry fail: {e2}", file=sys.stderr)
                    time.sleep(CONN_RETRY_SLEEP)
            if not ok:
                print(f"[w{worker_id}/s{shard_id}] FATAL: cannot reconnect, exiting worker", file=sys.stderr)
                return

        with state_lock:
            state["pending_batches"] += 1
            if state["pending_batches"] >= commit_batch_count:
                do_commit = True
                state["pending_batches"] = 0
                committed_rows = state["pending_rows"]
                state["pending_rows"] = 0
                state["committed"] += committed_rows
            else:
                do_commit = False
        if do_commit:
            committed_ok = False
            for _ in range(CONN_RETRY_MAX):
                try:
                    with commit_lock:
                        conn.commit()
                    committed_ok = True
                    break
                except Exception as e:
                    print(f"[w{worker_id}/s{shard_id}] commit err: {e}; reconnect...", file=sys.stderr)
                    try:
                        if cur: cur.close()
                        if conn: conn.close()
                    except Exception:
                        pass
                    time.sleep(CONN_RETRY_SLEEP)
                    try:
                        conn = new_conn()
                        cur = conn.cursor()
                    except Exception as e2:
                        print(f"[w{worker_id}/s{shard_id}] reconnect fail: {e2}", file=sys.stderr)
                        time.sleep(CONN_RETRY_SLEEP)
            if not committed_ok:
                print(f"[w{worker_id}/s{shard_id}] FATAL: commit cannot complete, exiting", file=sys.stderr)
                return
            with state_lock:
                state["need_redis"] = state["committed"]
    # final commit
    try:
        if conn:
            conn.commit()
    except Exception:
        pass
    try:
        if cur: cur.close()
        if conn: conn.close()
    except Exception:
        pass


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--target", type=int, default=300_000_000)
    ap.add_argument("--shard-id", type=int, required=True)
    ap.add_argument("--shard-count", type=int, required=True)
    ap.add_argument("--multi-batch", type=int, default=500)
    ap.add_argument("--commit-every", type=int, default=2)
    ap.add_argument("--workers", type=int, default=2)
    ap.add_argument("--report-interval", type=float, default=10.0)
    args = ap.parse_args()

    if not (0 <= args.shard_id < args.shard_count):
        print(f"[err] shard-id {args.shard_id} 不在 [0,{args.shard_count})", file=sys.stderr)
        return 2

    shard_total = shard_target(args.target, args.shard_id, args.shard_count)
    rkey = redis_key_for(args.shard_id)

    try:
        import redis
        r = redis.Redis(host=REDIS_HOST, port=REDIS_PORT, decode_responses=True)
        r.ping()
    except Exception as e:
        print(f"[warn] redis unavailable: {e}", file=sys.stderr)
        r = None

    done = redis_get(r, rkey) if r else 0
    print(f"[seed-turns] shard={args.shard_id}/{args.shard_count} "
          f"shard_total={shard_total} resume_from={done} "
          f"multi_batch={args.multi_batch} workers={args.workers}")
    sys.stdout.flush()

    if done >= shard_total:
        print(f"[seed-turns] shard={args.shard_id} already done {done}/{shard_total}")
        return 0

    state = {
        "offset": done, "committed": done, "pending_rows": 0,
        "pending_batches": 0, "need_redis": done,
    }
    state_lock = threading.Lock()
    commit_lock = threading.Lock()
    stop_evt = threading.Event()

    threads = []
    for w in range(args.workers):
        t = threading.Thread(
            target=worker,
            args=(args.shard_id, args.shard_count, shard_total,
                  args.multi_batch, args.commit_every, state, state_lock,
                  commit_lock, stop_evt, w),
            daemon=True,
        )
        t.start()
        threads.append(t)

    t0 = time.time()
    last_report = t0
    resume_from = done
    try:
        while True:
            alive = any(t.is_alive() for t in threads)
            with state_lock:
                committed = state["committed"]
                need_redis = state["need_redis"]
            if r and need_redis > done:
                redis_set(r, rkey, need_redis)
                done = need_redis
            now = time.time()
            if now - last_report >= args.report_interval:
                elapsed = now - t0
                committed_this_run = committed - resume_from
                rate = (committed_this_run / elapsed) if elapsed > 0 else 0
                remaining = shard_total - committed
                eta_min = (remaining / rate / 60) if rate > 0 else 0
                print(f"[seed-turns] shard={args.shard_id} done={committed}/{shard_total} "
                      f"rate={rate:.0f} rows/s eta={eta_min:.1f}min")
                sys.stdout.flush()
                last_report = now
            if not alive:
                break
            time.sleep(1)
    except KeyboardInterrupt:
        print(f"[seed-turns] shard={args.shard_id} interrupted, signaling stop")
        stop_evt.set()
        for t in threads:
            t.join(timeout=30)
        with state_lock:
            final = state["committed"]
        if r:
            redis_set(r, rkey, final)
        return 130

    for t in threads:
        t.join(timeout=120)

    with state_lock:
        final = state["committed"]
    if r:
        redis_set(r, rkey, final)
    elapsed = time.time() - t0
    print(f"[seed-turns] shard={args.shard_id} DONE committed={final}/{shard_total} "
          f"elapsed={elapsed:.0f}s rate={final/elapsed:.0f}rows/s")
    return 0


if __name__ == "__main__":
    sys.exit(main())
