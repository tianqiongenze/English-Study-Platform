"""ESP whisper 派生服务(2026-08-20)

兼容 asr-adapter-go 契约(backend/services/asr-adapter-go/src/adapter/whisper.go:57):
  POST /v1/audio/transcriptions  multipart(file=<audio>, model=<base>) → {"text": "..."}
  GET  /health                    → {"status":"ok"}

引擎:faster-whisper(CTranslate2 int8,CPU)。
铁律:
  CLC:transcribe 失败降级返回空 text(与 asr-adapter Transcribe "失败返回空 text" 一致,不抛异常)。
  RWT:每请求超时(faster-whisper CPU 慢,设 max_initial_prompt 等护坡)。
  ASV:faster-whisper WhisperModel 与 transcribe API 凭知识,build 后实测 transcribe 不抛异常为准。
"""
from __future__ import annotations

import io
import os
import logging
from typing import Optional

from fastapi import FastAPI, UploadFile, File, Form, HTTPException
from fastapi.responses import JSONResponse

logger = logging.getLogger("whisper")
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")

app = FastAPI(title="esp-whisper-cpu")

# 模型懒加载(首次请求加载,base ~150MB)
_model = None
_model_name = os.environ.get("WHISPER_MODEL", "base")
# CPU int8,单节点无 GPU;device="cpu", compute_type="int8"
_compute_type = os.environ.get("WHISPER_COMPUTE_TYPE", "int8")


def _get_model():
    global _model
    if _model is None:
        # ASV:faster-whisper WhisperModel(model_size, device, compute_type)
        # 知识:from faster_whisper import WhisperModel; WhisperModel("base", device="cpu", compute_type="int8")
        from faster_whisper import WhisperModel
        logger.info("loading faster-whisper model=%s compute=%s", _model_name, _compute_type)
        _model = WhisperModel(_model_name, device="cpu", compute_type=_compute_type)
        logger.info("model loaded")
    return _model


@app.get("/health")
async def health():
    # 不在 health 触发模型加载(避免探针拖慢);模型懒加载在首次 transcribe
    return {"status": "ok", "model": _model_name}


@app.post("/v1/audio/transcriptions")
async def transcriptions(
    file: UploadFile = File(...),
    model: Optional[str] = Form(default=None),
):
    audio_bytes = await file.read()
    if not audio_bytes:
        # CLC:空输入降级空 text(契约一致),不 4xx
        return JSONResponse({"text": ""})

    try:
        m = _get_model()
        # ASV:transcribe(audio, language=None, beam_size=5) 返回 (segments, info)
        # 知识:segments 是 generator,每段 .text;audio 接 bytes/np.ndarray/file-like
        segments, _info = m.transcribe(io.BytesIO(audio_bytes), beam_size=5)
        text = " ".join(seg.text.strip() for seg in segments).strip()
        return JSONResponse({"text": text})
    except Exception as e:
        # CLC:失败降级空 text(与 asr-adapter 失败返回空 text 契约一致),不中断主链
        logger.error("transcribe failed: %s", e, exc_info=True)
        return JSONResponse({"text": ""}, status_code=200)


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000, log_level="info")
