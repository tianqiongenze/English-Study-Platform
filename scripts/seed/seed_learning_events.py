#!/usr/bin/env python3
"""30 亿种子数据灌入 TiDB(learning_events)—— 分片并行版。

表 learning_events(id AUTO_RANDOM, user_id, event_type, payload JSON, created_at)。

分片策略(保证不重不漏):
  - 按 user_id 取模切分:user_id % shard_count == shard_id 的行由 shard_id 分片负责。
  - 每个分片有独立的 offset 游标(本分片已灌行数),Redis key 隔离:
      esp:ctx:seed:learning_events:shard:{shard_id}
    值 = 该分片已灌行数。
  - 总量 target 在各分片间按比例分摊:shard_target = target // shard_count
    (余数分给前 remainder 个分片,确保 sum(shard_target)==target)。
  - user_id 在分片内循环生成:对第 i 行(本分片局部编号),候选 uid 满足
    uid % shard_count == shard_id,即 uid = shard_id + 1 + k*shard_count
    其中 k = i % (USER_COUNT // shard_count)。这样既保证该行归本分片,
    又保证 user_id 不重复出现在其它分片(取模天然互斥)。
    注:user_id 是循环复用(5050 users),所以同一 user 会被多次写入不同 event 行,
    这是设计预期(为压测/分析提供历史 event 基数)。"不重不漏"指的是分片间
    不会交叉写同一个 (uid%shard_count) 区段,重启不会重灌已 commit 的 offset 段。

批量优化:
  - executemany 改为多值 INSERT(一条 INSERT ... VALUES (...),(...),... 拼 multi_batch 行/批)。
  - 单分片进程内开 --workers(默认 2)个 worker 线程,共享分片 offset(用 threading.Lock +
    原子推进)。每个 worker 独立 TiDB 连接。进度(已 commit 行数)由主线程统一更新 Redis。

断点续跑:
  - 启动读 shard Redis key = 本分片已 commit 行数 done。
  - offset 从 done 起步,跳过已 commit 行。
  - 每 ~100k 行(或每 commit-batch 批)commit 一次,commit 前 set Redis key。
    保证 key 永远 <= TiDB 实际已落盘行数(崩溃最多丢一个未 commit 批)。

可观测:
  - stdout 每报告间隔打印:shard={id} done={rows} rate={rows/s} eta={min}
  - 重定向到 logs/seed_learning_shard_{id}.log

用法(集群内执行):
  # 4 分片小批量验证
  for i in 0 1 2 3; do
    python3 seed_learning_events.py --target 100000 --shard-id $i --shard-count 4 \
      --multi-batch 500 --workers 2 > logs/seed_learning_shard_$i.log 2>&1 &
  done; wait

  # 全量(后续步骤,验证通过后再启):
  for i in $(seq 0 15); do
    python3 seed_learning_events.py --target 3000000000 --shard-id $i --shard-count 16 \
      --multi-batch 500 --workers 2 > logs/seed_learning_shard_$i.log 2>&1 &
  done; wait
"""
from __future__ import annotations
import argparse
import json
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

EVENT_TYPES = ("answer", "review", "hint", "mastery_update")
USER_COUNT = 5050  # 现有 users 数(user_id 1..5050)

# 构造一次多值 INSERT 的占位符段:"(%s,%s,%s,%s),(%s,%s,%s,%s),..."
VALUES_ONE = "(%s,%s,%s,FROM_UNIXTIME(%s))"  # created_at 用 FROM_UNIXTIME


def redis_key_for(shard_id: int) -> str:
    return f"esp:ctx:seed:learning_events:shard:{shard_id}"


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
    """本分片负责的行数(余数分给前 remainder 个分片)。"""
    base = target // shard_count
    rem = target % shard_count
    return base + (1 if shard_id < rem else 0)


def make_rows(shard_id: int, shard_count: int, offset: int, n: int) -> tuple[list, list]:
    """生成本分片第 offset..offset+n-1 行(局部编号)。

    返回 (values_flat, ...) —— values_flat 是平铺的参数列表,
    每行 4 个元素 [user_id, event_type, payload, created_at_unix]。
    user_id 满足 uid % shard_count == shard_id,确保分片互斥。
    """
    # 该分片可用的 user_id 集合:uid = shard_id+1 + k*shard_count, k=0..K-1
    # K = floor((USER_COUNT - (shard_id+1)) / shard_count) + 1,但简化用循环:
    # 实际可用 user 数 = USER_COUNT // shard_count(取整,可能略少于 5050/shard_count,
    # 落在 1..USER_COUNT 内且 uid%shard_count==shard_id)
    avail = []
    uid = shard_id + 1  # 1..USER_COUNT, uid%shard_count==shard_id
    while uid <= USER_COUNT:
        avail.append(uid)
        uid += shard_count
    K = len(avail) if avail else 1

    now = int(time.time())
    flat = []
    for i in range(n):
        local = offset + i
        u = avail[local % K]
        et = EVENT_TYPES[local % len(EVENT_TYPES)]
        payload = json.dumps({"kp": local % 100 + 1, "ok": (local % 3) == 0})
        ca = now - random.randint(0, 30 * 86400)
        flat.extend((u, et, payload, ca))
    return flat


def build_multi_sql(multi_batch: int) -> str:
    placeholders = ",".join([VALUES_ONE] * multi_batch)
    return (
        "INSERT INTO learning_events (user_id, event_type, payload, created_at) "
        f"VALUES {placeholders}"
    )


