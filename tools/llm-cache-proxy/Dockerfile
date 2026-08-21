# llm-cache-proxy —— LLM 多级缓存代理(LCO 铁律落地)
# 多阶段构建:builder(golang 工具链) → runtime(alpine)
# 注:LCO 代理设计为宿主进程(给本机工具接入),Dockerfile 用于可移植部署/多节点迁移
#
# ── SCS 供应链铁律(2026-08-20)─────────────────────────────────────────
# ① FROM digest pin(派生基础镜像不可变引用,SVD/SCS 第⑥条):
#    本地派生镜像 esp/golang-builder / esp/alpine-runtime 的 RepoDigest
#    (docker inspect 取得,本地构建镜像 digest==config Id)。
# ② 运行时安全(RWP 铁律):本镜像上 K8s 时,deploy yaml 必须设:
#      readOnlyRootFilesystem: true
#      runAsNonRoot: true / runAsUser: 10001(对齐 USER app)
#      allowPrivilegeEscalation: false
#      capabilities: { drop: [ALL] }
#      securityOpt: [no-new-privileges]
#    并挂 emptyDir 到 SDK 写路径:/.dubbo /logs /tmp(llm-cache-proxy 写 cache/日志用 /tmp)。
#    注:readOnlyRootFilesystem/cap_drop 是运行时参数,Dockerfile 不能设,故此处注释 +
#      在 deploy/k8s yaml 声明(llm-cache-proxy 当前无 K8s yaml,DEFERRED,见 report)。
# ────────────────────────────────────────────────────────────────────
FROM esp/golang-builder:1.25@sha256:de83b2e2491a0ef08bbcec6b76b3ef3cadba46d00d87a28a1307ed182d192ed9 AS builder
WORKDIR /app
COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
# go.mod 声明 go 1.26.5 而本派生 builder 镜像内置 1.25.13(GOTOOLCHAIN=local 不会自动取工具链);
# 设 auto 让 go 按 go.mod 指定版本自动拉取匹配 toolchain(官方行为,源 goproxy.cn)。
ENV GOTOOLCHAIN=auto
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/llm-cache-proxy ./cmd/llm-cache-proxy

FROM esp/alpine-runtime:3.22@sha256:5f503088ad18960d0e15369a72ed9aa724401bd14d655aff39696d3535cf6360
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/llm-cache-proxy /app/llm-cache-proxy
EXPOSE 15720
ENV LCO_LISTEN=0.0.0.0:15720
USER app
# HEALTHCHECK:探 /healthz(wget 确认存在于 esp/alpine-runtime:3.22)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:15720/healthz || exit 1
ENTRYPOINT ["/app/llm-cache-proxy"]
