#!/usr/bin/env python3
"""ESP 集群内 ClusterIP 直连压测 GET /api/scenarios。

证明 host port-forward 的 28 rps 是隧道开销,非服务瓶颈。
指标:总请求/并发/rps/成功率/状态码分布/P50/P95/P99。
"""
import requests
import time
import concurrent.futures
import collections
import statistics

TARGET = "http://gateway-go:80/api/scenarios"
TIMEOUT = 10

def bench(concurrency, total):
    """用线程池打 total 个请求,记录延迟与状态码。"""
    latencies = []
    status_counter = collections.Counter()
    errors = 0
    lock_placeholder = None  # list append is GIL-safe

    def one(_):
        nonlocal errors
        t0 = time.perf_counter()
        try:
            r = requests.get(TARGET, timeout=TIMEOUT)
            dt = (time.perf_counter() - t0) * 1000.0
            latencies.append(dt)
            status_counter[r.status_code] += 1
            if r.status_code != 200:
                errors += 1
        except Exception as e:
            dt = (time.perf_counter() - t0) * 1000.0
            latencies.append(dt)
            status_counter["EXC"] += 1
            errors += 1

    start = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as ex:
        list(ex.map(one, range(total)))
    elapsed = time.perf_counter() - start

    latencies.sort()
    n = len(latencies)
    def pct(p):
        if n == 0:
            return 0.0
        return latencies[min(int(n * p), n - 1)]

    print(f"\n===== concurrency={concurrency} total={total} =====")
    print(f"elapsed={elapsed:.3f}s  rps={total/elapsed:.1f}")
    print(f"success={total-errors}/{total} ({(total-errors)/total*100:.1f}%)")
    print(f"latency ms: P50={pct(0.50):.1f} P95={pct(0.95):.1f} P99={pct(0.99):.1f} max={latencies[-1] if latencies else 0:.1f}")
    print(f"status: {dict(status_counter)}")
    return total/elapsed, errors

if __name__ == "__main__":
    # warmup
    try:
        r = requests.get(TARGET, timeout=TIMEOUT)
        print(f"warmup: HTTP {r.status_code}, body {len(r.content)} bytes")
    except Exception as e:
        print(f"warmup FAILED: {e}")
        raise SystemExit(1)
    print(f"target: {TARGET}\n")
    # 阶梯加压: 10 -> 50 -> 100 并发
    for c, t in [(10, 200), (50, 500), (100, 1000), (200, 2000)]:
        bench(c, t)
        time.sleep(1)
