#!/usr/bin/env python3
"""
ESP CockroachDB 等量灌装器(Task #136 Part 1)
============================================
对 CRDB(按有界上下文分库,见 scripts/crdb-schema.sql)灌装与 TiDB 等量/降比例的种子数据,
作为 #107/#136 CRDB vs TiDB 对比压测的基线。总量 ≈ 32.6M 行。

与 TiDB 行数对比 + 比例(诚实可比):
  table                CRDB 目标     TiDB 实测(基准)                 比值
  users                1,000,000   / 1,002,651 (esp.users)           ≈1.0x
  scenarios               10,000   / 10,000                           =1.0x
  knowledge_points        10,000   / 10,000                           =1.0x
  user_mastery         5,000,000   / 4,998,730                       ≈1.0x
  learning_events     10,000,000   / 10,526,432 (esp_learning)       ≈0.95x(对主库 esp≈100M = 0.1x)
  sessions             5,000,000   / ≈50,000,000                      =0.1x
  session_turns        5,000,000   / ≈50,000,000                      =0.1x
  writing_tasks        2,000,000   / ≈20,000,000                      =0.1x
  lesson_plans         1,000,000   / 1,010,000                       ≈0.99x
  classes                500,000   / 500,000                          =1.0x
  assignments          2,000,000   / 5,030,000                       ≈0.4x
  submissions          2,000,000   / 10,084,999                      ≈0.2x
  learning_event_outbox       0    / 0                                =1.0x(空表,仅结构)
  app_logs               85,000   / 85,601 (esp_shared)              ≈0.99x
  合计 ≈ 32,605,000 行(对比 TiDB 主库量级 30 亿 ≈ 1%)

域库路由(每表一个独立连接,dbname 按域,psycopg2):
  esp_auth    : users, sessions
  esp_learning: learning_events, learning_event_outbox, session_turns, user_mastery, knowledge_points
  esp_content : scenarios
  esp_teacher : classes, assignments, submissions, lesson_plans
  esp_writing : writing_tasks
  esp_shared  : app_logs

设计要点:
- psycopg2 直连 CRDB(PG wire): host crdb-public:26257, user root, sslmode=disable。
  在能解析 crdb-public / redis-cluster 的主机跑(集群内 pod,或 kubectl port-forward 后本机)。
- 每批 5000 行:psycopg2 原生 executemany 是逐行 INSERT(慢),这里用 psycopg2.extras.execute_values
  生成一条多行 INSERT,等价 seed_3b(pymysql executemany)的真批量语义;批后 sleep SEED_THROTTLE(默认 0.3s)。
- 断点续跑:启动对当前表 COUNT(*) 取已灌量,按 delta 补灌;每 20 批 + 表完成写一次进度。
- 进度双写: Redis key esp:ctx:seed-crdb-progress(可用时)+ 本地 report/seed-crdb-progress.txt。
- 数据生成器复用 seed_3b.py 模板池(名字/essay/feedback/payload),单行 <2KB。
- 灌装顺序按依赖: users→scenarios→knowledge_points→sessions→session_turns→user_mastery
  →learning_events→writing_tasks→lesson_plans→classes→assignments→submissions→app_logs。

用法:
  python3 scripts/seed_crdb_full.py --dry-run            # 只打印计划,不插入
  python3 scripts/seed_crdb_full.py --only users         # 只灌 users
  python3 scripts/seed_crdb_full.py --limit 100000       # 每表上限(测试用)
环境变量: CRDB_HOST/CRDB_PORT(默认 crdb-public:26257)、REDIS_HOST/REDIS_PORT、
          SEED_THROTTLE(默认 0.3)、SEED_MAX_RETRY(默认 10)。
"""
import os, sys, json, time, random, string, argparse
from datetime import datetime, timedelta

try:
    import psycopg2
    from psycopg2.extras import execute_values
    HAVE_PSYCOPG2 = True
except ImportError:
    HAVE_PSYCOPG2 = False

try:
    import redis as redis_lib
    HAVE_REDIS = True
except ImportError:
    HAVE_REDIS = False

# ---------- 配置 ----------
CRDB_HOST = os.environ.get("CRDB_HOST", "crdb-public")
CRDB_PORT = int(os.environ.get("CRDB_PORT", "26257"))
REDIS_HOST = os.environ.get("REDIS_HOST", "redis-cluster")
REDIS_PORT = int(os.environ.get("REDIS_PORT", "6379"))
REPORT = "report/seed-crdb-progress.txt"
BATCH = 5000                                   # 每批行数(对齐 seed_3b)
THROTTLE_SLEEP = float(os.environ.get("SEED_THROTTLE", "0.3"))  # 每批后 sleep,降写放大

