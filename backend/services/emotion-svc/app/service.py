"""模块五:情感化与多模态交互服务。

多级缓存(esp-cache-py):visual_for_word 接图像生成模型时(真实化后)是缓存热点(相同 word+context
结果可复用,图像生成昂贵)。本期 mock 阶段 visual 仍走缓存以验证链路;sense_emotion 纯本地字符串
匹配,计算成本纳秒级,不入缓存。

读写分离(esp-db-py):情绪感知结果持久化到 learning_events(写池 crdb-public/esp_learning),
供学习分析回溯;查询用户近期情绪历史走读池(单端点阶段与写同址)。与 Java 侧 @DataSource 语义对齐。
"""
from __future__ import annotations

import json
import logging

from pydantic import BaseModel
from esp_cache import get_or_load
from esp_db import execute_write, execute_read

logger = logging.getLogger("emotion_service")


class SenseRequest(BaseModel):
    text: str = ""
    audio: str = ""  # base64 mock


class SenseResponse(BaseModel):
    emotion: str
    encouragement: str


class VisualRequest(BaseModel):
    word: str
    context: str = ""


class VisualResponse(BaseModel):
    image_url: str
    alt: str


NEGATIVE = {"sad", "tired", "frustrated", "angry", "anxious", "nervous", "worried", "afraid", "stressed", "difficult", "hard", "fail", "bad"}
POSITIVE = {"happy", "excited", "great", "good", "love", "confident", "proud", "easy", "fun", "wonderful", "amazing"}


def sense_emotion(text: str) -> SenseResponse:
    """FR-5.1 文本情绪感知:负面 -> 安慰;正面 -> 鼓励;中性 -> 通用鼓励。

    结果通过 persist_emotion 异步落库 learning_events(写池),非阻断。
    """
    low = (text or "").lower()
    if any(w in low for w in NEGATIVE):
        resp = SenseResponse(emotion="negative",
                             encouragement="It's okay to feel this way. Take a breath — progress takes time, and I'm here with you.")
    elif any(w in low for w in POSITIVE):
        resp = SenseResponse(emotion="positive",
                             encouragement="Wonderful! Keep up the great momentum — you're doing amazing!")
    else:
        resp = SenseResponse(emotion="neutral",
                             encouragement="You're making steady progress. Keep going, one step at a time!")
    return resp


async def persist_emotion(user_id: int, text: str, emotion: str) -> None:
    """情绪感知结果持久化到 learning_events(写池 crdb-public/esp_learning)。

    非阻断:失败只 warn,不影响情绪感知主链路。DB_ENABLED=false 时 no-op。
    """
    if not user_id:
        return
    try:
        await execute_write(
            "INSERT INTO learning_events (user_id, event_type, payload) VALUES (%s,%s,%s)",
            (user_id, "emotion",
             json.dumps({"text_snippet": (text or "")[:200], "emotion": emotion})),
        )
        logger.info("persisted emotion event for user %s to primary", user_id)
    except Exception as e:
        logger.warning("persist emotion to primary failed (non-fatal): %s", e)


async def recent_emotions(user_id: int, limit: int = 10) -> list[dict]:
    """查询用户近期情绪事件(读池读)。DB_ENABLED=false 返回空。"""
    if not user_id:
        return []
    try:
        rows = await execute_read(
            "SELECT event_type, payload, created_at FROM learning_events "
            "WHERE user_id=%s AND event_type='emotion' ORDER BY created_at DESC LIMIT %s",
            (user_id, limit),
        )
        return [{"event_type": r[0], "payload": r[1], "created_at": str(r[2])} for r in rows]
    except Exception as e:
        logger.warning("read recent emotions from follower failed (non-fatal): %s", e)
        return []


async def visual_for_word(word: str, context: str = "") -> VisualResponse:
    """FR-5.2 视觉辅助:本期返回占位图 URL(真实环境接图像生成模型)。
    走多级缓存:相同 word+context 结果稳定可复用,真实化后图像生成昂贵,缓存收益高。"""
    async def _load():
        w = (word or "word").strip() or "word"
        label = w.replace(" ", "%20")
        return VisualResponse(
            image_url=f"https://placehold.co/600x400?text={label}",
            alt=f"Illustration for the word '{w}'" + (f" in context: {context}" if context else ""),
        ).model_dump()
    cached = await get_or_load("emotion:visual", (word, context), _load, cache_empty=False)
    return VisualResponse(**cached) if cached else VisualResponse(**await _load())
