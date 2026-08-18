#!/usr/bin/env bash
# 查 ESP 数据封装(铁律十二 WTV:先定目标再查询)。
# 用法:
#   scripts/db_query.sh "SQL"                 # 默认 CRDB(esp 库,PG 协议) —— 服务当前真实存储(users/auth/学习业务等)
#   scripts/db_query.sh -db esp_learning "SQL"   # 指定库;跨 schema 带前缀:SELECT * FROM esp_auth.users
# 目标解释(2026-08-17 用户指令):TiDB 已彻底下电(scale 0,禁连);CRDB(crdb-public:26257)为唯一存储。
set -e
TARGET=crdb
SQL=""
DB="esp"
set -e
TARGET=crdb
SQL=""
DB="esp"
while [ $# -gt 0 ]; do
  case "$1" in
    -t) TARGET="$2"; shift 2 ;;
    -db) DB="$2"; shift 2 ;;
    *) SQL="$1"; shift ;;
  esac
done
[ -n "$SQL" ] || { echo "usage: db_query.sh [-t crdb] [-db <db|schema>] \"SQL\"" >&2; exit 1; }
RUNNER="dbq-$RANDOM"

kubectl -n esp-prod run "$RUNNER" --rm -i --restart=Never \
  --image=python:3.12-slim --timeout=180s \
  --env="ESP_SQL=$SQL" --env="ESP_DB=$DB" --env="ESP_TARGET=$TARGET" \
  --command -- bash -lc 'set -e
if [ "$ESP_TARGET" = "crdb" ]; then
  pip install --quiet -i https://pypi.tuna.tsinghua.edu.cn/simple psycopg2-binary 2>&1 | tail -1
else
  pip install --quiet -i https://pypi.tuna.tsinghua.edu.cn/simple pymysql 2>&1 | tail -1
fi
cat > /tmp/q.py <<"PYEOF"
import os
t=os.environ["ESP_TARGET"]
if t=="crdb":
    import psycopg2
    c=psycopg2.connect(dbname=os.environ["ESP_DB"],host="crdb-public",port=26257,user="root",sslmode="disable",connect_timeout=30)
    c.autocommit=True
else:
    import pymysql
    c=pymysql.connect(host="tidb-primary",port=4000,user="root",database=os.environ["ESP_DB"],read_timeout=90,write_timeout=90)
cur=c.cursor()
cur.execute(os.environ["ESP_SQL"])
rows=cur.fetchall()
if len(rows)==1 and len(rows[0])==1:
    print(rows[0][0])
else:
    for r in rows: print(r)
PYEOF
python3 /tmp/q.py' 2>&1 | grep -vE "^(pip|Collecting|Downloading|Installing|Successfully|WARNING|DEPRECATION|[notice]|.*new release of pip|.*To update:|^\s*$|pod .* (created|deleted|terminated)|If you don.*see a command prompt|All commands and output)" | tail -60