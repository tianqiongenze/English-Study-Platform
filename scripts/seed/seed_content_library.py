#!/usr/bin/env python3
"""
ESP 统一内容库种子灌装器(功能拓展阶段1)
==========================================
向 CRDB esp_content 库灌装 课程→课时→内容条目:
  courses / lessons / content_items

内容数据为数据驱动:自动发现 data/<course>_<skill>.py 模块并加载
  <course>_vocab.py    → WORDS     [(word, phonetic, pos, meaning_en, meaning_zh, example, cefr, difficulty)]
  <course>_reading.py  → READINGS  [{title, cefr, text, question, options, correct, explanation, source}]
  <course>_listening.py→ LISTENINGS[{title, cefr, script, question, options, correct, explanation, source}]
  <course>_grammar.py  → GRAMMAR   [{question, options, correct, explanation, cefr, difficulty, source}]
课程码 = 文件名前缀(cet4/cet6/toefl/ielts/textbook)。
幂等:课程按 code、课时按 (course,skill)、条目按 (lesson_id, 自然键 word/title/question) 先查后插,可重复跑。

运行方式:集群内 pod 或 kubectl port-forward 后本机,
环境变量 CRDB_HOST/CRDB_PORT(默认 crdb-public:26257, user root, sslmode=disable)。
版权约束:内容为原创/改写/公开领域,source/license 由数据模块标注。
"""
import importlib
import importlib.util
import logging
import os
import pkgutil
import sys
import time

import psycopg2
from psycopg2.extras import execute_values

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("seed_content")

CRDB_HOST = os.environ.get("CRDB_HOST", "crdb-public")
CRDB_PORT = int(os.environ.get("CRDB_PORT", "26257"))
DATA_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "data")

COURSES = {
    "cet4":  ("大学英语四级 CET-4", "CET4",  "A1-B1"),
    "cet6":  ("大学英语六级 CET-6", "CET6",  "B1-B2"),
    "toefl": ("托福 TOEFL",         "TOEFL", "B1-C1"),
    "ielts": ("雅思 IELTS",         "IELTS", "A2-C1"),
    "textbook": ("大学英语教材",     "GENERAL", "A1-B2"),
}
SKILL_SEQ = {"vocab": 1, "reading": 2, "listening": 3, "grammar": 4}
SKILL_TITLE = {"vocab": "核心词汇", "reading": "阅读理解", "listening": "听力理解", "grammar": "语法与词汇"}


class RobustCursor:
    """带自动重连+重试的 psycopg2 游标包装(对齐 seed_crdb_full.RobustCursor)。"""

    def __init__(self, db):
        self.conn = self._connect(db)
        self.cur = self.conn.cursor()

    def _connect(self, db):
        return psycopg2.connect(host=CRDB_HOST, port=CRDB_PORT, user="root",
                                dbname=db, sslmode="disable", connect_timeout=10)

    def _reconnect(self, db):
        try:
            self.conn.close()
        except Exception:
            pass
        self.conn = self._connect(db)
        self.cur = self.conn.cursor()

    def execute(self, sql, args=None, max_retry=5):
        for i in range(max_retry):
            try:
                self.cur.execute(sql, args)
                return
            except (psycopg2.OperationalError, psycopg2.InterfaceError) as e:
                log.warning("retry %d/%d after %s: %s", i + 1, max_retry, e, sql[:60])
                time.sleep(1 * (i + 1))
                self._reconnect("esp_content")
        raise RuntimeError(f"db unreachable: {sql[:60]}")

    def commit(self):
        self.conn.commit()

    def close(self):
        try:
            self.conn.close()
        except Exception:
            pass


def load_data_modules():
    """发现 data/*.py,返回 {course_code: {skill: module}}。"""
    found = {}
    for mod in pkgutil.iter_modules([DATA_DIR]):
        if not any(mod.name.endswith("_" + s) for s in SKILL_SEQ):
            continue
        course, _, skill = mod.name.rpartition("_")
        spec = importlib.util.spec_from_file_location(mod.name, os.path.join(DATA_DIR, mod.name + ".py"))
        m = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(m)
        found.setdefault(course, {})[skill] = m
    return found


