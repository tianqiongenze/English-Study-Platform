# content-go —— 统一内容库(功能拓展:四六级/托福/雅思/教材共享内容)
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
COPY shared/esp-common-go ./esp-common-go
COPY services/content-go/go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
# GOTOOLCHAIN=auto:go.mod 声明 go 1.26.0,基础镜像 esp/golang-builder:1.25 内 go1.25.13 不够;
# auto 经 GOPROXY(goproxy.cn 可达)自动下载 1.26 toolchain(mirror 挂不影响)。
ENV GOTOOLCHAIN=auto
RUN sed -i 's|replace esp/common-go => ../../shared/esp-common-go|replace esp/common-go => ./esp-common-go|' go.mod
RUN go mod download
COPY services/content-go/src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/content-go ./src

FROM esp/alpine-runtime:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/content-go /app/content-go
EXPOSE 8096
ENV PORT=8096
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=content-go
# RWP铁律:readOnlyRootFilesystem容器需显式ENV HOME锚定可写目录;nacos-sdk-go默认写./log=/app/log,
# OTel/gRPC SDK 可能写 $HOME/.cache;ENV HOME 防 HOME=/ → SDK 写根目录失败(对标 reading-go)。
ENV HOME=/home/app
USER app
ENTRYPOINT ["/app/content-go"]
