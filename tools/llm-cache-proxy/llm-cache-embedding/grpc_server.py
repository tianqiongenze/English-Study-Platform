"""LCO Embedding gRPC server(2026-08-20 会话23-20,Phase 5.2)

与现有 FastAPI HTTP 服务(15730)并行,监听 15731。
llm-cache-proxy 通过 LCO_EMBED_PROTOCOL=grpc 切换到 gRPC(protobuf 向量传输,~2.5x 体积↓)。

复用 app.py 的 _load_model/_unload_model(同进程,共享模型缓存)。
ASV:embed gRPC 返回的 vector 与 HTTP /embed 逐字节一致(同 model 同 text)。
CLC:gRPC 失败降级返回空 vector(不抛异常,与 HTTP 降级一致)。
"""
import os
import sys
import asyncio
import logging
from concurrent import futures

# 让 stub import 可用(grpc_stubs 目录)
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)) + "/grpc_stubs")
import grpc
import embedding_pb2
import embedding_pb2_grpc

log = logging.getLogger("llm-cache-embedding-grpc")

# 复用 app.py 的模型加载层(同进程 import)
# app.py 在同目录,import 其 _load_model/_models/_dims/_device/_preload_status
import app as _app


class EmbeddingServicer(embedding_pb2_grpc.EmbeddingServiceServicer):
    """gRPC servicer,复用 app.py 的模型层。"""

    def _embed_one(self, text: str, model: str):
        """单条向量化,复用 app.py 路由 + 加载。返回 (vector, model, dim, device)。"""
        # model 路由:复用 app.py 的 _route_model(按语言)或显式
        from app import _load_model, _models, _dims, _device, _route_model, FORCE_MODEL
        short = FORCE_MODEL or model or _route_model(text)
        if short not in _models:
            # 懒加载(若 qwen 未加载,首次触发加载)
            _load_model(short)
        m = _models[short]
        vec = m.encode(text, normalize_embeddings=True).tolist()
        return vec, short, _dims.get(short, len(vec)), _device

    def Embed(self, request, context):
        try:
            vec, model, dim, device = self._embed_one(request.text, request.model)
            return embedding_pb2.EmbedResponse(
                vector=vec, model=model, dim=dim, device=device
            )
        except Exception as e:
            log.error("gRPC Embed 失败: %s", e)
            # CLC 降级:返回空 vector(不抛异常)
            return embedding_pb2.EmbedResponse(vector=[], model=request.model, dim=0, device="")

    def EmbedBatch(self, request, context):
        try:
            results = []
            for item in request.items:
                vec, model, dim, device = self._embed_one(item.text, item.model)
                results.append(embedding_pb2.EmbedResponse(
                    vector=vec, model=model, dim=dim, device=device
                ))
            return embedding_pb2.EmbedBatchResponse(results=results)
        except Exception as e:
            log.error("gRPC EmbedBatch 失败: %s", e)
            return embedding_pb2.EmbedBatchResponse(results=[])

    def Health(self, request, context):
        from app import _models, _preload_status
        loaded = {k: (k in _models) for k in ("qwen", "minilm")}
        ready = all(
            _preload_status.get(t) in ("ready", "failed", "pending")
            for t in ("minilm",)  # minilm 是业务侧刚需
        )
        return embedding_pb2.HealthResponse(ready=ready, loaded=loaded)


def serve_grpc(port: int = 15731):
    """启动 gRPC server(与 FastAPI 并行)。"""
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    embedding_pb2_grpc.add_EmbeddingServiceServicer_to_server(EmbeddingServicer(), server)
    server.add_insecure_port(f"127.0.0.1:{port}")
    server.start()
    log.info("gRPC server 监听 127.0.0.1:%d(与 FastAPI 15730 并行)", port)
    return server


if __name__ == "__main__":
    # 独立测试入口(实际由 app.py lifespan 拉起)
    logging.basicConfig(level=logging.INFO)
    srv = serve_grpc()
    try:
        srv.wait_for_termination()
    except KeyboardInterrupt:
        srv.stop(0)
