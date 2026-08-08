# gamification-go(S7, P1)—— 学习目标/连胜/排行榜/成就/积分商城(事件驱动 Pulsar)
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/gamification-go ./src

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/gamification-go /app/gamification-go
EXPOSE 8092
ENV PORT=8092
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=gamification-go
USER app
ENTRYPOINT ["/app/gamification-go"]
