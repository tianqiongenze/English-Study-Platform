"""
ESP Spike 压测 — 突发流量(2-5x)验 HPA 扩容速度(P1-4)。
复用 locustfile 的用户类(SpeechUser/LearningEventUser/WritingUser/TeacherUser)+ mint_jwt,
但用 spawn-ramp 突发模型:短时间从 0→峰值再落,反复几次,看 HPA 是否扩容、扩容是否健康、
回落流量后 HPA 是否缩容。

单节点 Rancher Desktop 资源约束:峰值定 2500 CCU(5000 的 1/2,避免单节点 OOM),
突发模式:0→2500 in 30s,hold 90s,落 0,重复 3 轮。重点看扩容 lag 和失败率,
而非绝对吞吐(绝对吞吐单节点无意义,见架构评分 §三)。

运行(集群内 locust-runner pod):
  locust -f /locust/spike.py --headless -u 2500 -r 83 --run-time 7m \
      --host http://traefik.kube-system.svc.cluster.local:80 \
      --csv=report/spike --html=report/spike.html
  (-u 2500 -r 83: 约30s爬到峰值,run-time 覆盖3轮突发+间隔)
"""
from __future__ import annotations

import base64, hashlib, hmac, json, os, random
from locust import HttpUser, between, events, task

# 复用 locustfile 的鉴权与用户类
from locustfile import mint_jwt, SCENARIOS, SpeechUser, LearningEventUser, WritingUser, TeacherUser, ESPHttpUser  # noqa: F401,E402


@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    print(">>> SPIKE test: 0->2500 in 30s, hold 90s, drop, x3 rounds. Watch HPA + fail rate.")


@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    s = environment.stats.total
    p95 = s.get_response_time_percentile(0.95)
    print(f">>> SPIKE done. req={s.num_requests} fail={s.num_failures} "
          f"avg_rps={s.total_rps:.1f} p95_ms={p95:.1f} fail_rate={s.fail_ratio:.4f}")
