#!/usr/bin/env bash
# e2e-p1-8-idempotency.sh — P1-8 幂等端到端验证(学习事件经 Pulsar→worker→Disruptor→落库去重)
# 前置:① UNIQUE INDEX learning_events_message_id_key 已 public;② learning-worker 已恢复副本并 rollout 新镜像;
#      ③ 生产探针关闭(下方 PULSAR_TOPIC/REQ 前缀)。
# 原理:同 messageId 投递 2 次 → ON CONFLICT (message_id) DO NOTHING → learning_events 恰 1 行。
# 用法: bash scripts/e2e-p1-8-idempotency.sh
set -uo pipefail

NS=ekb-m2-aws-us-east-1
CRDB=esp-crdb
TOPIC=learning.event.report
PULSAR_PORT=6650
MID="p18-e2e-$(date +%s)"
USER_ID=99000001
PAYLOAD='{"page":"review","answer":"correct"}'
EVENT='{"messageId":"'"$MID"'","userId":'"$USER_ID"',"eventType":"practice_attempt","payload":"'"$PAYLOAD"'"}'

echo "==> messageId=$MID"
echo "==> [1/5] 向 Pulsar topic $TOPIC 投递同一事件 x2(python producer,避开 broker pod 640MiB cgroup 内 pulsar-client JVM OOM)"
# 连接走 service FQDN;broker pod 640MiB cgroup 偶发 OOM 重启致连接 flaky → 重试 6 次 x8s 背退
for attempt in 1 2 3 4 5 6; do
  OUT=$(kubectl -n $NS exec deploy/pulsar-broker -- env MID="$MID" UID2="$USER_ID" PLD="$PAYLOAD" python3 -c "
import pulsar, json, os
client = pulsar.Client('pulsar://pulsar-broker:$PULSAR_PORT')
producer = client.create_producer('$TOPIC')
ev = json.dumps({'messageId': os.environ['MID'], 'userId': int(os.environ['UID2']),
                 'eventType': 'practice_attempt', 'payload': os.environ['PLD']})
for i in range(2):
    producer.send(ev.encode('utf-8'))
producer.close(); client.close()
print('SENT_2_OK', os.environ['MID'])
" 2>&1 | grep -viE "^\s+at | INFO ")
  if echo "$OUT" | grep -q "SENT_2_OK"; then
    echo "    produce attempt=$attempt OK"
    break
  else
    echo "    produce attempt=$attempt failed: $(echo "$OUT" | tail -1)"
    [ "$attempt" = 6 ] && { echo "PRODUCE_FAILED_6_ATTEMPTS"; exit 1; }
    sleep 8
  fi
done

echo "==> [2/5] 等 worker 消费落库(Shared 去重 + Disruptor 批量,给 60s)"
sleep 60

echo "==> [3/5] 查 learning_events 落库行数(message_id 精确匹配)"
kubectl -n $CRDB exec crdb-0 -- ./cockroach sql --insecure --host=crdb-public:26257 \
  -d esp_learning -e "SELECT COUNT(*) AS cnt FROM learning_events WHERE message_id='$MID';" 2>&1 | tail -3

echo "==> [4/5] 兜底延迟查询(若 60s 未落,再等 30s 复查)"
sleep 30
kubectl -n $CRDB exec crdb-0 -- ./cockroach sql --insecure --host=crdb-public:26257 \
  -d esp_learning -e "SELECT COUNT(*) AS cnt2 FROM learning_events WHERE message_id='$MID';" 2>&1 | tail -3

echo "==> [5/5] outbox 兜底残留检查(应 0 行:本轮 RingBuffer 未满,DROP-fallback 不应触发)"
kubectl -n $CRDB exec crdb-0 -- ./cockroach sql --insecure --host=crdb-public:26257 \
  -d esp_learning -e "SELECT COUNT(*) AS obx FROM learning_event_outbox WHERE message_id='$MID';" 2>&1 | tail -3

echo "==> 判读:cnt/cnt2 均应=1(2 次投递→1 行);obx=0。若 cnt=2 则 ON CONFLICT 未生效,查 worker 日志。"
