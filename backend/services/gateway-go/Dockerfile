# 编译阶段
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
COPY shared/esp-common-go ./esp-common-go
COPY services/conversation-go ./conversation-go
COPY services/gateway-go/go.mod services/gateway-go/go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
# go.mod 声明 go 1.26.0(项目级统一),基础镜像 go1.25.13 不够;
# GOTOOLCHAIN=auto 让 go 自动经 GOPROXY 下载 1.26 toolchain(goproxy.cn 可达,mirror 挂不影响)。
ENV GOTOOLCHAIN=auto
RUN sed -i 's|replace esp/common-go => ../../shared/esp-common-go|replace esp/common-go => ./esp-common-go|' go.mod \
 && sed -i 's|replace esp/conversation-go => ../conversation-go|replace esp/conversation-go => ./conversation-go|' go.mod
RUN go mod download
COPY services/gateway-go/src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/gateway-go ./src

# 运行阶段
FROM esp/alpine-runtime:3.22
RUN apk add --no-cache ca-certificates
# 安全:非 root 运行(ADR-security-scanning §4 Semgrep run-as-non-root 修复)
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/gateway-go /app/gateway-go
EXPOSE 8080
ENV PORT=8080
# RWP(铁律①):显式 ENV HOME 锚定可写目录,避免 HOME=/ → SDK 写 /.xxx 失败(BVV: docker run echo $HOME 非空非根)。
ENV HOME=/home/app
# OpenTelemetry -> Collector in esp-observability ns
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_EXPORTER_OTLP_PROTOCOL=grpc
ENV OTEL_SERVICE_NAME=gateway-go
USER app
ENTRYPOINT ["/app/gateway-go"]
