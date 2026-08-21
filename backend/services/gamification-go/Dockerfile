# gamification-go(S7, P1)—— 学习目标/连胜/排行榜/成就/积分商城(事件驱动 Pulsar)
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
COPY shared/esp-common-go ./esp-common-go
COPY services/gamification-go/go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
# GOTOOLCHAIN=auto:go.mod 声明 go 1.26.0,基础镜像 esp/golang-builder:1.25 内 go1.25.13 不够;
# auto 经 GOPROXY(goproxy.cn 可达)自动下载 1.26 toolchain(mirror 挂不影响)。
ENV GOTOOLCHAIN=auto
RUN sed -i 's|replace esp/common-go => ../../shared/esp-common-go|replace esp/common-go => ./esp-common-go|' go.mod
RUN go mod download
COPY services/gamification-go/src ./src
# -tags pulsar_eda 启用 rankings_projector.go 真实 Pulsar CQRS 投影器(订阅 learning.event.report,
# Redis SETNX 幂等投影 ZSET);不加 tag 则 rankings_projector_stub.go(!pulsar_eda)参与编译,
# startRankingsProjector no-op——P1-D21 gap3 修复:配而未接 + stale-image-blindspot 双违规根治。
# go.mod 已声明 apache/pulsar-client-go v0.21.0 且第11行 go mod download 已拉取,加 tag 无额外网络依赖。
RUN go mod tidy && CGO_ENABLED=0 go build -tags pulsar_eda -o /out/gamification-go ./src

FROM esp/alpine-runtime:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/gamification-go /app/gamification-go
EXPOSE 8092
ENV PORT=8092
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=gamification-go
# RWP铁律:readOnlyRootFilesystem容器需显式ENV HOME锚定可写目录;nacos-sdk-go默认写./log=/app/log,
# OTel/gRPC SDK 可能写 $HOME/.cache;ENV HOME 防 HOME=/ → SDK 写根目录失败(对标 reading-go)。
ENV HOME=/home/app
USER app
ENTRYPOINT ["/app/gamification-go"]