# 表 → 域库路由(每表独立连接串库名)
TABLE_DB = {
    "users":                  "esp_auth",
    "sessions":               "esp_auth",
    "learning_events":        "esp_learning",
    "learning_event_outbox":  "esp_learning",
    "session_turns":          "esp_learning",
    "user_mastery":           "esp_learning",
    "knowledge_points":       "esp_learning",
    "scenarios":              "esp_content",
    "classes":                "esp_teacher",
    "assignments":            "esp_teacher",
    "submissions":            "esp_teacher",
    "lesson_plans":           "esp_teacher",
    "writing_tasks":          "esp_writing",
    "app_logs":               "esp_shared",
}

# 每表目标行数(小表精确对齐 TiDB,大表按 1/10 左右降比例;注释含 TiDB 基准与比值)
TARGETS = {
    "users":                  1_000_000,   # TiDB 1,002,651            ≈1.0x
    "scenarios":                 10_000,   # TiDB 10,000               =1.0x
    "knowledge_points":          10_000,   # TiDB 10,000               =1.0x
    "user_mastery":           5_000_000,   # TiDB 4,998,730            ≈1.0x
    "learning_events":       10_000_000,   # esp_learning 10,526,432   ≈0.95x(主库 esp≈100M 为 0.1x)
    "sessions":               5_000_000,   # TiDB ≈50M                 =0.1x
    "session_turns":          5_000_000,   # TiDB ≈50M                 =0.1x
    "writing_tasks":          2_000_000,   # TiDB ≈20M                 =0.1x
    "lesson_plans":           1_000_000,   # TiDB 1,010,000            ≈0.99x
    "classes":                  500_000,   # TiDB 500,000              =1.0x
    "assignments":            2_000_000,   # TiDB 5,030,000            ≈0.4x
    "submissions":            2_000_000,   # TiDB 10,084,999           ≈0.2x
    "learning_event_outbox":          0,   # TiDB 0(空表,仅结构)       =1.0x
    "app_logs":                  85_000,   # esp_shared 85,601         ≈0.99x
}
TIDB_BASELINE = {  # 仅用于 --dry-run 展示比值
    "users": 1_002_651, "scenarios": 10_000, "knowledge_points": 10_000,
    "user_mastery": 4_998_730, "learning_events": 10_526_432,
    "sessions": 50_000_000, "session_turns": 50_000_000, "writing_tasks": 20_000_000,
    "lesson_plans": 1_010_000, "classes": 500_000, "assignments": 5_030_000,
    "submissions": 10_084_999, "learning_event_outbox": 0, "app_logs": 85_601,
}
# 灌装顺序(依赖关系): 先被引用表,再引用它们的表
ORDER = ["users", "scenarios", "knowledge_points", "sessions", "session_turns",
         "user_mastery", "learning_events", "learning_event_outbox", "writing_tasks",
         "lesson_plans", "classes", "assignments", "submissions", "app_logs"]

# ---------- 数据生成器(复用 seed_3b 模板池,轻量,单行 <2KB) ----------
FIRST_NAMES = ["Alex", "Sam", "Jordan", "Taylor", "Morgan", "Riley", "Casey", "Jamie", "Avery", "Quinn",
               "李", "王", "张", "刘", "陈", "杨", "赵", "黄", "周", "吴"]
LAST_NAMES = ["Chen", "Wang", "Li", "Zhang", "Liu", "Yang", "Huang", "Zhao", "Sun", "Xu",
              "伟", "芳", "娜", "敏", "静", "强", "磊", "洋", "艳", "勇"]
SCENARIO_TITLES = ["Daily Conversation", "Business Meeting", "Travel English",
                   "Job Interview", "Restaurant Ordering", "Airport Check-in",
                   "Doctor Visit", "Shopping", "Weather Talk", "Movie Review"]
KP_TITLES = ["Present Simple", "Past Continuous", "Conditionals", "Articles", "Modal Verbs",
             "Passive Voice", "Reported Speech", "Phrasal Verbs", "Relative Clauses", "Gerunds"]
EVENT_TYPES = ["login", "lesson_start", "lesson_complete", "exercise_submit", "review",
               "speaking_attempt", "listening_practice", "quiz_pass", "quiz_fail", "logout"]
