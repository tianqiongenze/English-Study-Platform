# Locust 压测说明

## 安装
```bash
pip install locust websocket-client
```

## 目标(G-01 5000 CCU + 各项 G 指标)

| 场景 | 用户类 | 目标 |
|---|---|---|
| 口语陪练 | SpeechUser | 3000 长连接在线 |
| 学情事件 | LearningEventUser | 吞吐 >= 5000 ev/s (G-06) |
| 写作批改 | WritingUser | >= 50 req/s (G-04) |

## 运行(分布式,模拟 5000 并发)

单机:
```bash
locust -f deploy/locust/locustfile.py --headless \
    -u 5000 -r 100 --run-time 5m \
    --host http://localhost:8080
```

分布式(master + workers,推荐突破单机 GIL):
```bash
# master
locust -f deploy/locust/locustfile.py --master --expect-workers 4
# worker x4
locust -f deploy/locust/locustfile.py --worker --master-host=<master-ip>
```

## 验收标准
- 5 分钟内 5000 CCU 稳定不掉线。
- `POST /api/learning/event` 202 成功率 >= 99%,吞吐 >= 5000 ev/s。
- `POST /api/writing/grade` 吞吐 >= 50 req/s,P95 < 2s。
- 各服务无 OOM/崩溃,Prometheus 监控正常。

## 报告
控制台结束打印聚合统计;Web UI(http://localhost:8089)可看实时曲线。
导出 CSV:加 `--csv=report/esp`。
