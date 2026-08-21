#!/usr/bin/env bash
# publish-sentinel-rules.sh — 推 ESP Java 服务 Sentinel 规则到宿主 Nacos(RWT 铁律:限流规则配而未接根治)
#
# 现状(2026-08-21 scout3 查证):
#   - 仓库 deploy/nacos/sentinel/ 有 bff-java flow+degrade 规则 JSON
#   - Nacos 里规则丢失(宿主Nacos下沉重建/迁移致空)→ bff/learning/teacher 启动全 404
#     (SentinelRuleBootstrap fetch status=404 = 配而未接,违反 RWT)
#   - dataId 格式: <svc>-flow-rules / <svc>-degrade-rules, group=SENTINEL_GROUP, namespace=public
#
# 推送方式:Nacos OpenAPI POST /nacos/v1/cs/configs(body form)
#   宿主Nacos已下沉Mac(launchd常驻,host-middleware-startup.md:153),localhost:8848可达
#
# 范围(诚实,不臆造):
#   - bff-java: flow(3条,resource=api-default/api-learning-event/api-writing-grade,经RateLimitFilter明确映射) + degrade(2条)
#   - learning-java / teacher-java: degrade(SCA WebMvc拦截器 resource名=Spring BEST_MATCHING_PATTERN 完整模板如/api/learning/review,
#     ASV核实a11b5692:contextPath默认空→resource=纯pattern;grade=0 RT慢调用熔断对齐bff degrade格式,
#     count=慢响应阈值ms+slowRatioThreshold+timeWindow;配LLM慢调用接口lesson-plan/auto-grade count=30s因LLM推理慢)
#     flow规则暂不推(无RateLimitFilter自定义映射,SCA默认WebMvc已对每个URL过FlowSlot,如需限流另配)
#
# 用法: bash scripts/publish-sentinel-rules.sh
set -euo pipefail

NACOS="${NACOS_URL:-http://localhost:8848}"
SENTINEL_DIR="$(cd "$(dirname "$0")/.." && pwd)/deploy/nacos/sentinel"
GROUP="SENTINEL_GROUP"
NAMESPACE="public"   # tenant=public(Nacos OpenAPI tenant 空=public)

publish() {
  local data_id="$1" file="$2"
  if [ ! -f "$file" ]; then echo "SKIP $data_id (file not found: $file)"; return 0; fi
  local content
  content=$(cat "$file")
  # Nacos OpenAPI: POST form dataId/group/tenant/content/type
  local rc http_code
  http_code=$(curl -s -o /tmp/nacos-pub-resp.txt -w "%{http_code}" -X POST \
    "${NACOS}/nacos/v1/cs/configs" \
    --data-urlencode "dataId=${data_id}" \
    --data-urlencode "group=${GROUP}" \
    --data-urlencode "tenant=${NAMESPACE}" \
    --data-urlencode "type=json" \
    --data-urlencode "content=${content}" 2>/dev/null || echo "000")
  rc=$?
  local resp
  resp=$(cat /tmp/nacos-pub-resp.txt 2>/dev/null | head -c 200)
  if [ "$http_code" = "200" ] && echo "$resp" | grep -qi "true"; then
    echo "OK   $data_id (HTTP $http_code, len=$(echo -n "$content" | wc -c))"
  else
    echo "FAIL $data_id (HTTP $http_code, resp=$resp, rc=$rc)"
  fi
}

echo "=== 推送 Sentinel 规则到 Nacos ($NACOS) group=$GROUP ns=$NAMESPACE ==="
echo "--- bff-java (flow + degrade, 有 RateLimitFilter resource 映射) ---"
publish "bff-java-flow-rules"    "${SENTINEL_DIR}/bff-java-flow-rules.json"
publish "bff-java-degrade-rules" "${SENTINEL_DIR}/bff-java-degrade-rules.json"

echo "--- learning-java (degrade, SCA WebMvc pattern resource 名,RT慢调用熔断) ---"
publish "learning-java-degrade-rules" "${SENTINEL_DIR}/learning-java-degrade-rules.json"

echo "--- teacher-java (degrade, SCA WebMvc pattern resource 名,LLM慢调用熔断) ---"
publish "teacher-java-degrade-rules" "${SENTINEL_DIR}/teacher-java-degrade-rules.json"

echo ""
echo "=== 验证(GET config 确认入库) ==="
for did in bff-java-flow-rules bff-java-degrade-rules learning-java-degrade-rules teacher-java-degrade-rules; do
  val=$(curl -s "${NACOS}/nacos/v1/cs/configs?dataId=${did}&group=${GROUP}&tenant=${NAMESPACE}" 2>/dev/null | head -c 80)
  if [ -n "$val" ] && [ "$val" != "null" ]; then
    echo "VERIFY $did => $(echo "$val" | head -c 60)..."
  else
    echo "VERIFY $did => EMPTY (推失败或Nacos不可达)"
  fi
done
echo "=== done ==="
