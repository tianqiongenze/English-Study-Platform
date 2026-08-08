#!/usr/bin/env python3
"""
ESP 30亿行种子数据灌装降级方案 — 单节点 Rancher Desktop TiDB
============================================================
基线: session_turns 已有 ~3005万行(用户上次灌的)。TiKV PVC 仅 10Gi(已用~3Gi),
节点内存 88%。30亿行会 100% OOM/磁盘满。

降级目标: 总量 ~3亿行(300M),包含现有 3000万 session_turns,追加 ~2.7亿。
比例分配(降级后):
  users              100万   (1M,  用户主数据)
  scenarios          1万     (1万场景,新增)
  sessions           5000万  (50M,  会话)
  session_turns      追加到1亿 (50M+现有30M=80M,但追加 50M)
  knowledge_points   1万
  user_mastery       500万   (5M,  user*kp 抽样)
  learning_events    1亿     (100M, 主事件流)
  writing_tasks      2000万  (20M)
  lesson_plans       100万
  classes            50万
  assignments        500万
  submissions        1000万  (10M)
  app_logs           追加不灌(已有日志系统)
合计追加约 ~2.9亿行(290M),加现有 3000万 ≈ 3.2亿。接近 3亿降级目标。

策略:
- pymysql executemany,每批 5000 行,多表并发(但单 pod 内串行,避免 TiDB 写放大)。
- AUTO_RANDOM 主键由 TiDB 生成,user_id 等外键引用已灌的 users/scenarios。
- 大文本字段(essay/content/feedback)用预生成模板池随机选择,控制单行 <2KB。
- 进度写 Redis: SET esp:ctx:seed-progress {json};本地写 report/seed-progress.txt。
- 断点续跑: 启动前查各表当前行数,按 delta 灌。

注意: 本脚本在集群内 python pod 跑(能直连 tidb-primary)。通过 kubectl run 注入。
"""
import os, sys, json, time, random, string, argparse
import pymysql
from datetime import datetime, timedelta

try:
    import redis as redis_lib
    HAVE_REDIS = True
except ImportError:
    HAVE_REDIS = False

# ---------- 配置 ----------
TIDB_HOST = os.environ.get("TIDB_HOST", "tidb-primary")
TIDB_PORT = int(os.environ.get("TIDB_PORT", "4000"))
REDIS_HOST = os.environ.get("REDIS_HOST", "redis-cluster")
REDIS_PORT = int(os.environ.get("REDIS_PORT", "6379"))

REPORT = "/tmp/seed-progress.txt"

# 降级后的目标追加行数(每表)
TARGETS = {
    "users":          1_000_000,   # 1M
    "scenarios":      10_000,      # 1万
    "sessions":       50_000_000,  # 50M
    "session_turns":  50_000_000,  # 追加 50M(现有 30M)
    "knowledge_points": 10_000,
    "user_mastery":   5_000_000,   # 5M
    "learning_events":100_000_000, # 100M
    "writing_tasks":  20_000_000,  # 20M
    "lesson_plans":   1_000_000,
    "classes":        500_000,
    "assignments":    5_000_000,
    "submissions":    10_000_000,  # 10M
}
# 灌装顺序(依赖关系): 先 users/scenarios/knowledge_points,再引用它们的表
ORDER = ["users","scenarios","knowledge_points","sessions","session_turns",
         "user_mastery","learning_events","writing_tasks","lesson_plans",
         "classes","assignments","submissions"]

BATCH = 5000  # 每批行数

# ---------- 数据生成器(轻量,避免内存爆炸) ----------
FIRST_NAMES = ["Alex","Sam","Jordan","Taylor","Morgan","Riley","Casey","Jamie","Avery","Quinn",
               "李","王","张","刘","陈","杨","赵","黄","周","吴"]
LAST_NAMES = ["Chen","Wang","Li","Zhang","Liu","Yang","Huang","Zhao","Sun","Xu",
              "伟","芳","娜","敏","静","强","磊","洋","艳","勇"]
SCENARIO_TITLES = ["Daily Conversation","Business Meeting","Travel English",
                   "Job Interview","Restaurant Ordering","Airport Check-in",
                   "Doctor Visit","Shopping","Weather Talk","Movie Review"]
KP_TITLES = ["Present Simple","Past Continuous","Conditionals","Articles","Modal Verbs",
             "Passive Voice","Reported Speech","Phrasal Verbs","Relative Clauses","Gerunds"]
EVENT_TYPES = ["login","lesson_start","lesson_complete","exercise_submit","review",
               "speaking_attempt","listening_practice","quiz_pass","quiz_fail","logout"]
SERVICES = ["gateway","bff","learning","writing","teacher","emotion"]
LEVELS = ["INFO","WARN","ERROR"]

