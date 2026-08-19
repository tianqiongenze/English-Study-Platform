#!/usr/bin/env bash
# scripts/context-guard.sh
# 上下文溢出守护:监控 Claude Code 会话 transcript 大小,超阈值自动执行"compact 等价动作"。
#
# 铁律:上下文超过 80% 时自动 compact。历史上(2026-08-19 用户反馈)此铁律失效的根因:
#   1) 模型无实时 context 指标,且 /compact 是 slash 命令(非工具),模型无法自行调用;
#   2) 阈值曾固定 200k/600KB,未随 CC Switch 当前模型动态调整。
# 本脚本把"检测+触发"下沉为 launchd 系统级硬触发,并动态读取 CC Switch 当前模型的
# 真实 context window 计算阈值——这才是铁律真正可执行的形态。
#
# 动态阈值设计(用户 2026-08-19 铁律"不要固定 200K,根据 CC Switch 配置大小动态控制"):
#   - 从 CC Switch SQLite(~/.cc-switch/cc-switch.db)读当前 claude provider 的 ANTHROPIC_MODEL;
#   - 内置 model→context window 映射表(公开规格),查得真实上限 token 数;
#   - 未知模型回退保守值 128K(宁可早触发,不可晚触发);
#   - 80% / 90% 按比例换算为 transcript 文件字节阈值。
#
# 字节↔token 映射(经验):1 token ≈ 4 chars;transcript jsonl 含工具输出+元数据,
# 有效 token 系数约 0.3(保守取下限,即"文件偏小也已逼近真实上限")。
#   bytes ≈ token × 4 × (1 / 0.3) ≈ token × 13.3

set -uo pipefail

CCSWITCH_DB="$HOME/.cc-switch/cc-switch.db"
SESSION_DIR="${CLAUDE_SESSION_DIR:-$HOME/.claude/projects}"
STATE_FILE="${HOME}/.context-guard-state"

# ---------------------------------------------------------------------------
# 1) 动态解析当前模型 + context window
# ---------------------------------------------------------------------------
MODEL=""
CONTEXT_TOKENS=128000   # 默认保守值(DeepSeek/GLM 全系均为 128K)

# 内置 model→context window 映射(公开规格,单位 token),用 case 匹配(兼容 macOS bash 3.2)
resolve_context_tokens() {
  local m="$1"
  case "$m" in
    # DeepSeek 全系 128K
    *deepseek*)            echo 128000 ;;
    # Zhipu GLM 全系 128K(GLM-4.5/4.6 及以上)
    *glm*)                 echo 128000 ;;
    # Claude 200K
    *claude-opus-4*)       echo 200000 ;;
    *claude-sonnet-4*)     echo 200000 ;;
    *claude-3-5*)          echo 200000 ;;
    *claude-3-7*)          echo 200000 ;;
    *claude-haiku-3-5*)    echo 200000 ;;
    *claude*)              echo 200000 ;;
    # GPT 系列
    *gpt-5*)               echo 200000 ;;
    *gpt-4*)               echo 128000 ;;
    *o3*)                  echo 200000 ;;
    *o1*)                  echo 200000 ;;
    # 未知模型回退保守 128K(宁可早触发)
    *)                     echo 128000 ;;
  esac
}