SERVICES = ["gateway", "bff", "learning", "writing", "teacher", "emotion"]
LEVELS = ["INFO", "WARN", "ERROR"]
APP_EVENTS = ["auth.login", "auth.logout", "learning.event", "writing.grade", "behavior.click", "gateway.req"]


def rand_str(n=8):
    return ''.join(random.choices(string.ascii_lowercase + string.digits, k=n))


def gen_essay():
    base = ("English learning is a journey that requires dedication and consistent practice. "
            "Through reading, writing, speaking and listening, learners gradually build fluency. "
            "Regular exposure to authentic materials helps internalize grammar and vocabulary naturally.")
    return base[:200] + " " + rand_str(40)


def gen_feedback_json():
    return json.dumps({"score": random.randint(60, 100),
                       "grammar": random.randint(1, 10),
                       "vocab": random.randint(1, 10),
                       "coherence": random.randint(1, 10)})


def gen_payload_json():
    return json.dumps({"dur_ms": random.randint(100, 5000),
                       "score": random.randint(1, 100),
                       "kp": random.randint(1, 10000)})


# ---------- DB 连接 ----------
def conn(db):
    return psycopg2.connect(host=CRDB_HOST, port=CRDB_PORT, user="root",
                            dbname=db, sslmode="disable", connect_timeout=10)


class RobustCursor:
    """带自动重连+重试的 psycopg2 游标包装(对齐 seed_3b.RobustCursor)。

    CRDB/网络断开时 executemany 抛 OperationalError/InterfaceError → 重连重试
    (最多 SEED_MAX_RETRY 次,线性退避 5/10/...s);对外暴露与原生 cursor 一致的接口。
    autocommit=True:每批多行 INSERT 一个隐式事务即提交,不悬挂长事务。
    """
    def __init__(self, db):
        self.db = db
        self._reconnect()

    def _reconnect(self):
        try:
            if getattr(self, "c", None):
                self.c.close()
        except Exception:
            pass
        self.c = conn(self.db)
        self.c.autocommit = True
        self.cur = self.c.cursor()

    def _retry(self, fn):
        last = None
        max_retry = int(os.environ.get("SEED_MAX_RETRY", "10"))
        for i in range(max_retry):
            try:
                return fn()
            except (psycopg2.OperationalError, psycopg2.InterfaceError) as e:
                last = e
                wait = 5 * (i + 1)
                print(f"[warn] conn lost on {getattr(fn, '__name__', 'op')} ({e!r}), "
                      f"reconnect retry {i + 1}/{max_retry} after {wait}s", flush=True)
                time.sleep(wait)
                self._reconnect()
        raise last

    def executemany(self, sql, batch):
        """真 5000 行批量 INSERT。psycopg2 原生 executemany 是逐行(慢),
        这里用 execute_values 生成一条多行 INSERT,等价 seed_3b(MySQL executemany)语义。"""
        def _run():
            execute_values(self.cur, sql, batch, page_size=len(batch))
        self._retry(_run)
        if THROTTLE_SLEEP > 0:
            time.sleep(THROTTLE_SLEEP)
        return len(batch)

    def execute(self, sql, args=None):
        return self._retry(lambda: self.cur.execute(sql, args))

    def fetchone(self):
        return self.cur.fetchone()

    def close(self):
        try:
            self.cur.close()
            self.c.close()
        except Exception:
            pass


def table_count(table):
    """启动前查当前行数(COUNT(*),目标表最大 10M,CRDB 可接受;
    seed_3b 用 information_schema 估算是因为 TiDB 侧 100M+ 行 COUNT 会超时)。"""
    db = TABLE_DB.get(table)
    if not db:
        return 0
    try:
        cur = RobustCursor(db)
        try:
            cur.execute(f'SELECT count(*) FROM "{table}"')
            row = cur.fetchone()
            return int(row[0]) if row and row[0] is not None else 0
        finally:
            cur.close()
    except Exception as e:
        print(f"[warn] count {table}: {e!r}", flush=True)
        return 0


