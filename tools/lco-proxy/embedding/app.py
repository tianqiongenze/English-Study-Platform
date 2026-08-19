# lco-embedding —— LCO 代理 L2 语义缓存的 embedding 服务(宿主 python venv)
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
from fastapi import FastAPI
from fastapi.responses import Response
from pydantic import BaseModel
import uvicorn

logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(message)s')
log = logging.getLogger("lco-embedding")

LISTEN_HOST = os.getenv("LISTEN_HOST", "0.0.0.0")
LISTEN_PORT = int(os.getenv("LISTEN_PORT", "15730"))

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

app = FastAPI(title="LCO Embedding Service")
_models = {}   # 短名 -> SentenceTransformer
_dims = {}     # 短名 -> dim
_device = "cpu"

# Prometheus 指标
_metrics = {
    "embed_requests_total": 0,
    "embed_errors_total": 0,
    "embed_seconds_total": 0.0,
}


def _load_model(short_name: str):
    """惰性加载指定模型 + 自动选 device(MPS 优先)。"""
    global _device
    if short_name in _models:
        return _models[short_name]
    from sentence_transformers import SentenceTransformer
    import torch

    if torch.backends.mps.is_available():
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


class EmbedResponse(BaseModel):
    vector: list
    model: str
    dim: int
    device: str


@app.get("/healthz")
def healthz():
    loaded = {k: k in _models for k in MODELS}
    return {
        "status": "ok",
        "loaded": loaded,
        "device": _device,
        "force_model": FORCE_MODEL or "auto",
    }


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
