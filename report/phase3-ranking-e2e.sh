#!/usr/bin/env bash
# Phase3 排行榜 E2E(会话23-9 #197)。前置:cluster 可用 + bff port-forward 18090 + /tmp/rk-tokens.txt(3 用户令牌)。
# 目标:content 学习事件→learning_events(CRDB)→poll→ZSET→rankings 出数;region/类别维度;gap;opt-out。
# 用法:BASH_ENV=  bash report/phase3-ranking-e2e.sh
set -uo pipefail
BASE=http://localhost:18090

tok() { grep "^$1 " /tmp/rk-tokens.txt | awk '{print $3}'; }
uid() { grep "^$1 " /tmp/rk-tokens.txt | awk '{print $2}'; }
T1=$(tok rk-u1); T2=$(tok rk-u2); T3=$(tok rk-u3)
U1=$(uid rk-u1); U2=$(uid rk-u2); U3=$(uid rk-u3)
A1="Authorization: Bearer $T1"; A2="Authorization: Bearer $T2"; A3="Authorization: Bearer $T3"

echo "== [1] 浏览内容库(BFF) =="
COURSE_ID=$(curl -s -H "$A1" "$BASE/api/content/courses?status=published" | python3 -c 'import sys,json;print(json.load(sys.stdin)[0]["id"])')
LESSON_ID=$(curl -s -H "$A1" "$BASE/api/content/courses/$COURSE_ID/lessons" | python3 -c 'import sys,json;print(json.load(sys.stdin)[0]["id"])')
curl -s -H "$A1" "$BASE/api/content/lessons/$LESSON_ID/items" | python3 -c 'import sys,json;L=json.load(sys.stdin);print("items:",len(L));[print(" ",i["id"],i["item_type"]) for i in L[:5]]'
# 选一个 vocab 题(自动判分 correct)做事件注入
ITEM_V=$(curl -s -H "$A1" "$BASE/api/content/lessons/$LESSON_ID/items" | python3 -c 'import sys,json;L=json.load(sys.stdin);print(next((i["id"] for i in L if i["item_type"]=="vocab"), L[0]["id"]))')
echo "item=$ITEM_V"

echo "== [2] submitAnswer × 6 (rk-u1) / × 3 (rk-u2) / × 1 (rk-u3) =="
for i in 1 2 3 4 5 6; do curl -s -H "$A1" -H 'Content-Type: application/json' -X POST "$BASE/api/content/items/$ITEM_V/answer" -d '{"answers":{},"duration_ms":800}' | head -c 60; echo; done
for i in 1 2 3; do curl -s -H "$A2" -H 'Content-Type: application/json' -X POST "$BASE/api/content/items/$ITEM_V/answer" -d '{"answers":{},"duration_ms":800}' | head -c 60; echo; done
curl -s -H "$A3" -H 'Content-Type: application/json' -X POST "$BASE/api/content/items/$ITEM_V/answer" -d '{"answers":{},"duration_ms":800}' | head -c 60; echo
echo "wait 6s for outbox relay + feed poll..."; sleep 6

echo "== [3] rankings: global weekly xp =="
curl -s -H "$A1" "$BASE/api/gamification/rankings?type=xp&scope=global&scope_id=all&period=weekly&limit=10"
echo
echo "== [4] rankingGap(rk-u1) =="
curl -s -H "$A1" "$BASE/api/gamification/rankings/gap?type=xp&scope=global&scope_id=all&period=weekly"; echo
echo "== [5] region scope: CN/Guangdong(rk-u1+rk-u2 都应出现) =="
curl -s -H "$A1" "$BASE/api/gamification/rankings?type=xp&scope=region&scope_id=CN/Guangdong&period=weekly&limit=10"; echo
echo "== [5b] region scope: CN/Beijing(仅 rk-u3) =="
curl -s -H "$A1" "$BASE/api/gamification/rankings?type=xp&scope=region&scope_id=CN/Beijing&period=weekly&limit=10"; echo
echo "== [6] category scope: vocab =="
curl -s -H "$A1" "$BASE/api/gamification/rankings?type=xp&scope=category&scope_id=vocab&period=weekly&limit=10"; echo
echo "== [7] regions 清单 =="
curl -s -H "$A1" "$BASE/api/gamification/rankings/regions"; echo
echo "== [8] opt-out rk-u1 → global 榜应消失 =="
curl -s -H "$A1" -H 'Content-Type: application/json' -X POST "$BASE/api/gamification/users/optout" -d '{"opt_out":true}'; echo
curl -s -H "$A2" "$BASE/api/gamification/rankings?type=xp&scope=global&scope_id=all&period=weekly&limit=10"; echo
echo "== [9] 恢复 rk-u1 =="
curl -s -H "$A1" -H 'Content-Type: application/json' -X POST "$BASE/api/gamification/users/optout" -d '{"opt_out":false}'; echo
echo "== E2E DONE =="