def rand_str(n=8):
    return ''.join(random.choices(string.ascii_lowercase+string.digits, k=n))

def gen_essay():
    base = "English learning is a journey that requires dedication and consistent practice. " \
           "Through reading, writing, speaking and listening, learners gradually build fluency. " \
           "Regular exposure to authentic materials helps internalize grammar and vocabulary naturally."
    return base[:200] + " " + rand_str(40)

def gen_feedback_json():
    return json.dumps({"score": random.randint(60,100),
                       "grammar": random.randint(1,10),
                       "vocab": random.randint(1,10),
                       "coherence": random.randint(1,10)})

def gen_payload_json():
    return json.dumps({"dur_ms": random.randint(100,5000),
                       "score": random.randint(1,100),
                       "kp": random.randint(1,10000)})

# ---------- DB 连接 ----------
def conn():
    return pymysql.connect(host=TIDB_HOST, port=TIDB_PORT, user="root", database="esp",
                           read_timeout=120, write_timeout=120, connect_timeout=10, autocommit=True)

def current_count(cur, table):
    # 用 information_schema 估算避免大表 COUNT(*) 超时
    cur.execute("SELECT TABLE_ROWS FROM information_schema.TABLES WHERE TABLE_SCHEMA='esp' AND TABLE_NAME=%s",(table,))
    r=cur.fetchone()
    return int(r[0]) if r else 0

class RobustCursor:
    """带自动重连+重试的游标包装。

    根因(2026-08-02 故障):原脚本全程共用一条连接,长批次灌装(290M 行)过程中
    连接被 TiDB/网络层断开 → executemany 抛 InterfaceError(0,'') → 收尾 current_count
    复用死连接再次抛错且未捕获 → 进程退码 1 → K8s Job BackoffLimitExceeded。
    本包装在 executemany/execute 捕获 OperationalError/InterfaceError 后重连重试
    (最多 4 次,线性退避 3/6/9/12s),对外暴露与原生 cursor 一致的接口,
    seeders 代码无需改动。
    """
    def __init__(self):
        self._reconnect()
    def _reconnect(self):
        try:
            if getattr(self, "c", None): self.c.close()
        except Exception:
            pass
        self.c = conn()
        self.cur = self.c.cursor()
    def _with_retry(self, op, *args):
        last=None
        for i in range(4):
            try:
                return getattr(self.cur, op)(*args)
            except (pymysql.OperationalError, pymysql.InterfaceError) as e:
                last=e; wait=3*(i+1)
                print(f"[warn] conn lost on {op} ({e!r}), reconnect retry {i+1}/4 after {wait}s", flush=True)
                time.sleep(wait); self._reconnect()
        raise last
    def executemany(self, sql, batch):
        return self._with_retry("executemany", sql, batch)
    def execute(self, sql, args=None):
        return self._with_retry("execute", sql, args)
    def fetchone(self):
        return self.cur.fetchone()
    def close(self):
        try:
            self.cur.close(); self.c.close()
        except Exception:
            pass

# ---------- 批量插入 ----------
def insert_batch(cur, table, rows_sql, batch):
    cur.executemany(rows_sql, batch)

