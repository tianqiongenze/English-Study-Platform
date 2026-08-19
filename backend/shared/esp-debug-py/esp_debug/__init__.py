"""esp_debug — 全语言统一深度诊断面板(Step2 · Python 侧)。

Spring Boot Actuator 的 Python(Litestar)等价映射(admin 前端按同语义展示):

    env        → /debug/env        环境变量(脱敏:KEY/PASSWORD/SECRET/TOKEN 之键值打码)
    loggers    → /debug/loglevel   日志级别动态查询/调整(等价 loggers)
    threaddump → NA                Python 无 JVM 线程;asyncio 任务无标准 dump(标 NA,可扩展 faulthandler)
    heapdump   → NA                Python 无 hprof;可用 tracemalloc 等价(本版 DEFERRED)
    beans      → /debug/beans      route/handler 清单(等价 Spring beans)
    mappings   → /debug/mappings   路由表(等价 Spring mappings)

鉴权:本模块仅注册 handler;敏感端点鉴权由调用方在 Litestar Middleware 层包 admin-role JWT
(与 admin-go requireAdmin 一致)。ESP 约定 debug 端点仅经 admin-go 代理可达。
"""
from __future__ import annotations

import os
import threading
from typing import Any

from litestar import Litestar, Request, get, post
from litestar.exceptions import HTTPException

# 需脱敏的环境变量名子串(大小写不敏感包含即脱敏)。
_SENSITIVE = ("KEY", "PASSWORD", "PASSWD", "SECRET", "TOKEN", "CREDENTIAL", "PRIVATE")

# 运行期日志级别(Python logging 无全局级别,用模块级变量模拟动态调级)。
_log_lock = threading.Lock()
_log_level = "INFO"


def _mask(k: str, v: str) -> str:
    ku = k.upper()
    for s in _SENSITIVE:
        if s in ku:
            if len(v) <= 2:
                return "*" * len(v)
            return v[0] + "*" * (len(v) - 2) + v[-1]
    return v


@get("/debug/env", sync_to_thread=False)
def debug_env() -> dict[str, Any]:
    """环境变量(脱敏)。等价 actuator/env。"""
    env: dict[str, str] = {}
    for kv in os.environ.items():
        env[kv[0]] = _mask(kv[0], kv[1])
    return {
        "activeProfiles": [f"python/{os.name}", _py_ver()],
        "propertySources": [{"name": "systemEnvironment", "properties": env}],
        "_sanitized": True,
        "_note": "env values masked for keys matching: " + ", ".join(_SENSITIVE),
    }


@get("/debug/loglevel", sync_to_thread=False)
def debug_loglevel_get() -> dict[str, Any]:
    """日志级别查询。等价 actuator/loggers GET。"""
    return {
        "configuredLevel": _log_level,
        "effectiveLevel": _log_level,
        "loggers": {
            "root": _log_level,
            "http": _log_level,
            "db": _log_level,
            "redis": _log_level,
        },
    }


@post("/debug/loglevel", sync_to_thread=False)
def debug_loglevel_set(data: dict[str, str]) -> dict[str, str]:
    """日志级别动态调。等价 actuator/loggers POST {configuredLevel: LEVEL}。

    ASV:Litestar @post 的 body 参数解析:Litestar 自动将 JSON body 注入 dict 类型参数,
    无需 Body() 包装(与 writing-py @post(..., data: dict) 一致,已核实)。
    """
    global _log_level
    lvl = (data.get("configuredLevel") or data.get("level") or "").upper()
    if lvl in ("DEBUG", "INFO", "WARN", "ERROR", "TRACE"):
        with _log_lock:
            _log_level = lvl
        return {"configuredLevel": lvl, "effectiveLevel": lvl, "status": "updated"}
    raise HTTPException(status_code=400, detail="invalid level, one of DEBUG/INFO/WARN/ERROR/TRACE")


@get("/debug/beans", sync_to_thread=False)
def debug_beans(request: Request) -> dict[str, Any]:
    """route/handler 清单。等价 actuator/beans。

    ASV:Request 注入 + request.app(Litestar App)已核实:
    litestar/connection/base.py .app → scope["litestar_app"] 返回 Litestar 实例;
    writing-py grade(data, request: Request) 已用此模式。
    """
    routes = _collect_routes(request.app)
    beans = [{"bean": "litestarHandler:" + r, "route": r, "type": "Controller", "scope": "singleton"} for r in routes]
    return {
        "context": "main",
        "beans": beans,
        "_note": "Litestar 无 Spring Bean 容器;此处等价为已注册 HTTP 路由(handler 清单)",
    }


@get("/debug/mappings", sync_to_thread=False)
def debug_mappings(request: Request) -> dict[str, Any]:
    """路由表。等价 actuator/mappings。"""
    routes = _collect_routes(request.app)
    mappings = [{"pattern": r, "method": "ANY", "handler": "litestarHandler"} for r in routes]
    return {
        "contexts": {
            "application": {
                "mappings": mappings,
                "_note": "Litestar 路由表(等价 Spring RequestMappingHandlerMapping)",
            }
        }
    }


def _collect_routes(app: Litestar) -> list[str]:
    """从 Litestar app 收集已注册路由路径。ASV:Litestar App.routes: list[HTTPRoute|...], Route.path: str。"""
    out: list[str] = []
    try:
        for route in app.routes:
            p = getattr(route, "path", None)
            if p:
                out.append(p)
    except Exception:
        pass
    return sorted(set(out))


def _py_ver() -> str:
    import sys
    return f"python {sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}"


def debug_controllers() -> list:
    """返回所有 debug route controllers,供调用方加入 Litestar(route_handlers=[...])。"""
    return [debug_env, debug_loglevel_get, debug_loglevel_set, debug_beans, debug_mappings]
