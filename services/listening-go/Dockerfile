# listening-go(S4, P1)—— 精听/泛听/听写/语速自适应 + 音频转写流式
# 多阶段构建:go builder + alpine runtime(对标 course-path-go 0 漏洞基线)
# ADR: docs/design/adr-new-services.md / PRD §四 / schema esp_listening
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/listening-go ./src

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/listening-go /app/listening-go
EXPOSE 8089
ENV PORT=8089
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=listening-go
USER app
ENTRYPOINT ["/app/listening-go"]
