#!/usr/bin/env bash
# Phase3 全链路 E2E 冒烟测试
# 前置: kubectl port-forward 或 BFF NodePort 30009 / GW NodePort 30080 可达
set -euo pipefail
BFF="${BFF_URL:-http://127.0.0.1:30009}"
GW="${GW_URL:-http://127.0.0.1:30080}"
PASS=0; FAIL=0

check() {
  local name="$1" code="$2" expect="$3"
  if [ "$code" = "$expect" ]; then echo "[PASS] $name ($code)"; PASS=$((PASS+1))
  else echo "[FAIL] $name (got $code, expect $expect)"; FAIL=$((FAIL+1)); fi
}

echo "=== 1. Gateway 存活 ==="
check "gateway-health" "$(curl -s -o /dev/null -w '%{http_code}' "$GW/healthz")" "200"

echo "=== 2. BFF Auth ==="
# 注册
REG=$(curl -s -w '\n%{http_code}' -X POST "$BFF/api/auth/register" \
  -H 'Content-Type: application/json' \
  -d '{"email":"e2e-'$RANDOM'@test.com","password":"Test1234!","user_name":"E2E"}')
REG_CODE=$(echo "$REG" | tail -1); REG_BODY=$(echo "$REG" | head -1)
check "register" "$REG_CODE" "200"

# 登录(用固定测试账号,注册可能重复)
LOGIN=$(curl -s -w '\n%{http_code}' -X POST "$BFF/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"e2e-test@test.com","password":"Test1234!"}')
LOGIN_CODE=$(echo "$LOGIN" | tail -1)
TOKEN=$(echo "$LOGIN" | head -1 | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)
USERID=$(echo "$LOGIN" | head -1 | python3 -c "import sys,json; print(json.load(sys.stdin).get('user',{}).get('id',''))" 2>/dev/null || true)
check "login" "$LOGIN_CODE" "200"

if [ -z "$TOKEN" ]; then
  echo "[WARN] 无 token,后续鉴权测试可能失败"
fi
if [ -z "$USERID" ]; then
  echo "[WARN] 无 userId,依赖 userId 的测试将跳过"
fi

# JWT 无空格,用 -H 头部不带内层引号,避免 ${AUTH[@]} 展开时被 curl 拆分参数
AUTH=(-H "Authorization: Bearer $TOKEN")

echo "=== 3. Content Library ==="
check "courses" "$(curl -s -o /dev/null -w '%{http_code}' "$BFF/api/content/courses" "${AUTH[@]}")" "200"

echo "=== 4. Community ==="
check "circles" "$(curl -s -o /dev/null -w '%{http_code}' "$BFF/api/community/circles" "${AUTH[@]}")" "200"

echo "=== 5. Gamification ==="
check "rankings" "$(curl -s -o /dev/null -w '%{http_code}' "$BFF/api/gamification/rankings?type=xp&scope=global" "${AUTH[@]}")" "200"
check "digital-human" "$(curl -s -o /dev/null -w '%{http_code}' "$BFF/api/gamification/me/digital-human" "${AUTH[@]}")" "200"

echo "=== 6. Learning ==="
check "learning-review" "$(curl -s -o /dev/null -w '%{http_code}' "$BFF/api/learning/review" "${AUTH[@]}")" "200"
check "learning-recommend" "$(curl -s -o /dev/null -w '%{http_code}' "$BFF/api/learning/recommend?user_id=${USERID:-0}" "${AUTH[@]}")" "200"

echo "=== 7. Teacher ==="
check "teacher" "$(curl -s -o /dev/null -w '%{http_code}' "$BFF/api/teacher/${USERID:-0}/kp-stats" "${AUTH[@]}")" "200"

echo "=== 8. Writing ==="
check "writing" "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BFF/api/writing/grade" -H 'Content-Type: application/json' "${AUTH[@]}" -d '{"prompt":"Write a short essay","essay":"Hello world this is a test"}')" "200"

echo "=== 9. Emotion ==="
check "emotion" "$(curl -s -o /dev/null -w '%{http_code}' "$BFF/api/emotion/recent/${USERID:-0}?limit=1" "${AUTH[@]}" 2>/dev/null || echo 000)" "200"

echo "=== 10. Vocab ==="
check "vocab" "$(curl -s -o /dev/null -w '%{http_code}' "$BFF/api/vocab/due?uid=${USERID:-0}" "${AUTH[@]}")" "200"

echo "=== 11. ESP Admin ==="
check "admin" "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:30099/healthz 2>/dev/null || echo 000)" "200"

echo
echo "=== 结果: $PASS 通过 / $FAIL 失败 ==="
[ $FAIL -eq 0 ] && echo "ALL GREEN" || echo "HAS FAILURES"
exit $FAIL
