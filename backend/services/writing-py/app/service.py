"""写作批改/润色/翻译业务逻辑(异步)。

多级缓存(esp-cache-py):grade/polish/translate 三个幂等 LLM 接口走 L1 cachetools(LRU)
+ L2 redis.asyncio(Redis Cluster)。相同输入必同结果,显著省 LLM 调用成本。
LLM_MODE=mock 时缓存仍生效(确定性 mock 命中缓存);CACHE_ENABLED=false 直调。

读写分离(esp-db-py):批改结果持久化到 TiDB writing_tasks 表(写主库 tidb-primary);
查询用户历史作文走从库 tidb-follower(读副本)。与 Java 侧 @DataSource 语义对齐。
"""
from __future__ import annotations

import json
import logging

from esp_cache import get_or_load
from esp_db import execute_write, execute_read, execute_read_one

from .llm import LlmProvider, extract_json
from .schemas import (
    Coherence, GrammarIssue, GradeRequest, GradeResponse,
    PolishRequest, PolishResponse, Suggestion, TranslateRequest, TranslateResponse, VocabItem,
)

logger = logging.getLogger("writing_service")


class WritingService:
    def __init__(self, llm: LlmProvider):
        self.llm = llm

    async def grade(self, req: GradeRequest) -> GradeResponse:
        # 从库读用户历史(只读,走 tidb-follower);DB_ENABLED=false 时 no-op 返回空
        user_id = getattr(req, "user_id", 0) or 0
        if user_id:
            try:
                rows = await execute_read(
                    "SELECT id, score, feedback FROM writing_tasks WHERE user_id=%s ORDER BY created_at DESC LIMIT 5",
                    (user_id,),
                )
                if rows:
                    logger.info("grade: loaded %d history essays for user %s from follower", len(rows), user_id)
            except Exception as e:
                logger.warning("grade: read history from follower failed (non-fatal): %s", e)

        async def _load():
            system = (
                "You are a strict but constructive TOEFL/IELTS writing examiner. "
                "Return ONLY valid JSON with keys: score(0-30), feedback(str), "
                "grammar(list of {original,corrected,reason}), "
                "coherence({score 0-10, transitions:list[str], paragraph_flow:str})."
            )
            user = f"Exam: {req.exam}\nPrompt: {req.prompt}\nEssay:\n{req.essay}"
            raw = await self.llm.chat(system, user)
            data = extract_json(raw)
            coh = data.get("coherence", {})
            return GradeResponse(
                score=float(data.get("score", 0)),
                feedback=str(data.get("feedback", "")),
                grammar=[GrammarIssue(**g) for g in data.get("grammar", [])],
                coherence=Coherence(
                    score=float(coh.get("score", 0)),
                    transitions=list(coh.get("transitions", [])),
                    paragraph_flow=str(coh.get("paragraph_flow", "")),
                ),
            ).model_dump()
        # L1 cachetools → L2 redis → LLM;key 由 exam+prompt+essay 决定
        cached = await get_or_load("writing:grade",
                                   (req.exam, req.prompt, req.essay), _load, cache_empty=False)
        result = GradeResponse(**cached) if cached else await _load()
        # 持久化批改结果到 TiDB(写主库 tidb-primary);DB_ENABLED=false 时 no-op
        if user_id:
            try:
                await execute_write(
                    "INSERT INTO writing_tasks (user_id, prompt, essay, score, feedback) VALUES (%s,%s,%s,%s,%s)",
                    (user_id, req.prompt, req.essay, float(result.score),
                     json.dumps({"feedback": result.feedback, "grammar": [g.model_dump() for g in result.grammar]})),
                )
                logger.info("grade: persisted writing_task for user %s to primary", user_id)
            except Exception as e:
                logger.warning("grade: persist to primary failed (non-fatal): %s", e)
        return result

    async def polish(self, req: PolishRequest) -> PolishResponse:
        async def _load():
            system = (
                "You are an English writing editor. Polish the text for grammar and style, "
                "merging simple sentences into advanced ones where appropriate. "
                "Return ONLY JSON: {polished:str, suggestions:list[{original,polished,reason}]}."
            )
            raw = await self.llm.chat(system, f"Polish:\n{req.text}")
            data = extract_json(raw)
            return PolishResponse(
                polished=str(data.get("polished", req.text)),
                suggestions=[Suggestion(**s) for s in data.get("suggestions", [])],
            ).model_dump()
        cached = await get_or_load("writing:polish", (req.text,), _load, cache_empty=False)
        return PolishResponse(**cached) if cached else await _load()

    async def translate(self, req: TranslateRequest) -> TranslateResponse:
        async def _load():
            system = (
                "Translate the English text to Chinese and extract at least 3 vocabulary items. "
                "Return ONLY JSON: {translation:str, vocab:list[{word,pos,meaning,example}]}."
            )
            raw = await self.llm.chat(system, f"Text:\n{req.text}")
            data = extract_json(raw)
            return TranslateResponse(
                translation=str(data.get("translation", "")),
                vocab=[VocabItem(**v) for v in data.get("vocab", [])],
            ).model_dump()
        cached = await get_or_load("writing:translate", (req.text,), _load, cache_empty=False)
        return TranslateResponse(**cached) if cached else await _load()
