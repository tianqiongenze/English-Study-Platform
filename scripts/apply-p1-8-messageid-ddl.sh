#!/usr/bin/env bash
# apply-p1-8-messageid-ddl.sh — P1-8 learning_events 幂等列 DDL(幂等,可重跑)
# 背景:后台合并执行(ADD COLUMN + CREATE UNIQUE INDEX)在 backfill 阶段卡死(24%),
#      已 CANCEL(job 1201742016749142017,回滚中)。本脚本拆句执行,并用 num_replicas=1
#      临时降副本加速 UNIQUE INDEX backfill(复用 sessions 重建提速法,坑㉚ 方案)。
#
# 前置:等上一 schema change 回滚完成(revert 结束)再跑,否则同表 schema change 排队。
# 用法: kubectl -n esp-crdb cp scripts/apply-p1-8-messageid-ddl.sh crdb-0:/tmp/ && \
#        kubectl -n esp-crdb exec crdb-0 -- sh /tmp/apply-p1-8-messageid-ddl.sh
set -euo pipefail
SQL="cockroach sql --insecure --host=crdb-public:26257"

echo "==> [1/6] checkpoint 间隔 10s(进度可视,不影响速度)"
$SQL -e "SET CLUSTER SETTING bulkio.index_backfill.checkpoint_interval='10s';" || true

echo "==> [2/6] learning_events 临时降副本 num_replicas=1(backfill 提速 3x,完成后恢复)"
$SQL -d esp_learning -e "ALTER TABLE learning_events CONFIGURE ZONE USING num_replicas=1;"

echo "==> [3/6] ADD COLUMN message_id(元数据操作,不含回填,应秒级完成)"
$SQL -d esp_learning -e "ALTER TABLE learning_events ADD COLUMN IF NOT EXISTS message_id STRING;"

echo "==> [4/6] CREATE UNIQUE INDEX(唯一约束 backfill,本轮在 1 副本下执行)"
$SQL -d esp_learning -e "CREATE UNIQUE INDEX IF NOT EXISTS learning_events_message_id_key ON learning_events (message_id);"

echo "==> [5/6] 恢复 num_replicas=3(副本约束回滚到 z1/z2/z3 各 1)"
$SQL -d esp_learning -e "ALTER TABLE learning_events CONFIGURE ZONE USING num_replicas=3, constraints='{+zone=z1:1,+zone=z2:1,+zone=z3:1}', lease_preferences='[[+zone=z1]]';"

echo "==> [6/6] outbox 表加 message_id 列(无唯一约束,元数据操作)"
$SQL -d esp_learning -e "ALTER TABLE learning_event_outbox ADD COLUMN IF NOT EXISTS message_id STRING;"

echo "==> 验证"
$SQL -d esp_learning -e "SHOW COLUMNS FROM learning_events;"
$SQL -d esp_learning -e "SHOW INDEXES FROM learning_events;"
echo "DONE"
