"""Seed esp database with test data for 5000-CCU load test.

Idempotent: uses INSERT IGNORE / safe upserts so re-running is harmless.
- scenarios: airport / interview / cafe (+ extras)
- knowledge_points: 100 KP
- users: 5000 learners + 50 teachers
- classes / assignments / submissions: minimal for teacher endpoints
"""
from __future__ import annotations
import pymysql

HOST = "127.0.0.1"
PORT = 14000
USER = "root"
DB = "esp"


def main() -> None:
    c = pymysql.connect(host=HOST, port=PORT, user=USER, database=DB, autocommit=False)
    cur = c.cursor()

    # --- scenarios ---
    scenarios = [
        ("airport", "Airport Check-in", "You are an airport check-in agent. Help the user check in. Be polite.", 3, "en"),
        ("interview", "Job Interview", "You are an interviewer for a tech company. Ask about experience.", 4, "en"),
        ("cafe", "Coffee Shop Ordering", "You are a friendly barista. Help the user order coffee.", 2, "en"),
        ("hotel", "Hotel Check-in", "You are a hotel receptionist. Help with check-in and room options.", 3, "en"),
        ("restaurant", "Restaurant Ordering", "You are a waiter. Help the user order from the menu.", 3, "en"),
        ("shopping", "Shopping for Clothes", "You are a shop assistant. Help find sizes and styles.", 2, "en"),
    ]
    for code, title, sys, diff, lang in scenarios:
        cur.execute(
            "INSERT INTO scenarios (code,title,system_prompt,difficulty,lang) "
            "VALUES (%s,%s,%s,%s,%s) ON DUPLICATE KEY UPDATE title=VALUES(title), system_prompt=VALUES(system_prompt)",
            (code, title, sys, diff, lang),
        )
    c.commit()
    print(f"scenarios: {len(scenarios)} upserted")

    # --- knowledge_points: 100 ---
    for i in range(1, 101):
        cur.execute(
            "INSERT INTO knowledge_points (code,title,parent_id,difficulty) "
            "VALUES (%s,%s,NULL,%s) ON DUPLICATE KEY UPDATE title=VALUES(title)",
            (f"kp{i:03d}", f"Knowledge Point {i}", (i % 5) + 1),
        )
    c.commit()
    print("knowledge_points: 100 upserted")

    # --- users: 5000 learners + 50 teachers ---
    cur.execute("SELECT COUNT(*) FROM users")
    existing = cur.fetchone()[0]
    if existing < 5050:
        rows = []
        for i in range(1, 5051):
            role = "teacher" if i > 5000 else "learner"
            name = f"user{i}"
            email = f"user{i}@esp.test"
            rows.append((role, name, email, "x"))  # pwd_hash placeholder
        cur.executemany(
            "INSERT IGNORE INTO users (role,name,email,pwd_hash) VALUES (%s,%s,%s,%s)",
            rows,
        )
        c.commit()
    cur.execute("SELECT COUNT(*) FROM users")
    print(f"users: {cur.fetchone()[0]}")

    # --- classes (teacher 5001..5050) ---
    cur.execute("SELECT COUNT(*) FROM classes")
    if cur.fetchone()[0] == 0:
        cur.executemany(
            "INSERT INTO classes (teacher_id,name) VALUES (%s,%s)",
            [(5000 + t, f"Class-{t}") for t in range(1, 51)],
        )
        c.commit()
    cur.execute("SELECT COUNT(*) FROM classes")
    print(f"classes: {cur.fetchone()[0]}")

    # --- assignments (1 per class) ---
    cur.execute("SELECT COUNT(*) FROM assignments")
    if cur.fetchone()[0] == 0:
        cur.executemany(
            "INSERT INTO assignments (class_id,type,content) VALUES (%s,'written','Write 150 words on a given topic.')",
            [(i,) for i in range(1, 51)],
        )
        c.commit()
    cur.execute("SELECT COUNT(*) FROM assignments")
    print(f"assignments: {cur.fetchone()[0]}")

    # --- user_mastery: seed baseline for 500 learners x 100 KP (50k rows) ---
    cur.execute("SELECT COUNT(*) FROM user_mastery")
    if cur.fetchone()[0] == 0:
        batch = []
        for u in range(1, 501):
            for k in range(1, 101):
                batch.append((u, k, (u * k) % 100))
                if len(batch) >= 5000:
                    cur.executemany(
                        "INSERT IGNORE INTO user_mastery (user_id,kp_id,mastery_score,review_count) VALUES (%s,%s,%s,0)",
                        batch,
                    )
                    batch.clear()
        if batch:
            cur.executemany(
                "INSERT IGNORE INTO user_mastery (user_id,kp_id,mastery_score,review_count) VALUES (%s,%s,%s,0)",
                batch,
            )
        c.commit()
    cur.execute("SELECT COUNT(*) FROM user_mastery")
    print(f"user_mastery: {cur.fetchone()[0]}")

    cur.close()
    c.close()
    print("SEED DONE")


if __name__ == "__main__":
    main()