# ---------- 批量插入(每表一个 seeder,模板池随机生成) ----------
def seed_users(cur, total):
    sql = "INSERT INTO users(name,email,pwd_hash) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            nm = random.choice(FIRST_NAMES) + " " + random.choice(LAST_NAMES)
            batch.append((nm, f"u{rand_str(12)}@esp.test", rand_str(32)))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_scenarios(cur, total):
    sql = "INSERT INTO scenarios(code,title,system_prompt,difficulty,lang) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            batch.append((f"SC_{rand_str(10)}", random.choice(SCENARIO_TITLES),
                          "You are an English tutor." + rand_str(20), random.randint(1, 5), "en"))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_knowledge_points(cur, total):
    sql = "INSERT INTO knowledge_points(code,title,parent_id,difficulty) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            pid = random.randint(1, 10000) if done > 100 else None
            batch.append((f"KP_{rand_str(10)}", random.choice(KP_TITLES), pid, random.randint(1, 5)))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_sessions(cur, total, user_max, scn_max):
    sql = "INSERT INTO sessions(user_id,scenario_id,started_at,ended_at) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            st = datetime(2025, 1, 1) + timedelta(seconds=random.randint(0, 30 * 86400))
            et = st + timedelta(minutes=random.randint(5, 120)) if random.random() > 0.3 else None
            batch.append((random.randint(1, max(user_max, 1)), random.randint(1, max(scn_max, 1)), st, et))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_session_turns(cur, total, session_max):
    # TiDB 版(seed_3b)省略 role 靠 MySQL ENUM 隐式默认 'user';CRDB VARCHAR NOT NULL 无隐式默认,必须显式给。
    sql = "INSERT INTO session_turns(session_id,role,content,audio_ref,created_at) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            ct = datetime(2025, 1, 1) + timedelta(seconds=random.randint(0, 30 * 86400))
            ar = f"s3://audio/{rand_str(16)}.mp3" if random.random() > 0.5 else None
            batch.append((random.randint(1, max(session_max, 1)), random.choice(["user", "ai", "system"]),
                          "Student: " + rand_str(50), ar, ct))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_user_mastery(cur, total, user_max, kp_max):
    # MySQL INSERT IGNORE → CRDB ON CONFLICT DO NOTHING(冲突即已存在,跳过)
    sql = "INSERT INTO user_mastery(user_id,kp_id,last_review_at,review_count,next_review_at) VALUES %s ON CONFLICT DO NOTHING"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            uid = random.randint(1, max(user_max, 1))
            kid = random.randint(1, max(kp_max, 1))
            lr = datetime(2025, 7, 1) + timedelta(days=random.randint(0, 30))
            nr = lr + timedelta(days=random.randint(1, 14))
            batch.append((uid, kid, lr, random.randint(1, 50), nr))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_learning_events(cur, total, user_max):
    sql = "INSERT INTO learning_events(user_id,event_type,payload,created_at) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            ct = datetime(2025, 1, 1) + timedelta(seconds=random.randint(0, 200 * 86400))
            batch.append((random.randint(1, max(user_max, 1)), random.choice(EVENT_TYPES), gen_payload_json(), ct))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_writing_tasks(cur, total, user_max):
    sql = "INSERT INTO writing_tasks(user_id,prompt,essay,feedback,created_at) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            ct = datetime(2025, 1, 1) + timedelta(seconds=random.randint(0, 200 * 86400))
            batch.append((random.randint(1, max(user_max, 1)), "Write about " + rand_str(20),
                          gen_essay(), gen_feedback_json(), ct))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_lesson_plans(cur, total, teacher_max):
    sql = "INSERT INTO lesson_plans(teacher_id,topic,content,created_at) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            ct = datetime(2025, 1, 1) + timedelta(seconds=random.randint(0, 200 * 86400))
            batch.append((random.randint(1, max(teacher_max, 1)), random.choice(KP_TITLES), gen_payload_json(), ct))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_classes(cur, total, teacher_max):
    sql = "INSERT INTO classes(teacher_id,name,created_at) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            ct = datetime(2025, 1, 1) + timedelta(seconds=random.randint(0, 200 * 86400))
            batch.append((random.randint(1, max(teacher_max, 1)), "Class_" + rand_str(6), ct))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_assignments(cur, total, class_max):
    # TiDB 版(seed_3b)省略 type 靠 MySQL ENUM 隐式默认 'oral';CRDB 需显式给。
    sql = "INSERT INTO assignments(class_id,type,content,due_at,created_at) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            ct = datetime(2025, 1, 1) + timedelta(seconds=random.randint(0, 200 * 86400))
            dt = ct + timedelta(days=random.randint(1, 30)) if random.random() > 0.3 else None
            batch.append((random.randint(1, max(class_max, 1)), random.choice(["oral", "written"]),
                          rand_str(100), dt, ct))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_submissions(cur, total, asgn_max, user_max):
    sql = "INSERT INTO submissions(assignment_id,user_id,content,auto_feedback,created_at) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            ct = datetime(2025, 1, 1) + timedelta(seconds=random.randint(0, 200 * 86400))
            batch.append((random.randint(1, max(asgn_max, 1)), random.randint(1, max(user_max, 1)),
                          gen_essay(), gen_feedback_json(), ct))
        cur.executemany(sql, batch)
        done += n
        yield done


