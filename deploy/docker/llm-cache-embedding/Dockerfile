# LCO Embedding 服务跨平台 Dockerfile(2026-08-20 会话23-20)
# 支持 CPU / CUDA GPU 两种构建(多阶段,生产 Linux 部署用)。
#
# 构建:
#   CPU:   docker build -t esp/llm-cache-embedding:cpu --build-arg DEVICE=cpu .
#   CUDA:  docker build -t esp/llm-cache-embedding:cuda --build-arg DEVICE=cuda .
#
# 运行:
#   CPU:   docker run -p 15730:15730 -p 15731:15731 esp/llm-cache-embedding:cpu
#   CUDA:  docker run --gpus all -p 15730:15730 -p 15731:15731 esp/llm-cache-embedding:cuda
#
# Mac 开发:继续用宿主 venv(python3 tools/llm-cache-proxy/llm-cache-embedding/app.py),MPS 原生。
#
# 铁律合规:
#   SVD:非标 tag esp/llm-cache-embedding:{cpu,cuda},不劫持标准名
#   RWP:HOME=/home/esp,模型缓存 HF_HOME 挂 emptyDir/卷
#   ASV:build 后实测 embed 返回正确维度向量
#   CLC:加载失败降级,不阻断
#   SCS:依赖 pin 版本(requirements.txt + grpcio pin)

# 基础镜像:CPU 用 python slim,CUDA 用 nvidia cuda 镜像
ARG DEVICE=cpu
FROM python:3.11-slim-bookworm AS base-cpu
FROM nvidia/cuda:12.4.0-cudnn-runtime-ubuntu22.04 AS base-cuda
# base-cuda 需装 python
RUN apt-get update && apt-get install -y python3 python3-pip && rm -rf /var/lib/apt/lists/*

# 最终阶段(按 DEVICE 选择 base)
FROM base-${DEVICE}

# ffmpeg:无(MPS 编码不在容器);CPU/CUDA 推理不需 ffmpeg(纯向量化)
# git:HF 下载需 git lfs(模型克隆)
RUN apt-get update \
    && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 用户(RWP)
RUN groupadd -g 1000 esp 2>/dev/null || true \
    && useradd -r -u 1000 -g 1000 -d /home/esp -m esp
ENV HOME=/home/esp
ENV HF_HOME=/home/esp/.cache/huggingface

# 依赖(分层缓存)
COPY tools/llm-cache-proxy/llm-cache-embedding/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt \
    && pip install --no-cache-dir grpcio==1.83.0 grpcio-tools==1.83.0

# 代码
COPY tools/llm-cache-proxy/llm-cache-embedding/ /app/
COPY tools/llm-cache-proxy/proto/embedding.proto /app/proto/embedding.proto
RUN python3 -m grpc_tools.protoc -I/proto --python_out=/app/grpc_stubs --grpc_python_out=/app/grpc_stubs /app/proto/embedding.proto \
    && rm -rf /app/proto

RUN chown -R esp:esp /app /home/esp
USER esp

EXPOSE 15730 15731
# PRELOAD=minilm(qwen 懒加载省内存),GRPC_ENABLED=1(Phase 5.2)
ENV PRELOAD_MODELS=minilm \
    GRPC_ENABLED=1 \
    GRPC_PORT=15731 \
    LISTEN_HOST=0.0.0.0 \
    LISTEN_PORT=15730 \
    HF_ENDPOINT=https://hf-mirror.com

CMD ["python3", "app.py"]
