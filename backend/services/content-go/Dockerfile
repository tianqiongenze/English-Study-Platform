# content-go —— 统一内容库(功能拓展:四六级/托福/雅思/教材共享内容)
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
COPY shared/esp-common-go ./esp-common-go
COPY services/content-go/go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
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
USER app
ENTRYPOINT ["/app/content-go"]
