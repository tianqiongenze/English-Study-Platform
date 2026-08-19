#!/bin/bash
# LCO embedding 服务常驻启动脚本(launchd 调用)
# 启动 Qwen3-Embedding-4B / all-MiniLM MPS embedding 服务,监听 0.0.0.0:15730
# 供 lco-proxy L2 语义缓存调用。launchd 每 30s 健康检查,挂了自动重启。

set -euo pipefail

VENV=/Users/joelgong/lco-embedding-venv
APP=/Users/joelgong/claude-Projects/English-Study-Platform/tools/lco-proxy/embedding/app.py
LOG=/tmp/lco-embedding.log

# 若已在跑(launchd KeepAlive 会拉起,这里仅防脚本重复 exec 出多实例),
# 用 app.py 路径匹配(进程名是 Python.app/Contents/MacOS/Python 不是 python3)。
if pgrep -f "lco-proxy/embedding/app.py" >/dev/null 2>&1; then
  exit 0
fi

# 强制轻量模型 all-MiniLM-L6-v2(CPU ~0.1s/次),避免 Qwen3-Embedding-4B MPS 推理卡死(>90s)
# 导致 L2 语义缓存全超时(0 命中)。遵 memory esp-lco-embedding-vm-snowball。
# 跨语言语义弱于 Qwen3,但单节点 CPU 稳定可靠;多节点有 GPU 后可切回 qwen。
export EMBED_MODEL=minilm
exec "$VENV/bin/python3" "$APP"
