#!/usr/bin/env python3
"""
gateway-go 1.11.0 CRDB 归档路径验证(坑㉚ 重建后 End() 写 STRING id)
====================================================================
流程:POST /api/sessions 建会话 → WS 连 /ws/speech/<id>?token= → 发 {"type":"end"}
      → gateway-go End() 归档到 CRDB esp_speaking.sessions(STRING id)
      → 查 CRDB 确认 sess_xxx 行落库

依赖:
- env JWT_PRIV_PEM(esp-rsa-jwt-sign 私钥)或由脚本内部 kubectl 取
- env GATEWAY_URL(默认 http://gateway-go:80)
- env TEST_USER_ID(默认 999001)
- env SCENARIO_CODE(默认取 /api/scenarios 第一个 code)

用法(集群内 pod):
  kubectl -n esp-prod run gw-verify-$$ --rm -i --image=python:3.12-slim \
    --env JWT_PRIV_PEM="$(kubectl -n esp-prod get secret esp-rsa-jwt-sign -o jsonpath={.data.JWT_RSA_PRIVATE_KEY_PEM} | base64 -d)" \
    --command -- bash -lc 'pip install ... ; python3 -c "..."'
"""
import os, sys, time, json, base64, urllib.request
from datetime import datetime, timedelta, timezone

import jwt as pyjwt  # PyJWT(pip install PyJWT)
import websocket     # websocket-client(pip install websocket-client)
import psycopg2

GATEWAY = os.environ.get("GATEWAY_URL", "http://gateway-go:80")
WS_GATEWAY = os.environ.get("WS_GATEWAY_URL", "ws://gateway-go:80")
USER_ID = os.environ.get("TEST_USER_ID", "999001")
PRIV_PEM = os.environ.get("JWT_PRIV_PEM", "")


def get_priv_pem():
    if PRIV_PEM:
        return PRIV_PEM
    import subprocess
    out = subprocess.run(
        ["kubectl", "-n", "esp-prod", "get", "secret", "esp-rsa-jwt-sign",
         "-o", "jsonpath={.data.JWT_RSA_PRIVATE_KEY_PEM}"],
        capture_output=True, text=True, check=True)
    return base64.b64decode(out.stdout.strip()).decode()


def mint_token(user_id):
    now = datetime.now(timezone.utc)
    claims = {
        "sub": str(user_id),
        "iat": now,
        "nbf": now - timedelta(seconds=30),
        "exp": now + timedelta(hours=1),
    }
    return pyjwt.encode(claims, get_priv_pem(), algorithm="RS256")


def http_json(url, method="GET", body=None, token=None):
    req = urllib.request.Request(url, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    if body is not None:
        req.data = json.dumps(body).encode()
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read())


def main():
    tok = mint_token(USER_ID)
    print("token minted for user=%s (len=%d)" % (USER_ID, len(tok)))

    # 1) scenarios
    scs = http_json(GATEWAY + "/api/scenarios", token=tok)
    codes = [s["code"] for s in scs] if isinstance(scs, list) else []
    print("scenarios:", codes[:5], "... total=%d" % len(codes))
    if not codes:
        print("FAIL: no scenarios")
        sys.exit(1)
    code = os.environ.get("SCENARIO_CODE") or codes[0]

    # 2) create session
    resp = http_json(GATEWAY + "/api/sessions", method="POST",
                     body={"scenario_code": code}, token=tok)
    sid = resp.get("session_id")
    print("created session:", sid)
    if not sid:
        print("FAIL: no session_id in response", resp)
        sys.exit(1)

    # 3) WS connect + send end frame
    ws_url = "%s/ws/speech/%s?token=%s" % (WS_GATEWAY, sid, tok)
    ws = websocket.create_connection(ws_url, timeout=15)
    ws.send(json.dumps({"type": "end"}))
    reply = json.loads(ws.recv())
    print("ws end reply:", reply)
    ws.close()
    time.sleep(2)  # 等 End 异步归档落库

    # 4) CRDB check
    c = psycopg2.connect(host="crdb-public", port=26257, user="root",
                         dbname="esp_speaking", sslmode="disable", connect_timeout=15)
    cur = c.cursor()
    cur.execute("SELECT id, user_id, scenario_id, status, started_at, ended_at FROM sessions WHERE id=%s", (sid,))
    row = cur.fetchone()
    if row:
        print("ARCHIVE_OK: session archived:", row)
        print("  id_type STRING? -> %r" % (row[0],))
    else:
        print("ARCHIVE_MISSING: row not found in CRDB sessions")
        cur.execute("SELECT count(*) FROM sessions")
        print("  sessions count:", cur.fetchone()[0])
        sys.exit(1)

    # 5) ListHistory 读路径(经 idx_user 查 CRDB sessions)
    try:
        hist = http_json(GATEWAY + "/api/sessions/history?limit=5", token=tok)
        print("HISTORY_OK: user=%s sessions=%d" % (USER_ID, len(hist) if isinstance(hist, list) else -1))
        if isinstance(hist, list):
            for h in hist[:3]:
                print("   ", h)
    except Exception as e:
        print("HISTORY_FAIL:", e)
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
