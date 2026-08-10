# media-learn-rs(N11, P1)—— 真实语料学习
# 多阶段构建:rust builder + debian:bookworm-slim runtime
FROM docker.1ms.run/library/rust:1.88-bookworm AS builder

# 国内镜像加速 crates 拉取
RUN mkdir -p /usr/local/cargo && cat >> /usr/local/cargo/config.toml <<'EOF'
[source.crates-io]
replace-with = "ustc"
[source.ustc]
registry = "sparse+https://mirrors.ustc.edu.cn/crates.io-index/"
[net]
git-fetch-with-cli = true
EOF

# build context = services/(含 esp-common-rs + media-learn-rs)
# Cargo.toml 的 esp-common-rs = { path = "../esp-common-rs" } → 需 /workspace/esp-common-rs
WORKDIR /workspace/media-learn-rs
COPY esp-common-rs /workspace/esp-common-rs
COPY media-learn-rs/Cargo.toml Cargo.lock* ./
RUN mkdir -p src && echo "fn main() {}" > src/main.rs
# 国内镜像加速首次依赖拉取(哑编译预热依赖层)
RUN cargo build --release || true
COPY media-learn-rs/src ./src
RUN cargo build --release

# 运行阶段:debian:bookworm-slim(nonroot)
# 原 gcr.io/distroless/cc-debian12:nonroot 国内拉取超时(gcr.io 被墙),
# 降级为 debian:bookworm-slim + 非 root 用户,功能等价。
# ADR-security-scanning §4:对标 gateway-go/course-path-go 0 漏洞基线(运行时基线差异已记录)
FROM docker.1ms.run/library/debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libgcc-s1 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 65532 -r -s /usr/sbin/nologin nonroot
WORKDIR /app
COPY --from=builder /workspace/media-learn-rs/target/release/media-learn-rs /app/media-learn-rs
EXPOSE 8095
ENV PORT=8095
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=media-learn-rs
USER nonroot
ENTRYPOINT ["/app/media-learn-rs"]
