# 编译阶段
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/esp-common-go ./esp-common-go
COPY services/conversation-go ./conversation-go
COPY services/gateway-go/go.mod services/gateway-go/go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN sed -i 's|replace esp/common-go => ../../shared/esp-common-go|replace esp/common-go => ./esp-common-go|' go.mod \
 && sed -i 's|replace esp/conversation-go => ../conversation-go|replace esp/conversation-go => ./conversation-go|' go.mod
RUN go mod download
COPY services/gateway-go/src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/gateway-go ./src

# 运行阶段
FROM alpine:3.22
RUN apk add --no-cache ca-certificates
# 安全:非 root 运行(ADR-security-scanning §4 Semgrep run-as-non-root 修复)
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/gateway-go /app/gateway-go
EXPOSE 8080
ENV PORT=8080
# OpenTelemetry -> Collector in esp-observability ns
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_EXPORTER_OTLP_PROTOCOL=grpc
ENV OTEL_SERVICE_NAME=gateway-go
USER app
ENTRYPOINT ["/app/gateway-go"]
