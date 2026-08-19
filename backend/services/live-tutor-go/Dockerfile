# 编译阶段
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
COPY shared/esp-common-go ./esp-common-go
COPY services/live-tutor-go/go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
RUN sed -i 's|replace esp/common-go => ../../shared/esp-common-go|replace esp/common-go => ./esp-common-go|' go.mod
RUN go mod download
COPY services/live-tutor-go/src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/live-tutor-go ./src

# 运行阶段:distroless base-debian12(nonroot)
# - Go CGO_ENABLED=0 静态二进制,但服务发 HTTPS(otel/外部 API)需 ca-certificates
#   → 用 base-debian12(含 /etc/ssl/certs + glibc)而非 static-debian12(无 ca-certs)
# - nonroot 变体内置 user 65532:nonroot,无需 addgroup/adduser,无 shell 无 apk 无 apt
# 镜像拉取:国内拉 gcr.io 常超时,优先用本地 mirror(registry.aliyuncs.com/google_containers/distroless)。
# ADR-security-scanning §4:对标 gateway-go/course-path-go 0 漏洞基线
FROM esp/alpine-runtime:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/live-tutor-go /app/live-tutor-go
EXPOSE 8094
ENV PORT=8094
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_EXPORTER_OTLP_PROTOCOL=grpc
ENV OTEL_SERVICE_NAME=live-tutor-go
USER app
ENTRYPOINT ["/app/live-tutor-go"]