def seed_users(cur, total):
    sql="INSERT INTO users(name,email,pwd_hash) VALUES (%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            nm=random.choice(FIRST_NAMES)+" "+random.choice(LAST_NAMES)
            email=f"u{rand_str(12)}@esp.test"
            batch.append((nm,email,rand_str(32)))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_scenarios(cur,total):
    sql="INSERT INTO scenarios(code,title,system_prompt,difficulty,lang) VALUES (%s,%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            code=f"SC_{rand_str(10)}"
            batch.append((code,random.choice(SCENARIO_TITLES),"You are an English tutor."+rand_str(20),random.randint(1,5),"en"))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_knowledge_points(cur,total):
    sql="INSERT INTO knowledge_points(code,title,parent_id,difficulty) VALUES (%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            pid=random.randint(1,10000) if done>100 else None
            batch.append((f"KP_{rand_str(10)}",random.choice(KP_TITLES),pid,random.randint(1,5)))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_sessions(cur,total,user_max,scn_max):
    sql="INSERT INTO sessions(user_id,scenario_id,started_at,ended_at) VALUES (%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            st=datetime(2025,1,1)+timedelta(seconds=random.randint(0,30*86400))
            et=st+timedelta(minutes=random.randint(5,120)) if random.random()>0.3 else None
            batch.append((random.randint(1,max(user_max,1)),random.randint(1,max(scn_max,1)),st,et))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_session_turns(cur,total,session_max):
    sql="INSERT INTO session_turns(session_id,content,audio_ref,created_at) VALUES (%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            ct=datetime(2025,1,1)+timedelta(seconds=random.randint(0,30*86400))
            ar=f"s3://audio/{rand_str(16)}.mp3" if random.random()>0.5 else None
            batch.append((random.randint(1,max(session_max,1)),"Student: "+rand_str(50),ar,ct))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_user_mastery(cur,total,user_max,kp_max):
    sql="INSERT IGNORE INTO user_mastery(user_id,kp_id,last_review_at,review_count,next_review_at) VALUES (%s,%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            uid=random.randint(1,max(user_max,1)); kid=random.randint(1,max(kp_max,1))
            lr=datetime(2025,7,1)+timedelta(days=random.randint(0,30))
            nr=lr+timedelta(days=random.randint(1,14))
            batch.append((uid,kid,lr,random.randint(1,50),nr))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_learning_events(cur,total,user_max):
    sql="INSERT INTO learning_events(user_id,event_type,payload,created_at) VALUES (%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            ct=datetime(2025,1,1)+timedelta(seconds=random.randint(0,200*86400))
            batch.append((random.randint(1,max(user_max,1)),random.choice(EVENT_TYPES),gen_payload_json(),ct))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_writing_tasks(cur,total,user_max):
    sql="INSERT INTO writing_tasks(user_id,prompt,essay,feedback,created_at) VALUES (%s,%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            ct=datetime(2025,1,1)+timedelta(seconds=random.randint(0,200*86400))
            batch.append((random.randint(1,max(user_max,1)),"Write about "+rand_str(20),gen_essay(),gen_feedback_json(),ct))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_lesson_plans(cur,total,teacher_max):
    sql="INSERT INTO lesson_plans(teacher_id,topic,content,created_at) VALUES (%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            ct=datetime(2025,1,1)+timedelta(seconds=random.randint(0,200*86400))
            batch.append((random.randint(1,max(teacher_max,1)),random.choice(KP_TITLES),gen_payload_json(),ct))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_classes(cur,total,teacher_max):
    sql="INSERT INTO classes(teacher_id,name,created_at) VALUES (%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            ct=datetime(2025,1,1)+timedelta(seconds=random.randint(0,200*86400))
            batch.append((random.randint(1,max(teacher_max,1)),"Class_"+rand_str(6),ct))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_assignments(cur,total,class_max):
    sql="INSERT INTO assignments(class_id,content,due_at,created_at) VALUES (%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            ct=datetime(2025,1,1)+timedelta(seconds=random.randint(0,200*86400))
            dt=ct+timedelta(days=random.randint(1,30)) if random.random()>0.3 else None
            batch.append((random.randint(1,max(class_max,1)),rand_str(100),dt,ct))
        cur.executemany(sql,batch)
        done+=n
        yield done

def seed_submissions(cur,total,asgn_max,user_max):
    sql="INSERT INTO submissions(assignment_id,user_id,content,auto_feedback,created_at) VALUES (%s,%s,%s,%s,%s)"
    done=0
    while done<total:
        n=min(BATCH,total-done)
        batch=[]
        for _ in range(n):
            ct=datetime(2025,1,1)+timedelta(seconds=random.randint(0,200*86400))
            batch.append((random.randint(1,max(asgn_max,1)),random.randint(1,max(user_max,1)),gen_essay(),gen_feedback_json(),ct))
        cur.executemany(sql,batch)
        done+=n
        yield done

# ---------- Redis 进度 ----------
_r=None
def redis_cli():
    global _r
    if _r is None and HAVE_REDIS:
        try:
            _r=redis_lib.Redis(host=REDIS_HOST,port=REDIS_PORT,decode_responses=True,socket_timeout=5)
            _r.ping()
        except Exception as e:
            print(f"[warn] redis unavailable: {e}",flush=True)
            _r=False
    return _r if _r is not False else None

def push_progress(table,done,total,overall_done,overall_total):
    pct=round(100.0*done/total,2) if total else 0
    opct=round(100.0*overall_done/overall_total,2) if overall_total else 0
    r=redis_cli()
    payload={"table":table,"done":done,"total":total,"pct":pct,
             "overall_done":overall_done,"overall_total":overall_total,"overall_pct":opct,
             "ts":datetime.now().isoformat()}
    if r:
        try: r.set("esp:ctx:seed-progress",json.dumps(payload))
        except Exception as e: print(f"[warn] redis set: {e}",flush=True)
    with open(REPORT,"a") as f:
        f.write(f"[{payload['ts']}] {table}: {done}/{total} ({pct}%) | overall {overall_done}/{overall_total} ({opct}%)\n")

