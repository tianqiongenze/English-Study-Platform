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

# build context = backend/(含 shared/esp-common-rs + services/media-learn-rs)
# Cargo.toml 的 esp-common-rs = { path = "../../shared/esp-common-rs" } → 从 /workspace/media-learn-rs 上溯两级到 / + shared/esp-common-rs
WORKDIR /workspace/media-learn-rs
COPY shared/esp-common-rs /shared/esp-common-rs
COPY services/media-learn-rs/Cargo.toml Cargo.lock* ./
RUN mkdir -p src && echo "fn main() {}" > src/main.rs
# 国内镜像加速首次依赖拉取(哑编译预热依赖层)
RUN cargo build --release || true
COPY services/media-learn-rs/src ./src
# 坑⑱:dummy main 哑编译后 cargo fingerprint 缓存命中,真实 src COPY 后 cargo 认为无变化不重编(0.29s Finished)。
# 用 touch src/main.rs 强制 fingerprint 失效,确保编译真实代码。
RUN touch src/main.rs && cargo build --release

# 运行阶段:debian:bookworm-slim(nonroot)
# 原 gcr.io/distroless/cc-debian12:nonroot 国内拉取超时(gcr.io 被墙),
# 降级为 debian:bookworm-slim + 非 root 用户,功能等价。
# ADR-security-scanning §4:对标 gateway-go/course-path-go 0 漏洞基线(运行时基线差异已记录)
FROM docker.1ms.run/library/debian:bookworm-slim
# 换国内 debian 镜像源(RD VM 内 deb.debian.org 网络抖动致 apt-get timeout exit 100,模式⑬)
RUN sed -i 's|deb.debian.org|mirrors.ustc.edu.cn|g; s|security.debian.org|mirrors.ustc.edu.cn|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null \
    || sed -i 's|deb.debian.org|mirrors.ustc.edu.cn|g; s|security.debian.org|mirrors.ustc.edu.cn|g' /etc/apt/sources.list 2>/dev/null \
    || true
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libgcc-s1 curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 65532 -r -s /usr/sbin/nologin nonroot
WORKDIR /app
COPY --from=builder /workspace/media-learn-rs/target/release/media-learn-rs /app/media-learn-rs
COPY scripts/nacos-register.sh /app/nacos-register.sh
RUN chmod +x /app/nacos-register.sh
EXPOSE 8095
ENV PORT=8095
ENV SERVICE_NAME=media-learn-rs
ENV HEALTH_URL=/actuator/health/liveness
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=media-learn-rs
USER nonroot
ENTRYPOINT ["/bin/sh", "/app/nacos-register.sh", "/app/media-learn-rs"]
