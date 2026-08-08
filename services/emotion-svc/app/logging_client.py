"""结构化业务日志客户端 —— emotion-svc 异步写 TiDB app_logs(ADR-observability-rebuild)。

复用 writing-py/app/logging_client.py 的设计:后台线程 + 有界队列 + 降级不阻断。
service 名默认 'emotion'。
"""
from __future__ import annotations

import json
import logging
import os
import queue
import threading
import time
from typing import Any, Optional

import pymysql

logger = logging.getLogger(__name__)

_QUEUE: "queue.Queue[Optional[dict]]" = queue.Queue(maxsize=4096)
_WORKER_STARTED = False
_LOCK = threading.Lock()
_DROPPED = 0

_DB_CONFIG = {
    "host": os.environ.get("TIDB_HOST", "tidb-primary"),
    "port": int(os.environ.get("TIDB_PORT", "4000")),
    "user": os.environ.get("DB_USER", "root"),
    "password": os.environ.get("DB_PASS", ""),
    "database": os.environ.get("APP_LOG_DB", "esp_shared"),
    "autocommit": True,
    "connect_timeout": 2,
    "read_timeout": 5,
    "write_timeout": 5,
}

_SERVICE = os.environ.get("OTEL_SERVICE_NAME", "emotion")
_ENABLED = os.environ.get("ESP_APP_LOG_ENABLED", "true").lower() == "true"


def _current_trace_id() -> Optional[str]:
    try:
        from opentelemetry import trace
        ctx = trace.get_current_span().get_span_context()
        return ctx.trace_id if ctx.is_valid else None
    except Exception:
        return None


def _worker() -> None:
    conn = None
    while True:
        item = _QUEUE.get()
        if item is None:
            return
        try:
            if conn is None or not conn.open:
                conn = pymysql.connect(**_DB_CONFIG)
            payload_json = json.dumps(item["payload"]) if item.get("payload") else None
            with conn.cursor() as cur:
                cur.execute(
                    "INSERT INTO app_logs (ts, service, level, event, trace_id, user_id, payload) "
                    "VALUES (NOW(3), %s, %s, %s, %s, %s, CAST(%s AS JSON))",
                    (_SERVICE, item["level"], item["event"], item.get("trace_id"),
                     item.get("user_id"), payload_json),
                )
        except Exception as ex:
            logger.warning("app_logs insert failed (degraded): %s", ex)
            conn = None
        finally:
            _QUEUE.task_done()


def _ensure_worker() -> None:
    global _WORKER_STARTED
    if _WORKER_STARTED:
        return
    with _LOCK:
        if _WORKER_STARTED:
            return
        t = threading.Thread(target=_worker, name="app-log-writer", daemon=True)
        t.start()
        _WORKER_STARTED = True


def app_log(level: str, event: str, user_id: int | None = None,
            payload: dict[str, Any] | None = None, service: str | None = None) -> None:
    """异步写一条结构化业务日志。level: INFO/WARN/ERROR;event: emotion.detect/..."""
    if not _ENABLED:
        return
    _ensure_worker()
    item = {
        "service": service or _SERVICE,
        "level": level,
        "event": event,
        "trace_id": _current_trace_id(),
        "user_id": user_id,
        "payload": payload,
        "ts": time.time(),
    }
    try:
        _QUEUE.put_nowait(item)
    except queue.Full:
        global _DROPPED
        _DROPPED += 1


def shutdown() -> None:
    if _WORKER_STARTED:
        _QUEUE.put(None)
