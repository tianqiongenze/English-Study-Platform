"""
5000 CCU 在线连接容量测试 (WebSocket 长连接)。
# nosemgrep: python.lang.security.audit.insecure-websocket.protocol - 单节点验证环境用 ws://,生产 Ingress(Traefik)强制 wss(TLS 在 Ingress 层)

验证 G-01: 5000 人同时在线。gateway-go 为原生 Go 编译的无状态网关,
可支撑大量并发长连接。本脚本建立 5000 条 WebSocket 连接到 /ws/speech/<session>,
每条连接保持存活并周期性发送心跳帧,统计稳态在线数与掉线率。

前置:BFF(bff-java:8090)与 gateway-go 集群内可达;每条 worker 向 BFF 换 RS256 JWT(ADR-12)。
运行(集群内 python:3.12-slim pod):
  python deploy/locust/ccu5000_ws.py --target ws://gateway-go:80/ws/speech \
      --http-host gateway-go:80 --bff http://bff-java:8090 \
      --users 5000 --spawn-rate 100 --duration 120
"""
from __future__ import annotations

import argparse
import json
import os
import random
import threading
import time
import urllib.request
from collections import Counter
from urllib.parse import quote

try:
    import websocket  # websocket-client
except ImportError:
    raise SystemExit("pip install websocket-client")

SCENARIOS = ["airport", "interview", "cafe"]


# gateway-go 鉴权(ADR-12):RS256 公钥校验 BFF 签发的 JWT。不再接受自 mint 的 HS256 token。
# 每 worker 向 BFF(/api/auth/register 失败则 /api/auth/login)换真实 RS256 JWT,
# 再用该 token 创建会话 + 连 WS(?token= 传同一 JWT,owner 校验 sub==sess.UserID)。
def bff_token(bff_host: str, uid: int) -> str:
    email = f"ws_{uid}@loadtest.com"
    for path, body in (
        ("/api/auth/register", {"email": email, "password": "test123", "name": email, "role": "learner"}),
        ("/api/auth/login", {"email": email, "password": "test123"}),
    ):
        req = urllib.request.Request(
            f"{bff_host}{path}",
            data=json.dumps(body).encode(),
            headers={"Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req, timeout=10) as r:
                tok = json.loads(r.read()).get("token")
                if tok:
                    return tok
        except Exception:
            continue
    return ""


def create_session(http_host: str, host_header: str, token: str) -> str | None:
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
    uid = random.randint(1, 5000)
    token = bff_token(args.bff, uid)  # BFF 签发的 RS256 JWT;会话归属该 uid
    if not token:
        return
    sid = create_session(args.http_host, host_header, token)
    if not sid:
        return
    url = f"{ws_url}/{sid}" + (f"?token={quote(token, safe='')}" if token else "")
    try:
        ws = websocket.create_connection(url, timeout=10)
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
    p.add_argument("--target", default="ws://gateway-go:80/ws/speech")
    p.add_argument("--http-host", default="gateway-go:80")
    p.add_argument("--bff", default="http://bff-java:8090", help="BFF host for RS256 JWT")
    p.add_argument("--host", default="gateway-go", help="Host header for routing")
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
    # /locust 是 ConfigMap 只读挂载,report/ 子目录无法创建;写 /report(emptyDir 可写)。
    out_dir = "/report" if os.path.isdir("/report") else "report"
    os.makedirs(out_dir, exist_ok=True)
    with open(f"{out_dir}/ccu5000_ws_result.json", "w") as f:
        json.dump({"target_users": args.users, "peak_online": peak,
                   "avg_online": avg, "established": stats.established,
                   "dropped": stats.dropped,
                   "sustain_pct": 100 * peak / args.users}, f, indent=2)


if __name__ == "__main__":
    main()
