# ESP whisper 派生构建(2026-08-20)
# 根因:onerahmet/openai-whisper-api:latest 在 docker mirror 上 404 拉不下来(见 PROGRESS 23-15)。
# 方案:从本地既有 esp/python-runtime:3.13(=python:3.13-slim-bookworm)派生自建,用 faster-whisper
#       (CTranslate2 int8,CPU 友好,单节点无 GPU 下比 openai-whisper 轻)。
# 契约兼容 asr-adapter-go(backend/services/asr-adapter-go/src/adapter/whisper.go:57 whisperResp={text}):
#   POST /v1/audio/transcriptions  multipart(file=<audio>, model=<base>) → {"text": "..."}
#   GET  /health                    → {"status":"ok"}
#   端口 8000(Service whisper:80→targetPort 8000),runAsUser 1000(端口>1024 ✓)
# 铁律合规:
#   SVD:派生构建打非标 tag esp/whisper:base-cpu-ct2,禁 docker tag 劫持标准名
#   RWP:ENV HOME=/home/esp 锚定可写目录;HF_HOME 模型缓存挂 emptyDir(yaml 层挂)
#   ASV:faster-whisper API(WhisperModel.transcribe)凭知识,build 后实测 transcribe 不抛异常
#   CLC:transcribe 失败降级返回空 text(与 asr-adapter Transcribe 契约"失败返回空 text"一致)
#   SCS:依赖 pin 版本(requirements.txt),无秘密
FROM esp/python-runtime:3.13

# ffmpeg:faster-whisper 解码音频(解码非推理,用 ffmpeg)
# --no-install-recommends 减层;rm apt lists 减体积
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# 用户 + HOME(RWP 根治:ENV HOME 锚定,Dubbo/Nacos/SDK 默认路径落可写目录)
RUN groupadd -g 1000 esp 2>/dev/null || true \
    && useradd -r -u 1000 -g 1000 -d /home/esp -m esp
ENV HOME=/home/esp
# HF_HOME:faster-whisper 模型下载缓存(runAsUser 1000 下需可写,挂 emptyDir 在 yaml)
ENV HF_HOME=/home/esp/.cache/huggingface

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY server.py .

RUN chown -R esp:esp /app /home/esp
USER esp
EXPOSE 8000
CMD ["python", "server.py"]
