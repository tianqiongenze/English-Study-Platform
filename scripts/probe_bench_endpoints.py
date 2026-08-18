#!/usr/bin/env python3
"""probe_bench_endpoints.py — 验证全服务端点矩阵可测(状态码+平均延迟),与 k6/locust 同矩阵。
在 bench-runner pod 内运行:
  kubectl -n esp-prod exec bench-runner -- python3 /bench/probe_bench_endpoints.py
输出:每端点 status×3 命中率 + avg_ms;非 2xx 标 FAIL。
"""
import sys, json, time, urllib.request

sys.path.insert(0, '/bench')
from locustfile_fullbench import mint_jwt  # 复用 RS256 mint(env JWT_RSA_PRIVATE_KEY_PEM)

UID = 10000101
LEARNER = mint_jwt(str(UID))
TEACHER = mint_jwt(str(UID), role="teacher")


def one(method, url, headers, body, timeout=15):
    req = urllib.request.Request(url, data=body.encode() if body else None,
                                 headers=headers, method=method)
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, (time.time() - t0) * 1000
    except urllib.error.HTTPError as e:
        return e.code, (time.time() - t0) * 1000
    except Exception as e:
        return "ERR:" + str(e)[:60], (time.time() - t0) * 1000


# 先注册 bench_1..5 账号(幂等),login 才有合法凭据
for _n in range(1, 6):
    one("POST", "http://bff-java:8090/api/auth/register", {"Content-Type": "application/json"},
        json.dumps({"email": f"bench_{_n}@esp.test", "password": "test123", "name": f"bench{_n}", "role": "learner"}))

EPS = [
    ("learning_event", "POST", "http://learning-java:80/api/learning/event",
     {"Authorization": f"Bearer {LEARNER}", "Content-Type": "application/json"},
     json.dumps({"user_id": UID, "event_type": "practice_attempt", "payload": json.dumps({"kp_id": 1, "correct": True})})),
    ("learning_review", "GET", f"http://learning-java:80/api/learning/review?userId={UID}",
     {"Authorization": f"Bearer {LEARNER}"}, None),
    ("teacher_kpstats", "GET", f"http://teacher-java:80/api/teacher/{UID}/kp-stats",
     {"Authorization": f"Bearer {TEACHER}"}, None),
    ("bff_scenarios", "GET", "http://bff-java:8090/api/scenarios",
     {"Authorization": f"Bearer {LEARNER}"}, None),
    ("gw_scenarios", "GET", "http://gateway-go:80/api/scenarios",
     {"Authorization": f"Bearer {LEARNER}"}, None),
    ("bff_login", "POST", "http://bff-java:8090/api/auth/login",
     {"Content-Type": "application/json"}, json.dumps({"email": "bench_1@esp.test", "password": "test123"})),
    ("reading_passages", "GET", "http://reading-go:80/api/reading/passages?level=B1&limit=20", {}, None),
    ("listening_mat", "GET", "http://listening-go:80/api/listening/materials?limit=20", {}, None),
    ("gamification_prof", "GET", f"http://gamification-go:80/api/gamification/profile/{UID}", {}, None),
    ("gamification_pts", "POST", "http://gamification-go:80/api/gamification/points",
     {"Content-Type": "application/json"}, json.dumps({"user_id": UID, "points": 5, "reason": "probe"})),
    ("vocab_due", "GET", f"http://vocab-srs-go:80/api/vocab/due?uid={UID}&limit=20", {}, None),
    ("vocab_review", "POST", "http://vocab-srs-go:80/api/vocab/review",
     {"Content-Type": "application/json"}, json.dumps({"user_id": UID, "word_id": 1, "quality": 3})),
    ("emotion_recent", "GET", f"http://emotion-svc:80/api/emotion/recent/{UID}",
     {"X-User-Id": str(UID)}, None),
    ("writingcoach_tpl", "GET", "http://writing-coach:80/api/writing-coach/templates?category=IELTS", {}, None),
    ("assessment_report", "GET", f"http://assessment-rs:80/api/assessment/report/{UID}", {}, None),
    ("llm_dialogue", "GET", f"http://llm-orchestrator-rs:80/api/dialogue/sessions?user_id={UID}&limit=20", {}, None),
    ("course_path", "GET", f"http://course-path-go:80/api/path/?user_id={UID}", {}, None),
    ("writing_grade", "POST", "http://writing-py:80/api/writing/grade",
     {"X-User-Id": str(UID), "Content-Type": "application/json"},
     json.dumps({"prompt": "Write an essay about learning English.", "essay": "This is a probe essay.", "exam": "ielts", "user_id": UID})),
]


print(f"{'ep':22} {'statuses':>22} {'avg_ms':>8}  verdict")
for name, method, url, headers, body in EPS:
    statuses, times = [], []
    for _ in range(3):
        st, ms = one(method, url, headers, body)
        statuses.append(str(st))
        times.append(ms)
    avg = sum(times) / len(times)
    ok = all(s.startswith("2") for s in statuses)
    print(f"{name:22} {'/'.join(statuses):>22} {avg:8.0f}  {'OK' if ok else 'FAIL'}")
