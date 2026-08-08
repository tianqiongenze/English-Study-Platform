#!/usr/bin/env bash
# ESP e2e 人工测试脚本(集群外入口:host localhost:80 + Host: esp.example.com)
# 单节点铁律:纯串行 + 10s 间隔 + 分3组 + 组间30s,避免 CPU burst 打挂 API server。
# 用法: bash deploy/newman/run-e2e-host.sh
set +e
HOST_HDR="Host: esp.example.com"
BASE="http://localhost:80"
PASS=0; FAIL=0; TOTAL=0
declare -a RESULTS
GAP=10        # 单步间隔
BGAP=30       # 组间间隔
REPORT_DIR="${REPORT_DIR:-report/e2e}"
mkdir -p "$REPORT_DIR"

check() {
  TOTAL=$((TOTAL+1))
  local name="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then
    RESULTS+=("✅ $name -> $got")
    PASS=$((PASS+1))
  else
    RESULTS+=("❌ $name -> got=$got want=$want")
    FAIL=$((FAIL+1))
  fi
}

echo "================ ESP e2e (host ingress) ================"
echo "入口: $BASE (Host: esp.example.com)  单步间隔 ${GAP}s 组间 ${BGAP}s"
echo

# ============================================================
# 组1: 鉴权 + 场景(拿 JWT,后续依赖)
# ============================================================
echo "==== 组1/3 鉴权+场景 ===="

