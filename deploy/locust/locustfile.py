"""
English-Study-Platform Locust 性能与压力测试。

目标(G-01): 5000 CCU 同时在线,企业级可用。
覆盖:
  - 口语陪练会话 (SpeechUser): 创建会话 + 场景读 + 模拟卡壳
  - 学情事件上报 (LearningEventUser): Disruptor 入口吞吐 (G-06 >= 5000 ev/s)
  - 写作批改 (WritingUser): G-04 >= 50 req/s, P95 < 2s
  - 教师教案 (TeacherUser): 教师端负载

入口: 通过 Traefik Ingress NodePort(31032)统一路由,所有请求带 `Host: esp.example.com` 头。
运行:
  locust -f deploy/locust/locustfile.py --headless \
      -u 5000 -r 100 --run-time 5m \
      --host http://192.168.64.6:31032 \
      --csv=report/esp --html=report/esp.html
"""
from __future__ import annotations

import base64
import hashlib
import hmac
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

SCENARIOS = ["airport", "interview", "cafe"]  # gateway-go 内置场景


def _b64url(b: bytes) -> bytes:
    return base64.urlsafe_b64encode(b).rstrip(b"=")


# gateway-go:1.4.1 用 RS256 校验 JWT(env JWT_RSA_PUBLIC_KEY_PEM,golang-jwt/v5,
# 校验 exp 必须有且未过期,sub=数字字符串 userID,无 issuer 校验)。
# BFF 同样用 RS256 + PKCS8 私钥(secret esp-rsa-jwt, key JWT_RSA_PRIVATE_KEY_PEM)签发。
# 压测机直接复用同一私钥签 RS256 dev token,使 gateway-go 公钥校验通过。
_RSA_PRIVATE_KEY_PEM = os.environ.get("JWT_RSA_PRIVATE_KEY_PEM", "")
# 兼容降级:RS256 私钥未注入时退回 HS256(仅用于排查,生产压测必须注入私钥)。
_JWT_SECRET = os.environ.get("ESP_JWT_SECRET", "").encode()
_JWT_TTL = 3600  # 1h,与 BFF 对齐


def _rs256_sign_openssl(signing_input: bytes, pem: str) -> bytes:
    """openssl dgst -sha256 -sign 兜底签 RS256(无 cryptography 时)。"""
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


def mint_jwt(sub: str) -> str:
    """签 JWT(sub=userID)。

    优先 RS256(与 gateway-go/bff 一致,env JWT_RSA_PRIVATE_KEY_PEM 注入 PKCS8 PEM 私钥):
      1) cryptography 库(首选);
      2) openssl CLI 兜底(locust 镜像无 cryptography 时)。
    私钥缺失时退回 HS256(ESP_JWT_SECRET);两者皆空返回空串(端点将 401,暴露配置问题)。
    """
    now = int(time.time())
    payload = {"sub": sub, "iat": now, "exp": now + _JWT_TTL}
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


@events.init.add_listener
def _set_host_header(environment, **kwargs):
    # Traefik 按 host 规则路由;为每个 user 的 session 注入 Host 头。
    for user in environment.runner.user_classes if environment.runner else []:
        user.host_header = "esp.example.com"


class ESPHttpUser(HttpUser):
    """基类:统一注入 Traefik 路由所需的 Host 头 + gateway-go 鉴权 Bearer token。"""
    abstract = True
    host_header = "esp.example.com"

    def on_start(self):
        self.client.headers.update({"Host": self.host_header})
        # 每个虚拟用户随机分配 userID(1..5000)并签发对应 JWT;
        # learning/writing/teacher 端点不校验 token,带上也无害;sessions 端点必须。
        self.user_id = str(random.randint(1, 5000))
        tok = mint_jwt(self.user_id)
        if tok:
            self.client.headers.update({"Authorization": f"Bearer {tok}"})


