"""
p1_d10_hpa_staircase.py — P1-D10 HPA/容量阶梯压测(RWT 铁律)。

目标:验证 HPA 在阶梯负载(100→500→1000→2000→5000 CCU)下的扩缩容响应时间 +
失败率 + AI 长尾影响。与 spike.py(突发)不同,本脚本用 Locust LoadTestShape 做
确定性阶梯 ramp-up:每档爬升后稳态 2 分钟采 HPA 数据,便于对照"扩容触发延迟"。

复用 locustfile.py 的用户类(SpeechUser/LearningEventUser/WritingUser/TeacherUser)
+ mint_jwt(RS256 鉴权),与全量基准同条件,只换负载模型。

== 单节点限制(HSA 铁律,必读)==
单节点 Rancher Desktop 9 核 VM 无法真正测水平扩展:HPA 扩到 N 副本但都挤同一节点,
CPU/内存共享同一物理资源,N 副本只是"切片"不是"倍增"。因此单节点 P1-D10 只验证:
  ① HPA 是否按阶梯触发扩容(CPU 超 target 60-70% → replicas 增长);
  ② 扩容后失败率是否下降(扩容有效性的唯一可观测信号);
  ③ AI 长尾(>10s)占比随 CCU 变化。
绝对吞吐 / 5000 CCU 真实承载 = DEFERRED 多节点(见 plan 文档 §单节点限制)。

== 阶梯设计 ==
STAGES = [(100, 60s), (500, 120s), (1000, 120s), (2000, 120s), (5000, 120s)]
  - 每档: ramp 30s 爬到目标 CCU + 稳态采 HPA(剩余时长)。
  - 5000 档单节点很可能 OOM/CPU 饱和,记录失败率拐点即停(不追求"通过")。
  - 环境变量覆盖:STAIR_USERS / STAIR_HOLD(稳态秒)/ STAIR_RAMP(爬升秒)。

== 运行(集群内 locust-runner pod,主线程串行调度,不与本脚本并行)==
  locust -f /locust/p1_d10_hpa_staircase.py --headless \
      --host http://traefik.kube-system.svc.cluster.local:80 \
      --csv=report/p1d10 --html=report/p1d10.html
  (实际用户数由 LoadTestShape 控制,-u/-r 被 shape 覆盖,无需传)

== 配套 ==
  scripts/p1-d10-hpa-watch.sh 每 10s 采 kubectl get hpa + top pod 到
  report/loadtest/hpa-snapshots.log,与本脚本时间轴对齐看扩容 lag。
"""
from __future__ import annotations

import os
import time

from locust import LoadTestShape, events
# 复用 locustfile 的鉴权与用户类(同 locustfile.py / spike.py)
from locustfile import (  # noqa: F401,E402
    SpeechUser, LearningEventUser, WritingUser, TeacherUser,
)


# ---- 阶梯参数(环境变量可覆盖)----
# 每档:(目标 CCU, 该档总时长秒)。总时长 = ramp + hold;ramp 内线性爬升。
STAIR_USERS = [int(x) for x in os.environ.get(
    "STAIR_USERS", "100,500,1000,2000,5000").split(",")]
STAIR_HOLD = int(os.environ.get("STAIR_HOLD", "120"))   # 每档稳态秒(采 HPA)
STAIR_RAMP = int(os.environ.get("STAIR_RAMP", "30"))    # 每档爬升秒
# spawn-rate 上限(避免单档 ramp 过陡打垮单节点 VM)
SPAWN_RATE = int(os.environ.get("SPAWN_RATE", "50"))


def _stages():
    """构造 LoadTestShape stages:[(user_count, end_time, spawn_rate), ...]"""
    stages = []
    t = 0
    for u in STAIR_USERS:
        t += STAIR_RAMP + STAIR_HOLD  # 每档总时长
        stages.append((u, t, min(SPAWN_RATE, u)))
    return stages


STAGES = _stages()
STAGE_LABELS = [f"{u}CCU" for u in STAIR_USERS]


@events.test_start.add_listener
def _on_test_start(environment, **kwargs):
    print(f">>> P1-D10 STAIRCASE start. stages={list(zip(STAGE_LABELS, [s[1] for s in STAGES]))}s-end")
    print(">>> 单节点限制:只验 HPA 触发扩容+扩容后失败率降,绝对吞吐/5000承载 DEFERRED 多节点。")
    print(">>> 配合:scripts/p1-d10-hpa-watch.sh 每10s采 hpa+top pod 到 report/loadtest/hpa-snapshots.log")


@events.test_stop.add_listener
def _on_test_stop(environment, **kwargs):
    s = environment.stats.total
    p95 = s.get_response_time_percentile(0.95)
    p99 = s.get_response_time_percentile(0.99)
    print(f">>> P1-D10 STAIRCASE done. req={s.num_requests} fail={s.num_failures} "
          f"avg_rps={s.total_rps:.1f} p95_ms={p95:.1f} p99_ms={p99:.1f} "
          f"fail_rate={s.fail_ratio:.4f}")
    print(">>> 下一步:对照 report/loadtest/hpa-snapshots.log 看 5000CCU 档是否触发扩容+失败率拐点。")


class StaircaseShape(LoadTestShape):
    """
    阶梯负载形态。tick() 每秒由 Locust 调用,返回 (user_count, spawn_rate) 或 None 终止。

    每档内细分 ramp 段(线性爬升)与 hold 段(稳态采 HPA),并在档切换时打印标记,
    便于与 hpa-snapshots.log 时间轴对齐算"扩容触发延迟"。
    """

    def tick(self):
        run_time = self.get_run_time()
        for i, (target, end, rate) in enumerate(STAGES):
            if run_time < end:
                # 当前档起始时间 = 之前所有档 end 之和
                stage_start = end - STAIR_RAMP - STAIR_HOLD
                elapsed_in_stage = run_time - stage_start
                if elapsed_in_stage < STAIR_RAMP:
                    # ramp 段:线性爬升到 target
                    frac = elapsed_in_stage / STAIR_RAMP
                    cur = max(1, int(target * frac))
                    # 仅在 ramp 起点打印一次档标记
                    if int(elapsed_in_stage) == 0:
                        print(f">>> [stage {i}] {STAGE_LABELS[i]} ramp → target={target} "
                              f"(hold {STAIR_HOLD}s 采 HPA)")
                    return (cur, rate)
                else:
                    # hold 段:稳态 target
                    return (target, rate)
        # 所有档完成
        return None
