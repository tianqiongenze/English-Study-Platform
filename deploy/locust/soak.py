"""
ESP Soak 长稳测试 harness(P1-4)。
目标:8h 持续负载查内存泄漏/GC 抖动/连接泄漏。单节点 + 会话长度约束,本 harness 跑
短版(可配,默认用环境变量 SOAK_TIME 覆盖),完整 8h 留多节点环境。

复用 locustfile 用户类,稳态负载:中等 CCU(1000)持续,关注长尾延迟漂移和失败率累积。
"""
from __future__ import annotations
from locust import events
from locustfile import SpeechUser, LearningEventUser, WritingUser, TeacherUser  # noqa: F401

import os
SOAK_MINUTES = int(os.environ.get("SOAK_TIME", "20"))  # 默认 20min 短版;多节点跑 SOAK_TIME=480

@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    print(f">>> SOAK test: steady 1000 CCU for {SOAK_MINUTES}min. Watch latency drift + mem leak.")

@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    s = environment.stats.total
    p95 = s.get_response_time_percentile(0.95)
    print(f">>> SOAK done. req={s.num_requests} fail={s.num_failures} "
          f"avg_rps={s.total_rps:.1f} p95_ms={p95:.1f} fail_rate={s.fail_ratio:.4f}")
