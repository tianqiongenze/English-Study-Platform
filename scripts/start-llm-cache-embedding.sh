#!/bin/bash
# LCO embedding 服务常驻启动脚本(launchd 调用)
# 启动 Qwen3-Embedding-4B / all-MiniLM MPS embedding 服务,监听 0.0.0.0:15730
# 供 llm-cache-proxy L2 语义缓存调用。launchd 每 30s 健康检查,挂了自动重启。

set -euo pipefail

VENV=/Users/joelgong/llm-cache-embedding-venv
APP=/Users/joelgong/claude-Projects/English-Study-Platform/tools/llm-cache-proxy/llm-cache-embedding/app.py
LOG=/tmp/llm-cache-embedding.log

# 若已在跑(launchd KeepAlive 会拉起,这里仅防脚本重复 exec 出多实例),
# 用 app.py 路径匹配(进程名是 Python.app/Contents/MacOS/Python 不是 python3)。
if pgrep -f "llm-cache-proxy/llm-cache-embedding/app.py" >/dev/null 2>&1; then
  exit 0
fi

# 模型由调用方显式传 model 字段决定(不在此强制 EMBED_MODEL):
#   - Claude Code LCO(lco_prompts,2560 维)→ l2.go 显式传 model=qwen
#   - 业务 LCO(esp_llm_semantic,384 维)→ conversation-go/orchestrator-rs 显式传 model=minilm
# 调用方未显式传 model 时,app.py 走 _route_model 按语言路由(中文→qwen/英文→minilm)。
# 2026-08-20 改动:此前强制 EMBED_MODEL=minilm 是为规避 Qwen3 MPS>90s 卡死(esp-lco-embedding-vm-snowball),
# 但导致 Claude Code LCO(lco_prompts 2560 维)产 384 维向量维度不匹配全 miss。
# 现调用方各自显式声明 model,服务端不再强制单一模型,两个 LCO 各取所需。
# 注意:Qwen3-4B 在单节点 VM 上 MPS 推理 8-34s/次,可能超时(l2.go L2TimeoutSec 默认 20s);
# 待 Milvus/Neo4j 下沉腾 CPU + 限流/离峰后 Qwen 才稳定可跑。超时则 L2 降级直穿(L1 精确缓存不受影响)。
#
# 2026-08-20 续:启动预加载双模型(方案 B,esp-lco-dual-model-isolation-2026-08-20)。
# 根因:此前 app.py 懒加载(用到才 load),导致 loaded:{qwen:true, minilm:false} →
#   业务侧(orchestrator-rs/conversation-go,用 minilm 384维)首次请求才加载 → 延迟/失败 → L2 降级。
# 修法:启动时 PRELOAD_MODELS=all 预加载 minilm(轻量~2s)+ qwen(重 8-34s),
#   app.py lifespan 同步加载完才对外服务(readyz 在加载中返回 503 防流量过早打到未就绪模型)。
# 代价:多占~2GB 内存;启动变慢(qwen 加载完才 ready,~10-35s)。零正确性风险。
# 可调:PRELOAD_MODELS=all(全预加载,qwen 重 ~8G 常驻)/ minilm(只预加载轻量,省内存/快启动,qwen 懒加载)/ none(纯懒加载)。
# 2026-08-20(会话23-19,切流回退后):默认 minilm。Claude Code 不走 LCO 后 qwen 常驻 ~8G 纯浪费,
#   改懒加载(首次 qwen 请求 8-34s 加载)。需重时 PRELOAD_MODELS=all 启动或 POST /unload?model=qwen 后重载。
export PRELOAD_MODELS="${PRELOAD_MODELS:-minilm}"
exec "$VENV/bin/python3" "$APP"
