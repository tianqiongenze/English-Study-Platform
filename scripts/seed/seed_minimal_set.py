#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""ESP 最小集灌装(classes/assignments/submissions/lesson_plans/writing_tasks)。

目标:补齐 0 值表,够 warm up + 功能验证 + 压测,不做百万级。
幂等:先 SELECT count,达标(>= 目标)则 skip。
连接 tidb-primary:4000 root esp,集群内 pod 执行。

依赖既有种子:
  - users(100万,role=learner 999998 + teacher 2 + admin 1)
  - knowledge_points(29954)
  - scenarios(10000)

策略:
  - teacher 不足:从既有 learner 中挑 100 个升 role='teacher'(不删原记录,只改 role,
    避免 AUTO_RANDOM id 变动)。这样 classes/lesson_plans 的 teacher_id 都指向真实 user。
  - classes: 100 条,teacher 从新 teacher 池随机。
  - assignments: 500 条,关联 classes + 随机 type/content。
  - submissions: 2000 条,关联 assignments + learner。
  - lesson_plans: 100 条,teacher + topic + JSON content。
  - writing_tasks: 补到 500 条(已有 1),user + prompt + essay 文本。
"""
import sys
import json
import random
import pymysql

HOST = "tidb-primary"
PORT = 4000
USER = "root"
DB = "esp"

random.seed(20260804)

# 目标量
TARGET_TEACHERS = 100          # 总 teacher 数(含原 2)
TARGET_CLASSES = 100
TARGET_ASSIGNMENTS = 500
TARGET_SUBMISSIONS = 2000
TARGET_LESSON_PLANS = 100
TARGET_WRITING_TASKS = 500

# ---------- 文本模板 ----------
ASSIGN_TOPICS = [
    "Write a 200-word essay on your favorite hobby.",
    "Describe your hometown in 150 words.",
    "Give a 3-minute oral presentation on a book you recently read.",
    "Record yourself ordering food at a restaurant.",
    "Write an email to your teacher asking for an extension.",
    "Practice role-play: job interview self-introduction (oral).",
    "Write a compare-and-contrast essay on city vs country life.",
    "Summarize today's lesson in 5 sentences (written).",
    "Record a voicemail message asking a friend to call back.",
    "Write a short story (300 words) using past tense.",
]
SUBMISSION_CONTENTS = [
    "My favorite hobby is reading. I enjoy novels and science fiction because they transport me to different worlds.",
    "I live in a small town in the south. The people are friendly and the food is delicious.",
    "Good morning, my name is Joel. I have five years of experience in software engineering.",
    "Hello, I would like to place an order. Could I get the chicken sandwich with a side salad?",
    "Dear Professor, I am writing to request a two-day extension on the assignment due to illness.",
    "City life is fast-paced while country life is quiet. Both have their advantages.",
    "Today we learned about the present perfect tense and how to use it with 'just', 'already', and 'yet'.",
    "Hi Tom, this is Lisa. Please call me back when you get a chance. Talk to you soon.",
    "Once upon a time, a young girl found a mysterious key in her grandmother's attic.",
]
ESSAY_PROMPTS = [
    "Write about a challenge you overcame and what you learned.",
    "Describe a person who has influenced your life.",
    "Explain the importance of learning a foreign language.",
    "What are the advantages and disadvantages of remote work?",
    "Discuss the impact of social media on communication.",
]
ESSAY_TEXTS = [
    "One of the biggest challenges I faced was moving to a new city alone. At first, I felt lonely and overwhelmed, but I gradually made friends and learned to navigate the city. This experience taught me resilience and independence, and I now see change as an opportunity for growth rather than a threat.",
    "My high school English teacher, Ms. Wang, profoundly influenced my life. She not only taught grammar and vocabulary but also encouraged me to think critically and express my opinions. Her patience and passion inspired me to pursue a career in education, hoping to impact others as she impacted me.",
    "Learning a foreign language opens doors to new cultures, ideas, and opportunities. It enhances cognitive flexibility, improves career prospects, and fosters empathy by allowing us to see the world through different lenses. In an increasingly globalized world, multilingualism is a vital skill.",
    "Remote work offers flexibility and eliminates commute time, improving work-life balance. However, it can blur boundaries between work and personal life, reduce social interaction, and make collaboration harder. Successful remote work requires discipline, clear communication, and supportive company culture.",
    "Social media has transformed how we communicate, making it faster and more far-reaching. Yet it also reduces face-to-face interaction, spreads misinformation, and can harm mental health. The key is mindful usage: leveraging its benefits while being aware of its pitfalls.",
]
LESSON_TOPICS = [
    "Present Perfect vs Past Simple",
    "Conditional Sentences (Type 1 & 2)",
    "Business Email Etiquette",
    "Pronunciation: Connected Speech",
    "IELTS Speaking Part 2 Strategies",
    "Writing Cohesion and Linking Words",
    "Modal Verbs for Polite Requests",
    "Vocabulary for Academic Writing",
    "Common Phrasal Verbs in Daily Life",
    "Narrative Tenses for Storytelling",
]


def connect():
    return pymysql.connect(host=HOST, port=PORT, user=USER, database=DB,
                           charset="utf8mb4", autocommit=False,
                           connect_timeout=10, read_timeout=120, write_timeout=120)


def cur_count(cur, table):
    cur.execute(f"SELECT COUNT(*) FROM {table}")  # nosemgrep: python.lang.security.audit.sqli - table 来自内部硬编码常量,非用户输入,seed 灌装脚本无注入风险
    return cur.fetchone()[0]


def ensure_teachers(cur, conn):
    """把 learner 升级为 teacher 直到达 TARGET_TEACHERS。返回 teacher id 列表。"""
    cur.execute("SELECT COUNT(*) FROM users WHERE role='teacher'")
    n = cur.fetchone()[0]
    if n < TARGET_TEACHERS:
        need = TARGET_TEACHERS - n
        print(f"[teachers] need {need} more, promoting learners -> teacher")
        cur.execute(
            "UPDATE users SET role='teacher' WHERE role='learner' ORDER BY id LIMIT %s",
            (need,))
        conn.commit()
        print(f"[teachers] promoted {cur.rowcount} rows")
    cur.execute("SELECT id FROM users WHERE role='teacher' ORDER BY id LIMIT %s",
                (TARGET_TEACHERS,))
    tids = [r[0] for r in cur.fetchall()]
    print(f"[teachers] total teacher ids = {len(tids)}")
    return tids


def seed_classes(cur, conn, teacher_ids):
    have = cur_count(cur, "classes")
    print(f"[classes] existing={have} target={TARGET_CLASSES}")
    if have >= TARGET_CLASSES:
        print("[classes] SKIP (enough)")
        return
    need = TARGET_CLASSES - have
    rows = []
    for i in range(need):
        tid = random.choice(teacher_ids)
        name = f"Class-{have+i+1:04d}"
        rows.append((tid, name))
    cur.executemany("INSERT INTO classes (teacher_id, name) VALUES (%s,%s)", rows)
    conn.commit()
    print(f"[classes] inserted {cur.rowcount}, total now {cur_count(cur,'classes')}")


def get_class_ids(cur):
    cur.execute("SELECT id FROM classes LIMIT 10000")
    return [r[0] for r in cur.fetchall()]


def seed_assignments(cur, conn, class_ids):
    have = cur_count(cur, "assignments")
    print(f"[assignments] existing={have} target={TARGET_ASSIGNMENTS}")
    if have >= TARGET_ASSIGNMENTS:
        print("[assignments] SKIP")
        return
    need = TARGET_ASSIGNMENTS - have
    rows = []
    for i in range(need):
        cid = random.choice(class_ids)
        tp = random.choice(["oral", "written"])
        content = random.choice(ASSIGN_TOPICS)
        rows.append((cid, tp, content, None))
    cur.executemany(
        "INSERT INTO assignments (class_id, type, content, due_at) VALUES (%s,%s,%s,%s)",
        rows)
    conn.commit()
    print(f"[assignments] inserted {cur.rowcount}, total now {cur_count(cur,'assignments')}")


def get_assignment_ids(cur):
    cur.execute("SELECT id FROM assignments LIMIT 10000")
    return [r[0] for r in cur.fetchall()]


def get_learner_ids(cur, n):
    cur.execute("SELECT id FROM users WHERE role='learner' ORDER BY id LIMIT %s", (n,))
    return [r[0] for r in cur.fetchall()]


def seed_submissions(cur, conn, assignment_ids, learner_ids):
    have = cur_count(cur, "submissions")
    print(f"[submissions] existing={have} target={TARGET_SUBMISSIONS}")
    if have >= TARGET_SUBMISSIONS:
        print("[submissions] SKIP")
        return
    need = TARGET_SUBMISSIONS - have
    rows = []
    for i in range(need):
        aid = random.choice(assignment_ids)
        uid = random.choice(learner_ids)
        content = random.choice(SUBMISSION_CONTENTS)
        score = round(random.uniform(60, 98), 2)
        status = random.choice(["pending", "auto_graded", "finalized"])
        fb = json.dumps({"score": score, "comment": "Good effort. Watch grammar."})
        rows.append((aid, uid, content, score, fb, score if status == "finalized" else None, status))
    cur.executemany(
        "INSERT INTO submissions (assignment_id, user_id, content, auto_score, auto_feedback, final_score, status) "
        "VALUES (%s,%s,%s,%s,%s,%s,%s)", rows)
    conn.commit()
    print(f"[submissions] inserted {cur.rowcount}, total now {cur_count(cur,'submissions')}")


def seed_lesson_plans(cur, conn, teacher_ids):
    have = cur_count(cur, "lesson_plans")
    print(f"[lesson_plans] existing={have} target={TARGET_LESSON_PLANS}")
    if have >= TARGET_LESSON_PLANS:
        print("[lesson_plans] SKIP")
        return
    need = TARGET_LESSON_PLANS - have
    rows = []
    for i in range(need):
        tid = random.choice(teacher_ids)
        topic = random.choice(LESSON_TOPICS)
        content = json.dumps({
            "objective": f"Students will understand {topic}",
            "duration": 45,
            "activities": ["warm-up", "presentation", "practice", "production", "review"],
            "materials": ["textbook", "handout", "whiteboard"],
        }, ensure_ascii=False)
        rows.append((tid, topic, content))
    cur.executemany(
        "INSERT INTO lesson_plans (teacher_id, topic, content) VALUES (%s,%s,%s)", rows)
    conn.commit()
    print(f"[lesson_plans] inserted {cur.rowcount}, total now {cur_count(cur,'lesson_plans')}")


def get_scenario_ids(cur):
    cur.execute("SELECT id FROM scenarios LIMIT 10000")
    return [r[0] for r in cur.fetchall()]


def seed_writing_tasks(cur, conn, learner_ids, scenario_ids):
    have = cur_count(cur, "writing_tasks")
    print(f"[writing_tasks] existing={have} target={TARGET_WRITING_TASKS}")
    if have >= TARGET_WRITING_TASKS:
        print("[writing_tasks] SKIP")
        return
    need = TARGET_WRITING_TASKS - have
    rows = []
    for i in range(need):
        uid = random.choice(learner_ids)
        sid = random.choice(scenario_ids)
        prompt = random.choice(ESSAY_PROMPTS)
        essay = random.choice(ESSAY_TEXTS)
        score = round(random.uniform(70, 95), 2)
        fb = json.dumps({"grammar": 8, "vocabulary": 7, "coherence": 8,
                         "overall": score, "comment": "Well structured."})
        rows.append((uid, prompt, essay, score, fb))
    cur.executemany(
        "INSERT INTO writing_tasks (user_id, prompt, essay, score, feedback) VALUES (%s,%s,%s,%s,%s)",
        rows)
    conn.commit()
    print(f"[writing_tasks] inserted {cur.rowcount}, total now {cur_count(cur,'writing_tasks')}")


def main():
    conn = connect()
    cur = conn.cursor()
    try:
        teacher_ids = ensure_teachers(cur, conn)
        seed_classes(cur, conn, teacher_ids)
        class_ids = get_class_ids(cur)
        seed_assignments(cur, conn, class_ids)
        assignment_ids = get_assignment_ids(cur)
        learner_ids = get_learner_ids(cur, 50000)
        seed_submissions(cur, conn, assignment_ids, learner_ids)
        seed_lesson_plans(cur, conn, teacher_ids)
        scenario_ids = get_scenario_ids(cur)
        seed_writing_tasks(cur, conn, learner_ids, scenario_ids)
        print("[ALL DONE]")
    finally:
        cur.close()
        conn.close()


if __name__ == "__main__":
    main()
