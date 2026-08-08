#!/usr/bin/env python3
"""
ESP PreToolUse Bash 守卫:拦截会把大量输出灌进主上下文的命令。
铁律:禁裸 kubectl logs(无 --tail)/裸 cat 大文件/裸全量 build test stdout。

输入(stdin): Claude Code PreToolUse hook 的 JSON,含 tool_input.command。
输出: 单行 JSON。
  - 放行: {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}
  - 阻断: {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"..."}}
"""
import json, sys, re


def check(cmd):
    """返回阻断理由;None=放行。"""
    if not cmd:
        return None
    # 去掉环境变量前缀和换行,聚焦命令本身
    c = re.sub(r"^\w+=\S+\s*", "", cmd.strip())
    c_one = " ".join(c.split())

    # 1. kubectl logs 必须带 --tail 或 -N 限制
    if re.search(r"\bkubectl\b.*\blogs\b", c) and "--tail" not in c and not re.search(
        r"--tail=\d+", c
    ):
        # 允许 logs --previous 也需限流
        return "kubectl logs 必须加 --tail=N 限流(铁律:禁裸 kubectl logs)。请改 kubectl ... logs <pod> --tail=20。"

    # 2. 裸 cat 大文件(非 /tmp/ 小文件)—— 禁 cat 日志/jsonl/大配置
    m = re.search(r"\bcat\s+(\S+)", c)
    if m:
        target = m.group(1)
        # 允许 cat 小的 /tmp 或 .claude 配置
        if not target.startswith("/tmp/") and not target.startswith(".claude/"):
            if re.search(r"\.(log|jsonl|csv)$", target) or "PROGRESS" in target or target.endswith(".md"):
                return f"禁止裸 cat {target}(铁律:大文件用 Read 切片或 grep/head 限流)。"

    # 3. 全量 build/test stdout 未限流:maven/go test/pytest 不带 -q 或未重定向
    # mvn: 允许 -q 或有 2>&1 | tail/grep
    if re.search(r"\bmvn\b", c) and "-q" not in c and not re.search(r"\|\s*(tail|grep|head)", c):
        return "mvn 命令未限流(铁律:build/test 只看 RC+失败摘要)。请加 -q 或 2>&1 | tail -30。"
    if re.search(r"\bgo\s+(test|build)\b", c) and not re.search(r"2>&1\s*\|\s*(tail|grep|head)", c) and ">/dev/null" not in c:
        # go build 静默 OK,go test 需限流
        if re.search(r"\bgo\s+test\b", c):
            return "go test 未限流。请加 2>&1 | tail -30。"
    if re.search(r"\bpytest\b", c) and "-q" not in c and not re.search(r"\|\s*(tail|grep|head)", c):
        return "pytest 未限流。请加 -q 或 2>&1 | tail -30。"

    # 4. locust 压测必须重定向到文件
    if re.search(r"\blocust\b", c) and not re.search(r">\s*\S+", c) and "--headless" in c:
        return "locust 压测必须重定向到 report/ 或 logs/(铁律:大输出落文件,主线程只读摘要)。"

    return None


def main():
    try:
        data = json.load(sys.stdin)
    except Exception:
        # 解析失败放行(不阻断业务)
        print(json.dumps({"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "allow"}}))
        return
    cmd = (data.get("tool_input") or {}).get("command", "") or ""
    reason = check(cmd)
    if reason:
        print(json.dumps({"hookSpecificOutput": {"hookEventName": "PreToolUse",
                                                  "permissionDecision": "deny",
                                                  "permissionDecisionReason": reason}}, ensure_ascii=False))
    else:
        print(json.dumps({"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "allow"}}))


if __name__ == "__main__":
    main()