class SpeechUser(ESPHttpUser):
    """模拟学习者建立口语会话并持续交互(占在线 CCU 主体,约 60%)。"""
    wait_time = between(0.5, 1.5)

    def on_start(self):
        super().on_start()
        code = random.choice(SCENARIOS)
        with self.client.post("/api/sessions", json={"scenario_code": code},
                              name="POST /api/sessions", catch_response=True) as r:
            if r.status_code != 200:
                r.failure(f"create session failed: {r.status_code}")
                self.session_id = None
                return
            self.session_id = r.json().get("session_id")

    @task(3)
    def list_scenarios(self):
        if not self.session_id:
            return
        with self.client.get("/api/scenarios", name="GET /api/scenarios", catch_response=True) as r:
            if r.status_code != 200:
                r.failure("scenarios failed")

    @task(1)
    def idle(self):
        # 模拟用户思考/卡壳(保持长在线,占连接)
        pass

    def on_stop(self):
        self.session_id = None


class LearningEventUser(ESPHttpUser):
    """高频学情事件上报,验证 Disruptor 入口吞吐 (G-06 >= 5000 ev/s)。"""
    wait_time = between(0.01, 0.05)  # 每用户 ~20-100 ev/s

    @task
    def report_event(self):
        payload = {
            "user_id": random.randint(1, 5000),
            "event_type": random.choice(["answer", "speak", "review"]),
            "payload": json.dumps({"kp_id": random.randint(1, 100),
                                   "correct": random.random() > 0.4}),
        }
        with self.client.post("/api/learning/event", json=payload,
                              name="POST /api/learning/event", catch_response=True) as r:
            if r.status_code == 202:
                r.success()
            else:
                r.failure(f"event rejected: {r.status_code}")


class WritingUser(ESPHttpUser):
    """写作批改负载 (G-04 >= 50 req/s, P95 < 2s)。"""
    wait_time = between(1, 3)

    @task
    def grade(self):
        payload = {
            "prompt": "Do you agree that technology improves education?",
            "essay": ("Technology has transformed education in remarkable ways. "
                      "However, some argue it distracts students from deep learning. "
                      "I believe technology, when used properly, greatly improves learning outcomes."),
            "exam": random.choice(["toefl", "ielts"]),
        }
        with self.client.post("/api/writing/grade", json=payload,
                              name="POST /api/writing/grade", catch_response=True) as r:
            if r.status_code == 200:
                r.success()
            else:
                r.failure(f"grade failed: {r.status_code}")


class TeacherUser(ESPHttpUser):
    """教师端负载:生成教案 + 班级学情(约 5%)。"""
    wait_time = between(2, 5)

    @task(2)
    def lesson_plan(self):
        payload = {"topic": random.choice(["Environment", "Travel", "AI", "Health"]),
                   "level": random.choice(["A2", "B1", "B2"])}
        with self.client.post("/api/teacher/lesson-plan", json=payload,
                              name="POST /api/teacher/lesson-plan", catch_response=True) as r:
            if r.status_code == 200:
                r.success()
            else:
                r.failure(f"lesson-plan failed: {r.status_code}")

    @task(1)
    def analytics(self):
        with self.client.get(f"/api/teacher/classes/{random.randint(1,50)}/analytics",
                             name="GET /api/teacher/classes/analytics", catch_response=True) as r:
            if r.status_code == 200:
                r.success()
            else:
                r.failure(f"analytics failed: {r.status_code}")


@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    print(">>> Locust test starting. Targets: 5000 CCU, G-04>=50rps, G-06>=5000ev/s")


@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    stats = environment.stats.total
    p95 = stats.get_response_time_percentile(0.95)
    print(f">>> Test stopped. "
          f"requests={stats.num_requests} failures={stats.num_failures} "
          f"avg_rps={stats.total_rps:.1f} "
          f"avg_latency_ms={stats.avg_response_time:.1f} "
          f"p95_ms={p95:.1f} "
          f"fail_rate={stats.fail_ratio:.4f}")
