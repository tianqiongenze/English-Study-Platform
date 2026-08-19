# asr-adapter-go —— ASR 适配微服务(纯 Go,无状态转发层)
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/esp-common-go ./esp-common-go
COPY services/asr-adapter-go/go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
RUN sed -i 's|replace esp/common-go => ../../shared/esp-common-go|replace esp/common-go => ./esp-common-go|' go.mod
RUN go mod download
COPY services/asr-adapter-go/src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/asr-adapter-go ./src

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/asr-adapter-go /app/asr-adapter-go
EXPOSE 8097
ENV PORT=8097
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=asr-adapter-go
USER app
ENTRYPOINT ["/app/asr-adapter-go"]
