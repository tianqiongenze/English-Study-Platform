#!/usr/bin/env python3
"""
ESP 前后端连调功能测试(接口 e2e)—— Newman collection 的 Python 等价实现。
对应 deploy/newman/esp-e2e.postman_collection.json 的断言语义。

为何不用 newman:集群内无 Node 运行时;用 Python stdlib(urllib)实现等价断言,
在 writing-py pod 内运行,经 ClusterIP 直连各服务,验证前后端契约连通性。

用法(集群内):
  kubectl -n esp-prod exec deploy/writing-py -- python3 /tmp/esp_e2e.py
  kubectl -n esp-prod exec deploy/writing-py -- python3 /tmp/esp_e2e.py --json   # 结构化 JSON 输出(供落盘报告)
"""
import argparse
import json
import os
import shutil
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from xml.sax.saxutils import escape as xml_escape  # nosemgrep: python.lang.security.audit.xxe.xml-sax - 仅用 escape 转义输出,非解析不可信 XML,无 XXE 风险

BASE = {
    "gateway": "http://gateway-go",
    "bff": "http://bff-java:8090",
    "learning": "http://learning-java",
    "writing": "http://writing-py",
    "emotion": "http://emotion-svc",
    "teacher": "http://teacher-java",
}

passed = 0
failed = 0
fails = []
# 结构化结果收集(供 --json 落盘报告)
results = []


