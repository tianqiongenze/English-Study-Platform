# llm-cache-embedding —— LLM cache 代理 L2 语义缓存的 embedding 服务(宿主 python venv)
#
# 设计:
# - 多模型支持 + 语言自动路由:
#   * 中文/多语言 prompt → Qwen/Qwen3-Embedding-4B(2560 维,中文语义强)
#   * 英文 prompt → sentence-transformers/all-MiniLM-L6-v2(384 维,轻量)
#   * env 可强制指定单模型(EMBED_MODEL)跳过路由
# - 监听 0.0.0.0:15730(允许 lima VM 反向访问,集群内 ESP 服务经 host.lima.internal:15730 可达)
# - MPS 加速(若可用,自动检测)
# - POST /embed {"text":"...","model":"auto|qwen|minilm"} → {"vector":[...],"model":"...","dim":...,"device":...}
import os
import json
import logging
import time
from contextlib import asynccontextmanager
from fastapi import FastAPI
from fastapi.responses import Response
from pydantic import BaseModel
import uvicorn

logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(message)s')
log = logging.getLogger("llm-cache-embedding")

LISTEN_HOST = os.getenv("LISTEN_HOST", "0.0.0.0")
LISTEN_PORT = int(os.getenv("LISTEN_PORT", "15730"))

# 启动预加载开关:PRELOAD_MODELS=minilm 只预加载 minilm(省内存/快启动);
# PRELOAD_MODELS=all 预加载全部(qwen 重 ~8G 常驻,慢启动 8-34s);PRELOAD_MODELS=none 纯懒加载。
# 2026-08-20(会话23-19,切流回退后优化):默认 minilm 而非 all。
#   根因:Claude Code 切流回退后不走 LCO,llm_cache_prompts(qwen 2560维)仅手动 probe 用,
#   qwen 常驻 ~8G(RSS 8.4G+swap 8.5G)纯浪费。minilm(384维,~100MB)服务 ESP 业务侧 orchestrator 足够。
#   qwen 改懒加载:首次 qwen 请求才加载(8-34s),后续常驻至 /unload 或进程退出。
PRELOAD = os.getenv("PRELOAD_MODELS", "minilm").strip().lower()

# 模型定义:key=短名, value=(HF model id, 本地目录, 语言)
MODELS = {
    "qwen": {
        "hf": "Qwen/Qwen3-Embedding-4B",
        "local": os.path.expanduser("~/lco-models/qwen3-embedding-4b"),
        "lang": "zh",
    },
    "minilm": {
        "hf": "sentence-transformers/all-MiniLM-L6-v2",
        "local": os.path.expanduser("~/lco-models/all-minilm-l6-v2"),
        "lang": "en",
    },
}
# 强制单模型(可选):EMBED_MODEL=qwen|minilm;为空则按语言自动路由
FORCE_MODEL = os.getenv("EMBED_MODEL", "").strip()

_models = {}   # 短名 -> SentenceTransformer
_dims = {}     # 短名 -> dim
_device = "cpu"
# 预加载状态:loading/ready/failed,供 healthz 反映(探针在 loading 时判 not ready)
_preload_status = {"qwen": "pending", "minilm": "pending"}

# Prometheus 指标
_metrics = {
    "embed_requests_total": 0,
    "embed_errors_total": 0,
    "embed_seconds_total": 0.0,
}


def _load_model(short_name: str):
    """惰性加载指定模型 + 自动选 device(跨平台:CUDA > MPS > CPU)。

    启动预加载(lifespan)也会调本函数;若模型已在加载中,直接等已有结果(防并发重复加载)。
    2026-08-20 会话23-20:device 选择兼容全平台——
      生产 Linux NVIDIA GPU:cuda(torch.cuda.is_available)
      开发 Mac M1+:mps(torch.backends.mps.is_available)
      无 GPU / CPU 推理:cpu
    Python torch 是唯一原生支持 CPU/CUDA/MPS 全三平台的 ML 框架
    (Go/Rust 用 ONNX 在 MPS/模型兼容性上反而更弱,故保持 Python)。
    """
    global _device
    if short_name in _models:
        return _models[short_name]
    from sentence_transformers import SentenceTransformer
    import torch

    # device 优先级:cuda(生产 GPU) > mps(Mac 开发) > cpu(兜底)
    if torch.cuda.is_available():
        _device = "cuda"
    elif torch.backends.mps.is_available():
        _device = "mps"
    else:
        _device = "cpu"

    spec = MODELS[short_name]
    # 优先本地目录(已下载),否则走 HF(镜像)
    model_path = spec["local"] if os.path.isdir(spec["local"]) else spec["hf"]
    log.info("加载模型 %s (device=%s, path=%s)", short_name, _device, model_path)
    model = SentenceTransformer(model_path, device=_device)
    _models[short_name] = model
    _dims[short_name] = model.get_sentence_embedding_dimension()
    log.info("模型 %s 加载完成: dim=%d", short_name, _dims[short_name])
    return model


