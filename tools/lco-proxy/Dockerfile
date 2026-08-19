# lco-proxy —— LLM 多级缓存代理(LCO 铁律落地)
# 多阶段构建:builder(golang 工具链) → runtime(alpine)
# 注:LCO 代理设计为宿主进程(给本机工具接入),Dockerfile 用于可移植部署/多节点迁移
FROM esp/golang-builder:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/lco-proxy ./cmd/lco-proxy

FROM esp/alpine-runtime:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/lco-proxy /app/lco-proxy
EXPOSE 15720
ENV LCO_LISTEN=0.0.0.0:15720
USER app
ENTRYPOINT ["/app/lco-proxy"]