def call(url, method="GET", headers=None, data=None, timeout=10):
    req = urllib.request.Request(url, headers=headers or {}, method=method, data=data)
    try:
        r = urllib.request.urlopen(req, timeout=timeout)
        return r.status, r.read().decode(errors="replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode(errors="replace")
    except Exception as e:
        return None, str(e)


def check(name, ok, detail="", group="", http_code=None, duration_ms=0):
    global passed, failed
    status = "PASS" if ok else "FAIL"
    if ok:
        passed += 1
        print(f"  ✅ {name}")
    else:
        failed += 1
        fails.append(name)
        print(f"  ❌ {name}  {detail}")
    results.append({
        "name": name,
        "group": group,
        "status": status,
        "http_code": http_code,
        "detail": detail if not ok else "",
        "duration_ms": duration_ms,
    })


def jget(body):
    try:
        return json.loads(body)
    except Exception:
        return None


def timed_call(url, method="GET", headers=None, data=None, timeout=10):
    """带耗时的 call,返回 (status, body, duration_ms)。"""
    t0 = time.monotonic()
    st, body = call(url, method, headers, data, timeout)
    return st, body, int((time.monotonic() - t0) * 1000)


def write_reports(report_dir, run_ts, total, passed, failed):
    """落盘 JSON + JUnit XML 报告(带时间戳 + latest 覆盖),返回路径清单。"""
    os.makedirs(report_dir, exist_ok=True)
    ts = run_ts.replace(":", "").replace("-", "")
    json_name = f"esp-e2e-{ts}.json"
    junit_name = f"esp-e2e-{ts}.junit.xml"
    json_path = os.path.join(report_dir, json_name)
    junit_path = os.path.join(report_dir, junit_name)

    report = {
        "run_ts": run_ts,
        "entry": "cluster-internal ClusterIP",
        "total": total,
        "passed": passed,
        "failed": failed,
        "results": results,
    }
    with open(json_path, "w", encoding="utf-8") as f:
        json.dump(report, f, ensure_ascii=False, indent=2)

    # JUnit XML
    lines = ['<?xml version="1.0" encoding="UTF-8"?>']
    lines.append(f'<testsuites tests="{total}" failures="{failed}">')
    lines.append(f'<testsuite name="esp-e2e" tests="{total}" failures="{failed}">')
    for r in results:
        classname = xml_escape(r.get("group") or "e2e")
        tname = xml_escape(r.get("name") or "unknown")
        dur = r.get("duration_ms") or 0
        lines.append(f'<testcase name="{tname}" classname="{classname}" time="{dur/1000.0:.3f}">')
        if r.get("status") == "FAIL":
            detail = xml_escape(r.get("detail") or "(no detail)")
            lines.append(f'<failure>{detail}</failure>')
        lines.append('</testcase>')
    lines.append('</testsuite></testsuites>')
    with open(junit_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")

    # latest 覆盖(cp 避 symlink 权限问题)
    shutil.copyfile(json_path, os.path.join(report_dir, "esp-e2e-latest.json"))
    shutil.copyfile(junit_path, os.path.join(report_dir, "esp-e2e-latest.junit.xml"))

    return [json_path, junit_path]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--json", action="store_true",
                        help="输出结构化 JSON(供落盘报告,默认仍打印人类可读)")
    parser.add_argument("--report-dir", default="report/e2e",
                        help="落盘报告目录(默认 report/e2e)")
    args = parser.parse_args()
    run_ts = time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime())

    print("=" * 60)
    print("ESP 前后端连调功能测试(接口 e2e)")
    print("=" * 60)

    # ── 健康检查 ──
    print("\n[健康检查]")
    for name, url in [("gateway", BASE["gateway"] + "/healthz"),
                      ("writing", BASE["writing"] + "/healthz")]:
        st, _, dur = timed_call(url, timeout=5)
        check(f"HC {name} 200", st == 200, f"got {st}", group="健康检查",
              http_code=st, duration_ms=dur)

    # ── 鉴权链路 ──
    print("\n[鉴权链路]")
    st, body, dur = timed_call(BASE["bff"] + "/api/auth/wx-login", "POST",
                               {"Content-Type": "application/json"},
                               b'{"code":"e2e-test-code"}')
    j = jget(body)
    token = j.get("token") if j else None
    check("AUTH-1 wx-login 200", st == 200, f"got {st}", group="鉴权链路",
          http_code=st, duration_ms=dur)
    check("AUTH-1 返回 token", bool(token), group="鉴权链路",
          http_code=st, duration_ms=dur)
    check("AUTH-1 role=learner", j and j.get("user", {}).get("role") == "learner",
          f"got {j}", group="鉴权链路", http_code=st, duration_ms=dur)

    # 无 token 应被拒
    st, _, dur = timed_call(BASE["gateway"] + "/api/sessions", "POST",
                            {"Content-Type": "application/json"},
                            b'{"scenario_code":"restaurant_order"}')
    check("AUTH-2 无 token → 401/403", st in (401, 403), f"got {st}",
          group="鉴权链路", http_code=st, duration_ms=dur)

    # 伪造 X-User-Id 无 token 应被拒
    st, _, dur = timed_call(BASE["gateway"] + "/api/sessions", "POST",
                            {"X-User-Id": "9999", "X-Role": "admin",
                             "Content-Type": "application/json"},
                            b'{"scenario_code":"restaurant_order"}')
    check("AUTH-3 伪造头(无 token)→ 401/403", st in (401, 403), f"got {st}",
          group="鉴权链路", http_code=st, duration_ms=dur)

    # ── 口语场景 ──
    print("\n[口语场景 gateway-go]")
    st, body, dur = timed_call(BASE["gateway"] + "/api/scenarios", "GET",
                               {"Authorization": f"Bearer {token}"} if token else None)
    j = jget(body)
    check("SPK-1 场景列表 200", st == 200, f"got {st}", group="口语场景",
          http_code=st, duration_ms=dur)
    check("SPK-1 返回非空数组", isinstance(j, list) and len(j) > 0, f"got {type(j)}",
          group="口语场景", http_code=st, duration_ms=dur)
    check("SPK-1 场景含 code", isinstance(j, list) and j and isinstance(j[0].get("code"), str),
          group="口语场景", http_code=st, duration_ms=dur)

    if token:
        st, body, dur = timed_call(BASE["gateway"] + "/api/sessions", "POST",
                                   {"Authorization": f"Bearer {token}",
                                    "Content-Type": "application/json"},
                                   b'{"scenario_code":"restaurant_order"}')
        j = jget(body)
        sid = j.get("session_id") if j else None
        check("SPK-2 创建会话 200", st == 200, f"got {st} {body[:80]}",
              group="口语场景", http_code=st, duration_ms=dur)
        check("SPK-2 返回 session_id", bool(sid), group="口语场景",
              http_code=st, duration_ms=dur)
        check("SPK-2 含 scenario", j and isinstance(j.get("scenario"), dict),
              group="口语场景", http_code=st, duration_ms=dur)

        if sid:
            sid_enc = urllib.parse.quote(sid, safe="")
            # WS 越权:错误 token 连会话应被拒
            url = f"{BASE['gateway']}/ws/speech/{sid_enc}?token=invalid-token"
            st, _, dur = timed_call(url, "GET",
                                    {"Upgrade": "websocket", "Connection": "Upgrade"},
                                    timeout=5)
            check("SPK-3 越权 WS → 403/401", st in (403, 401), f"got {st}",
                  group="口语场景", http_code=st, duration_ms=dur)

    # ── 下游业务服务 ──
    print("\n[下游业务服务]")
    hdr = {"Authorization": f"Bearer {token}", "X-User-Id": "1001",
           "Content-Type": "application/json"}

    st, _, dur = timed_call(BASE["learning"] + "/api/learning/event", "POST", hdr,
                            b'{"user_id":1001,"event_type":"answer","skill":"grammar","correct":true}')
    check("LRN-1 学情上报 非5xx", st is not None and st < 500, f"got {st}",
          group="下游业务", http_code=st, duration_ms=dur)

    st, _, dur = timed_call(BASE["learning"] + "/api/learning/review?user_id=1001", "GET",
                            {"Authorization": f"Bearer {token}", "X-User-Id": "1001"})
    check("LRN-2 复习推荐 非5xx", st is not None and st < 500, f"got {st}",
          group="下游业务", http_code=st, duration_ms=dur)

    st, _, dur = timed_call(BASE["writing"] + "/api/writing/grade", "POST", hdr,
                            b'{"text":"I goes to school.","prompt":"past tense"}', timeout=20)
    check("WRT-1 写作批改 服务可达", st is not None, f"got {st}",
          group="下游业务", http_code=st, duration_ms=dur)

    st, _, dur = timed_call(BASE["emotion"] + "/api/emotion/sense", "POST", hdr,
                            b'{"text":"I feel great!","user_id":1001}')
    check("EMO-1 情绪感知 非5xx", st is not None and st < 500, f"got {st}",
          group="下游业务", http_code=st, duration_ms=dur)

    st, _, dur = timed_call(BASE["teacher"] + "/api/teacher/1001/kp-stats", "GET",
                            {"Authorization": f"Bearer {token}", "X-User-Id": "1001"})
    check("TCH-1 知识点统计 非5xx", st is not None and st < 500, f"got {st}",
          group="下游业务", http_code=st, duration_ms=dur)

    # ── 汇总 ──
    print("\n" + "=" * 60)
    total = passed + failed
    print(f"结果: {passed}/{total} 通过, {failed} 失败")
    if fails:
        print("失败用例: " + ", ".join(fails))
    print("=" * 60)

    if args.json:
        report = {
            "run_ts": run_ts,
            "entry": "cluster-internal ClusterIP",
            "total": total,
            "passed": passed,
            "failed": failed,
            "results": results,
        }
        print("\n__JSON__")
        print(json.dumps(report, ensure_ascii=False, indent=2))

    # 落盘报告(无论 pass/fail 都写)
    report_paths = write_reports(args.report_dir, run_ts, total, passed, failed)
    print("\n[落盘报告]")
    for p in report_paths:
        print(f"  {p}")

    sys.exit(0 if failed == 0 else 1)


if __name__ == "__main__":
    main()
