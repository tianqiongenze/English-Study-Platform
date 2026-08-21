# reading-go(S6, P1)—— 分级阅读 + 长难句解析 + 题目自动生成
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
COPY shared/esp-common-go ./esp-common-go
COPY services/reading-go/go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
# GOTOOLCHAIN=auto:go.mod 声明 go 1.26.0,基础镜像 esp/golang-builder:1.25 内 go1.25.13 不够;
# auto 经 GOPROXY(goproxy.cn 可达)自动下载 1.26 toolchain(mirror 挂不影响)。
ENV GOTOOLCHAIN=auto
RUN sed -i 's|replace esp/common-go => ../../shared/esp-common-go|replace esp/common-go => ./esp-common-go|' go.mod
RUN go mod download
COPY services/reading-go/src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/reading-go ./src

FROM esp/alpine-runtime:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app -h /home/app app
WORKDIR /app
COPY --from=builder /out/reading-go /app/reading-go
EXPOSE 8091
# RWP铁律(必须修):readOnlyRootFilesystem容器需显式ENV HOME锚定可写目录,nacos-sdk-go默认写./log=/app/log,
# OTel/gRPC SDK 可能写 $HOME/.cache;ENV HOME 防 HOME=/ → SDK 写根目录失败。
ENV HOME=/home/app
ENV PORT=8091
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=reading-go
USER app
ENTRYPOINT ["/app/reading-go"]