def seed_app_logs(cur, total, user_max):
    sql = "INSERT INTO app_logs(ts,service,level,event,trace_id,user_id,payload) VALUES %s"
    done = 0
    while done < total:
        n = min(BATCH, total - done)
        batch = []
        for _ in range(n):
            ts = datetime(2025, 1, 1) + timedelta(seconds=random.randint(0, 200 * 86400))
            uid = random.randint(1, max(user_max, 1)) if random.random() > 0.2 else None
            batch.append((ts, random.choice(SERVICES), random.choice(LEVELS), random.choice(APP_EVENTS),
                          rand_str(16), uid, gen_payload_json()))
        cur.executemany(sql, batch)
        done += n
        yield done


def build_seeder(table, counts):
    """按表返回 seeder 函数;被引用表数量优先取本轮已灌/将灌的投影值,缺失则现查。"""
    def ref(name):
        if name in counts:
            return counts[name]
        c = table_count(name)
        counts[name] = c
        return c

    users_max = ref("users")
    scn_max = ref("scenarios")
    kp_max = ref("knowledge_points")
    session_max = ref("sessions")
    class_max = ref("classes")
    asgn_max = ref("assignments")

    if table == "users":
        return lambda cur, total: seed_users(cur, total)
    if table == "scenarios":
        return lambda cur, total: seed_scenarios(cur, total)
    if table == "knowledge_points":
        return lambda cur, total: seed_knowledge_points(cur, total)
    if table == "sessions":
        return lambda cur, total: seed_sessions(cur, total, users_max, scn_max)
    if table == "session_turns":
        return lambda cur, total: seed_session_turns(cur, total, session_max)
    if table == "user_mastery":
        return lambda cur, total: seed_user_mastery(cur, total, users_max, kp_max)
    if table == "learning_events":
        return lambda cur, total: seed_learning_events(cur, total, users_max)
    if table == "writing_tasks":
        return lambda cur, total: seed_writing_tasks(cur, total, users_max)
    if table == "lesson_plans":
        return lambda cur, total: seed_lesson_plans(cur, total, users_max)
    if table == "classes":
        return lambda cur, total: seed_classes(cur, total, users_max)
    if table == "assignments":
        return lambda cur, total: seed_assignments(cur, total, class_max)
    if table == "submissions":
        return lambda cur, total: seed_submissions(cur, total, asgn_max, users_max)
    if table == "app_logs":
        return lambda cur, total: seed_app_logs(cur, total, users_max)
    raise KeyError(table)


# ---------- Redis + 本地进度(双写) ----------
_r = None


def redis_cli():
    global _r
    if _r is None and HAVE_REDIS:
        try:
            # Redis 真 Cluster(hostname 公告): 单点 Redis() 不跟随 MOVED,set() 跨槽位必失败。
            # 优先用 RedisCluster(跟随 MOVED 重定向); 退化为单点 Redis(伪集群/单节点时兼容)。
            try:
                from redis.cluster import RedisCluster
                _r = RedisCluster(host=REDIS_HOST, port=REDIS_PORT, decode_responses=True,
                                  socket_timeout=5, socket_connect_timeout=5, skip_full_coverage_check=True)
            except Exception:
                _r = redis_lib.Redis(host=REDIS_HOST, port=REDIS_PORT, decode_responses=True, socket_timeout=5)
            _r.ping()
        except Exception as e:
            print(f"[warn] redis unavailable: {e}", flush=True)
            _r = False
    return _r if _r is not False else None


