"""LCO Phase3 — L3 Neo4j 知识图谱(writing-coach)。

Prompt → [:ANSWERED] → Response 图谱,用于上下文增强(prompt 关联历史响应)。
- l3_record: 记录 prompt↔response 关系(MERGE 幂等)。
- l3_enhance: 查找 prompt 的历史响应,返回作为上下文。
- 全程降级安全:任何异常 log warn + return None,绝不阻断主链路。
- CLC nil-safe:driver 显式 None 守卫;session 每次新建用完即关(避免跨线程复用)。
- ASV:neo4j 6.2.0 API 已核实(GraphDatabase.driver(uri, *, auth=None, **config);
  session.execute_write(tx_fn, *args, **kwargs);tx.run(query, parameters=None, **kw);
  session 为 context manager __enter__/__exit__)。源码核实 2026-08-19。
- 写入路径经 asyncio.to_thread 执行(sync driver 阻塞调用放线程池,不阻塞事件循环)。
"""
from __future__ import annotations

import asyncio
import logging
import os

logger = logging.getLogger("writing_coach_l3")

DEFAULT_NEO4J_URL = "bolt://neo4j:7687"
DEFAULT_NEO4J_USER = "neo4j"
# 开发默认值;生产优先从 Secret 注入(NEO4J_PASSWORD env)。不硬编码到 git 提交的敏感值以外。
DEFAULT_NEO4J_PASSWORD = "neo4j-password-1"  # dev fallback only — overridden by env in deploy


class L3KnowledgeGraph:
    """Neo4j 知识图谱(L3)。降级安全:任何错误 → log warn + return None/不写。"""

    def __init__(self) -> None:
        self.url = os.getenv("NEO4J_URL", DEFAULT_NEO4J_URL).strip()
        self.user = os.getenv("NEO4J_USER", DEFAULT_NEO4J_USER).strip()
        self.password = os.getenv("NEO4J_PASSWORD", DEFAULT_NEO4J_PASSWORD)
        self.enabled = os.getenv("L3_ENABLED", "true").strip().lower() != "false"
        # CLC:显式空守卫 — 未配则禁用
        if not self.url or not self.user:
            self.enabled = False
            logger.warning("L3 disabled: url=%r user=%r", self.url, self.user)
        self._driver = None  # 延迟初始化(lazy),避免 import 期建连失败炸进程
        if self.enabled:
            logger.info("L3 Neo4j knowledge graph enabled: url=%s user=%s", self.url, self.user)

    def _get_driver(self):
        """延迟初始化 driver。失败 → 置 disabled,返回 None(CLC nil-safe)。"""
        if not self.enabled:
            return None
        if self._driver is None:
            try:
                from neo4j import (
                    GraphDatabase,  # ASV:6.2.0 driver(cls, uri, *, auth=None, **config)
                )
                self._driver = GraphDatabase.driver(self.url, auth=(self.user, self.password))
                logger.info("L3 Neo4j driver created")
            except Exception as e:
                logger.warning("L3 Neo4j driver init failed (non-fatal, disabled): %s", e)
                self.enabled = False
                self._driver = None
        return self._driver

    @staticmethod
    def _sha16(text: str) -> str:
        import hashlib
        return hashlib.sha256(text.encode()).hexdigest()[:16]

    def _record_sync(self, prompt: str, response: str, model: str, scene: str) -> None:
        """同步执行写入(safe to run in thread)。CLC:driver/session 判空。"""
        driver = self._get_driver()
        if driver is None:  # CLC nil-safe
            return
        try:
            # ASV:session 为 context manager;execute_write(tx_fn, *args)
            with driver.session() as session:
                def _tx(tx) -> None:
                    # ASV:tx.run(query, parameters=None, **kw) → Result
                    tx.run(
                        """
                        MERGE (p:Prompt {sha16: $sha16})
                        SET p.text = $text, p.model = $model, p.scene = $scene, p.ts = timestamp()
                        MERGE (r:Response {sha16: $rsha16})
                        SET r.text = $rtext, r.model = $model, r.ts = timestamp()
                        MERGE (p)-[:ANSWERED]->(r)
                        """,
                        sha16=self._sha16(prompt),
                        text=prompt,
                        model=model,
                        scene=scene,
                        rsha16=self._sha16(response),
                        rtext=response,
                    )
                session.execute_write(_tx)
        except Exception as e:
            logger.warning("L3 neo4j record failed (non-fatal, degrade): %s", e)

    async def l3_record(self, prompt: str, response: str, model: str = "", scene: str = "") -> None:
        """L3 写入(fire-and-forget,降级安全)。sync driver 经 to_thread 不阻塞事件循环。"""
        if not self.enabled or not prompt or not response:
            return
        try:
            await asyncio.to_thread(self._record_sync, prompt, response, model, scene)
        except Exception as e:
            logger.warning("L3 l3_record dispatcher failed (non-fatal): %s", e)

    def _enhance_sync(self, prompt: str) -> str | None:
        """同步查找 prompt 的历史响应。CLC:driver/session 判空。"""
        driver = self._get_driver()
        if driver is None:  # CLC nil-safe
            return None
        try:
            with driver.session() as session:
                def _tx(tx):
                    result = tx.run(
                        """
                        MATCH (p:Prompt {sha16: $sha16})-[:ANSWERED]->(r:Response)
                        RETURN r.text AS resp ORDER BY r.ts DESC LIMIT 1
                        """,
                        sha16=self._sha16(prompt),
                    )
                    rec = result.single(strict=False)  # strict=False: 无记录返 None 不抛
                    return rec["resp"] if rec else None
                return session.execute_read(_tx)
        except Exception as e:
            logger.warning("L3 neo4j enhance failed (non-fatal, degrade): %s", e)
            return None

    async def l3_enhance(self, prompt: str) -> str | None:
        """L3 查询:返回 prompt 的历史响应作为上下文增强,无则 None。"""
        if not self.enabled or not prompt:
            return None
        try:
            return await asyncio.to_thread(self._enhance_sync, prompt)
        except Exception as e:
            logger.warning("L3 l3_enhance dispatcher failed (non-fatal): %s", e)
            return None

    async def close(self) -> None:
        if self._driver is not None:
            try:
                await asyncio.to_thread(self._driver.close)
            except Exception as e:
                logger.warning("L3 neo4j close failed (non-fatal): %s", e)
            self._driver = None