def worker(
    shard_id: int,
    shard_count: int,
    shard_total: int,
    multi_batch: int,
    commit_batch_count: int,  # 每 N 个 multi_batch commit 一次
    state: dict,
    state_lock: threading.Lock,
    commit_lock: threading.Lock,
    stop_evt: threading.Event,
):
    """单个 worker 线程:循环抢 offset 段 -> 生成 -> 多值 INSERT -> 累加到 commit 队列。"""
    sql = build_multi_sql(multi_batch)
    conn = pymysql.connect(
        host=TIDB_HOST, port=TIDB_PORT, user=TIDB_USER, database=TIDB_DB,
        autocommit=False, charset="utf8mb4",
    )
    cur = conn.cursor()
    try:
        while not stop_evt.is_set():
            # 抢一段 offset
            with state_lock:
                if state["offset"] >= shard_total:
                    break
                take = min(multi_batch, shard_total - state["offset"])
                seg_start = state["offset"]
                state["offset"] = seg_start + take
                state["pending_rows"] += take
            flat = make_rows(shard_id, shard_count, seg_start, take)
            cur.execute(sql, flat)
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
                with commit_lock:
                    conn.commit()
                # commit 后更新 Redis 进度(主线程负责,这里只记到 state)
                with state_lock:
                    state["need_redis"] = state["committed"]
    finally:
        try:
            conn.commit()
        except Exception:
            pass
        cur.close()
        conn.close()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--target", type=int, default=3_000_000_000)
    ap.add_argument("--shard-id", type=int, required=True)
    ap.add_argument("--shard-count", type=int, required=True)
    ap.add_argument("--multi-batch", type=int, default=500,
                    help="单条多值 INSERT 的行数")
    ap.add_argument("--commit-every", type=int, default=2,
                    help="每 N 个 multi_batch commit 一次(每 commit 行数=N*multi-batch)")
    ap.add_argument("--workers", type=int, default=2,
                    help="分片内 worker 线程数")
    ap.add_argument("--report-interval", type=float, default=10.0)
    args = ap.parse_args()

    if not (0 <= args.shard_id < args.shard_count):
        print(f"[err] shard-id {args.shard_id} 不在 [0,{args.shard_count})", file=sys.stderr)
        return 2

    shard_total = shard_target(args.target, args.shard_id, args.shard_count)
    rkey = redis_key_for(args.shard_id)

    # Redis
    try:
        import redis
        r = redis.Redis(host=REDIS_HOST, port=REDIS_PORT, decode_responses=True)
        r.ping()
    except Exception as e:
        print(f"[warn] redis unavailable: {e}", file=sys.stderr)
        r = None

    done = redis_get(r, rkey) if r else 0
    print(f"[seed-le] shard={args.shard_id}/{args.shard_count} "
          f"shard_total={shard_total} resume_from={done} "
          f"multi_batch={args.multi_batch} workers={args.workers}")
    sys.stdout.flush()

    if done >= shard_total:
        print(f"[seed-le] shard={args.shard_id} already done {done}/{shard_total}")
        return 0

    state = {
        "offset": done,        # 下一个待写行(本分片局部编号)
        "committed": done,     # 已 commit 行数(用于 Redis/报告)
        "pending_rows": 0,
        "pending_batches": 0,
        "need_redis": done,
    }
    state_lock = threading.Lock()
    commit_lock = threading.Lock()
    stop_evt = threading.Event()

    # 启动 workers
    threads = []
    for _ in range(args.workers):
        t = threading.Thread(
            target=worker,
            args=(args.shard_id, args.shard_count, shard_total,
                  args.multi_batch, args.commit_every, state, state_lock,
                  commit_lock, stop_evt),
            daemon=True,
        )
        t.start()
        threads.append(t)

    # 主线程:监控 + Redis 进度更新
    t0 = time.time()
    last_report = t0
    resume_from = done
    try:
        while True:
            alive = any(t.is_alive() for t in threads)
            with state_lock:
                committed = state["committed"]
                need_redis = state["need_redis"]
            # 更新 Redis 进度(只增不减)
            if r and need_redis > done:
                redis_set(r, rkey, need_redis)
                done = need_redis
            now = time.time()
            if now - last_report >= args.report_interval:
                elapsed = now - t0
                committed_this_run = committed - resume_from
                # rate = 本 run 已 commit 增量 / 已用时间
                rate = (committed_this_run / elapsed) if elapsed > 0 else 0
                remaining = shard_total - committed
                eta_min = (remaining / rate / 60) if rate > 0 else 0
                print(f"[seed-le] shard={args.shard_id} done={committed}/{shard_total} "
                      f"rate={rate:.0f} rows/s eta={eta_min:.1f}min")
                sys.stdout.flush()
                last_report = now
            if not alive:
                break
            time.sleep(1)
    except KeyboardInterrupt:
        print(f"[seed-le] shard={args.shard_id} interrupted, signaling stop")
        stop_evt.set()
        for t in threads:
            t.join(timeout=30)
        # 落最后进度
        with state_lock:
            final = state["committed"]
        if r:
            redis_set(r, rkey, final)
        return 130

    # 等所有 worker 退出
    for t in threads:
        t.join(timeout=60)

    # 收尾:把残余 pending commit 落盘 + 更新 Redis
    with state_lock:
        final = state["committed"]
    if r:
        redis_set(r, rkey, final)
    elapsed = time.time() - t0
    print(f"[seed-le] shard={args.shard_id} DONE committed={final}/{shard_total} "
          f"elapsed={elapsed:.0f}s rate={final/elapsed:.0f}rows/s")
    return 0


if __name__ == "__main__":
    sys.exit(main())