def _unload_model(short_name: str):
    """卸载指定模型释放内存(2026-08-20 会话23-19,qwen 懒加载配套)。

    SentenceTransformer 无显式 close,靠 del + GC 释放 MPS 张量。
    卸载后 _preload_status 回 pending(下次请求触发懒加载)。
    用途:qwen 懒加载后长时间空闲,手动 /unload?qwen 或定时清理省 ~8G 常驻。
    """
    if short_name not in _models:
        return False
    import gc
    model = _models.pop(short_name)
    # SentenceTransformer 持有 torch.Tensor(MPS),del 触发 MPS 缓存释放
    try:
        del model
    except Exception:
        pass
    gc.collect()
    # 尝试清空 MPS cache(若 torch 已 import)
    try:
        import torch
        if hasattr(torch, "mps") and hasattr(torch.mps, "empty_cache"):
            torch.mps.empty_cache()
    except Exception:
        pass
    _preload_status[short_name] = "pending"
    _dims.pop(short_name, None)
    log.info("模型 %s 已卸载(释放内存,_preload_status→pending)", short_name)
    return True


@asynccontextmanager
async def lifespan(app: FastAPI):
    """启动时预加载模型,避免业务侧首次请求才加载的延迟/失败。

    顺序:minilm 先(轻量 ~2s,业务侧急需)→ qwen 后(重 8-34s,Claude Code 侧用)。
    预加载失败不阻断启动(降级为懒加载,首次请求重试)。
    """
    targets = []
    if PRELOAD in ("all", "both"):
        targets = ["minilm", "qwen"]   # 轻的先
    elif PRELOAD == "minilm":
        targets = ["minilm"]
    # none / 其他 → 不预加载,纯懒加载

    for name in targets:
        _preload_status[name] = "loading"
        try:
            _load_model(name)
            _preload_status[name] = "ready"
            log.info("预加载 %s ready", name)
        except Exception as e:
            _preload_status[name] = "failed"
            log.error("预加载 %s 失败(降级懒加载): %s", name, e)

    # gRPC server 启动(2026-08-20 会话23-20 Phase 5.2,与 FastAPI 并行)
    # 监听 15731,llm-cache-proxy 经 LCO_EMBED_PROTOCOL=grpc 切换。
    # GRPC_ENABLED=0 可禁用(默认开)。失败不阻断(降级纯 HTTP)。
    grpc_server = None
    if os.getenv("GRPC_ENABLED", "1") != "0":
        try:
            from grpc_server import serve_grpc
            grpc_port = int(os.getenv("GRPC_PORT", "15731"))
            grpc_server = serve_grpc(grpc_port)
            log.info("gRPC server 启动完成(端口 %d)", grpc_port)
        except Exception as e:
            log.error("gRPC server 启动失败(降级纯 HTTP): %s", e)

    yield   # 服务运行期
    # 关闭 gRPC server
    if grpc_server:
        try:
            grpc_server.stop(0)
            log.info("gRPC server 已停止")
        except Exception:
            pass
    # 关闭时清理(SentenceTransformer 无显式 close,靠 GC)
    _models.clear()

app = FastAPI(title="LCO Embedding Service", lifespan=lifespan)


def _detect_lang(text: str) -> str:
    """粗判语言:含中文字符 → zh,否则 en。"""
    for ch in text:
        if '一' <= ch <= '鿿':
            return "zh"
    return "en"


def _route_model(text: str) -> str:
    """选模型:强制模型优先,否则按语言路由。"""
    if FORCE_MODEL:
        return FORCE_MODEL
    lang = _detect_lang(text)
    return "qwen" if lang == "zh" else "minilm"


class EmbedRequest(BaseModel):
    text: str
    model: str = "auto"  # auto|qwen|minilm


class EmbedBatchRequest(BaseModel):
    """批处理 embed:多个 text 一次请求,省往返 + 利用模型批处理并行。

    单条失败不阻断整批:返回对应位置 None + error 字段,调用方按需降级。
    """
    texts: list[str]
    model: str = "auto"


class EmbedResponse(BaseModel):
    vector: list
    model: str
    dim: int
    device: str


class EmbedBatchItem(BaseModel):
    """批处理单项结果。失败时 vector=None + error 填原因。"""
    vector: list | None = None
    model: str | None = None
    dim: int | None = None
    error: str | None = None


@app.get("/healthz")
def healthz():
    loaded = {k: k in _models for k in MODELS}
    return {
        "status": "ok",
        "loaded": loaded,
        "preload_status": dict(_preload_status),
        "device": _device,
        "force_model": FORCE_MODEL or "auto",
    }


