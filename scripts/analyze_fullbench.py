#!/usr/bin/env python3
"""analyze_fullbench.py — 汇总 CRDB 全服务基准矩阵(k6+locust),与旧 TiDB/CRDB fullbench 对照。

输入:
  report/benchmark-2026-08-16/k6-<ep>-v<vus>.log      (k6 summary)
  report/benchmark-2026-08-16/locust-<ep>-v<vus>.log  (locust headless)
  report/crdb-vs-tidb-fullbench-{tidb,cockroachdb}-2026-08-15.json (旧对照)
输出:
  report/benchmark-2026-08-16/analysis-2026-08-16.md  + 控制台摘要
运行: python3 scripts/analyze_fullbench.py
"""
import glob, json, os, re, sys

R = "report/benchmark-2026-08-16"

# 旧 JSON 端点 key → 本轮 ep 名
OLD_KEY = {
    "learning_event_write": "learning_event",
    "learning_review_read": "learning_review",
    "teacher_kpstats_read": "teacher_kpstats",
    "bff_scenarios_read": "bff_scenarios",
    "gw_scenarios_ref": "gw_scenarios",
}
# ep → (语言, 服务)
LANG = {
    "learning_event": ("Java", "learning-java"), "learning_review": ("Java", "learning-java"),
    "teacher_kpstats": ("Java", "teacher-java"), "bff_scenarios": ("Java", "bff-java"),
    "gw_scenarios": ("Go", "gateway-go"), "bff_login": ("Java", "bff-java"),
    "reading_passages": ("Go", "reading-go"), "listening_mat": ("Go", "listening-go"),
    "gamification_prof": ("Go", "gamification-go"), "gamification_pts": ("Go", "gamification-go"),
    "vocab_due": ("Go", "vocab-srs-go"), "vocab_review": ("Go", "vocab-srs-go"),
    "emotion_recent": ("Python", "emotion-svc"), "writingcoach_tpl": ("Python", "writing-coach"),
    "assessment_report": ("Rust", "assessment-rs"), "llm_dialogue": ("Rust", "llm-orchestrator-rs"),
    "course_path": ("Go", "course-path-go"), "writing_grade": ("Python", "writing-py"),
}

def parse_ms(v):
    v = v.strip()
    if v.endswith("ms"):
        return round(float(v[:-2]), 1)
    if v.endswith("s"):
        return round(float(v[:-1]) * 1000, 1)
    try:
        return round(float(v), 1)
    except ValueError:
        return None

def parse_k6(path):
    """返回 {rps, p50, p90, p95, p99, fail_pct, reqs}"""
    try:
        txt = open(path).read()
    except FileNotFoundError:
        return None
    out = {}
    m = re.search(r"http_reqs\.*:\s+\d+\s+([\d.]+)/s", txt)
    if m: out["rps"] = float(m.group(1))
    m = re.search(r"http_req_duration\.*:.*?med=([\d.]+[ms]?s?)\s+.*?p\(90\)=([\d.]+[ms]?s?)\s+p\(95\)=([\d.]+[ms]?s?)(?:\s+p\(99\)=([\d.]+[ms]?s?))?", txt)
    if m:
        out["p50"] = parse_ms(m.group(1)); out["p90"] = parse_ms(m.group(2)); out["p95"] = parse_ms(m.group(3))
        out["p99"] = parse_ms(m.group(4)) if m.group(4) else None
    m = re.search(r"http_req_failed\.*:\s+([\d.]+)%\s+(\d+) out of (\d+)", txt)
    if m: out["fail_pct"] = float(m.group(1)); out["reqs"] = int(m.group(3))
    m = re.search(r"http_reqs\.*:\s+(\d+)\s", txt)
    if m and "reqs" not in out: out["reqs"] = int(m.group(1))
    return out or None

def parse_locust(path):
    """返回 {rps, p95, fail_pct, reqs, avg_ms}"""
    try:
        txt = open(path).read()
    except FileNotFoundError:
        return None
    m = re.search(r">>> Test stopped\. requests=(\d+) failures=(\d+) avg_rps=([\d.]+) avg_latency_ms=([\d.]+) p95_ms=([\d.]+) fail_rate=([\d.]+)", txt)
    if not m:
        return None
    return {
        "reqs": int(m.group(1)), "fails": int(m.group(2)), "rps": float(m.group(3)),
        "avg_ms": float(m.group(4)), "p95": float(m.group(5)), "fail_pct": float(m.group(6)) * 100,
    }

def load_old():
    old = {}
    for mode in ("tidb", "cockroachdb"):
        p = f"report/crdb-vs-tidb-fullbench-{mode}-2026-08-15.json"
        if not os.path.exists(p):
            continue
        d = json.load(open(p))
        rows = {}
        for key, ep in OLD_KEY.items():
            rows[ep] = {r["concurrency"]: r for r in d["results"].get(key, [])}
        old[mode] = {"time": d.get("time", ""), "rows": rows}
    return old

def fmt_cell(v, width=14):
    return (str(v) if v is not None else "-").rjust(width)