# AUTH-1 wx-login 拿 RS256 JWT
echo "[1/12] AUTH-1 wx-login ..."
LOGIN=$(curl -s -m 60 -H "$HOST_HDR" -H "Content-Type: application/json" -X POST -d '{"code":"e2e_host_001"}' "$BASE/api/auth/wx-login")
TOKEN=$(echo "$LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null)
if [ -n "$TOKEN" ]; then
  check "AUTH-1 wx-login 签发JWT" "200" "200"
else
  check "AUTH-1 wx-login 签发JWT" "000" "200"
  echo "  登录响应: $(echo "$LOGIN" | head -c 200)"
fi
sleep $GAP

# AUTH-2 无token 受保护资源
# 注意:gateway-go handleCreateSession 先解 body(scenario_code),body 缺失直接 400,先校验 token 拿不到。
# 因此用合法 body + 无 token → 期望 401,反映鉴权拦截。
echo "[2/12] AUTH-2 无token应401 ..."
C=$(curl -s -m 15 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" -H "Content-Type: application/json" -X POST -d '{"scenario_code":"airport_checkin"}' "$BASE/api/sessions")
check "AUTH-2 无token应401" "$C" "401"
sleep $GAP

# AUTH-3 伪造 X-User-Id 应401
echo "[3/12] AUTH-3 伪造X-User-Id应401 ..."
C=$(curl -s -m 15 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" -H "Content-Type: application/json" -H "X-User-Id: 999" -X POST -d '{"scenario_code":"airport_checkin"}' "$BASE/api/sessions")
check "AUTH-3 伪造头应401" "$C" "401"
sleep $GAP

# SPK-1 场景列表
echo "[4/12] SPK-1 场景列表 ..."
SC=$(curl -s -m 20 -H "$HOST_HDR" "$BASE/api/scenarios" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[0]['code'] if d else '')" 2>/dev/null)
C=$(curl -s -m 20 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" "$BASE/api/scenarios")
check "SPK-1 场景列表" "$C" "200"
[ -z "$SC" ] && SC="airport_checkin"
sleep $BGAP

# ============================================================
# 组2: 会话 + 学情(依赖 TOKEN)
# ============================================================
echo "==== 组2/3 会话+学情 ===="

# SPK-2 合法token创建会话
echo "[5/12] SPK-2 创建会话 ..."
SESS=$(curl -s -m 20 -H "$HOST_HDR" -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" -X POST -d "{\"scenario_code\":\"$SC\"}" "$BASE/api/sessions")
C=$(echo "$SESS" | python3 -c "import sys; sys.exit(0)" >/dev/null 2>&1; curl -s -m 20 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" -X POST -d "{\"scenario_code\":\"$SC\"}" "$BASE/api/sessions")
check "SPK-2 创建会话" "$C" "200"
SESS_ID=$(echo "$SESS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('session_id',''))" 2>/dev/null)
sleep $GAP

# LRN-1 学情事件上报(期望 202 Accepted)
echo "[6/12] LRN-1 学情事件上报 ..."
C=$(curl -s -m 20 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" -X POST -d '{"user_id":591904,"event_type":"study","payload":"{\"action\":\"answer\",\"correct\":true}"}' "$BASE/api/learning/event")
check "LRN-1 学情事件上报(202)" "$C" "202"
sleep $GAP

# LRN-2 复习推荐
echo "[7/12] LRN-2 复习推荐 ..."
C=$(curl -s -m 20 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" "$BASE/api/learning/review?user_id=591904")
check "LRN-2 复习推荐" "$C" "200"
sleep $GAP

# behavior top
echo "[8/12] behavior top ..."
C=$(curl -s -m 20 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" "$BASE/api/learning/behavior/top")
check "behavior top" "$C" "200"
sleep $BGAP

# ============================================================
# 组3: 推荐 + 教师 + 写作 + 情绪
# ============================================================
echo "==== 组3/3 推荐+教师+写作+情绪 ===="

# recommend
echo "[9/12] recommend ..."
C=$(curl -s -m 20 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" "$BASE/api/learning/recommend?user_id=591904")
check "recommend" "$C" "200"
sleep $GAP

# TCH-1 教师知识点统计
echo "[10/12] TCH-1 教师统计 ..."
C=$(curl -s -m 30 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" "$BASE/api/teacher/591904/kp-stats")
check "TCH-1 教师统计" "$C" "200"
sleep $GAP

# WRT-1 写作批改(AI 长尾,超时放宽到 90s)
echo "[11/12] WRT-1 写作批改 ..."
C=$(curl -s -m 90 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" -H "Content-Type: application/json" -X POST -d '{"prompt":"Write about weekend","essay":"I went to the park. It was sunny. I played with my dog."}' "$BASE/api/writing/grade")
check "WRT-1 写作批改" "$C" "200"
sleep $GAP

# EMO-1 情绪感知
echo "[12/12] EMO-1 情绪感知 ..."
C=$(curl -s -m 45 -o /dev/null -w "%{http_code}" -H "$HOST_HDR" -H "Content-Type: application/json" -X POST -d '{"text":"I am very happy today"}' "$BASE/api/emotion/sense")
check "EMO-1 情绪感知" "$C" "200"

echo
echo "================ 结果 ================"
for r in "${RESULTS[@]}"; do echo "$r"; done
echo
echo "通过 $PASS / $TOTAL,失败 $FAIL"
[ "$FAIL" = "0" ] && echo "ALL_PASS" || echo "HAS_FAIL"

# ============================================================
# 落盘报告(JSON + JUnit XML + 人类可读 txt)
# ============================================================
TS=$(date -u +"%Y%m%dT%H%M%SZ")
TXT_FILE="$REPORT_DIR/esp-e2e-host-${TS}.txt"
JUNIT_FILE="$REPORT_DIR/esp-e2e-host-${TS}.junit.xml"

# 人类可读 txt
{
  echo "ESP e2e (host ingress) 报告"
  echo "运行时间(UTC): $TS"
  echo "入口: $BASE (Host: esp.example.com)"
  echo
  for r in "${RESULTS[@]}"; do echo "$r"; done
  echo
  echo "通过 $PASS / $TOTAL,失败 $FAIL"
  [ "$FAIL" = "0" ] && echo "ALL_PASS" || echo "HAS_FAIL"
} > "$TXT_FILE"

# JUnit XML(&<> 转义)
xml_esc() { sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g'; }
{
  echo '<?xml version="1.0" encoding="UTF-8"?>'
  echo "<testsuites tests=\"$TOTAL\" failures=\"$FAIL\">"
  echo "<testsuite name=\"esp-e2e-host\" tests=\"$TOTAL\" failures=\"$FAIL\">"
  for r in "${RESULTS[@]}"; do
    name=$(echo "$r" | sed -e 's/^[✅❌] //' | xml_esc)
    if [[ "$r" == ✅* ]]; then
      echo "<testcase name=\"$name\" classname=\"host\"/>"
    else
      detail=$(echo "$r" | xml_esc)
      echo "<testcase name=\"$name\" classname=\"host\"><failure>$detail</failure></testcase>"
    fi
  done
  echo '</testsuite></testsuites>'
} > "$JUNIT_FILE"

# latest 覆盖
cp "$TXT_FILE" "$REPORT_DIR/esp-e2e-host-latest.txt"
cp "$JUNIT_FILE" "$REPORT_DIR/esp-e2e-host-latest.junit.xml"

echo
echo "[落盘报告]"
echo "  $TXT_FILE"
echo "  $JUNIT_FILE"
echo "  $REPORT_DIR/esp-e2e-host-latest.txt"
echo "  $REPORT_DIR/esp-e2e-host-latest.junit.xml"

if [ "$FAIL" = "0" ]; then exit 0; else exit 1; fi
