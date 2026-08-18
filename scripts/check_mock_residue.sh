#!/usr/bin/env bash
# 生产域 Mock 零残留门禁(铁律十一 MPB,2026-08-17)
#
# 可移植门禁:任何 Claude Code 项目可直接复用。默认扫 ESP 布局,通过环境变量
# 覆盖扫描根即可适配其他项目,无需改脚本:
#   MPB_MANIFEST_DIRS  K8s manifest 目录(空格分隔,默认 deploy/k8s)
#   MPB_CODE_DIRS     后端代码根目录(默认 backend/services)
#   MPB_SECRET_FILE    生产 Secret 清单(默认 deploy/k8s/00-namespace-secret.yaml)
#
# 扫描三类违例,命中任一则退出码 1(接 CI / 本地 pre-push):
#   1. manifest:mock 模式配置 / 伪密钥
#   2. 生产代码:调用点硬编码 MockProvider、占位输出(占位/placeholder/placehold.co)
#   3. Secret:伪凭据模式(mock-key/loadtest/placeholder)
#
# 豁免设计(防误报):
#   - mock 实现文件(mock*.go)、构建标签隔离 stub(*_stub.go)、测试文件、依赖目录
#   - provider 工厂里 LLM_MODE=mock 显式开关内的 mock 分支(合法测试域,不算违例;
#     生产走 mock 由 manifest 扫描兜住)
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MANIFEST_DIRS="${MPB_MANIFEST_DIRS:-$ROOT/deploy/k8s}"
CODE_DIRS="${MPB_CODE_DIRS:-$ROOT/backend/services}"
SECRET_FILE="${MPB_SECRET_FILE:-$ROOT/deploy/k8s/00-namespace-secret.yaml}"
violations=0

red() { printf '\033[31m%s\033[0m\n' "$*"; }

# ---------- 1) 生产 K8s manifest ----------
for dir in $MANIFEST_DIRS; do
  [ -d "$dir" ] || continue
  while IFS= read -r f; do
    [ -f "$f" ] || continue
    hits=$(grep -nE 'LLM_MODE[^#]*mock|ASR_MODE[^#]*mock|TTS_MODE[^#]*mock|[Mm]ock-[Kk]ey|loadtest|placeholder' "$f" 2>/dev/null || true)
    if [ -n "$hits" ]; then
      red "VIOLATION(manifest): $f"
      echo "$hits"
      violations=1
    fi
  done < <(find "$dir" -type f \( -name '*.yaml' -o -name '*.yml' \) 2>/dev/null)
done

# ---------- 2) 生产代码(硬编码 mock 接线 / 占位输出 / 外链占位图) ----------
for dir in $CODE_DIRS; do
  [ -d "$dir" ] || continue
  while IFS= read -r f; do
    case "$f" in
      */mock*.go|*_stub.go|*_test.go|*/tests/*|*/test/*|*/.venv/*|*/target/*|*/node_modules/*|*/__pycache__/*) continue ;;
      *) ;;
    esac
    [ -f "$f" ] || continue
    # DEFERRED 豁免(铁律⑤+⑧):文件内显式标注 DEFERRED- 的整文件跳过
    # (多节点/外部引擎未接入的合规延迟项,审查兜底=多节点清单+Phase5 就绪度报告)
    if grep -q 'DEFERRED-' "$f" 2>/dev/null; then
      continue
    fi
    hits=$(grep -nE '&(asr|tts)\.MockProvider\{\}|placehold\.co|占位(句|翻译|图片|文本|音频|结果|score)?|placeholder (transcript|image|audio)' "$f" 2>/dev/null || true)
    if [ -n "$hits" ]; then
      red "VIOLATION(code): $f"
      echo "$hits"
      violations=1
    fi
  done < <(grep -rlE 'placehold\.co|占位|placeholder' "$dir" --include='*.go' --include='*.py' --include='*.rs' 2>/dev/null || true)
done

# ---------- 3) 生产 Secret 伪凭据 ----------
if [ -f "$SECRET_FILE" ]; then
  hits=$(grep -nE 'mock-key|loadtest|placeholder|sk-ant-mock' "$SECRET_FILE" 2>/dev/null || true)
  if [ -n "$hits" ]; then
    red "VIOLATION(secret): $SECRET_FILE 含伪凭据"
    echo "$hits"
    violations=1
  fi
fi

if [ "$violations" -ne 0 ]; then
  red "MPB gate FAILED: 生产域存在 mock/占位/伪密钥残留,上线前必须清零(铁律十一)"
  exit 1
fi
echo "MPB gate PASSED: 生产路径无 mock/占位/伪密钥残留"
exit 0