def course_id(db, code):
    db.execute("SELECT id FROM courses WHERE code=%s", (code,))
    row = db.cur.fetchone()
    if row:
        return row[0]
    title, tag, cefr = COURSES[code]
    db.execute("INSERT INTO courses (code, title, exam_tag, cefr_range, status) "
               "VALUES (%s,%s,%s,%s,'published') RETURNING id", (code, title, tag, cefr))
    return db.cur.fetchone()[0]


def lesson_id(db, cid, skill):
    db.execute("SELECT id FROM lessons WHERE course_id=%s AND skill=%s", (cid, skill))
    row = db.cur.fetchone()
    if row:
        return row[0]
    db.execute("INSERT INTO lessons (course_id, seq, title, skill, status) "
               "VALUES (%s,%s,%s,%s,'published') RETURNING id",
               (cid, SKILL_SEQ[skill], SKILL_TITLE[skill], skill))
    return db.cur.fetchone()[0]


def item_exists(db, lid, keycol, key):
    db.execute(f"SELECT id FROM content_items WHERE lesson_id=%s AND prompt->>'{keycol}'=%s", (lid, key))
    return db.cur.fetchone() is not None


def upsert_vocab(db, lid, course, words):
    created = dup = 0
    vals = []
    for word, ph, pos, men, mzh, ex, cefr, diff in words:
        if item_exists(db, lid, "word", word):
            dup += 1
            continue
        prompt = {"word": word, "phonetic": ph, "pos": pos, "meaning_en": men, "meaning_zh": mzh}
        answer = {"meaning": mzh, "meaning_en": men}
        vals.append((lid, "vocab", psycopg2.extras.Json(prompt), psycopg2.extras.Json(answer),
                     ex, diff, cefr, [f"exam:{course}"], "esp-curated", "esp-original"))
        created += 1
    if vals:
        execute_values(db.cur,
            "INSERT INTO content_items (lesson_id,item_type,prompt,answer,explanation,difficulty,cefr,tags,source,license) VALUES %s",
            vals, template="(%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)")
    return created, dup


def upsert_skill(db, lid, course, skill, items):
    created = dup = 0
    for it in items:
        if skill in ("reading", "listening"):
            keycol, key = "title", it["title"]
            prompt = {"title": it["title"], "cefr": it.get("cefr", ""),
                      "text": it.get("text", ""), "script": it.get("script", ""),
                      "question": it["question"], "options": it["options"]}
        else:
            keycol, key = "question", it["question"]
            prompt = {"question": it["question"], "options": it["options"]}
        if item_exists(db, lid, keycol, key):
            dup += 1
            continue
        db.execute(
            "INSERT INTO content_items (lesson_id,item_type,prompt,answer,explanation,difficulty,cefr,tags,source,license) "
            "VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)",
            (lid, skill, psycopg2.extras.Json(prompt), psycopg2.extras.Json({"correct_option": it["correct"]}),
             it.get("explanation", ""), it.get("difficulty", 3), it.get("cefr", ""),
             [f"exam:{course}"], it.get("source", "esp-curated"), it.get("license", "esp-original")))
        created += 1
    return created, dup


def main():
    if not os.path.isdir(DATA_DIR):
        log.error("data dir missing: %s", DATA_DIR)
        sys.exit(2)
    data = load_data_modules()
    if not data:
        log.error("no data modules found under %s", DATA_DIR)
        sys.exit(2)

    db = RobustCursor("esp_content")
    total_created = total_dup = 0
    try:
        for course in sorted(data):
            cid = course_id(db, course)
            for skill in sorted(data[course], key=lambda s: SKILL_SEQ[s]):
                mod = data[course][skill]
                lid = lesson_id(db, cid, skill)
                if skill == "vocab":
                    created, dup = upsert_vocab(db, lid, course, mod.WORDS)
                else:
                    items = getattr(mod, {"reading": "READINGS", "listening": "LISTENINGS", "grammar": "GRAMMAR"}[skill])
                    created, dup = upsert_skill(db, lid, course, skill, items)
                db.commit()
                total_created += created
                total_dup += dup
                log.info("course=%s skill=%s items_created=%d dup_skip=%d", course, skill, created, dup)
        db.commit()
    finally:
        db.close()
    log.info("DONE: total_created=%d dup_skip=%d", total_created, total_dup)


if __name__ == "__main__":
    main()
