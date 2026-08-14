import sys, types
"""Test-only stubs for otel packages unavailable on local PyPI index."""
# stub opentelemetry.instrumentation.litestar (PyPI placeholder only)
m = types.ModuleType("opentelemetry.instrumentation.litestar")
class _Instr:
    @staticmethod
    def instrument_app(app): pass
m.LitestarInstrumentor = _Instr
sys.modules.setdefault("opentelemetry.instrumentation.litestar", m)
