#!/usr/bin/env python3
"""
CRDB 功能拓展 schema 应用器:只应用 scripts/crdb-schema.sql 的
"feature-expansion(功能拓展)" 区段(216 行起至文件尾),对已存在对象幂等跳过。
连接:CRDB_HOST/CRDB_PORT(默认 crdb-public:26257, root, sslmode=disable)。
"""
import logging
import os
import re
import sys
import time

import psycopg2

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("apply_schema")

CRDB_HOST = os.environ.get("CRDB_HOST", "crdb-public")
CRDB_PORT = int(os.environ.get("CRDB_PORT", "26257"))
SCHEMA = os.environ.get("SCHEMA_PATH") or os.path.join(os.path.dirname(os.path.abspath(__file__)), "crdb-schema.sql")


def connect():
    for i in range(8):
        try:
            return psycopg2.connect(host=CRDB_HOST, port=CRDB_PORT, user="root",
                                    dbname="root", sslmode="disable", connect_timeout=10)
        except psycopg2.OperationalError as e:
            log.warning("connect retry %d/8: %s", i + 1, e)
            time.sleep(2 * (i + 1))
    raise RuntimeError("CRDB unreachable")


def extract_section(path):
    lines = open(path).read().splitlines()
    start = None
    for i, ln in enumerate(lines):
        if "-- feature-expansion" in ln:
            start = i
            break
    if start is None:
        raise SystemExit("feature-expansion section not found in schema")
    body = "\n".join(lines[start:])
    body = re.sub(r"^--.*$", "", body, flags=re.M)   # 去行注释
    return [s.strip() for s in body.split(";") if s.strip()]


def main():
    stmts = extract_section(SCHEMA)
    log.info("statements to apply: %d", len(stmts))
    conn = connect()
    conn.autocommit = True
    cur = conn.cursor()
    ok = skip = fail = 0
    for s in stmts:
        try:
            cur.execute(s)
            ok += 1
            log.info("OK  %s", s.splitlines()[0][:70])
        except psycopg2.errors.DuplicateTable as e:
            skip += 1
            log.info("SKIP already exists: %s (%s)", s.splitlines()[0][:50], e.diag.message_primary or "")
        except psycopg2.errors.DuplicateObject as e:
            skip += 1
            log.info("SKIP already exists: %s (%s)", s.splitlines()[0][:50], e.diag.message_primary or "")
        except psycopg2.errors.DuplicateColumn as e:
            skip += 1
            log.info("SKIP column exists: %s (%s)", s.splitlines()[0][:50], e.diag.message_primary or "")
        except psycopg2.Error as e:
            fail += 1
            log.error("FAIL %s: %s", s.splitlines()[0][:70], e)
    cur.close()
    conn.close()
    log.info("DONE ok=%d skip=%d fail=%d", ok, skip, fail)
    sys.exit(1 if fail else 0)


if __name__ == "__main__":
    main()
