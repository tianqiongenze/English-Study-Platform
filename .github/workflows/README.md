# CI/CD Pipeline(ADR-工程化)

> 大厂标准质量门禁:PR 跑 unit+lint+build(fast),main 跑 +integration+image+deploy(slow)。
> 本项目当前非 git 仓库(CLAUDE.md),pipeline 本地可用(`act`)、迁入 git 零改动。

## 设计

`.github/workflows/ci.yml`,5 个 job 矩阵:

| Job | 触发 | 内容 | 门禁 |
|---|---|---|---|
| `java-commons` | 全部 | esp-disruptor/esp-rw-java/esp-cache 单测 + jacoco | **行覆盖 ≥80%** |
| `java-services` | 全部 | bff/learning/teacher 单测 + jacoco | **bff 安全核心(JwtService/AuthFilter/AuthController/RateLimitFilter)100%** |
| `go-gateway` | 全部 | go vet + `go test -race -cover` | race + coverage |
| `python-services` | 全部 | writing/emotion ruff + pytest | maxfail=5 |
| `build-images` | main/手动 | 6 服务 Docker buildx(load,不推) | 构建成功 |
| `contract` | 全部 | Pact 占位(P1) | — |

## 门禁规则
- 公共包覆盖率 <80% → CI 红(公共库被多服务依赖,bug 放大)。
- bff 安全核心 <100% → CI 红(安全代码零容忍)。
- `build-images` 依赖前 4 个 job 全绿才跑(质量门禁前置)。

## 启用镜像推送(迁入 git 后)
ci.yml 末尾注释段:登录 ghcr.io + 推 `ghcr.io/<org>/esp-<svc>:${{ github.sha }}`。

## 本地验证
```bash
# 非 git 仓库期,手动验证各步:
cd services/esp-disruptor && mvn -q test jacoco:report   # 公共包
cd services/bff-java && mvn -q test jacoco:report          # bff(安全核心)
cd services/gateway-go && go test -race ./...
cd services/writing-py && pytest -q
# 或用 act 跑完整 pipeline(需 Docker):
act -j java-commons
```

## 当前实测状态(2026-08-02)
- ✅ bff-java:21 测试全绿,安全核心覆盖率 100%(JwtService/AuthFilter/AuthController)。
- ⏳ 公共包单测:正在补(esp-disruptor/esp-rw-java/esp-cache),目标 ≥80%。
- ⏳ 契约测试(Pact):P1,CI 占位。