def push_progress(table, done, total, overall_done, overall_total, extra=""):
    pct = round(100.0 * done / total, 2) if total else 0
    opct = round(100.0 * overall_done / overall_total, 2) if overall_total else 0
    r = redis_cli()
    payload = {"table": table, "done": done, "total": total, "pct": pct,
               "overall_done": overall_done, "overall_total": overall_total, "overall_pct": opct,
               "extra": extra, "ts": datetime.now().isoformat()}
    if r:
        try:
            r.set("esp:ctx:seed-crdb-progress", json.dumps(payload))
        except Exception as e:
            print(f"[warn] redis set: {e}", flush=True)
    with open(REPORT, "a") as f:
        f.write(f"[{payload['ts']}] {table}: {done}/{total} ({pct}%){(' | ' + extra) if extra else ''}"
                f" | overall {overall_done}/{overall_total} ({opct}%)\n")


# ---------- 主流程 ----------
def main():
    ap = argparse.ArgumentParser(description="CRDB 等量灌装器(Task #136 Part 1)")
    ap.add_argument("--dry-run", action="store_true", help="只打印计划(各表现有/目标/delta/比值),不插入")
    ap.add_argument("--only", help="只灌某张表(如 users)")
    ap.add_argument("--limit", type=int, help="每表追加行数上限(测试用,覆盖 delta)")
    args = ap.parse_args()

    if not HAVE_PSYCOPG2:
        sys.exit("[FATAL] psycopg2 未安装: pip install psycopg2-binary")
    os.makedirs(os.path.dirname(REPORT) or ".", exist_ok=True)

    # ---- 计划预览(无论是否 dry-run 都打印) ----
    print("=== CRDB 灌装计划 ===", flush=True)
    for t in ORDER:
        if args.only and args.only != t:
            continue
        have = table_count(t)
        full = TARGETS[t]
        delta = max(0, full - have)
        tidb = TIDB_BASELINE.get(t, 0)
        ratio = f"{full / tidb:.2f}x" if tidb else "-"
        print(f"[plan] {t:<20} db={TABLE_DB[t]:<13} existing={have:>11,} "
              f"target={full:>11,} delta={delta:>11,} ratio(vs TiDB)={ratio}", flush=True)
    print(f"[plan] total target = {sum(TARGETS.values()):,} rows", flush=True)
    if args.dry_run:
        print("[dry-run] 退出,未插入任何行", flush=True)
        return

    # ---- 灌装 ----
    overall_total = sum(TARGETS.values())
    base_overall = 0
    counts = {}
    t0 = time.time()
    for t in ORDER:
        if args.only and args.only != t:
            continue
        full = TARGETS[t]
        if full == 0:
            print(f"[skip] {t}: target 0(空表,仅结构)", flush=True)
            push_progress(t, 0, 0, base_overall, overall_total, extra="target=0")
            continue
        have = table_count(t)
        delta = max(0, full - have)
        if args.limit:
            delta = min(delta, args.limit)
        if delta == 0:
            print(f"[skip] {t}: existing {have:,} >= full {full:,}", flush=True)
            counts[t] = have
            base_overall += full
            continue
        counts[t] = have + delta
        print(f"[seed] {t}: existing={have:,} +{delta:,} -> projected {counts[t]:,} (full {full:,})", flush=True)
        cur = RobustCursor(TABLE_DB[t])
        try:
            fn = build_seeder(t, counts)
            last_report = 0
            for done in fn(cur, delta):
                if done - last_report >= BATCH * 20 or done == delta:
                    push_progress(t, have + done, full, base_overall + have + done, overall_total)
                    last_report = done
            base_overall += full
            push_progress(t, full, full, base_overall, overall_total)
        except Exception as e:
            print(f"[ERR] {t}: {e!r} (重连重试仍失败,记录后继续下一表)", flush=True)
            with open(REPORT, "a") as f:
                f.write(f"[ERROR] {t}: {e!r}\n")
            base_overall += full  # 失败也按 full 计入,避免进度卡低位误导监控
        finally:
            cur.close()

    elapsed = time.time() - t0
    final = {t: table_count(t) for t in ORDER}
    print(f"[done] elapsed={elapsed:.1f}s final_counts={final}", flush=True)
    summary = {"status": "done", "elapsed_sec": round(elapsed, 1), "final_counts": final,
               "overall_done": base_overall, "overall_total": overall_total,
               "ts": datetime.now().isoformat()}
    r = redis_cli()
    if r:
        try:
            r.set("esp:ctx:seed-crdb-status", json.dumps(summary))
        except Exception:
            pass
    with open(REPORT, "a") as f:
        f.write(f"\n=== SUMMARY elapsed={elapsed:.1f}s overall={base_overall}/{overall_total} final={final}\n")


if __name__ == "__main__":
    main()
