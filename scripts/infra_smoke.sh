#!/usr/bin/env bash
# 平台底座 6 组件连通冒烟测试(单节点验证)
# 用法: scripts/infra_smoke.sh [tidb|redis|pulsar|milvus|neo4j|all]
# 输出落 report/infra-smoke-<ts>.log,主线程只看摘要
set -e
COMP="${1:-all}"
TS=$(date +%Y%m%d-%H%M%S 2>/dev/null || echo "manual")
mkdir -p report
LOG="report/infra-smoke-${TS}.log"
PASS=0; FAIL=0
K="rdctl shell sudo kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml"

probe() {
  local name="$1"; local cmd="$2"
  echo "--- $name ---" | tee -a "$LOG"
  if echo "$cmd" | sh 2>&1 | tee -a "$LOG" | grep -qE "OK|1|true|empty|RESULT"; then
    echo "✅ $name PASS" | tee -a "$LOG"; PASS=$((PASS+1))
  else
    echo "❌ $name FAIL" | tee -a "$LOG"; FAIL=$((FAIL+1))
  fi
}

# TiDB: SELECT 1
[ "$COMP" = "tidb" ] || [ "$COMP" = "all" ] && {
  probe "tidb-ping" "$K -n esp-prod run tidb-q-\$RANDOM --rm -i --restart=Never --image=python:3.12-slim --timeout=120s --command -- bash -lc 'pip install -q -i https://pypi.tuna.tsinghua.edu.cn/simple pymysql 2>&1|tail -1; python3 -c \"import pymysql;c=pymysql.connect(\\\"tidb-primary\\\",4000,\\\"root\\\",\\\"esp\\\");cur=c.cursor();cur.execute(\\\"SELECT 1\\\");print(cur.fetchone()[0])\"'"
}

# Redis: SET/GET esp:cache:smoke
[ "$COMP" = "redis" ] || [ "$COMP" = "all" ] && {
  probe "redis-set-get" "$K -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c SET esp:cache:smoke OK EX 60"
  probe "redis-get" "$K -n ekb-m2-aws-us-east-1 exec redis-cluster-new-0 -- redis-cli -c GET esp:cache:smoke"
}

# Pulsar: broker up
[ "$COMP" = "pulsar" ] || [ "$COMP" = "all" ] && {
  probe "pulsar-broker-up" "$K -n ekb-m2-aws-us-east-1 get deploy pulsar-broker -o jsonpath='{.status.readyReplicas}'"
}

# Milvus: pod Ready + pymilvus 连通
[ "$COMP" = "milvus" ] || [ "$COMP" = "all" ] && {
  probe "milvus-pod-ready" "$K -n ekb-m2-aws-us-east-1 get deploy milvus -o jsonpath='{.status.readyReplicas}'"
  probe "milvus-pymilvus-connect" "$K -n esp-prod run tmp-mil-\$RANDOM --rm -i --restart=Never --image=python:3.12-slim --timeout=120s --command -- bash -lc 'pip install -q -i https://pypi.tuna.tsinghua.edu.cn/simple pymilvus 2>/dev/null; python3 -c \"from pymilvus import connections,utility;connections.connect(host=\\\"milvus.ekb-m2-aws-us-east-1.svc.cluster.local\\\",port=19530,timeout=15);print(\\\"collections:\\\",utility.list_collections() or \\\"(empty)\\\")\"'"
}

# Neo4j: pod Ready + HTTP RETURN 1(社区版,外部 pod HTTP API)
[ "$COMP" = "neo4j" ] || [ "$COMP" = "all" ] && {
  probe "neo4j-pod-ready" "$K -n ekb-m2-aws-us-east-1 get deploy neo4j -o jsonpath='{.status.readyReplicas}'"
  probe "neo4j-http-query" "$K -n esp-prod run tmp-neo-\$RANDOM --rm -i --restart=Never --image=python:3.12-slim --timeout=90s --command -- bash -lc 'python3 -c \"import urllib.request,json,base64;a=base64.b64encode(b\\\"neo4j:neo4j-password-1\\\").decode();h={\\\"Content-Type\\\":\\\"application/json\\\",\\\"Authorization\\\":\\\"Basic \\\"+a};u=\\\"http://neo4j.ekb-m2-aws-us-east-1.svc.cluster.local:7474/db/neo4j/tx/commit\\\";b=json.dumps({\\\"statements\\\":[{\\\"statement\\\":\\\"RETURN 1 AS n\\\"}]}).encode();r=urllib.request.urlopen(urllib.request.Request(u,data=b,headers=h),timeout=20);d=json.loads(r.read());print(\\\"RESULT\\\",d[\\\"results\\\"][0][\\\"data\\\"][0][\\\"row\\\"])\"'"
}

echo "=== SMOKE RESULT: PASS=$PASS FAIL=$FAIL ===" | tee -a "$LOG"
echo "log: $LOG"
[ "$FAIL" = "0" ]
