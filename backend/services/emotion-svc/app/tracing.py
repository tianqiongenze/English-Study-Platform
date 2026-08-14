"""OpenTelemetry tracing 初始化(ADR-observability).

在 Litestar app 创建前调用 setup_tracing():
- OTLP gRPC exporter → OTel Collector (esp-observability ns)
- BatchSpanProcessor
- Litestar 自动埋点
"""
from __future__ import annotations

import os
import logging

from opentelemetry import trace
from opentelemetry.sdk.resources import Resource, SERVICE_NAME, SERVICE_NAMESPACE, DEPLOYMENT_ENVIRONMENT
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
# Litestar 2.x 内置 OTel 插件(opentelemetry-instrumentation-litestar 在 PyPI 不存在真实发布,
# 改用 litestar.contrib.opentelemetry.OpenTelemetryPlugin,等价自动埋点)。
from litestar.contrib.opentelemetry import OpenTelemetryPlugin

_logger = logging.getLogger(__name__)
_initialized = False

DEFAULT_ENDPOINT = "http://otel-collector.esp-observability.svc.cluster.local:4317"
DEFAULT_SERVICE_NAME = "emotion-svc"


def setup_tracing(service_name: str = DEFAULT_SERVICE_NAME) -> None:
    """初始化全局 tracer。幂等。在创建 Litestar app 之前调用。"""
    global _initialized
    if _initialized:
        return

    svc = os.environ.get("OTEL_SERVICE_NAME", service_name)
    endpoint = os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT", DEFAULT_ENDPOINT)
    extra_attrs = os.environ.get("OTEL_RESOURCE_ATTRIBUTES", "")

    resource = Resource.create(
        {
            SERVICE_NAME: svc,
            SERVICE_NAMESPACE: "esp-prod",
            DEPLOYMENT_ENVIRONMENT: "esp-prod",
        }
    )
    if extra_attrs:
        for kv in extra_attrs.split(","):
            if "=" in kv:
                k, v = kv.split("=", 1)
                resource = resource.merge(Resource.create({k.strip(): v.strip()}))

    provider = TracerProvider(resource=resource)
    exporter = OTLPSpanExporter(endpoint=endpoint, insecure=True)
    provider.add_span_processor(BatchSpanProcessor(exporter))
    trace.set_tracer_provider(provider)

    _logger.info("OTel tracing initialized: service=%s endpoint=%s", svc, endpoint)
    _initialized = True


def instrument_app(app) -> None:
    """对 Litestar app 应用自动埋点。在 app 创建后调用。

    Litestar 2.x 的 OTel 埋点由 OpenTelemetryPlugin 在 app 初始化时注入中间件。
    本函数为幂等占位:推荐在构建 Litestar 时通过 plugins=[OpenTelemetryPlugin()] 注册。
    """
    try:
        registered = app.plugins.get(OpenTelemetryPlugin) if hasattr(app.plugins, "get") else None
    except Exception:
        registered = None
    if registered is None:
        _logger.warning(
            "OpenTelemetryPlugin not registered on Litestar app; "
            "add plugins=[OpenTelemetryPlugin()] in Litestar(...) for OTel auto-instrumentation."
        )


__all__ = ["setup_tracing", "instrument_app"]