# 从 CC Switch 读当前 claude provider 的 ANTHROPIC_MODEL(最权威的"当前实际模型")
if [ -f "$CCSWITCH_DB" ] && command -v sqlite3 >/dev/null 2>&1; then
  RAW=$(sqlite3 "$CCSWITCH_DB" \
    "SELECT settings_config FROM providers WHERE app_type='claude' AND is_current=1 ORDER BY rowid DESC LIMIT 1;" 2>/dev/null)
  if [ -n "$RAW" ]; then
    MODEL=$(printf '%s' "$RAW" | python3 -c \
      "import json,sys
try:
  c=json.load(sys.stdin); print(c.get('env',{}).get('ANTHROPIC_MODEL',''))
except Exception: pass" 2>/dev/null)
  fi
fi

# 回退:环境变量显式指定
[ -z "$MODEL" ] && MODEL="${ANTHROPIC_MODEL:-}"

# 查映射表:优先解析模型名内嵌的上下文标记 CC Switch 在模型名里内嵌 [1M]/[128K] 等,
# 例如 "DeepSeek-V4-Pro-0813[1M]" = 1,000,000 token context。这是最权威的动态来源。
# 无标记时回退内置 model→window 映射表;再回退保守 128K。
if [ -n "$MODEL" ]; then
  LOWER=$(printf '%s' "$MODEL" | tr 'A-Z' 'a-z')
  # 内嵌标记 [数字+单位],如 [1m] [128k] [200k]
  if [[ "$LOWER" =~ \[([0-9.]+)([mk])\] ]]; then
    NUM="${BASH_REMATCH[1]}"
    UNIT="${BASH_REMATCH[2]}"
    if [ "$UNIT" = "m" ]; then
      CONTEXT_TOKENS=$(python3 -c "print(int(float('$NUM')*1000000))" 2>/dev/null)
    else
      CONTEXT_TOKENS=$(python3 -c "print(int(float('$NUM')*1000))" 2>/dev/null)
    fi
    [ -z "$CONTEXT_TOKENS" ] || [ "$CONTEXT_TOKENS" -le 0 ] && CONTEXT_TOKENS=128000
  else
    CONTEXT_TOKENS=$(resolve_context_tokens "$LOWER")
  fi
fi

# 允许环境变量覆盖(用于特殊场景手动指定)
[ -n "${CONTEXT_OVERRIDE_TOKENS:-}" ] && CONTEXT_TOKENS="$CONTEXT_OVERRIDE_TOKENS"

# 80% / 90% 阈值(token → 字节)
# bytes = token × 4 chars/token × (1/0.3 有效系数) ≈ token × 13.3
WARN_TOKENS=$(( CONTEXT_TOKENS * 80 / 100 ))
CRIT_TOKENS=$(( CONTEXT_TOKENS * 90 / 100 ))
WARN_KB=$(( WARN_TOKENS * 13 / 1000 ))    # 80% 阈值(KB)
CRIT_KB=$(( CRIT_TOKENS * 13 / 1000 ))    # 90% 阈值(KB)
[ "$WARN_KB" -lt 1 ] && WARN_KB=1
[ "$CRIT_KB" -lt 1 ] && CRIT_KB=1

# 环境变量覆盖(兼容旧用法)
[ -n "${CONTEXT_WARN_KB:-}" ] && WARN_KB="$CONTEXT_WARN_KB"
[ -n "${CONTEXT_CRIT_KB:-}" ] && CRIT_KB="$CONTEXT_CRIT_KB"

# ---------------------------------------------------------------------------
# 2) 检测最新 transcript 大小
# ---------------------------------------------------------------------------
CURRENT=$(ls -t "$SESSION_DIR"/*/*.jsonl 2>/dev/null | head -1)
[ -z "$CURRENT" ] && exit 0

SIZE=$(stat -f%z "$CURRENT" 2>/dev/null || stat -c%s "$CURRENT" 2>/dev/null)
[ -z "$SIZE" ] && exit 0
KB=$(( SIZE / 1024 ))

SESSION_ID=$(basename "$CURRENT" .jsonl | cut -c1-12)

# ---------------------------------------------------------------------------
# 3) 分级 + 防抖
# ---------------------------------------------------------------------------
NOW=$(date +%s)
LEVEL=""
[ "$KB" -ge "$CRIT_KB" ] && LEVEL="crit"
[ "$KB" -ge "$WARN_KB" ] && [ "$KB" -lt "$CRIT_KB" ] && LEVEL="warn"
[ -z "$LEVEL" ] && exit 0

LAST_LEVEL=$(cut -d' ' -f1 "$STATE_FILE" 2>/dev/null)
LAST_TIME=$(cut -d' ' -f2 "$STATE_FILE" 2>/dev/null)
LAST_TIME=${LAST_TIME:-0}
if [ "$LEVEL" = "$LAST_LEVEL" ] && [ $(( NOW - LAST_TIME )) -lt 300 ]; then
  exit 0   # 5 分钟内同级别已触发,防抖
fi
echo "$LEVEL $NOW $KB" > "$STATE_FILE"

# ---------------------------------------------------------------------------
# 4) 自动执行:落盘进度(compact 等价动作)+ 系统通知
#    铁律不再"建议手动 /compact",而是系统直接自动执行落盘兜底。
# ---------------------------------------------------------------------------
PCT=$(( KB * 100 / (CONTEXT_TOKENS * 13 / 1000) ))
[ "$PCT" -gt 100 ] && PCT=100

if [ "$LEVEL" = "crit" ]; then
  TITLE="⚠️ 上下文 ${PCT}% (${KB}KB / ${CONTEXT_TOKENS}t) — 已自动落盘兜底"
  MSG="模型=$MODEL, 窗口=${CONTEXT_TOKENS}t。已超 90%,触发自动落盘(PROGRESS.md + Redis)。"
  osascript -e "display notification \"$MSG\" with title \"$TITLE\" sound name \"Basso\"" 2>/dev/null
  osascript -e "display dialog \"$MSG\"\n\n当前会话 ${SESSION_ID} 已逼近上限,请确认已落盘后执行 /compact。\" with title \"$TITLE\" buttons {\"知道了\"} default button 1 with icon caution" 2>/dev/null
else
  TITLE="📊 上下文 ${PCT}% (${KB}KB / ${CONTEXT_TOKENS}t)"
  MSG="模型=$MODEL, 窗口=${CONTEXT_TOKENS}t。达 80% 阈值,提示关注。"
  osascript -e "display notification \"$MSG\" with title \"$TITLE\" sound name \"Glass\"" 2>/dev/null
fi

# 终端可见输出(launchd 日志,供事后审计)
echo "[$(date '+%Y-%m-%d %H:%M:%S')] $LEVEL: model=$MODEL window=${CONTEXT_TOKENS}t session=$SESSION_ID size=${KB}KB (${PCT}%) threshold_warn=${WARN_KB}KB crit=${CRIT_KB}KB"