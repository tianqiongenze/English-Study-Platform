#!/bin/bash
# Claude Code 走 LCO 切流验证脚本(会话23-18 切流后执行)
#
# 前置:已完全 exit 旧 Claude Code,重开新 zsh(zshrc:169 export 15720 生效)+ 新 claude。
# 本脚本验证切流是否成功:Claude Code 主进程 TCP 连 15720(LCO)而非 15721(CC Switch)。
#
# 用法:bash scripts/verify-llm-cache-switch.sh
# 期望:全部 PASS。若 FAIL,查 memory cc-switch-locks-anthropic-base-url-15721。

set -uo pipefail
PASS=0; FAIL=0
ok(){ echo "✅ PASS: $1"; PASS=$((PASS+1)); }
no(){ echo "❌ FAIL: $1"; FAIL=$((FAIL+1)); }

echo "=== 1. 当前 shell env(应为 15720)==="
if [ "$ANTHROPIC_BASE_URL" = "http://127.0.0.1:15720" ]; then ok "shell env ANTHROPIC_BASE_URL=15720"; else no "shell env=$ANTHROPIC_BASE_URL(期望15720,zshrc:169 未生效?需重开 zsh)"; fi

echo "=== 2. LCO 进程 + 端口 15720 ==="
if lsof -nP -iTCP:15720 -sTCP:LISTEN >/dev/null 2>&1; then ok "LCO 15720 监听中"; else no "LCO 15720 未监听(启动 scripts/start-lco.sh)"; fi

echo "=== 3. Claude Code 主进程 TCP 连接(应为 15720)==="
# 找 claude 主进程(非 cc-switch / llm-cache-proxy)
CC_PID=$(pgrep -f 'claude' | while read p; do
  cmd=$(ps -o command= -p $p 2>/dev/null)
  case "$cmd" in *cc-switch*|*llm-cache-proxy*|*grep*|*verify-llm-cache*) ;; *) echo $p; break;; esac
done)
if [ -n "$CC_PID" ]; then
  CONN=$(lsof -nP -p "$CC_PID" 2>/dev/null | grep -oE '127\.0\.0\.1:[0-9]+->127\.0\.0\.1:1572[01]' | head -1)
  echo "   Claude Code PID $CC_PID 连接: $CONN"
  case "$CONN" in *15720*) ok "Claude Code 连 LCO(15720)";; *15721*) no "Claude Code 仍连 CC Switch(15721),未切 LCO";; *) no "Claude Code 未连 15720/15721($CONN)";; esac
else
  no "找不到 Claude Code 主进程"
fi

echo "=== 4. LCO metrics 命中率(切流后应增长)==="
curl -s --max-time 5 http://127.0.0.1:15720/metrics 2>/dev/null | grep -E 'lco_cache_hits_total|lco_cache_misses_total|lco_l2_hits|lco_l2_misses' | while read line; do echo "   $line"; done
echo "   (对比切流前基线:hits{L2}=3 / misses{L1}=6。新会话对话后应大幅增长)"

echo "=== 5. LCO healthz(三层状态)==="
HZ=$(curl -s --max-time 5 http://127.0.0.1:15720/healthz 2>/dev/null)
echo "   $HZ"

echo ""
echo "=== 结果: $PASS PASS / $FAIL FAIL ==="
if [ $FAIL -eq 0 ]; then
  echo "🎉 切流成功!Claude Code LLM 调用走 LCO 三层缓存。"
  echo "下一步:正常使用 Claude Code 几轮对话,再跑本脚本看命中率增长。"
else
  echo "⚠ 切流未完全成功。查 FAIL 项 + memory cc-switch-locks-anthropic-base-url-15721。"
fi
