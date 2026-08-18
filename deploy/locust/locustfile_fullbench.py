"""
locustfile_fullbench.py — ESP 全服务×全语言端点基准(Locust 端,与 k6 scenarios-fullbench.js 同矩阵)。
集群内 ClusterIP 直连(不走 traefik,与 k6 同条件,便于工具相关性对照)。
每个 run 只测一个端点:env EP=xxx 选择,VUS/-u 控制并发。
JWT:复用 locustfile.py 的 RS256 mint(env JWT_RSA_PRIVATE_KEY_PEM 注入,bench-runner 有)。
运行(在 bench-runner pod 内):
  EP=learning_review VUS=20 locust -f /bench/locustfile_fullbench.py --headless \
      -u 20 -r 20 --run-time 45s --host=http://gateway-go:80
"""
from __future__ import annotations

import base64
import hashlib
import hmac
import itertools
import json
import os
import random
import subprocess
import time

from locust import HttpUser, between, events, task

try:
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import padding
    _HAS_CRYPTO = True
except ImportError:
    _HAS_CRYPTO = False

_RSA_PRIVATE_KEY_PEM = os.environ.get("JWT_RSA_PRIVATE_KEY_PEM", "")
_JWT_SECRET = os.environ.get("ESP_JWT_SECRET", "").encode()
_JWT_TTL = 3600


def _b64url(b: bytes) -> bytes:
    return base64.urlsafe_b64encode(b).rstrip(b"=")


def _rs256_sign_openssl(signing_input: bytes, pem: str) -> bytes:
    import tempfile
    with tempfile.NamedTemporaryFile(mode="w", suffix=".pem", delete=False) as kf:
        kf.write(pem)
        key_path = kf.name
    try:
        out = subprocess.run(
            ["openssl", "dgst", "-sha256", "-sign", key_path],
            input=signing_input, capture_output=True, check=True)
        return out.stdout
    finally:
        os.unlink(key_path)


def mint_jwt(sub: str, role: str = "learner") -> str:
    now = int(time.time())
    payload = {"sub": sub, "role": role, "iat": now, "exp": now + _JWT_TTL}
    p = _b64url(json.dumps(payload, separators=(",", ":")).encode())
    h = _b64url(json.dumps({"alg": "RS256", "typ": "JWT"}, separators=(",", ":")).encode())
    signing_input = h + b"." + p
    if _RSA_PRIVATE_KEY_PEM:
        if _HAS_CRYPTO:
            key = serialization.load_pem_private_key(_RSA_PRIVATE_KEY_PEM.encode(), password=None)
            sig = key.sign(signing_input, padding.PKCS1v15(), hashes.SHA256())
        else:
            sig = _rs256_sign_openssl(signing_input, _RSA_PRIVATE_KEY_PEM)
        return (signing_input + b"." + _b64url(sig)).decode()
    if _JWT_SECRET:
        sig = hmac.new(_JWT_SECRET, signing_input, hashlib.sha256).digest()
        return (signing_input + b"." + _b64url(sig)).decode()
    return ""


EP = os.environ.get("EP", "gw_scenarios")
LEARNER_TOKENS = os.environ.get("LEARNER_TOKENS", "").split(",")
TOKENS = [t for t in LEARNER_TOKENS if t]


def _uid(i: int) -> str:
    return str(10000100 + i)


# 坑㉚:Locust 2.46 User 无 self.user_index 属性(runner spawn 不设置)→ on_start 抛 AttributeError
# → 全部 0 请求。改为模块级计数器,每个 user 的 on_start 取一次,与旧 user_index 语义等价。
_USER_SEQ = itertools.count()


