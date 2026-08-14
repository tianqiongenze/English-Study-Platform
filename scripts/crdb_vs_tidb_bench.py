#!/usr/bin/env python3
"""
CRDB 应用层连接池压测(消除 kube-exec per-query 开销,对齐 #107 plan §4 双跑方法论)。
通过 writing-py pod 内 cockroach 自带 `cockroach sql` 客户端循环批量写入/查询,
比 kube-exec per-query 快一个数量级(单次 exec 仅进程启动,连接复用 cockroach sql 会话)。

跑两组:
  A) CRDB(esp-crdb ns, crdb-public:26257)
  B) TiDB(ekb-m2-aws-us-east-1 ns, tidb-primary:4000 / esp 库,既有种子)

输出 report/crdb-vs-tidb-app-bench.json 供对比。
"""
import os, subprocess, time, json, random, string, sys

def crdb_sql_via_pod(sql: str, db: str = "esp") -> str:
    """经 writing-py pod 内 cockroach sql 客户端执行(避免 kube-exec 开销)。
    writing-py 无 cockroach 二进制,改回 kubectl exec crdb-0(但批量单次会话)。
    """
    cmd = ["kubectl", "-n", "esp-crdb", "exec", "crdb-0", "--",
           "/cockroach/cockroach", "sql", "--insecure", "--host", "crdb-public:26257",
           "-d", db, "-e", sql]
    p = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    return p.stdout + p.stderr

def tidb_sql_via_pod(sql: str, db: str = "esp") -> str:
    """经 TiDB(pymysql pod / db_query.sh)执行。这里用 kubectl exec 现有 toolbox。
    简化:直接用 kubectl exec 一个有 mysql 客户端的 pod。ESP 无 mysql client pod,
    退而用 writing-py 的 pymysql。为对等(都走单次 exec),用 python -c pymysql。"""
    py = f"""
import pymysql
c=pymysql.connect(host='tidb-primary',port=4000,user='root',database='{db}',autocommit=True)
cur=c.cursor()
cur.execute('''{sql.replace("'", "''")}''')
try:
    rows=cur.fetchall()
    for r in rows[:5]: print(r)
except: pass
c.close()
"""
    cmd = ["kubectl", "-n", "esp-prod", "exec", "deploy/writing-py", "--", "python3", "-c", py]
    p = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    return p.stdout + p.stderr

def rand_str(n=8):
    return "".join(random.choices(string.ascii_lowercase, k=n))

def bench_write(label, sql_fn, n=4000, batch=200):
    """批量写入 learning_events,测 INSERT/s。"""
    t0 = time.time()
    rounds = n // batch
    for _ in range(rounds):
        rows = []
        for _ in range(batch):
            uid = random.randint(1, 100)
            et = random.choice(['answer','review','streak','xp_gain'])
            pl = '{"k":"' + rand_str(4) + '"}'
            rows.append(f"({uid},'{et}','{pl}',NOW())")
        sql_fn(f"INSERT INTO learning_events(user_id,event_type,payload,created_at) VALUES {','.join(rows)};")
    dt = time.time() - t0
    return {"rows": n, "elapsed_s": round(dt, 2), "insert_s": round(n / dt, 1)}

def main():
    os.makedirs("report", exist_ok=True)
    print("=== CRDB vs TiDB app-layer write bench (single-VM, serial) ===")
    # 确保 CRDB esp.learning_events 有基线种子
    crdb_sql_via_pod("INSERT INTO users(role,name,email,pwd_hash) SELECT 'learner', 'b'||s, 'b'||s||'@e.test', 'x' FROM generate_series(1,100) s ON CONFLICT DO NOTHING;")
    crdb_sql_via_pod("INSERT INTO scenarios(code,title,system_prompt,difficulty,lang) VALUES ('bench','Bench','t',3,'en') ON CONFLICT DO NOTHING;")

    print("[CRDB] write bench 4000 rows...")
    crdb_w = bench_write("CRDB", crdb_sql_via_pod, n=4000, batch=200)
    print(json.dumps(crdb_w))

    print("[TiDB] write bench 4000 rows...")
    tidb_w = bench_write("TiDB", tidb_sql_via_pod, n=4000, batch=200)
    print(json.dumps(tidb_w))

    result = {
        "env": "single RD VM, 3 CRDB pods + TiDB(1 tidb+1 tikv) 共存,串行 bench",
        "crdb": {"write": crdb_w},
        "tidb": {"write": tidb_w},
        "ratio_crdb_over_tidb": round(crdb_w["insert_s"] / max(tidb_w["insert_s"], 0.01), 2),
        "caveat": "单节点 RD VM 资源共享,3 CRDB pod 与 TiDB 抢 CPU/磁盘;此为可行性+量级对比,非生产容量结论。真实多区域对比见 #107 plan §4(需 3master+6worker 集群)。",
    }
    with open("report/crdb-vs-tidb-app-bench.json", "w") as f:
        json.dump(result, f, indent=2, ensure_ascii=False)
    print(json.dumps(result, indent=2, ensure_ascii=False))
    print("\n[report] report/crdb-vs-tidb-app-bench.json")

if __name__ == "__main__":
    main()
