#!/usr/bin/env python3
"""ESP Redis Cluster warm-up: 热点数据 TiDB -> Redis.

- scenarios(~1万) 全量: esp:cache:scenario:{id}
- knowledge_points(~3万) 全量: esp:cache:kp:{id}
- sessions 近期 top 1000: esp:cache:session:{id}
- user_mastery top 1000 高活跃用户: esp:cache:mastery:{user_id}
- users top 1000 高活跃: esp:cache:user:{id}

TTL 1h. 用 redis-py Cluster 客户端, pipeline 批量写.
"""
import datetime
import json
import sys
import time
from decimal import Decimal

import pymysql
from redis.cluster import RedisCluster, ClusterNode

TIDB_HOST = "tidb-primary"
TIDB_PORT = 4000
TIDB_USER = "root"
TIDB_DB = "esp"

REDIS_STARTUP = "redis-cluster-new-0.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local:6379"
TTL = 3600
BATCH = 500

STARTUP_NODE = ClusterNode(REDIS_STARTUP.split(":")[0], int(REDIS_STARTUP.split(":")[1]))


def db_conn():
    return pymysql.connect(
        host=TIDB_HOST, port=TIDB_PORT, user=TIDB_USER, database=TIDB_DB,
        read_timeout=60, write_timeout=60, connect_timeout=20,
        cursorclass=pymysql.cursors.SSDictCursor,
    )


def decimal_default(o):
    if isinstance(o, Decimal):
        return float(o)
    if isinstance(o, (datetime.datetime, datetime.date)):
        return o.isoformat()
    raise TypeError


def stream_to_redis(sql, key_fn, row_to_dict, label):
    """流式查询 TiDB, 批量 pipeline 写 Redis. 返回 (rows, set_ok).
    每批独立提交; 出错时该表中断, 返回已写入数.
    """
    rc = RedisCluster(startup_nodes=[STARTUP_NODE], decode_responses=True,
                      socket_timeout=10, socket_connect_timeout=10, retry_on_timeout=True)
    conn = db_conn()
    cur = conn.cursor()
    try:
        cur.execute(sql)
    except Exception as e:
        print(f"[{label}] query error: {e}", file=sys.stderr, flush=True)
        cur.close(); conn.close()
        return 0, 0
    rows = 0
    set_ok = 0
    pipe = rc.pipeline()
    batch_count = 0
    t0 = time.time()
    while True:
        try:
            row = cur.fetchone()
        except Exception as e:
            print(f"[{label}] fetch error at row {rows}: {e}", file=sys.stderr, flush=True)
            break
        if row is None:
            break
        rows += 1
        try:
            k = key_fn(row)
            v = json.dumps(row_to_dict(row), default=decimal_default, ensure_ascii=False)
            pipe.set(k, v, ex=TTL)
            batch_count += 1
            set_ok += 1
            if batch_count >= BATCH:
                try:
                    pipe.execute()
                except Exception as e:
                    print(f"[{label}] flush err at row {rows}: {e}", file=sys.stderr, flush=True)
                pipe.reset()
                batch_count = 0
        except Exception as e:
            print(f"[{label}] row {rows} error: {type(e).__name__}: {e}", file=sys.stderr, flush=True)
    if batch_count > 0:
        try:
            pipe.execute()
        except Exception as e:
            print(f"[{label}] final flush error: {e}", file=sys.stderr, flush=True)
    cur.close(); conn.close()
    dt = time.time() - t0
    print(f"[{label}] rows={rows} set_ok={set_ok} elapsed={dt:.1f}s", flush=True)
    return rows, set_ok


def main():
    total_rows = 0
    total_set = 0
    t0 = time.time()

    # 1. scenarios 全量
    r, s = stream_to_redis(
        "SELECT id, code, title, system_prompt, difficulty, lang FROM scenarios",
        lambda row: f"esp:cache:scenario:{row['id']}",
        lambda row: row,
        "scenarios",
    )
    total_rows += r; total_set += s

    # 2. knowledge_points 全量
    r, s = stream_to_redis(
        "SELECT id, code, title, parent_id, difficulty FROM knowledge_points",
        lambda row: f"esp:cache:kp:{row['id']}",
        lambda row: row,
        "knowledge_points",
    )
    total_rows += r; total_set += s

    # 3. sessions top 1000 (按 PK 顺序扫, 避免 ORDER BY 排序大表超时)
    r, s = stream_to_redis(
        "SELECT id, user_id, scenario_id, status, started_at, ended_at "
        "FROM sessions LIMIT 1000",
        lambda row: f"esp:cache:session:{row['id']}",
        lambda row: row,
        "sessions",
    )
    total_rows += r; total_set += s

    # 4. user_mastery top 1000 (顺序扫前 1000 行, 避免聚合大表超时)
    r, s = stream_to_redis(
        "SELECT user_id, kp_id, mastery_score, last_review_at, review_count, "
        "next_review_at, version, updated_at FROM user_mastery LIMIT 1000",
        lambda row: f"esp:cache:mastery:{row['user_id']}:{row['kp_id']}",
        lambda row: row,
        "user_mastery",
    )
    total_rows += r; total_set += s

    # 5. users top 1000 (顺序扫前 1000 行)
    r, s = stream_to_redis(
        "SELECT id, role, name, email, created_at FROM users LIMIT 1000",
        lambda row: f"esp:cache:user:{row['id']}",
        lambda row: row,
        "users",
    )
    total_rows += r; total_set += s

    dt = time.time() - t0
    print(f"=== WARMUP DONE total_rows={total_rows} total_set={total_set} elapsed={dt:.1f}s ===", flush=True)


if __name__ == "__main__":
    main()
