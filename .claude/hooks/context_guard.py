#!/usr/bin/env python3
"""
ESP 上下文防溢出 harness 钩子脚本(铁律强制版)。

机制:
  - 解析当前会话 transcript(.jsonl),取最后一条 assistant 消息的 usage。
  - 估算当前上下文占用 = input_tokens + cache_read_input_tokens + cache_creation_input_tokens
    (这三者之和 ≈ 进入模型的上下文窗口大小,最接近真实占用)。
  - 1M 上下文模型: 80% = 800k 注入提醒; 90% = 900k 强制阻断 + 提示 /compact。
  - 兜底: 若取不到 usage,用 transcript 文件大小估算(1 字节 ≈ 0.27 token,经验值)。

用法(hook 调用时):
  python3 context_guard.py [warn|block]
    warn  : 返回 JSON,在 systemMessage 注入提醒(不阻断)。
    block : 返回 JSON,decision=block + reason,强制阻断继续执行。

输出: 单行 JSON 到 stdout,供 hook 用 matcher 解析。
  {"level":"ok|warn|block","pct":NN,"est_tokens":NNNNN,"limit":1000000,"msg":"..."}
"""
import json, os, sys, glob, time

LIMIT = 200_000  # 实际模型上下文窗口 200k(glm_for_coding)。80%=160k 提醒;90%=180k 阻断。
WARN_PCT = 80
BLOCK_PCT = 90
TRANSCRIPT_DIR = os.path.expanduser(
    "~/.claude/projects/-Users-joelgong-claude-Projects-English-Study-Platform"
)


def latest_transcript():
    files = glob.glob(os.path.join(TRANSCRIPT_DIR, "*.jsonl"))
    if not files:
        return None
    files.sort(key=os.path.getmtime, reverse=True)
    return files[0]


def estimate_tokens(transcript_path):
    """优先用真实 usage;取不到则按文件大小估算。返回 (est_tokens, source)。"""
    est = 0
    try:
        with open(transcript_path, "r", errors="ignore") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    obj = json.loads(line)
                except Exception:
                    continue
                msg = obj.get("message") or {}
                usage = msg.get("usage")
                if usage and isinstance(usage, dict):
                    inp = usage.get("input_tokens", 0) or 0
                    cache_read = usage.get("cache_read_input_tokens", 0) or 0
                    cache_create = usage.get("cache_creation_input_tokens", 0) or 0
                    # 取窗口最大值:input_tokens 通常已含 cache 部分?实际 API 里 input+cache_read+cache_create = 总入口 token
                    cur = inp + cache_read + cache_create
                    if cur > est:
                        est = cur
        if est > 0:
            return est, "usage"
    except Exception:
        pass
    # 兜底: 文件大小估算
    try:
        size = os.path.getsize(transcript_path)
        est = int(size * 0.27)
        return est, "filesize"
    except Exception:
        return 0, "none"


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "warn"
    tp = latest_transcript()
    if not tp:
        print(json.dumps({"level": "ok", "pct": 0, "est_tokens": 0, "limit": LIMIT,
                          "msg": "no transcript"}))
        return
    est, source = estimate_tokens(tp)
    pct = int(est * 100 / LIMIT) if LIMIT else 0
    level = "ok"
    msg = ""
    if est >= LIMIT * BLOCK_PCT // 100:
        level = "block"
        msg = (f"⚠️ 上下文铁律强制阻断:当前占用 ~{est:,} tokens ({pct}% of {LIMIT:,},估算源={source})。"
               f"已达 {BLOCK_PCT}% 强制止损阈值。必须先落盘当前状态(PROGRESS.md + Redis)再执行 /compact。"
               f"禁止继续启动新长任务/大输出命令。")
    elif est >= LIMIT * WARN_PCT // 100:
        level = "warn"
        msg = (f"🔔 上下文铁律提醒:当前占用 ~{est:,} tokens ({pct}% of {LIMIT:,},估算源={source})。"
               f"已达 {WARN_PCT}% 主动 /compact 阈值。建议:完成当前小步后立即评估 /compact,"
               f"并双轨落盘(PROGRESS.md + Redis esp:ctx:session:recovery-*)。")
    else:
        msg = f"上下文占用 ~{est:,} tokens ({pct}%, 源={source})。"

    out = {"level": level, "pct": pct, "est_tokens": est, "limit": LIMIT, "source": source, "msg": msg}
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