class FullbenchUser(HttpUser):
    wait_time = between(0, 0)  # 稳态无思考时间,测纯吞吐
    teacher_token = ""

    def on_start(self):
        self.user_index = next(_USER_SEQ)
        self.uid = _uid(self.user_index)
        if TOKENS:
            self.tok = TOKENS[self.user_index % len(TOKENS)]
        else:
            self.tok = mint_jwt(self.uid)
        if EP == "teacher_kpstats" and not self.teacher_token:
            self.teacher_token = mint_jwt(self.uid, role="teacher")

    @task
    def hit(self):
        uid = self.uid
        H = {"Content-Type": "application/json"}
        auth = {"Authorization": f"Bearer {self.tok}"}
        ep = EP

        if ep == "learning_event":
            url = "http://learning-java:80/api/learning/event"
            hd = {**auth, "Content-Type": "application/json"}
            bd = json.dumps({"user_id": int(uid), "event_type": "practice_attempt",
                             "payload": json.dumps({"kp_id": (int(uid) % 100) + 1, "correct": True})})
            r = self.client.post(url, data=bd, headers=hd)
        elif ep == "learning_review":
            r = self.client.get(f"http://learning-java:80/api/learning/review?userId={uid}", headers=auth)
        elif ep == "teacher_kpstats":
            r = self.client.get(f"http://teacher-java:80/api/teacher/{uid}/kp-stats",
                                headers={"Authorization": f"Bearer {self.teacher_token}"})
        elif ep == "bff_scenarios":
            r = self.client.get("http://bff-java:8090/api/scenarios", headers=auth)
        elif ep == "gw_scenarios":
            r = self.client.get("http://gateway-go:80/api/scenarios", headers=auth)
        elif ep == "bff_login":
            r = self.client.post("http://bff-java:8090/api/auth/login", data=json.dumps(
                {"email": f"bench_{(self.user_index % 5) + 1}@esp.test", "password": "test123"}), headers=H)
        elif ep == "reading_passages":
            r = self.client.get("http://reading-go:80/api/reading/passages?level=B1&limit=20")
        elif ep == "listening_mat":
            r = self.client.get("http://listening-go:80/api/listening/materials?limit=20")
        elif ep == "gamification_prof":
            r = self.client.get(f"http://gamification-go:80/api/gamification/profile/{uid}")
        elif ep == "gamification_pts":
            r = self.client.post("http://gamification-go:80/api/gamification/points",
                                 data=json.dumps({"user_id": int(uid), "points": 5, "reason": "bench"}), headers=H)
        elif ep == "vocab_due":
            r = self.client.get(f"http://vocab-srs-go:80/api/vocab/due?uid={uid}&limit=20")
        elif ep == "vocab_review":
            r = self.client.post("http://vocab-srs-go:80/api/vocab/review",
                                 data=json.dumps({"user_id": int(uid), "word_id": (int(uid) % 50) + 1, "quality": 3}), headers=H)
        elif ep == "emotion_recent":
            r = self.client.get(f"http://emotion-svc:80/api/emotion/recent/{uid}",
                                headers={"X-User-Id": uid})
        elif ep == "writingcoach_tpl":
            r = self.client.get("http://writing-coach:80/api/writing-coach/templates?category=IELTS")
        elif ep == "assessment_report":
            r = self.client.get(f"http://assessment-rs:80/api/assessment/report/{uid}")
        elif ep == "llm_dialogue":
            r = self.client.get(f"http://llm-orchestrator-rs:80/api/dialogue/sessions?user_id={uid}&limit=20")
        elif ep == "course_path":
            r = self.client.get(f"http://course-path-go:80/api/path/?user_id={uid}")
        elif ep == "writing_grade":
            r = self.client.post("http://writing-py:80/api/writing/grade",
                                 data=json.dumps({"prompt": "Write an essay about learning English.",
                                                  "essay": "This is a benchmark essay about learning English effectively.",
                                                  "exam": "ielts", "user_id": int(uid)}),
                                 headers={"X-User-Id": uid, "Content-Type": "application/json"})
        else:
            r = self.client.get("http://gateway-go:80/api/scenarios", headers=auth)

        if r.status_code >= 400:
            r.failure(f"{EP} HTTP {r.status_code}")


@events.test_stop.add_listener
def _on_test_stop(environment, **kw):
    """与 run_locust_bench.sh / analyze_fullbench.py 对齐的单行摘要(requests/failures/avg_rps/latency/p95/fail_rate)。"""
    s = environment.stats.total
    print(f">>> Test stopped. requests={s.num_requests} failures={s.num_failures} "
          f"avg_rps={s.total_rps:.2f} avg_latency_ms={s.avg_response_time:.2f} "
          f"p95_ms={s.get_response_time_percentile(0.95):.2f} fail_rate={s.fail_ratio:.4f}")
