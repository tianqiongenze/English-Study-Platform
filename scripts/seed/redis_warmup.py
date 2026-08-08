#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""ESP Redis Cluster 预热脚本(对齐 esp-cache 实际 key 命名)。

背景:docs/redis-warmup-plan.md 设计了 W1~W4 预热方案,但 plan 里的 key 前缀
(`esp:cache:scenario:*` / `esp:cache:user:*` / `esp:cache:mastery:*`)是旧结论,
与当前 esp-cache 代码实际使用不符。本脚本以代码为准:

key 命名对齐结论(plan vs 代码实际):
| plan 设计                  | 代码实际                              | 差异说明                                            |
|---------------------------|---------------------------------------|----------------------------------------------------|
| esp:cache:scenario:{code} | (无 Redis)                            | gateway-go scenario 走进程内 map(repository.go),  |
|                           |                                       | 不经 Redis。W1 取消预热(预热了用不上)。            |
| esp:cache:user:{id}       | user:{id}                             | bff AuthController put(CACHE_USER,"user:"+id,...)。|
|                           |                                       | 无 esp:cache: 前缀。value 是 Jackson default-typing|
|                           |                                       | JSON(Map): ["java.util.LinkedHashMap",{...}]。     |
| esp:cache:mastery:{uid}   | learning:due:{uid} / learning:weak:{uid} | learning LearningService dueReview/weakestPoints。|
|                           |                                       | value 是 List<UserMastery> 的 Jackson default-typ |
|                           |                                       | ing 序列化,Python 复刻类型签名脆弱;CacheService   |
|                           |                                       | 读 L2 失败会 try-catch 降级回 DB(不报错),故预热    |
|                           |                                       | 失败不影响正确性,仅失去缓存命中。W3 为尽力而为。    |
| esp:cache:turns:recent:*  | (无业务读取)                          | 无服务读该 key,W4 跳过(压测新会话由 session:* 自生)。|

实际灌装阶段(对齐代码):
  W2  user:{id}        STRING  bff 用户画像(必灌,压测 BFF 鉴权热路径)
  W3a learning:due:{uid} STRING learning 今日复习清单(尽力而为,反序列化失败降级 DB)
  W3b learning:weak:{uid} STRING learning 薄弱知识点(尽力而为)
  W5  esp:ctx:warmup:done / esp:ctx:warmup:progress  运行态

value 序列化:esp-cache RedisTemplate 用 GenericJackson2JsonRedisSerializer +
  activateDefaultTyping(NON_FINAL),value 形如:
    ["java.util.LinkedHashMap",{"id":1,"email":"...","name":"...","role":"..."}]
  UserMastery 列表形如:
    ["java.util.ArrayList",[["com.esp.learning.domain.UserMastery",{...}],...]]
  本脚本手写此格式(bff user 可靠;learning 列表字段需与 UserMastery POJO 对齐,
  若 POJO 改字段需同步改本脚本,否则反序列化失败→降级 DB,失去缓存效果但不报错)。

幂等:SET 覆盖,重复跑安全(无 DEL 前缀扫描,避免误删运行态 session:*)。
进度:写 Redis key esp:ctx:warmup:progress(JSON: stage/done/total/ts)。

运行方式(集群内,需 pymysql + redis 库):
  # 1. 找一个装了 pymysql+redis 的 pod(writing-py 已装),把脚本 cp 进去
  kubectl -n esp-prod cp scripts/seed/redis_warmup.py \
    deploy/writing-py:/tmp/redis_warmup.py
  # 2. 执行(默认全跑 W2,W3a,W3b)
  kubectl -n esp-prod exec deploy/writing-py -- python3 /tmp/redis_warmup.py
  # 冒烟(限量 50 用户)
  kubectl -n esp-prod exec deploy/writing-py -- python3 /tmp/redis_warmup.py --limit 50
  # 只跑 W2
  kubectl -n esp-prod exec deploy/writing-py -- python3 /tmp/redis_warmup.py --stage W2