@app.get("/readyz")
def readyz():
    """就绪探针:预加载目标全部 ready 才 200,否则 503。

    PRELOAD=all 时要求 qwen+minilm 都 ready;qwen 加载慢(8-34s),启动期 readyz 返回 503
    防止流量过早打到未就绪的模型。预加载完成或降级为懒加载后转 200。
    """
    targets = []
    if PRELOAD in ("all", "both"):
        targets = ["minilm", "qwen"]
    elif PRELOAD == "minilm":
        targets = ["minilm"]
    # none → 永远 ready(纯懒加载)

    not_ready = [t for t in targets if _preload_status.get(t) not in ("ready", "failed")]
    # failed 也算 ready(降级懒加载,首次请求会重试加载)
    if not_ready:
        return Response(
            content=json.dumps({"ready": False, "pending": not_ready}),
            status_code=503,
            media_type="application/json",
        )
    return {"ready": True, "loaded": {k: k in _models for k in MODELS}}


@app.post("/unload")
def unload_model(model: str = ""):
    """卸载指定模型释放内存(2026-08-20 会话23-19)。

    用途:qwen 懒加载后长时间空闲,手动卸载省 ~8G 常驻。
    用法:POST /unload?model=qwen。卸载后下次 qwen 请求触发懒加载(8-34s)。
    不卸载 minilm(ESP 业务侧 orchestrator 常用)。
    """
    model = model.strip().lower()
    if model not in MODELS:
        return Response(
            content=json.dumps({"error": f"未知模型 {model},可选 {list(MODELS.keys())}"}),
            status_code=400, media_type="application/json",
        )
    unloaded = _unload_model(model)
    return {"unloaded": unloaded, "model": model, "loaded": {k: k in _models for k in MODELS}}


@app.get("/metrics")
def metrics():
    m = [
        "# HELP lco_embed_requests_total Total embedding requests.",
        "# TYPE lco_embed_requests_total counter",
        f"lco_embed_requests_total {_metrics['embed_requests_total']}",
        "# HELP lco_embed_errors_total Total embedding errors.",
        "# TYPE lco_embed_errors_total counter",
        f"lco_embed_errors_total {_metrics['embed_errors_total']}",
        "# HELP lco_embed_seconds_total Total embedding time in seconds.",
        "# TYPE lco_embed_seconds_total counter",
        f"lco_embed_seconds_total {_metrics['embed_seconds_total']:.4f}",
    ]
    return Response(content="\n".join(m) + "\n", media_type="text/plain")


@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest):
    _metrics["embed_requests_total"] += 1
    t0 = time.time()
    try:
        short = req.model if req.model in MODELS else _route_model(req.text)
        model = _load_model(short)
        vec = model.encode(req.text, normalize_embeddings=True)
    except Exception as e:
        _metrics["embed_errors_total"] += 1
        log.error("embed 失败: %s", e)
        raise
    _metrics["embed_seconds_total"] += time.time() - t0
    return EmbedResponse(
        vector=vec.tolist(),
        model=short,
        dim=_dims[short],
        device=_device,
    )


@app.post("/embed_batch")
def embed_batch(req: EmbedBatchRequest):
    """批处理 embed:一次编码多条文本。

    用同一模型批量 encode(模型内部批处理并行,MPS 利用率更高)。
    auto 路由下若中英混合,按首条语言统一选模型(避免跨模型批处理维度不一致);
    调用方需中英分模型批处理时显式传 model=qwen/minilm 分两次调。
    单条失败不阻断:对应位置返回 error 字段。
    """
    _metrics["embed_requests_total"] += len(req.texts)
    t0 = time.time()
    # 选模型:显式 > 按首条文本语言路由(auto 模式下统一用首条语言,保证维度一致)
    if req.model in MODELS:
        short = req.model
    elif req.texts:
        short = _route_model(req.texts[0])
    else:
        short = "minilm"  # 空批默认 minilm
    results = []
    try:
        model = _load_model(short)
        # 批量 encode:model.encode 接受 list[str],返回 numpy [N, dim]
        vecs = model.encode(req.texts, normalize_embeddings=True)
        for i, vec in enumerate(vecs):
            results.append(EmbedBatchItem(
                vector=vec.tolist(),
                model=short,
                dim=_dims[short],
            ))
    except Exception as e:
        _metrics["embed_errors_total"] += len(req.texts)
        log.error("embed_batch 批量失败(整批降级): %s", e)
        # 整批失败:每条都填 error
        results = [EmbedBatchItem(error=str(e)) for _ in req.texts]
    _metrics["embed_seconds_total"] += time.time() - t0
    return {
        "items": results,
        "model": short,
        "dim": _dims.get(short),
        "device": _device,
        "count": len(results),
    }


@app.get("/models")
def models():
    return {
        "current": FORCE_MODEL or "auto-routing",
        "routing": "zh→qwen3-embedding-4b, en→all-minilm-l6-v2",
        "models": {k: {"hf": v["hf"], "lang": v["lang"], "dim": _dims.get(k)} for k, v in MODELS.items()},
        "switch_via": "EMBED_MODEL=qwen|minilm (强制) 或请求参数 model=qwen|minilm",
    }


if __name__ == "__main__":
    log.info("LCO embedding 服务启动: %s:%d routing=%s", LISTEN_HOST, LISTEN_PORT, FORCE_MODEL or "auto")
    uvicorn.run(app, host=LISTEN_HOST, port=LISTEN_PORT, log_level="info")
