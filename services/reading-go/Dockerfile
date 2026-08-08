# reading-go(S6, P1)—— 分级阅读 + 长难句解析 + 题目自动生成
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/reading-go ./src

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/reading-go /app/reading-go
EXPOSE 8091
ENV PORT=8091
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=reading-go
USER app
ENTRYPOINT ["/app/reading-go"]
