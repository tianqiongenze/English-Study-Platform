"""LCO Phase2 — L2 Milvus 语义缓存(writing-coach).

复用共享 collection `esp_llm_semantic`(conversation-go 创建,384dim minilm,cosine)。
- l2_search: embed prompt → Milvus 余弦搜索,radius≥0.92 命中直返 response。
- l2_store:  embed + insert(dynamic fields: prompt_sha16/response/model/scene/ts)。
- 全程降级安全:任何异常 log warn + return None,绝不阻断主链路。
- CLC nil-safe:client/driver 显式 None 守卫;env 未配则禁用对应层。
- WTV:已对真实 Milvus REST 实测确认路径/字段/v2-vectordb-collections API(2026-08-19)。
"""
from __future__ import annotations

import hashlib
import logging
import os
import time
from typing import Any

import httpx

logger = logging.getLogger("writing_coach_l2")

# 目标系统核对(WTV):已实测确认的真实端点/路径/字段
DEFAULT_EMBEDDING_URL = "http://host.lima.internal:15730/embed"
DEFAULT_MILVUS_ADDR = "milvus:19530"
EMBED_MODEL = "minilm"  # 固定 minilm 保 384dim 稳定(LCO 跨服务一致)
COLLECTION = "esp_llm_semantic"  # 共享 collection(跨服务语义缓存,LCO 原则)
SEMANTIC_RADIUS = 0.92  # cosine 命中阈值(LCO 规定)


class L2SemanticCache:
    """Milvus 语义缓存(L2)。降级安全:任何错误 → log warn + return None。"""

    def __init__(
        self,
        embedding_url: str | None = None,
        milvus_addr: str | None = None,
    ) -> None:
        self.embedding_url = (embedding_url or os.getenv("EMBEDDING_URL", DEFAULT_EMBEDDING_URL)).strip()
        self.milvus_addr = (milvus_addr or os.getenv("MILVUS_ADDR", DEFAULT_MILVUS_ADDR)).strip()
        self.enabled = os.getenv("L2_ENABLED", "true").strip().lower() != "false"
        # CLC:显式 None 守卫 — 未配端点则禁用
        if not self.embedding_url or not self.milvus_addr:
            self.enabled = False
            logger.warning("L2 disabled: embedding_url=%r milvus_addr=%r", self.embedding_url, self.milvus_addr)
        if self.enabled:
            logger.info("L2 Milvus semantic cache enabled: milvus=%s collection=%s", self.milvus_addr, COLLECTION)

    @staticmethod
    def _sha16(text: str) -> str:
        return hashlib.sha256(text.encode()).hexdigest()[:16]

    async def _embed(self, text: str) -> list[float] | None:
        """调 embedding 服务,返回 384dim 向量;失败降级 None。"""
        try:
            async with httpx.AsyncClient(timeout=3.0) as client:
                resp = await client.post(
                    self.embedding_url,
                    json={"text": text, "model": EMBED_MODEL},
                )
                resp.raise_for_status()
                vec = resp.json().get("vector")
                if not vec or not isinstance(vec, list):
                    logger.warning("L2 embed: invalid vector (type=%s)", type(vec).__name__)
                    return None
                return vec
        except Exception as e:
            logger.warning("L2 embed failed (non-fatal, degrade): %s", e)
            return None

    async def _milvus_search(self, vector: list[float]) -> dict[str, Any] | None:
        """Milvus v2 REST 余弦搜索;radius=0.92。失败降级 None。"""
        url = f"http://{self.milvus_addr}/v2/vectordb/entities/search"
        body = {
            "collectionName": COLLECTION,
            "data": [vector],
            "limit": 1,
            "outputFields": ["response", "model", "scene"],
            "params": {"radius": SEMANTIC_RADIUS},
        }
        try:
            async with httpx.AsyncClient(timeout=5.0) as client:
                resp = await client.post(url, json=body)
                resp.raise_for_status()
                data = resp.json()
                if data.get("code") != 0:
                    logger.warning("L2 milvus search code!=0: %s", str(data)[:120])
                    return None
                rows = data.get("data") or []
                return rows[0] if rows else None
        except Exception as e:
            logger.warning("L2 milvus search failed (non-fatal, degrade): %s", e)
            return None

    async def l2_search(self, prompt: str, model: str = "", scene: str = "") -> str | None:
        """L2 查询:prompt → embed → Milvus 搜索 → 命中返回 response,否则 None。"""
        if not self.enabled:
            return None
        # CLC:prompt 空守卫
        if not prompt:
            return None
        vec = await self._embed(prompt)
        if vec is None:
            return None
        row = await self._milvus_search(vec)
        if row is None:
            return None
        # 可选:scene/model 过滤(命中但 scene 不匹配则视为未命中,避免跨场景串味)
        if scene and row.get("scene") not in (None, "", scene):
            logger.debug("L2 hit but scene mismatch (got=%s want=%s)", row.get("scene"), scene)
            return None
        resp = row.get("response")
        if not resp:
            return None
        logger.info("L2 hit (model=%s scene=%s)", model, scene)
        return str(resp)

    async def l2_store(self, prompt: str, response: str, model: str = "", scene: str = "") -> None:
        """L2 写入:embed prompt + insert(dynamic fields)。失败降级。"""
        if not self.enabled:
            return
        # CLC:空 prompt/response 守卫
        if not prompt or not response:
            return
        vec = await self._embed(prompt)
        if vec is None:
            return
        url = f"http://{self.milvus_addr}/v2/vectordb/entities/insert"
        body = {
            "collectionName": COLLECTION,
            "data": [{
                "vector": vec,
                "prompt_sha16": self._sha16(prompt),
                "response": response,
                "model": model,
                "scene": scene,
                "ts": int(time.time()),
            }],
        }
        try:
            async with httpx.AsyncClient(timeout=5.0) as client:
                resp = await client.post(url, json=body)
                resp.raise_for_status()
                data = resp.json()
                if data.get("code") != 0:
                    logger.warning("L2 milvus insert code!=0: %s", str(data)[:120])
        except Exception as e:
            logger.warning("L2 milvus store failed (non-fatal, degrade): %s", e)
