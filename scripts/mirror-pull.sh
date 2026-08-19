#!/bin/bash
# Manual docker image pull via mirror GET+token (bypass daemon HEAD 403), then docker load into RD VM.
# Usage: mirror-pull.sh <repo> <tag>
set +e
REPO="$1"; TAG="$2"
MIRROR="https://docker.1ms.run"
SCOPE="repository:${REPO}:pull"
TOKEN=$(curl -s -m 30 "${MIRROR}/openapi/v1/auth/token?service=docker.1ms.run&scope=${SCOPE}" | python3 -c 'import json,sys;print(json.load(sys.stdin)["token"])')
[ -n "$TOKEN" ] || { echo "no token"; exit 1; }
echo "token OK (${#TOKEN} chars)"
AUTH="Authorization: Bearer ${TOKEN}"

# 1. fetch index/manifest-list, find target-arch digest (default arm64: rd-vm 是 Apple Silicon arm64)
# 用法: mirror-pull.sh <repo> <tag> [arch]   (arch 默认 arm64,可选 amd64)
ARCH="${3:-arm64}"
IDX=$(mktemp -d)
curl -s -m 60 -H "$AUTH" -H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json" \
  -o "$IDX/manifest" "${MIRROR}/v2/${REPO}/manifests/${TAG}"
DIGEST=$(python3 - "$IDX" "$ARCH" <<'EOF'
import json,sys
m=json.load(open(sys.argv[1]+"/manifest")); arch=sys.argv[2]
if 'manifests' in m:
    for e in m['manifests']:
        p=e.get('platform',{})
        if p.get('architecture')==arch and p.get('os')=='linux':
            print(e['digest']); break
else:
    print('SELF')
EOF
)
if [ "$DIGEST" = "SELF" ] || [ -z "$DIGEST" ]; then
  echo "single manifest ($ARCH assumed or single-arch)"; DIGEST=""
  cp "$IDX/manifest" "$IDX/image-manifest"
else
  echo "$ARCH digest: $DIGEST"
  curl -s -m 60 -H "$AUTH" -H "Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json" \
    -o "$IDX/image-manifest" "${MIRROR}/v2/${REPO}/manifests/${DIGEST}"
fi

# 2. config + layers
CONFIG=$(python3 -c 'import json;print(json.load(open("'$IDX'/image-manifest"))["config"]["digest"])')
curl -s -m 60 -H "$AUTH" -o "$IDX/config.json" "${MIRROR}/v2/${REPO}/blobs/${CONFIG}"
LAYERS_FILE="$IDX/layers.txt"
python3 -c 'import json;[print(l["digest"]) for l in json.load(open("'$IDX'/image-manifest"))["layers"]]' > "$LAYERS_FILE"
NL=$(wc -l < "$LAYERS_FILE" | tr -d ' ')
echo "layers: $NL"

mkdir -p "$IDX/layer-tars"
i=0
while IFS= read -r L; do
  i=$((i+1)); echo "layer $i/$NL $L"
  curl -sL -m 600 -H "$AUTH" -o "$IDX/layer-tars/$i.tar" "${MIRROR}/v2/${REPO}/blobs/${L}"
  ls -la "$IDX/layer-tars/$i.tar" | awk '{print "  size:", $5}'
done < "$LAYERS_FILE"

# 3. assemble docker-archive (legacy manifest.json format)
CID=$(echo "$CONFIG" | cut -d: -f2).json
cp "$IDX/config.json" "$IDX/$CID"
python3 - "$IDX" "$CID" "$NL" <<'EOF'
import json,sys
d,cid,n=sys.argv[1],sys.argv[2],int(sys.argv[3])
m=[{"Config":cid,"RepoTags":["REPO_TAG"],"Layers":[f"layer-tars/{i}.tar" for i in range(1,n+1)]}]
json.dump(m,open(d+"/manifest.json","w"))
EOF
sed -i '' "s|REPO_TAG|${REPO##*/}:${TAG}-manual|" "$IDX/manifest.json" 2>/dev/null || true
echo "assembled at $IDX"

# 4. tar + docker load into RD VM via rdctl shell
echo "packing + loading into RD VM..."
tar -C "$IDX" -cf - "$CID" manifest.json layer-tars | rdctl shell docker load
echo "LOAD DONE"
