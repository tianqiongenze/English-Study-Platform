#!/usr/bin/env python3
"""gen_bench_tokens.py — 压测前刷新 JWT(坑㉟d 教训:token 过期整轮基准 100% 401)。
签发 20 个 learner(10000100..10000119)+1 个 teacher(10000101),RS256,
用 secret esp-rsa-jwt-sign 的私钥签名(与现有服务验签公钥同源),merge-patch 刷新 configmap esp-bench-tokens。
用法: python3 scripts/gen_bench_tokens.py [exp_hours=24]
"""
import base64, json, subprocess, sys, time
import jwt

NS = "esp-prod"


def get_priv_pem():
    out = subprocess.check_output([
        "kubectl", "-n", NS, "get", "secret", "esp-rsa-jwt-sign",
        "-o", "jsonpath={.data.JWT_RSA_PRIVATE_KEY_PEM}",
    ]).decode().strip()
    return base64.b64decode(out)


def main():
    exp_h = int(sys.argv[1]) if len(sys.argv) > 1 else 24
    key = get_priv_pem()
    now = int(time.time())
    expiry = now + exp_h * 3600
    toks = [jwt.encode({"sub": str(10000100 + i), "role": "learner",
                        "iat": now, "exp": expiry}, key, algorithm="RS256")
            for i in range(20)]
    teacher = jwt.encode({"sub": "10000101", "role": "teacher",
                          "iat": now, "exp": expiry}, key, algorithm="RS256")
    data = {"data": {"LEARNER_TOKENS": ",".join(toks), "TEACHER_TOKEN": teacher}}
    subprocess.check_call(
        ["kubectl", "-n", NS, "patch", "configmap", "esp-bench-tokens",
         "--type", "merge", "-p", json.dumps(data)],
        stdout=subprocess.DEVNULL)
    print(f"ok: 20 learner(10000100..10000119)+1 teacher, iat={now} exp={expiry}(+{exp_h}h)")


if __name__ == "__main__":
    main()
