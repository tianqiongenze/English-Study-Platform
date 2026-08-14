#!/usr/bin/env python3
"""
Manual docker image pull via mirror GET+token (bypass daemon HEAD 403),
then docker load into RD VM via rdctl shell.

Usage: python3 scripts/mirror_pull.py <repo> <tag> [--load]
  e.g. python3 scripts/mirror_pull.py cockroachdb/cockroach v24.3.11 --load
"""
import json, sys, os, time, subprocess, tempfile, tarfile, urllib.request, urllib.error

REPO = sys.argv[1]
TAG = sys.argv[2]
DO_LOAD = "--load" in sys.argv
ARCH = "amd64"
for a in sys.argv[3:]:
    if a.startswith("arch="):
        ARCH = a.split("=", 1)[1]
MIRROR = "https://docker.1ms.run"

def req(url, token=None, headers=None, timeout=900):
    """Fetch via curl (handles CloudFront signed-URL redirects + drops auth header correctly)."""
    import shlex
    h = headers.copy() if headers else {}
    cmd = ["curl", "-sL", "-m", str(timeout), "-o", "-"]
    if token:
        cmd += ["-H", f"Authorization: Bearer {token}"]
    for k, v in h.items():
        cmd += ["-H", f"{k}: {v}"]
    cmd.append(url)
    p = subprocess.run(cmd, capture_output=True, timeout=timeout+30)
    if p.returncode != 0:
        raise RuntimeError(f"curl rc={p.returncode} stderr={p.stderr.decode(errors='replace')[:200]}")
    return p.stdout

# 1. token
scope = f"repository:{REPO}:pull"
tok_raw = req(f"{MIRROR}/openapi/v1/auth/token?service=docker.1ms.run&scope={scope}", timeout=30)
TOKEN = json.loads(tok_raw)["token"]
print(f"token OK ({len(TOKEN)} chars)", flush=True)

# 2. manifest-list -> amd64
accept_idx = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"
ml = json.loads(req(f"{MIRROR}/v2/{REPO}/manifests/{TAG}", TOKEN, {"Accept": accept_idx}, timeout=60))
digest = None
if "manifests" in ml:
    for e in ml["manifests"]:
        p = e.get("platform", {})
        if p.get("architecture") == ARCH and p.get("os") == "linux":
            digest = e["digest"]; break
    if not digest:
        # fallback first
        digest = ml["manifests"][0]["digest"]
    print(f"{ARCH} digest: {digest}", flush=True)
    accept_m = "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"
    m = json.loads(req(f"{MIRROR}/v2/{REPO}/manifests/{digest}", TOKEN, {"Accept": accept_m}, timeout=60))
else:
    print("single manifest", flush=True)
    m = ml

config_digest = m["config"]["digest"]
layers = [l["digest"] for l in m["layers"]]
print(f"config: {config_digest}", flush=True)
print(f"layers: {len(layers)}", flush=True)

OUT = tempfile.mkdtemp(prefix="crdbimg_")
print(f"workdir: {OUT}", flush=True)

# 3. config
cfg_bytes = req(f"{MIRROR}/v2/{REPO}/blobs/{config_digest}", TOKEN, timeout=120)
cid_name = config_digest.split(":")[1] + ".json"
with open(os.path.join(OUT, cid_name), "wb") as f:
    f.write(cfg_bytes)

# 4. layers (follow redirects manually, retry)
layer_dir = os.path.join(OUT, "layer-tars")
os.makedirs(layer_dir)
import urllib.request as ur
for i, L in enumerate(layers, 1):
    dest = os.path.join(layer_dir, f"{i}.tar")
    url = f"{MIRROR}/v2/{REPO}/blobs/{L}"
    ok = False
    for attempt in range(4):
        try:
            data = req(url, TOKEN, timeout=900)
            if len(data) > 0:
                with open(dest, "wb") as f:
                    f.write(data)
                print(f"layer {i}/{len(layers)} {len(data)} bytes", flush=True)
                ok = True; break
        except Exception as e:
            print(f"layer {i} attempt {attempt+1} err: {e}", flush=True)
            time.sleep(2)
    if not ok:
        print(f"layer {i} FAILED to download", flush=True)
        sys.exit(2)

# 5. manifest.json (docker-archive legacy)
img_tag = f"{REPO.split('/')[-1]}:{TAG}-manual"
manifest = [{
    "Config": cid_name,
    "RepoTags": [img_tag],
    "Layers": [f"layer-tars/{i}.tar" for i in range(1, len(layers)+1)]
}]
with open(os.path.join(OUT, "manifest.json"), "w") as f:
    json.dump(manifest, f)
print(f"assembled docker-archive, tag={img_tag}", flush=True)

if not DO_LOAD:
    print("dry-run: skip load. workdir:", OUT, flush=True)
    sys.exit(0)

# 6. tar to stdout | rdctl shell docker load
print("loading into RD VM (this may take a while)...", flush=True)
tar_cmd = ["tar", "-C", OUT, "-cf", "-", cid_name, "manifest.json", "layer-tars"]
load_cmd = ["rdctl", "shell", "docker", "load"]
p1 = subprocess.Popen(tar_cmd, stdout=subprocess.PIPE)
p2 = subprocess.Popen(load_cmd, stdin=p1.stdout, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
p1.stdout.close()
out, _ = p2.communicate()
print(out.decode(errors="replace"), flush=True)
print(f"load done RC={p2.returncode}", flush=True)
sys.exit(p2.returncode)
