# pronunciation-rs(S3, P0)—— 音素级发音评分(GOP)+ 音标/重音/语调纠错
# 多阶段构建:rust builder + debian:bookworm-slim runtime
# ADR: docs/design/adr-new-services.md / PRD §三 / schema esp_pronunciation
# NOTE: docker.io 直拉 i/o timeout(Rancher Desktop VM 网络),改 1ms 镜像源
FROM docker.1ms.run/library/rust:1.88-bookworm AS builder
WORKDIR /app

# 国内镜像加速 crates 拉取
RUN mkdir -p /usr/local/cargo && cat >> /usr/local/cargo/config.toml <<'EOF'
[source.crates-io]
replace-with = "ustc"
[source.ustc]
registry = "sparse+https://mirrors.ustc.edu.cn/crates.io-index/"
[net]
git-fetch-with-cli = true
EOF

COPY Cargo.toml Cargo.lock* ./
COPY src ./src
RUN cargo build --release

# 运行阶段:debian:bookworm-slim(nonroot)
FROM docker.1ms.run/library/debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libgcc-s1 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 65532 -r -s /usr/sbin/nologin nonroot
WORKDIR /app
COPY --from=builder /app/target/release/pronunciation-rs /app/pronunciation-rs
EXPOSE 8088
ENV PORT=8088
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=pronunciation-rs
USER nonroot
ENTRYPOINT ["/app/pronunciation-rs"]
