#!/usr/bin/env bash
# gen-rsa-keypair.sh — 生成 ESP JWT RS256 密钥对(PKCS8 私钥 + PKCS8/X.509 公钥 PEM)
# 用法: ./scripts/gen-rsa-keypair.sh [输出目录]
# 默认输出到 services/bff-java/src/test/resources/rsa/
set -euo pipefail

OUT_DIR="${1:-$(dirname "$0")/../services/bff-java/src/test/resources/rsa}"
mkdir -p "$OUT_DIR"

PRIV="$OUT_DIR/private.pem"
PUB="$OUT_DIR/public.pem"

# 2048-bit RSA;生产可用 4096,但 JWT 签发频繁 2048 性能更优
openssl genpkey -algorithm RSA -out "$PRIV" -pkeyopt rsa_keygen_bits:2048
openssl rsa -in "$PRIV" -pubout -out "$PUB"

echo "=== Private key (PKCS8) ==="
echo "$PRIV"
echo "=== Public key (X.509 SPKI) ==="
echo "$PUB"
echo ""
echo "=== K8s Secret 灌密钥说明(不自动执行) ==="
echo "kubectl -n esp-prod create secret generic esp-rsa-jwt \\"
echo "  --from-literal=JWT_RSA_PRIVATE_KEY_PEM=\"\$(cat $PRIV)\" \\"
echo "  --from-literal=JWT_RSA_PUBLIC_KEY_PEM=\"\$(cat $PUB)\" \\"
echo "  --dry-run=client -o yaml | kubectl apply -f -"
echo ""
echo "然后修改 bff-java Deployment:"
echo "  envFrom: [{secretRef: {name: esp-rsa-jwt}}]"
echo "修改 learning-java / gateway-go Deployment 同理(只需 JWT_RSA_PUBLIC_KEY_PEM)。"
