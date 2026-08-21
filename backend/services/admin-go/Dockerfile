# esp-admin-go — 统一管理员监测门户后端 + 中间件 BFF 代理(2026-08-19)。
# 零外部依赖(仅 stdlib + esp/common-go),构建无需网络。
#
# 构建上下文 = backend/services(含本服务 admin-go/ + 共享 shared/esp-common-go)。
# go.mod replace esp/common-go => ../../shared/esp-common-go 在此上下文可解析。
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
# 构建上下文 = backend/(含 services/admin-go/ + shared/esp-common-go/)。
# go.mod replace esp/common-go => ../../shared/esp-common-go(相对 admin-go/ 目录);
# admin-go/ 在 /app/admin-go/,../../shared 解析到 /shared/esp-common-go。
COPY services/admin-go/go.mod ./admin-go/go.mod
COPY services/admin-go/go.sum ./admin-go/go.sum
COPY services/admin-go/src ./admin-go/src
# admin-go/ 在 /app/admin-go/,../../shared 解析到 /shared/esp-common-go(非 /app/shared)。
COPY shared/esp-common-go /shared/esp-common-go
ENV GOPROXY=https://goproxy.cn,direct
# go.mod 声明 go 1.26.0(项目级统一),基础镜像 go1.25.13 不够;
# GOTOOLCHAIN=auto 让 go 自动经 GOPROXY 下载 1.26 toolchain(goproxy.cn 可达,mirror 挂不影响)。
ENV GOTOOLCHAIN=auto
RUN CGO_ENABLED=0 go build -C admin-go -o /out/admin-go ./src

FROM esp/alpine-runtime:3.22
RUN apk add --no-cache ca-certificates
# 安全:非 root 运行(与 gateway-go 一致)
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/admin-go /app/admin-go
EXPOSE 8099
ENV PORT=8099
# RWP(铁律①):显式 ENV HOME 锚定可写目录,避免 HOME=/ → SDK 写 /.xxx 失败(BVV: docker run echo $HOME 非空非根)。
ENV HOME=/home/app
USER app
ENTRYPOINT ["/app/admin-go"]
