# listening-go(S4, P1)—— 精听/泛听/听写/语速自适应 + 音频转写流式
# 多阶段构建:go builder + alpine runtime(对标 course-path-go 0 漏洞基线)
# ADR: docs/design/adr-new-services.md / PRD §四 / schema esp_listening
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
# esp-common-go 通过 replace 指令引用(../esp-common-go),build context 需含同级公共包目录。
# 此处 COPY esp-common-go 到 /app/../esp-common-go 不可行,故改放 /app/esp-common-go 并用 build arg 覆盖 replace。
COPY shared/esp-common-go ./esp-common-go
COPY services/listening-go/go.mod ./
ENV GOPROXY=https://goproxy.cn,direct
# GOTOOLCHAIN=auto:go.mod 声明 go 1.26.0,基础镜像 esp/golang-builder:1.25 内 go1.25.13 不够;
# auto 经 GOPROXY(goproxy.cn 可达)自动下载 1.26 toolchain(mirror 挂不影响)。
ENV GOTOOLCHAIN=auto
# 本地 replace 指向 ../esp-common-go(宿主 IDE 用),容器内重写为 ./esp-common-go 以匹配 COPY 位置。
RUN sed -i 's|replace esp/common-go => ../../shared/esp-common-go|replace esp/common-go => ./esp-common-go|' go.mod
RUN go mod download
COPY services/listening-go/src ./src
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/listening-go ./src

FROM esp/alpine-runtime:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/listening-go /app/listening-go
EXPOSE 8089
ENV PORT=8089
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=listening-go
# RWP铁律:readOnlyRootFilesystem容器需显式ENV HOME锚定可写目录;nacos-sdk-go默认写./log=/app/log,
# OTel/gRPC SDK 可能写 $HOME/.cache;ENV HOME 防 HOME=/ → SDK 写根目录失败(对标 reading-go)。
ENV HOME=/home/app
USER app
ENTRYPOINT ["/app/listening-go"]
