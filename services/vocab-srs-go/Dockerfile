# 编译阶段
FROM golang:1.25-alpine AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod ./
RUN go mod download
COPY src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/vocab-srs-go ./src

# 运行阶段:distroless base-debian12(nonroot)
# - Go CGO_ENABLED=0 静态二进制,但服务发 HTTPS(otel/外部 API)需 ca-certificates
#   → 用 base-debian12(含 /etc/ssl/certs + glibc)而非 static-debian12(无 ca-certs)
# - nonroot 变体内置 user 65532:nonroot,无需 addgroup/adduser,无 shell 无 apk 无 apt
# 镜像拉取:国内拉 gcr.io 可能失败,可替换为
#   registry.aliyuncs.com/google_containers/distroless 或建本地 mirror
# ADR-security-scanning §4:对标 gateway-go/course-path-go 0 漏洞基线
FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/vocab-srs-go /app/vocab-srs-go
EXPOSE 8087
ENV PORT=8087
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_EXPORTER_OTLP_PROTOCOL=grpc
ENV OTEL_SERVICE_NAME=vocab-srs-go
USER nonroot
ENTRYPOINT ["/app/vocab-srs-go"]
