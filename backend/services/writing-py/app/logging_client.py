"""结构化业务日志客户端 —— 异步写 CRDB esp_shared.app_logs 表(ADR-observability-rebuild)。

设计:
- 异步:后台线程 + 有界队列,不阻塞业务主路径(Litestar 请求处理)。
- 降级:CRDB 不可用时仅 logging.warning,不抛异常(参考 esp-cache-py 降级)。
- traceId:从 OTel 当前 span 提取(若已初始化 tracer)。
- 连接:走 crdb-public(主写),psycopg 直连(轻量,不引入 SQLAlchemy)。

使用:
    from app.logging_client import app_log
    app_log("INFO", "writing.grade", user_id=42, payload={"score": 8.5, "duration_ms": 1200})
"""
from __future__ import annotations

import json
import logging
import os
import queue
import threading
import time
from typing import Any, Optional

import psycopg

logger = logging.getLogger(__name__)

# 后台写线程 + 有界队列(满则丢弃,保业务)
_QUEUE: "queue.Queue[Optional[dict]]" = queue.Queue(maxsize=4096)
_WORKER_STARTED = False
_LOCK = threading.Lock()
_DROPPED = 0

_DB_CONFIG = {
    "host": os.environ.get("CRDB_HOST", "crdb-public"),
    "port": int(os.environ.get("CRDB_PORT", "26257")),
    "user": os.environ.get("DB_USER", "root"),
    "password": os.environ.get("DB_PASS", ""),
    "dbname": os.environ.get("APP_LOG_DB", "esp_shared"),
    "autocommit": True,
    "connect_timeout": 2,
}

_SERVICE = os.environ.get("OTEL_SERVICE_NAME", "writing")
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
            if conn is None or conn.closed or conn.broken:
                conn = psycopg.connect(**_DB_CONFIG)
            payload_json = json.dumps(item["payload"]) if item.get("payload") else None
            with conn.cursor() as cur:
                cur.execute(
                    "INSERT INTO app_logs (ts, service, level, event, trace_id, user_id, payload) "
                    "VALUES (now(), %s, %s, %s, %s, %s, %s::jsonb)",
                    (_SERVICE, item["level"], item["event"], item.get("trace_id"),
                     item.get("user_id"), payload_json),
                )
        except Exception as ex:
            # 降级:仅 warn,不阻断
            logger.warning("app_logs insert failed (degraded): %s", ex)
            try:
                if conn is not None:
                    conn.close()
            except Exception:
                pass
            conn = None  # 下次重连
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
    """异步写一条结构化业务日志。level: INFO/WARN/ERROR;event: writing.grade/..."""
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
    """优雅停机:等待队列排空。"""
    if _WORKER_STARTED:
        _QUEUE.put(None)