# ---------- 主流程 ----------
def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--only",help="只灌某张表")
    ap.add_argument("--limit",type=int,help="限制总行数(测试用)")
    args=ap.parse_args()

    cur=RobustCursor()
    # 基线(断点续跑依据:各表当前行数)
    baseline={t:current_count(cur,t) for t in ORDER}
    print(f"[start] baseline counts: {baseline}",flush=True)

    overall_total=sum(TARGETS.values())
    if args.limit: overall_total=min(args.limit,overall_total)
    overall_done=0
    t0=time.time()

    # 已灌的 user/scenario/kp 数量(用于外键引用上限)
    user_cnt=baseline["users"]; scn_cnt=baseline["scenarios"]; kp_cnt=baseline["knowledge_points"]
    session_cnt=baseline["sessions"]; class_cnt=baseline["classes"]; asgn_cnt=baseline["assignments"]

    SEEDERS=[
        ("users",lambda cur,t: seed_users(cur,t)),
        ("scenarios",lambda cur,t: seed_scenarios(cur,t)),
        ("knowledge_points",lambda cur,t: seed_knowledge_points(cur,t)),
        ("sessions",lambda cur,t: seed_sessions(cur,t,user_cnt+TARGETS["users"],scn_cnt+TARGETS["scenarios"])),
        ("session_turns",lambda cur,t: seed_session_turns(cur,t,session_cnt+TARGETS["sessions"])),
        ("user_mastery",lambda cur,t: seed_user_mastery(cur,t,user_cnt+TARGETS["users"],kp_cnt+TARGETS["knowledge_points"])),
        ("learning_events",lambda cur,t: seed_learning_events(cur,t,user_cnt+TARGETS["users"])),
        ("writing_tasks",lambda cur,t: seed_writing_tasks(cur,t,user_cnt+TARGETS["users"])),
        ("lesson_plans",lambda cur,t: seed_lesson_plans(cur,t,user_cnt+TARGETS["users"])),  # teacher=users
        ("classes",lambda cur,t: seed_classes(cur,t,user_cnt+TARGETS["users"])),
        ("assignments",lambda cur,t: seed_assignments(cur,t,class_cnt+TARGETS["classes"])),
        ("submissions",lambda cur,t: seed_submissions(cur,t,asgn_cnt+TARGETS["assignments"],user_cnt+TARGETS["users"])),
    ]

    # delta 断点续灌:每表只灌 (full - 已有量);已完成表跳过。overall 按 full 计入。
    base_overall=0
    for table,fn in SEEDERS:
        if args.only and args.only!=table: continue
        full=TARGETS[table]
        delta=max(0, full - baseline[table])       # 本轮需追加量
        already=full - delta                         # 已存在量(= min(full, baseline))
        target=delta
        if args.limit: target=min(target,args.limit)
        if target==0:
            print(f"[skip] {table}: already {baseline[table]} >= full {full}", flush=True)
            base_overall+=full
            push_progress(table, full, full, base_overall, overall_total)
            continue
        print(f"[seed] {table}: have {baseline[table]}, +{target} (full {full})", flush=True)
        try:
            last_report=0
            for done in fn(cur,target):
                if done-last_report>=BATCH*20 or done==target:
                    push_progress(table, already+done, full, base_overall+already+done, overall_total)
                    last_report=done
            base_overall+=full
            push_progress(table, full, full, base_overall, overall_total)
        except Exception as e:
            print(f"[ERR] {table}: {e} (已重连重试仍失败,记录后继续下一表)", flush=True)
            with open(REPORT,"a") as f:
                f.write(f"[ERROR] {table}: {e}\n")
            # 失败也按 full 计入 overall,避免进度卡在低位误导监控
            base_overall+=full
    overall_done=base_overall

    elapsed=time.time()-t0
    # 收尾 current_count 容错:长批次灌装后连接可能已失效,RobustCursor 重连时若遇 DNS 抖动
    # (单节点 RD NodeNotReady 间隙)会抛 OperationalError 致整个 Job 退码 1,
    # 但前面 seeding 主体已完成。收尾失败不应判定整批失败 —— 改为逐表 try,失败记 0 并落报告。
    final={}
    for t in ORDER:
        try:
            final[t]=current_count(cur,t)
        except Exception as e:
            print(f"[warn] final_count {table} failed: {e!r}, record 0", flush=True)
            final[t]=0
    print(f"[done] elapsed={elapsed:.1f}s final_counts={final}",flush=True)
    r=redis_cli()
    summary={"status":"done","elapsed_sec":round(elapsed,1),
             "final_counts":final,"overall_done":overall_done,
             "overall_total":overall_total,"ts":datetime.now().isoformat()}
    if r:
        try: r.set("esp:ctx:seed-status",json.dumps(summary))
        except Exception: pass
    with open(REPORT,"a") as f:
        f.write(f"\n=== SUMMARY elapsed={elapsed:.1f}s overall={overall_done}/{overall_total} final={final}\n")

if __name__=="__main__":
    main()
