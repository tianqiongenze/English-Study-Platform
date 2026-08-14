"""Pulsar producer(EDA):writing-py 发 writing.graded 事件到 teacher-java。

设计:
- producer 延迟初始化(首次 publish 时创建),避免服务启动即连 Pulsar。
- send_async 失败降级:只 log 不抛(批改结果正常返回,不被 EDA 阻断)。
- PULSAR_ENABLED=false 时直接 no-op(默认,本地/CI 环境)。
- PULSAR_URL 默认 pulsar-broker:6650(K8s ExternalName)。
"""
from __future__ import annotations

import json
import logging
import os
import threading
from typing import Any

logger = logging.getLogger(__name__)

_PULSAR_URL = os.getenv("PULSAR_URL", "pulsar://pulsar-broker:6650")
_PULSAR_ENABLED = os.getenv("PULSAR_ENABLED", "false").lower() in ("1", "true", "yes")

_CLIENT = None
_PRODUCER = None
_LOCK = threading.Lock()


def _get_producer():
    """延迟初始化 Pulsar client + producer。失败返回 None。"""
    global _CLIENT, _PRODUCER
    if _PRODUCER is not None:
        return _PRODUCER
    with _LOCK:
        if _PRODUCER is not None:
            return _PRODUCER
        try:
            import pulsar  # type: ignore

            _CLIENT = pulsar.Client(_PULSAR_URL)
            _PRODUCER = _CLIENT.create_producer(
                "writing.graded",
                producer_name="writing-py",
                send_timeout_millis=2000,
            )
            logger.info("pulsar producer ready: topic=writing.graded url=%s", _PULSAR_URL)
        except Exception as e:  # noqa: BLE001
            logger.warning("pulsar producer init failed (non-fatal): %s", e)
            _PRODUCER = None
        return _PRODUCER


def publish_writing_graded(payload: dict[str, Any]) -> None:
    """异步发送 writing.graded 事件;任何失败只 log 不抛。

    payload 约定字段:submission_id/user_id/thread_id/status='auto_graded'。
    """
    if not _PULSAR_ENABLED:
        return
    producer = _get_producer()
    if producer is None:
        logger.warning("pulsar producer unavailable, skip writing.graded: %s", payload.get("thread_id"))
        return
    try:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")

        def _on_ok(_msg_id, _msg):
            pass

        def _on_fail(_producer, _msg, exc):
            logger.warning("pulsar send writing.graded failed (non-fatal): %s", exc)

        producer.send_async(body, callback=_on_fail)
    except Exception as e:  # noqa: BLE001
        logger.warning("pulsar publish writing.graded exception (non-fatal): %s", e)
