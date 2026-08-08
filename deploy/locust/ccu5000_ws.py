"""
5000 CCU 在线连接容量测试 (WebSocket 长连接)。
# nosemgrep: python.lang.security.audit.insecure-websocket.protocol - 单节点验证环境用 ws://,生产 Ingress(Traefik)强制 wss(TLS 在 Ingress 层)

验证 G-01: 5000 人同时在线。gateway-go 为原生 Go 编译的无状态网关,
可支撑大量并发长连接。本脚本建立 5000 条 WebSocket 连接到 /ws/speech/<session>,
每条连接保持存活并周期性发送心跳帧,统计稳态在线数与掉线率。

前置:先用 REST 创建会话拿到 session_id 列表(脚本内置创建)。
运行:
  python deploy/locust/ccu5000_ws.py --target ws://192.168.64.6:31032/ws/speech \
      --users 5000 --spawn-rate 100 --duration 120 --host esp.example.com
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import hmac
import json
import os
import random
import threading
import time
from collections import Counter

try:
    import websocket  # websocket-client
except ImportError:
    raise SystemExit("pip install websocket-client")

SCENARIOS = ["airport", "interview", "cafe"]

# gateway-go 鉴权:HS256 JWT(sub=userID),secret 经 ESP_JWT_SECRET 注入(来自 esp-secrets,不入库)。
# WS 路径用 ?token= 传 JWT,且 token 的 userID 必须等于会话归属 UserID,故每条连接 mint 独立 token。
_JWT_SECRET = os.environ.get("ESP_JWT_SECRET", "").encode()


def _b64url(b: bytes) -> bytes:
    return base64.urlsafe_b64encode(b).rstrip(b"=")


def mint_jwt(sub: str) -> str:
    if not _JWT_SECRET:
        return ""
    h = _b64url(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
    p = _b64url(json.dumps({"sub": sub}, separators=(",", ":")).encode())
    sig = hmac.new(_JWT_SECRET, h + b"." + p, hashlib.sha256).digest()
    return (h + b"." + p + b"." + _b64url(sig)).decode()


def create_session(http_host: str, host_header: str, token: str) -> str | None:
    import urllib.request
    code = random.choice(SCENARIOS)
    body = json.dumps({"scenario_code": code}).encode()
    headers = {"Content-Type": "application/json", "Host": host_header}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(
        f"http://{http_host}/api/sessions", data=body, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            return json.loads(r.read()).get("session_id")
    except Exception:
        return None


class Stats:
    def __init__(self):
        self.online = 0
        self.dropped = 0
        self.established = 0
        self.lock = threading.Lock()

    def inc_online(self):
        with self.lock:
            self.online += 1
            self.established += 1

    def dec_online(self):
        with self.lock:
            self.online -= 1
            self.dropped += 1


def worker(ws_url: str, host_header: str, stats: Stats, stop_evt: threading.Event):
    uid = str(random.randint(1, 5000))
    token = mint_jwt(uid)  # 会话归属该 uid;WS 连接必须用同 token 才通过 owner 校验
    sid = create_session(args.http_host, host_header, token)
    if not sid:
        return
    url = f"{ws_url}/{sid}" + (f"?token={token}" if token else "")
    try:
        ws = websocket.create_connection(url, timeout=10,
                                         header=[f"Host: {host_header}"])
        stats.inc_online()
        # heartbeat: send a text frame every 5s, keep alive until stop
        while not stop_evt.is_set():
            ws.send(json.dumps({"type": "text", "text": "hi"}))
            time.sleep(5)
        ws.close()
    except Exception:
        try:
            stats.dec_online()
        except Exception:
            pass


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--target", default="ws://192.168.64.6:31032/ws/speech")
    p.add_argument("--http-host", default="192.168.64.6:31032")
    p.add_argument("--host", default="esp.example.com", help="Host header for ingress routing")
    p.add_argument("--users", type=int, default=5000)
    p.add_argument("--spawn-rate", type=int, default=100, help="new conns per second")
    p.add_argument("--duration", type=int, default=120, help="steady-state seconds")
    args2 = p.parse_args()
    global args
    args = args2

    stats = Stats()
    stop_evt = threading.Event()
    threads = []

    print(f">>> Spawning {args.users} WS connections @ {args.spawn_rate}/s ...")
    spawned = 0
    t0 = time.time()
    while spawned < args.users:
        batch = min(args.spawn_rate, args.users - spawned)
        for _ in range(batch):
            t = threading.Thread(target=worker,
                                 args=(args.target, args.host, stats, stop_evt),
                                 daemon=True)
            t.start()
            threads.append(t)
        spawned += batch
        time.sleep(1)
        with stats.lock:
            print(f"  spawned={spawned} online={stats.online} dropped={stats.dropped}")

    print(f">>> Spawn complete in {time.time()-t0:.1f}s. Steady state for {args.duration}s.")
    peak = 0
    samples = []
    for i in range(args.duration):
        time.sleep(1)
        with stats.lock:
            cur = stats.online
            samples.append(cur)
            peak = max(peak, cur)
        if i % 10 == 9:
            print(f"  t={i+1}s online={cur} peak={peak} established={stats.established} dropped={stats.dropped}")

    stop_evt.set()
    time.sleep(3)
    avg = sum(samples) / len(samples) if samples else 0
    print(f">>> DONE. peak_online={peak} avg_online={avg:.0f} "
          f"total_established={stats.established} dropped={stats.dropped}")
    print(f">>> Sustained {peak}/{args.users} ({100*peak/args.users:.1f}%) concurrent online.")
    with open("report/ccu5000_ws_result.json", "w") as f:
        json.dump({"target_users": args.users, "peak_online": peak,
                   "avg_online": avg, "established": stats.established,
                   "dropped": stats.dropped,
                   "sustain_pct": 100 * peak / args.users}, f, indent=2)


if __name__ == "__main__":
    main()
