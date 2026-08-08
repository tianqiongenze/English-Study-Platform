#!/usr/bin/env bash
# 冒烟测试: 灌 1000 行 users + 1000 行 learning_events,验证脚本和 TiDB 写入性能
set -e
kubectl -n esp-prod run seed-smoke-$RANDOM --rm -i --restart=Never \
  --image=python:3.12-slim --timeout=300s \
  --env="TIDB_HOST=tidb-primary" --env="REDIS_HOST=redis-cluster" \
  --command -- bash -lc '
pip install -i https://pypi.tuna.tsinghua.edu.cn/simple pymysql redis >/tmp/pi.log 2>&1
python3 <<"PY"
import time,pymysql
c=pymysql.connect(host="tidb-primary",port=4000,user="root",database="esp",autocommit=True,read_timeout=120,write_timeout=120)
cur=c.cursor()
# 测 users 1000 行
t0=time.time()
sql="INSERT IGNORE INTO users(name,email,pwd_hash) VALUES (%s,%s,%s)"
batch=[("testuser_"+str(i),f"sm{i}@esp.test","x"*32) for i in range(1000)]
cur.executemany(sql,batch)
dt=time.time()-t0
print(f"users 1000 rows: {dt:.2f}s -> {1000/dt:.0f} rows/s")
# 测 learning_events 1000 行
t0=time.time()
import json,random
sql2="INSERT INTO learning_events(user_id,event_type,payload,created_at) VALUES (%s,%s,%s,NOW())"
batch2=[(random.randint(1,7),"login",json.dumps({"x":1})) for _ in range(1000)]
cur.executemany(sql2,batch2)
dt2=time.time()-t0
print(f"learning_events 1000 rows: {dt2:.2f}s -> {1000/dt2:.0f} rows/s")
print("SMOKE OK")
PY
' 2>&1 | grep -vE "^(pip|Collecting|Downloading|Installing|Successfully|WARNING|DEPRECATION|[notice]|.*new release|.*To update:|^\s*$|All commands|If you don)" | tail -15
