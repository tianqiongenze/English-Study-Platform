# writing-py 依赖冲突修复记录 (2026-08-03)

## 背景
P1-3 langgraph 0.2→1.x 升级受阻:pyproject.toml 中 opentelemetry 相关 `==1.30.0` 精确锁版与 langgraph>=1.0 / litestar 间接依赖 ResolutionImpossible,导致 writing-py 线上长期停留在 1.1.0 re-tag 冒充 1.3.0。

## 修复前(冲突锁版)
```toml
"opentelemetry-api==1.30.0",
"opentelemetry-sdk==1.30.0",
"opentelemetry-exporter-otlp==1.30.0",
"opentelemetry-instrumentation-asgi==0.49b0",
```

## 修复后(松绑为兼容范围)
```toml
"opentelemetry-api>=1.30.0,<2.0",
"opentelemetry-sdk>=1.30.0,<2.0",
"opentelemetry-exporter-otlp>=1.30.0,<2.0",
"opentelemetry-instrumentation-asgi>=0.49b0,<1.0",
```
保留 langgraph>=1.0 / langgraph-checkpoint>=1.0(P1-3 核心,未降级)。litestar>=2.24.0 本已是范围约束,无需改动。业务代码零改动。

## 解析验证(dry-run,pip,python:3.13-slim 容器)
- 命令:`pip install --dry-run ./services/writing-py`(预装 esp-cache-py / esp-db-py 后)
- 结果:**RC=0,ResolutionImpossible 消失**

## 最终依赖树关键项
| 依赖 | 锁定版本 |
|------|---------|
| langgraph | 1.2.10 |
| langgraph-checkpoint | 4.1.1 |
| litestar | 2.24.0 |
| opentelemetry-api | 1.44.0 |
| opentelemetry-sdk | 1.44.0 |
| opentelemetry-exporter-otlp | 1.44.0 |
| opentelemetry-instrumentation-asgi | 0.65b0 |
| granian | 2.7.9 |
| anthropic | 0.120.2 |
| msgspec | 0.21.1 |

## 结论
OTel 松绑到 `>=1.30.0,<2.0` 后,resolver 自动选定 OTel 1.44.0,与 langgraph 1.2.10 / litestar 2.24.0 共存,冲突根除。
