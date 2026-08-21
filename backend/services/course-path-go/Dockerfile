# 编译阶段
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
COPY shared/esp-common-go ./esp-common-go
COPY services/course-path-go/go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
# GOTOOLCHAIN=auto:go.mod 声明 go 1.26.0,基础镜像 esp/golang-builder:1.25 内 go1.25.13 不够;
# auto 经 GOPROXY(goproxy.cn 可达)自动下载 1.26 toolchain(mirror 挂不影响)。
ENV GOTOOLCHAIN=auto
RUN sed -i 's|replace esp/common-go => ../../shared/esp-common-go|replace esp/common-go => ./esp-common-go|' go.mod
RUN go mod download
COPY services/course-path-go/src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/course-path-go ./src

# 运行阶段
FROM esp/alpine-runtime:3.22
RUN apk add --no-cache ca-certificates
# 安全:非 root 运行(ADR-security-scanning §4)
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/course-path-go /app/course-path-go
EXPOSE 8093
ENV PORT=8093
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_EXPORTER_OTLP_PROTOCOL=grpc
ENV OTEL_SERVICE_NAME=course-path-go
USER app
ENTRYPOINT ["/app/course-path-go"]
