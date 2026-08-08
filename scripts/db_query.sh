#!/usr/bin/env bash
# 查 TiDB 的封装(集群内 pymysql pod)。用法: scripts/db_query.sh "SQL"
# 例: scripts/db_query.sh "select count(*) from learning_events"
set -e
SQL="${1:?usage: db_query.sh \"SQL\"}"
RUNNER="dbq-$RANDOM"

# 把 SQL 写进一个 python 脚本,通过 ConfigMap 挂载太重,改用 env 传 SQL 最稳。
kubectl -n esp-prod run "$RUNNER" --rm -i --restart=Never \
  --image=python:3.12-slim --timeout=180s \
  --env="ESP_SQL=$SQL" \
  --command -- bash -lc '
set -e
pip install --quiet -i https://pypi.tuna.tsinghua.edu.cn/simple pymysql 2>&1 | tail -1
python3 -c "
import os,pymysql
c=pymysql.connect(host=\"tidb-primary\",port=4000,user=\"root\",database=\"esp\",read_timeout=90,write_timeout=90)
cur=c.cursor()
cur.execute(os.environ[\"ESP_SQL\"])
rows=cur.fetchall()
if len(rows)==1 and len(rows[0])==1:
    print(rows[0][0])
else:
    for r in rows: print(r)
"
' 2>&1 | grep -vE "^(pip|Collecting|Downloading|Installing|Successfully|WARNING|DEPRECATION|[notice]|.*new release of pip|.*To update:|^\s*$|pod .* (created|deleted|terminated)|If you don.*see a command prompt|All commands and output)" | tail -60
