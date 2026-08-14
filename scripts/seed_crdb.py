#!/usr/bin/env python3
"""
CRDB seed + smoke loadtest(对齐 #107 CRDB vs TiDB 对比压测 plan)。
通过 pymysql pod 内连 CRDB(CRDB 兼容 PostgreSQL 协议,但集群内有 psycopg2/psql 客户端更稳;
此处用 cockroach sql --insecure 直连,避免依赖 psycopg2 pod)。

本脚本:1) 灌小规模种子(对比压测用基线数据,非 30 亿);2) 跑写入冒烟(INSERT 速率);
3) 跑读取冒烟(点查 + 范围查);4) 输出对比摘要。

用法:在能 kubectl 的主机跑。实际 SQL 经 `kubectl exec crdb-0 -- cockroach sql` 执行。
"""
import os, subprocess, time, json, random, string, sys

NS = "esp-crdb"
POD = "crdb-0"
HOST = "crdb-public:26257"
REPORT = "report/crdb-smoke-loadtest.json"

def crdb_sql(sql: str, db: str = "esp") -> str:
    """经 kubectl exec 执行 CRDB SQL,返回 stdout。"""
    cmd = ["kubectl", "-n", NS, "exec", POD, "--",
           "/cockroach/cockroach", "sql", "--insecure", "--host", HOST,
           "-d", db, "-e", sql]
    p = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    if p.returncode != 0:
        sys.stderr.write(f"[CRDB SQL ERR] {p.stderr[:300]}\n")
    return p.stdout

def rand_str(n=8):
    return "".join(random.choices(string.ascii_lowercase, k=n))

def seed():
    """灌种子:100 users / 3 scenarios / 500 sessions / 2000 session_turns / 2000 learning_events。"""
    print("[seed] inserting 100 users...")
    rows = []
    for i in range(100):
        rows.append(f"('u{rand_str(8)}','learner','u{rand_str(6)}@esp.test','{rand_str(32)}')")
    crdb_sql(f"INSERT INTO users(role,name,email,pwd_hash) VALUES {','.join(rows)};")

    print("[seed] inserting 3 scenarios...")
    crdb_sql("INSERT INTO scenarios(code,title,system_prompt,difficulty,lang) VALUES "
             "('airport','Airport Check-in','You are at airport check-in',3,'en'),"
             "('interview','Job Interview','You are in a job interview',4,'en'),"
             "('cafe','Cafe Ordering','You are ordering at a cafe',2,'en');")

    print("[seed] inserting 500 sessions...")
    rows = []
    for i in range(500):
        uid = random.randint(1, 100); sid = random.randint(1, 3)
        st = random.choice(['active','ended'])
        rows.append(f"({uid},{sid},'{st}',NOW(),NULL)")
    # 分批 100/批
    for b in range(0, len(rows), 100):
        crdb_sql(f"INSERT INTO sessions(user_id,scenario_id,status,started_at,ended_at) VALUES {','.join(rows[b:b+100])};")

    print("[seed] inserting 2000 session_turns...")
    for b in range(20):
        rows = []
        for _ in range(100):
            sess = random.randint(1, 500)
            role = random.choice(['user','ai','system'])
            rows.append(f"({sess},'{role}','turn content {rand_str(20)}',NULL,NOW())")
        crdb_sql(f"INSERT INTO session_turns(session_id,role,content,audio_ref,created_at) VALUES {','.join(rows)};")

    print("[seed] inserting 2000 learning_events...")
    for b in range(20):
        rows = []
        for _ in range(100):
            uid = random.randint(1, 100)
            et = random.choice(['answer','review','streak','xp_gain'])
            rows.append(f"({uid},'{et}','{{\"k\":\"{rand_str(6)}\"}}'::JSONB,NOW())")
        crdb_sql(f"INSERT INTO learning_events(user_id,event_type,payload,created_at) VALUES {','.join(rows)};")

def bench_write(n=2000):
    """写入吞吐:批量 INSERT learning_events,测 INSERT/s。"""
    t0 = time.time()
    for b in range(20):
        rows = []
        for _ in range(n // 20):
            uid = random.randint(1, 100)
            et = random.choice(['answer','review','streak','xp_gain'])
            rows.append(f"({uid},'{et}','{{\"b\":\"{rand_str(4)}\"}}'::JSONB,NOW())")
        crdb_sql(f"INSERT INTO learning_events(user_id,event_type,payload,created_at) VALUES {','.join(rows)};")
    dt = time.time() - t0
    return {"rows": n, "elapsed_s": round(dt, 2), "insert_s": round(n / dt, 1)}

def bench_point_read(n=2000):
    """点查:按 PK 查 sessions,测 QPS。"""
    t0 = time.time()
    for _ in range(n):
        sid = random.randint(1, 500)
        crdb_sql(f"SELECT * FROM sessions WHERE id IN (SELECT id FROM sessions ORDER BY id LIMIT 1 OFFSET {sid % 500});")
    dt = time.time() - t0
    # 上面每次一个 kubectl exec 开销大,真实压测应走应用层连接池;此处仅冒烟
    return {"queries": n, "elapsed_s": round(dt, 2), "qps_via_kubeexec": round(n / dt, 1),
            "note": "kubectl exec per-query overhead dominates; real QPS via app connection pool is much higher"}

def bench_range_read():
    """范围查:扫描某 user 的 learning_events,测延迟。"""
    t0 = time.time()
    out = crdb_sql("SELECT count(*) FROM learning_events WHERE user_id=1 AND created_at > '2020-01-01';")
    dt = time.time() - t0
    cnt = [l for l in out.splitlines() if l.strip().isdigit()]
    return {"latency_ms": round(dt * 1000, 1), "count": cnt[0] if cnt else "?"}

def main():
    os.makedirs("report", exist_ok=True)
    print("=== CRDB smoke loadtest (vs TiDB baseline) ===")
    # 版本
    ver = crdb_sql("SELECT version();", db="defaultdb").strip()
    print(f"[version] {ver.splitlines()[-1] if ver else '?'}")

    seed()
    # 种子后计数
    counts = {}
    for t in ["users","scenarios","sessions","session_turns","learning_events"]:
        out = crdb_sql(f"SELECT count(*) FROM {t};")
        c = [l for l in out.splitlines() if l.strip().isdigit()]
        counts[t] = c[0] if c else "?"

    w = bench_write(2000)
    r1 = bench_point_read(100)  # 减小,因 kube exec 开销
    r2 = bench_range_read()
    # 写后总计数
    out = crdb_sql("SELECT count(*) FROM learning_events;")
    c = [l for l in out.splitlines() if l.strip().isdigit()]
    total_le = c[0] if c else "?"

    result = {
        "db": "CockroachDB v24.3.11 (3-node, single-VM RD, arm64)",
        "seed_counts": counts,
        "after_seed_learning_events": counts.get("learning_events"),
        "write_bench": w,
        "point_read_bench": r1,
        "range_read_bench": r2,
        "total_learning_events_after_write": total_le,
        "caveat": "单节点 RD VM 3 CRDB pod 共享同一 VM CPU/磁盘,kube exec per-query 开销显著;真实对比需经应用层连接池(见 #107 plan)。此为冒烟级可行性验证。",
    }
    with open(REPORT, "w") as f:
        json.dump(result, f, indent=2, ensure_ascii=False)
    print(json.dumps(result, indent=2, ensure_ascii=False))
    print(f"\n[report] {REPORT}")

if __name__ == "__main__":
    main()
