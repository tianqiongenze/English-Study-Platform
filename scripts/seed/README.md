# scripts/seed — 种子数据脚本

灌入 TiDB(esp 库)大批量测试数据,供压测/分析。进度写 Redis(`esp:ctx:seed:*`),可断点续跑。

## 脚本
- `seed_learning_events.py` —— `learning_events` 表(目标 30 亿行,默认 `--target 3000000000`)。每批 20000 行,每 5 批 commit,进度写 `esp:ctx:seed:learning_events`。
- `seed_session_turns.py` —— `session_turns` 表(目标 3 亿行)。进度写 `esp:ctx:seed:session_turns`。

## 执行方式(集群内 pod 直连 tidb-primary:4000)
脚本需在能解析 `tidb-primary` 与 `redis-cluster` 短名的 pod 里跑(即 esp-prod ns)。用临时 pod:
```bash
kubectl -n esp-prod run seed-job --rm -i --restart=Never \
  --image=python:3.12-slim --timeout=0 \
  -- sh -c 'pip install -q pymysql redis && python3 - <<PY
<脚本内容>
PY'
```
或把脚本挂进 ConfigMap 后 exec。

## 现实约束(单节点 Rancher Desktop)
30 亿行在单节点无法短时间灌完。脚本持续推进,Redis 记录实际进度;主线程按可接受时长(如 10~30 分钟)决定停点,如实记录"已灌 N 行"而非"30 亿已灌完"。生产多节点推算见测试报告 03。

## 查进度
```bash
bash scripts/ctx_state.sh get seed:learning_events
bash scripts/ctx_state.sh get seed:session_turns
```
