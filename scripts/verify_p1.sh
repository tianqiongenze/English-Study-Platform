#!/usr/bin/env bash
# P1 验证清单(会话22-24):幂等/outbox/ratelimit/cache metrics/JWT refresh+logout
# 入口:traefik 127.0.0.1:80 + Host: esp.example.com(单节点 RD localhost 转发)
set -u
BASE="http://127.0.0.1"
H='Host: esp.example.com'
PASS=0; FAIL=0
ok()   { PASS=$((PASS+1)); echo "  ✅ $1"; }
bad()  { FAIL=$((FAIL+1)); echo "  ❌ $1"; }

echo "=== 1. 注册/登录获取 token ==="
TS=$(date +%s)
REG=$(curl -s -m 10 -H "$H" -X POST $BASE/api/auth/register -H 'Content-Type: application/json' \
  -d "{\"email\":\"p1verify_$TS@test.com\",\"password\":\"pw123456\",\"name\":\"p1\"}")
TOKEN=$(echo "$REG" | python3 -c 'import sys,json;print(json.load(sys.stdin).get("token",""))' 2>/dev/null)
REFRESH=$(echo "$REG" | python3 -c 'import sys,json;print(json.load(sys.stdin).get("refreshToken",""))' 2>/dev/null)
if [ -n "$TOKEN" ] && [ "$TOKEN" != "" ]; then ok "register 签发 token+refresh"; else bad "register: $REG"; TOKEN=""; fi

echo "=== 2. P1-9 JWT refresh(rotation) ==="
if [ -n "$REFRESH" ] && [ "$REFRESH" != "" ]; then
  R1=$(curl -s -m 10 -H "$H" -X POST $BASE/api/auth/refresh -H 'Content-Type: application/json' -d "{\"refreshToken\":\"$REFRESH\"}")
  NEWTOKEN=$(echo "$R1" | python3 -c 'import sys,json;print(json.load(sys.stdin).get("token",""))' 2>/dev/null)
  [ -n "$NEWTOKEN" ] && [ "$NEWTOKEN" != "" ] && ok "refresh 换新 token" || bad "refresh: $R1"
  # 旧 refresh 重放应被拒(一次性)
  R2=$(curl -s -m 10 -o /dev/null -w "%{http_code}" -H "$H" -X POST $BASE/api/auth/refresh -H 'Content-Type: application/json' -d "{\"refreshToken\":\"$REFRESH\"}")
  [ "$R2" = "401" ] || [ "$R2" = "400" ] && ok "旧 refresh 重放被拒($R2)" || bad "旧 refresh 重放未被拒($R2)"
  TOKEN="$NEWTOKEN"
else
  bad "无 refreshToken,跳过 refresh 验证"
fi

echo "=== 3. P1-8 幂等重放(learning event 同 key 二次提交) ==="
IDEM="p1-verify-$TS"
B1=$(curl -s -m 10 -H "$H" -H "Authorization: Bearer $TOKEN" -H "Idempotency-Key: $IDEM" \
  -X POST $BASE/api/learning/event -H 'Content-Type: application/json' \
  -d '{"event_type":"answer","payload":"{\"kp\":1}"}')
B2=$(curl -s -m 10 -H "$H" -H "Authorization: Bearer $TOKEN" -H "Idempotency-Key: $IDEM" \
  -X POST $BASE/api/learning/event -H 'Content-Type: application/json' \
  -d '{"event_type":"answer","payload":"{\"kp\":1}"}')
if [ "$B1" = "$B2" ] && [ -n "$B1" ]; then ok "同 key 重放返回同 body($B1)"; else bad "重放 body 不一致: [$B1] vs [$B2]"; fi

echo "=== 4. P1-9 logout(拉黑 jti) ==="
if [ -n "$TOKEN" ]; then
  LO=$(curl -s -m 10 -o /dev/null -w "%{http_code}" -H "$H" -H "Authorization: Bearer $TOKEN" -X POST $BASE/api/auth/logout)
  [ "$LO" = "200" ] && ok "logout 200" || bad "logout=$LO"
  V=$(curl -s -m 10 -o /dev/null -w "%{http_code}" -H "$H" -H "Authorization: Bearer $TOKEN" $BASE/api/learning/review)
  [ "$V" = "401" ] && ok "logout 后旧 token 被拒($V)" || bad "logout 后旧 token 仍可用($V)"
fi

echo "=== 5. P1-12 cache metrics ==="
CM=$(kubectl -n esp-prod exec deploy/learning-java -- wget -qO- -T 5 http://localhost:8081/actuator/metrics/cache.gets 2>/dev/null | head -c 300)
echo "$CM" | grep -q "COUNT\|measurements" && ok "cache.gets 有数据" || bad "cache.gets: $CM"

echo "=== 6. P1-10 outbox 表状态 ==="
OB=$(bash scripts/db_query.sh "SELECT status, COUNT(*) c FROM esp.learning_event_outbox GROUP BY status" 2>/dev/null | tail -4)
[ -n "$OB" ] && ok "outbox: $OB" || bad "outbox 查询失败"

echo "=== 7. P1-11 learning-worker 消费 ==="
kubectl -n esp-prod get deploy learning-worker --no-headers 2>/dev/null | awk '{print "  worker: "$2" (ready)"}'

echo ""
echo "===== 结果: PASS=$PASS FAIL=$FAIL ====="