def md_table(headers, rows):
    lines = ["| " + " | ".join(headers) + " |", "|" + "|".join("---" for _ in headers) + "|"]
    for r in rows:
        lines.append("| " + " | ".join(str(c) for c in r) + " |")
    return "\n".join(lines)

def main():
    old = load_old()
    k6 = {}   # ep -> vus -> data
    loc = {}
    for p in sorted(glob.glob(f"{R}/k6-*.log")):
        name = os.path.basename(p)[len("k6-"):-len(".log")]
        ep, vus = name.rsplit("-v", 1)
        d = parse_k6(p)
        if d: k6.setdefault(ep, {})[int(vus)] = d
    for p in sorted(glob.glob(f"{R}/locust-*.log")):
        name = os.path.basename(p)[len("locust-"):-len(".log")]
        ep, vus = name.rsplit("-v", 1)
        d = parse_locust(p)
        if d: loc.setdefault(ep, {})[int(vus)] = d

    all_eps = sorted(set(list(k6) + list(loc)))
    md = [f"# ESP 全服务基准分析(2026-08-16,CRDB 3副本,单节点)", ""]
    md.append(f"> 本轮条件:单节点 RD(9核/30G,loadavg~24,CPU 饥饿)集群内 ClusterIP 直连,25s 稳态;"
              f"旧对照为 2026-08-15 同端点 fullbench(locust-python)。CAVEAT:旧代码无 messageId 幂等/旧条件/节点负载不同,纵向对照仅供量级参考。")
    md.append("")

    # A. CRDB vs 旧TiDB 对照(core 5 × v20/v60)
    md.append("## A. CRDB-vs-旧TiDB 对照(核心5端点 × v20/v60)")
    md.append("")
    rows = []
    for ep in ["learning_event", "learning_review", "teacher_kpstats", "bff_scenarios", "gw_scenarios"]:
        for vus in (20, 60):
            old_t = old.get("tidb", {}).get("rows", {}).get(ep, {}).get(vus, {})
            old_c = old.get("cockroachdb", {}).get("rows", {}).get(ep, {}).get(vus, {})
            k = (k6.get(ep) or {}).get(vus, {})
            l = (loc.get(ep) or {}).get(vus, {})
            rows.append([
                ep, vus,
                fmt_cell(old_t.get("rps")), fmt_cell(old_t.get("P95_ms")), fmt_cell(old_t.get("fail_rate")),
                fmt_cell(old_c.get("rps")), fmt_cell(old_c.get("P95_ms")), fmt_cell(old_c.get("fail_rate")),
                fmt_cell(k.get("rps")), fmt_cell(k.get("p95")), fmt_cell(k.get("fail_pct")),
                fmt_cell(l.get("rps")), fmt_cell(l.get("p95")), fmt_cell(l.get("fail_pct")),
            ])
    md.append(md_table(["端点", "并发", "旧TiDB rps", "P95", "fail%", "旧CRDB rps", "P95", "fail%",
                        "本轮k6 rps", "P95", "fail%", "本轮loc rps", "P95", "fail%"], rows))
    md.append("")

    # B. 全语言全服务对比(k6 v10;无则 locust v10)
    md.append("## B. 全语言×全服务对比(k6 v10,同一稳态)")
    md.append("")
    rows = []
    for ep in all_eps:
        lang, svc = LANG.get(ep, ("?", "?"))
        k = (k6.get(ep) or {}).get(10, {})
        if not k:
            continue
        rows.append([ep, svc, lang, fmt_cell(k.get("rps")), fmt_cell(k.get("p50")), fmt_cell(k.get("p95")),
                     fmt_cell(k.get("p99")), fmt_cell(k.get("fail_pct")), fmt_cell(k.get("reqs"))])
    md.append(md_table(["端点", "服务", "语言", "RPS", "P50(ms)", "P95(ms)", "P99(ms)", "fail%", "请求数"], rows))
    md.append("")

    # C. k6 vs locust 相关
    md.append("## C. k6-vs-locust 相关性(同端点背对背)")
    md.append("")
    rows = []
    for ep in all_eps:
        for vus in sorted(set((k6.get(ep) or {})) & set((loc.get(ep) or {}))):
            k = (k6.get(ep) or {}).get(vus, {}); l = (loc.get(ep) or {}).get(vus, {})
            if not k or not l:
                continue
            ratio = round(k.get("rps", 0) / l.get("rps", 0), 2) if l.get("rps") else None
            rows.append([ep, vus, fmt_cell(k.get("rps")), fmt_cell(l.get("rps")), fmt_cell(ratio),
                         fmt_cell(k.get("p95")), fmt_cell(l.get("p95"))])
    if rows:
        md.append(md_table(["端点", "并发", "k6 rps", "locust rps", "k6/loc", "k6 p95", "loc p95"], rows))
    else:
        md.append("(无重叠端点)")
    md.append("")

    out = f"{R}/analysis-2026-08-16.md"
    open(out, "w").write("\n".join(md))
    print(f"==> {out} ({len(all_eps)} eps; k6 files={len(k6)} locust={len(loc)})")
    print(f"==> k6 汇总(v10):" + " ".join(f"{e}={k6[e][10]['rps']}r/s" for e in sorted(k6) if 10 in k6[e]))

if __name__ == "__main__":
    main()
