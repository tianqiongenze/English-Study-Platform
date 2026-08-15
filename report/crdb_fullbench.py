#!/usr/bin/env python3
"""ESP #137 全服务 TiDB vs CRDB 应用层压测对比(集群内 ClusterIP 直连)。

覆盖每服务一条 DB 路径(避免 gateway/BFF 代理链主导延迟,聚焦 DB 差异):
  learning POST /api/learning/event   写:learning_events(Disruptor→writer pool)
  learning GET  /api/learning/review  读:复习清单(user_mastery)
  teacher  GET  /api/teacher/{uid}/kp-stats  读:user_mastery(teacher 角色)
  bff      GET  /api/scenarios        读:场景跨域(CRDB 首次启用)
  gateway  GET  /api/scenarios        参考:非 DB(静态内置场景)

用法:python3 crdb_fullbench.py <mode>   # mode=tidb|cockroachdb
输出:JSON 到 stdout。env 需 JWT_RSA_PRIVATE_KEY_PEM(签 RS256)。
"""
from __future__ import annotations
import base64, hashlib, hmac, json, os, subprocess, sys, time, collections, statistics
import concurrent.futures
import requests

KEY = os.environ.get("JWT_RSA_PRIVATE_KEY_PEM", "")
SECRET = os.environ.get("ESP_JWT_SECRET", "").encode()
MODE = sys.argv[1] if len(sys.argv) > 1 else os.environ.get("BENCH_MODE", "unknown")

def _b64url(b: bytes) -> str:
    return base64.urlsafe_b64encode(b).rstrip(b"=").decode()

def mint(sub, role="learner"):
    now = int(time.time())
    payload = {"sub": str(sub), "role": role, "iat": now, "exp": now + 3600}
    h = _b64url(json.dumps({"alg": "RS256", "typ": "JWT"}, separators=(",", ":")).encode())
    p = _b64url(json.dumps(payload, separators=(",", ":")).encode())
    si = (h + "." + p).encode()
    if KEY:
        try:
            from cryptography.hazmat.primitives import hashes, serialization
            from cryptography.hazmat.primitives.asymmetric import padding
            k = serialization.load_pem_private_key(KEY.encode(), password=None)
            sig = k.sign(si, padding.PKCS1v15(), hashes.SHA256())
            return h + "." + p + "." + _b64url(sig)
        except ImportError:
            import tempfile
            with tempfile.NamedTemporaryFile(mode="w", suffix=".pem", delete=False) as f:
                f.write(KEY)
                path = f.name
            try:
                out = subprocess.run(["openssl", "dgst", "-sha256", "-sign", path],
                                     input=si, capture_output=True, check=True)
                return h + "." + p + "." + _b64url(out.stdout)
            finally:
                os.unlink(path)
    sig = hmac.new(SECRET, si, hashlib.sha256).digest()
    return h + "." + p + "." + _b64url(sig)

# 每个端点并发用户预签 token(避免压测中重复签 5000 次)
TOKENS = {uid: mint(uid, "teacher" if uid % 10 == 0 else "learner") for uid in range(1, 121)}

def make_headers(uid):
    return {"Authorization": "Bearer " + TOKENS[uid], "Content-Type": "application/json"}

def ep_event(uid):
    return ("POST", "http://learning-java:80/api/learning/event", make_headers(uid),
            json.dumps({"user_id": uid,
                        "event_type": "answer",
                        "payload": json.dumps({"kp_id": (uid % 100) + 1, "correct": True})}))

def ep_review(uid):
    return ("GET", f"http://learning-java:80/api/learning/review?userId={uid}", make_headers(uid), None)

def ep_kpstats(uid):
    # 只用 teacher 角色用户(uid%10==0),否则 403
    t = mint(uid, "teacher")
    hdr = {"Authorization": "Bearer " + t}
    return ("GET", f"http://teacher-java:80/api/teacher/{uid}/kp-stats", hdr, None)

def ep_bff_scen(uid):
    return ("GET", "http://bff-java:8090/api/scenarios", make_headers(uid), None)

def ep_gw_scen(uid):
    return ("GET", "http://gateway-go:80/api/scenarios", make_headers(uid), None)

def bench(name, total, concurrency, ep_fn):
    """并发打 total 个请求,统计延迟/成功率/状态码。"""
    lat, status, errs = [], collections.Counter(), 0
    def one(idx):
        nonlocal errs
        uid = (idx % 120) + 1
        method, url, headers, body = ep_fn(uid)
        t0 = time.perf_counter()
        try:
            if method == "POST":
                r = requests.post(url, headers=headers, data=body, timeout=30)
            else:
                r = requests.get(url, headers=headers, timeout=30)
            dt = (time.perf_counter() - t0) * 1000.0
            lat.append(dt); status[r.status_code] += 1
            if not (200 <= r.status_code < 300):
                errs += 1
        except Exception:
            dt = (time.perf_counter() - t0) * 1000.0
            lat.append(dt); status["EXC"] += 1; errs += 1
    start = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as ex:
        list(ex.map(one, range(total)))
    elapsed = time.perf_counter() - start
    lat.sort(); n = len(lat)
    def pct(p):
        return lat[min(int(n * p), n - 1)] if n else 0.0
    return {
        "endpoint": name, "concurrency": concurrency, "total": total,
        "elapsed_s": round(elapsed, 2), "rps": round(total / elapsed, 1),
        "success": f"{total - errs}/{total}",
        "fail_rate": round(errs / total, 4),
        "P50_ms": round(pct(0.50), 1), "P95_ms": round(pct(0.95), 1),
        "P99_ms": round(pct(0.99), 1), "max_ms": round(lat[-1], 1) if lat else 0,
        "status": {str(k): v for k, v in status.items()},
    }

def ladder(name, ep_fn, levels=((20, 300), (60, 900))):
    out = []
    for c, t in levels:
        out.append(bench(name, t, c, ep_fn))
        time.sleep(1)
    return out

if __name__ == "__main__":
    # warmup:确认所有端点可达 + 鉴权通
    for name, fn in [("event", ep_event), ("review", ep_review), ("kp-stats", ep_kpstats),
                     ("bff-scenarios", ep_bff_scen), ("gw-scenarios", ep_gw_scen)]:
        try:
            m, u, h, b = fn(61)
            r = requests.post(u, headers=h, data=b, timeout=30) if m == "POST" else requests.get(u, headers=h, timeout=30)
            print(f"warmup {name}: HTTP {r.status_code} {len(r.content)}B", flush=True)
        except Exception as e:
            print(f"warmup {name}: FAIL {e}", flush=True)
    results = {}
    results["learning_event_write"] = ladder("POST /api/learning/event", ep_event)
    results["learning_review_read"] = ladder("GET /api/learning/review", ep_review)
    results["teacher_kpstats_read"] = ladder("GET /api/teacher/{uid}/kp-stats", ep_kpstats)
    results["bff_scenarios_read"] = ladder("GET /api/scenarios (bff)", ep_bff_scen)
    results["gw_scenarios_ref"] = ladder("GET /api/scenarios (gateway, non-DB)", ep_gw_scen)
    print("BENCH_JSON_START", flush=True)
    print(json.dumps({"mode": MODE, "time": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                      "results": results}, indent=2), flush=True)
    print("BENCH_JSON_END", flush=True)