前置依赖:users / user_mastery 种子已灌入 TiDB(本脚本只读不写 TiDB)。
本脚本不创建/修改 TiDB 数据。
"""
import argparse
import json
import sys
import time
import traceback
from datetime import datetime, timezone

import pymysql
import redis

# ---------- 连接常量(与 seed_basic_data.py 同风格) ----------
TIDB_HOST = "tidb-follower"   # 读副本,预热只读
TIDB_PORT = 4000
TIDB_USER = "root"
TIDB_DB = "esp"

REDIS_HOST = "redis-cluster"  # ExternalName 短名
REDIS_PORT = 6379

PROGRESS_KEY = "esp:ctx:warmup:progress"
DONE_KEY = "esp:ctx:warmup:done"

# esp-cache value 序列化:Jackson default-typing(NON_FINAL)
# Map → ["java.util.LinkedHashMap", {...}]
MAP_TYPE = "java.util.LinkedHashMap"
# ArrayList → ["java.util.ArrayList", [...]]
LIST_TYPE = "java.util.ArrayList"
# UserMastery POJO 全限定名(与 services/learning-java domain 包对齐)
USER_MASTERY_TYPE = "com.esp.learning.domain.UserMastery"

# esp-cache L2 TTL(秒);与 application.yml esp.cache.l2-ttl-seconds 默认 300 对齐
TTL_USER = 300        # bff login put 传 300
TTL_LEARNING = 300    # learning dueReview/weakestPoints 传 0→用默认 l2TtlSeconds=300


def now_ts() -> str:
    return datetime.now(timezone.utc).isoformat()


def write_progress(r: redis.Redis, stage: str, done: int, total: int):
    r.set(PROGRESS_KEY, json.dumps({
        "stage": stage, "done": done, "total": total, "ts": now_ts()
    }))


def jackson_map(d: dict) -> str:
    """构造 Jackson default-typing Map 序列化串。"""
    return json.dumps([MAP_TYPE, d], ensure_ascii=False, separators=(",", ":"))


def jackson_list(items: list) -> str:
    """构造 Jackson default-typing ArrayList 序列化串。"""
    return json.dumps([LIST_TYPE, items], ensure_ascii=False, separators=(",", ":"))


def jackson_pojo(pojo_type: str, d: dict) -> str:
    """构造 Jackson default-typing POJO 序列化串。"""
    return json.dumps([pojo_type, d], ensure_ascii=False, separators=(",", ":"))


W2_BATCH_SELECT = 5000   # W2 每次从 TiDB 拉取的行数(流式分块,避免 OOM)
W2_BATCH_PIPE = 500      # pipeline 每 500 SET flush 一次
# W3 mastery 查询批次:窗口函数 ROW_NUMBER() OVER(PARTITION BY) 在大 IN 列表上
# 会吃 TiDB server 内存,实测 5000 用户触发 tidb_server_memory_limit OOM。
# 缩到 200 用户/查询,单查询内存可控;keyset 分页仍保证流式。
W3_BATCH_SELECT = 200


def stage_w2_user(r: redis.Redis, cur, limit: int | None) -> int:
    """W2: 灌 user:{id} STRING(bff 用户画像)。必灌,可靠。

    流式分块:LIMIT 5000 OFFSET k 逐批拉取,每批内 pipeline 500 SET 后清空,
    不累积全表 rows 在内存。配合 use cursor 时 server-side 流式更好,
    但 pymysql 默认 cursor 会一次性 fetch,故用 LIMIT/OFFSET 显式分页。
    TiDB LIMIT/OFFSET 大 offset 有性能损失,但 10 万级可接受;
    若需更大规模可改 keyset pagination(WHERE id > last_id ORDER BY id)。
    """
    total_target = limit if limit else 1_000_000
    total = 0
    offset = 0
    pipe = r.pipeline(transaction=False)
    pending = 0
    while offset < total_target:
        chunk = min(W2_BATCH_SELECT, total_target - offset)
        sql = (
            "SELECT id, role, name, email FROM users "
            f"ORDER BY id LIMIT {chunk} OFFSET {offset}"
        )
        cur.execute(sql)
        rows = cur.fetchall()
        if not rows:
            break
        for (uid, role, name, email) in rows:
            val = jackson_map({
                "id": uid,
                "email": email,
                "name": name,
                "role": role,
            })
            pipe.set(f"user:{uid}", val, ex=TTL_USER)
            pending += 1
            if pending % W2_BATCH_PIPE == 0:
                pipe.execute()
                total += W2_BATCH_PIPE
                write_progress(r, "W2", total, total_target)
                print(f"[W2] user:{uid} {total}/{total_target}")
        offset += len(rows)
        # 释放本批引用,防止累积
        del rows
    if pending % W2_BATCH_PIPE != 0:
        pipe.execute()
        total += pending % W2_BATCH_PIPE
    write_progress(r, "W2", total, total_target)
    print(f"[W2] DONE users={total}")
    return total


def _user_mastery_row(row) -> str:
    """单行 user_mastery → Jackson POJO 序列化。

    row: (user_id, kp_id, mastery_score, review_count, last_review_at, next_review_at)
    UserMastery POJO 字段需与 domain/UserMastery.java 对齐;若反序列化失败,
    CacheService 会 try-catch 降级回 DB,不影响正确性。
    """
    user_id, kp_id, mastery_score, review_count, last_review_at, next_review_at = row
    # BigDecimal/Instant 在 Jackson default-typing 下:
    # BigDecimal → ["java.math.BigDecimal","0.65"]
    # Instant → ["java.time.Instant","2026-...Z"]
    d = {
        "id": jackson_map({"userId": user_id, "kpId": kp_id}),  # UserMasteryId 嵌入
        "userId": user_id,
        "kpId": kp_id,
        "masteryScore": ["java.math.BigDecimal", str(mastery_score)] if mastery_score is not None else None,
        "reviewCount": review_count if review_count is not None else 0,
        "lastReviewAt": ["java.time.Instant", last_review_at.isoformat()] if last_review_at else None,
        "nextReviewAt": ["java.time.Instant", next_review_at.isoformat()] if next_review_at else None,
    }
    return jackson_pojo(USER_MASTERY_TYPE, d)


def _stream_user_mastery_by_user(cur, limit_users: int | None,
                                 order_sql: str, extra_where: str,
                                 rows_per_user_cap: int | None):
    """流式按 user_id 聚合 user_mastery 行,避免一次性 fetch 全表。

    策略:先 keyset 分页取 user_id 列表(每批 W2_BATCH_SELECT 个),
    对每批 user_id 用 WHERE user_id IN (...) 查 mastery 行,在内存中只保留
    本批用户的分组结果,写 Redis 后清空。limit_users 控制总用户数。
    order_sql: 用于 mastery 行的排序(如 'ORDER BY user_id')。
    extra_where: 附加 WHERE 条件(如 "AND next_review_at IS NOT NULL AND next_review_at <= NOW()")。
    rows_per_user_cap: 每用户最多取多少行(None=不限制,交给 SQL LIMIT 实现 top-N)。
    """
    total_target = limit_users if limit_users else 1_000_000
    total_users = 0
    last_uid = 0
    while total_users < total_target:
        chunk = min(W3_BATCH_SELECT, total_target - total_users)
        cur.execute(
            "SELECT id FROM users WHERE role='learner' AND id > %s "
            f"ORDER BY id LIMIT {chunk}", (last_uid,))
        uids = [r[0] for r in cur.fetchall()]
        if not uids:
            break
        last_uid = uids[-1]
        # 查这批用户的 mastery 行
        if rows_per_user_cap:
            sql = (
                "SELECT user_id, kp_id, mastery_score, review_count, last_review_at, next_review_at FROM ("
                f"  SELECT *, ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY mastery_score ASC) AS rn "
                f"  FROM user_mastery WHERE user_id IN ({','.join(['%s']*len(uids))}) "
                f"  AND mastery_score IS NOT NULL{extra_where}"
                f") t WHERE rn <= {int(rows_per_user_cap)} ORDER BY user_id"
            )
        else:
            sql = (
                "SELECT user_id, kp_id, mastery_score, review_count, last_review_at, next_review_at "
                f"FROM user_mastery WHERE user_id IN ({','.join(['%s']*len(uids))})"
                f"{extra_where} ORDER BY user_id"
            )
        cur.execute(sql, uids)
        rows = cur.fetchall()
        by_user: dict[int, list] = {}
        for row in rows:
            uid = row[0]
            by_user.setdefault(uid, []).append(_user_mastery_row(row))
        del rows
        yield by_user
        total_users += len(uids)


def stage_w3a_due(r: redis.Redis, cur, limit: int | None) -> int:
    """W3a: 灌 learning:due:{uid} STRING(今日复习清单)。

    尽力而为:Python 复刻 List<UserMastery> Jackson default-typing 脆弱,
    若 POJO 字段不匹配,服务端反序列化失败会降级回 DB(CacheService 有 try-catch)。
    流式:按 user_id 分批 IN 查询,每批 5000 用户,内存只保留本批分组。
    """
    total_target = limit if limit else 1_000_000
    total = 0
    pipe = r.pipeline(transaction=False)
    pending = 0
    extra = " AND next_review_at IS NOT NULL AND next_review_at <= NOW()"
    gen = _stream_user_mastery_by_user(cur, limit, "", extra, None)
    for by_user in gen:
        for uid, items in by_user.items():
            if not items:
                continue
            val = jackson_list(items)
            pipe.set(f"learning:due:{uid}", val, ex=TTL_LEARNING)
            pending += 1
            if pending % W2_BATCH_PIPE == 0:
                pipe.execute()
                total += W2_BATCH_PIPE
                write_progress(r, "W3a", total, total_target)
                print(f"[W3a] learning:due:{uid} {total}/{total_target}")
        del by_user
    if pending % W2_BATCH_PIPE != 0:
        pipe.execute()
        total += pending % W2_BATCH_PIPE
    write_progress(r, "W3a", total, total_target)
    print(f"[W3a] DONE due-users={total}")
    return total


def stage_w3b_weak(r: redis.Redis, cur, limit: int | None) -> int:
    """W3b: 灌 learning:weak:{uid} STRING(薄弱知识点,取 mastery 最低 top10)。"""
    total_target = limit if limit else 1_000_000
    total = 0
    pipe = r.pipeline(transaction=False)
    pending = 0
    gen = _stream_user_mastery_by_user(cur, limit, "", "", 10)
    for by_user in gen:
        for uid, items in by_user.items():
            if not items:
                continue
            val = jackson_list(items)
            pipe.set(f"learning:weak:{uid}", val, ex=TTL_LEARNING)
            pending += 1
            if pending % W2_BATCH_PIPE == 0:
                pipe.execute()
                total += W2_BATCH_PIPE
                write_progress(r, "W3b", total, total_target)
                print(f"[W3b] learning:weak:{uid} {total}/{total_target}")
        del by_user
    if pending % W2_BATCH_PIPE != 0:
        pipe.execute()
        total += pending % W2_BATCH_PIPE
    write_progress(r, "W3b", total, total_target)
    print(f"[W3b] DONE weak-users={total}")
    return total


def main():
    ap = argparse.ArgumentParser(description="ESP Redis Cluster warm-up")
    ap.add_argument("--stage", default="W2,W3a,W3b",
                    help="逗号分隔阶段,默认 W2,W3a,W3b")
    ap.add_argument("--limit", type=int, default=100000,
                    help="限量用户数(默认 10 万,避免全量灌装耗时+OOM)")
    ap.add_argument("--flush", action="store_true",
                    help="灌前删除已知 warmup key 前缀(user:*,learning:due:*,learning:weak:*)。"
                         "默认不删(SET 覆盖即幂等),避免误删运行态。")
    args = ap.parse_args()

    # 保留大小写(W2/W3a/W3b),仅 strip;upper 会把 W3a -> W3A 导致校验失败
    stages = [s.strip() for s in args.stage.split(",") if s.strip()]
    valid = {"W2", "W3a", "W3b"}
    bad = [s for s in stages if s not in valid]
    if bad:
        print(f"ERROR: unknown stage(s) {bad}, valid: {sorted(valid)}", file=sys.stderr)
        sys.exit(2)

    limit = args.limit

    # 连 Redis:当前为真正的 Redis Cluster(redis-cluster-new,6 节点)。
    # 用 RedisCluster 客户端自动处理 MOVED/ASK;pipeline 需用 cluster pipeline
    # (per-node 分片,非事务,与原 transaction=False 语义一致)。
    from redis.cluster import RedisCluster
    try:
        r = RedisCluster(host=REDIS_HOST, port=REDIS_PORT, decode_responses=True,
                         socket_timeout=10, socket_connect_timeout=10)
        # RedisCluster 无 ping()?用 cluster_keyslot 兜底验证连通
        r.cluster_keyslot("esp:ctx:warmtest")
    except Exception as e:
        print(f"ERROR: cannot connect redis cluster {REDIS_HOST}:{REDIS_PORT}: {e}", file=sys.stderr)
        sys.exit(1)
    print(f"[init] redis cluster connected {REDIS_HOST}:{REDIS_PORT}")

    # 连 TiDB 读副本
    try:
        conn = pymysql.connect(host=TIDB_HOST, port=TIDB_PORT, user=TIDB_USER,
                               database=TIDB_DB, charset="utf8mb4",
                               connect_timeout=10, read_timeout=60)
    except Exception as e:
        print(f"ERROR: cannot connect tidb {TIDB_HOST}:{TIDB_PORT}: {e}", file=sys.stderr)
        sys.exit(1)
    cur = conn.cursor()
    print(f"[init] tidb connected {TIDB_HOST}:{TIDB_PORT}")

    # 可选 flush
    if args.flush:
        for pat in ("user:*", "learning:due:*", "learning:weak:*"):
            n = 0
            for k in r.scan_iter(match=pat, count=1000):
                r.delete(k)
                n += 1
            print(f"[flush] deleted {n} keys matching {pat}")

    write_progress(r, "START", 0, len(stages))
    counts = {}
    try:
        if "W2" in stages:
            counts["W2"] = stage_w2_user(r, cur, limit)
        if "W3a" in stages:
            counts["W3a"] = stage_w3a_due(r, cur, limit)
        if "W3b" in stages:
            counts["W3b"] = stage_w3b_weak(r, cur, limit)
        r.set(DONE_KEY, json.dumps({"ts": now_ts(), "counts": counts}))
        write_progress(r, "DONE", len(stages), len(stages))
        print(f"[DONE] counts={counts}")
    except Exception as e:
        write_progress(r, "ERROR", 0, len(stages))
        print(f"[ERROR] {e}", file=sys.stderr)
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)
    finally:
        cur.close()
        conn.close()
        r.close()


if __name__ == "__main__":
    main()
