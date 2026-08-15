# assessment-rs(S5, P1)—— CEFR 定级 + 雅思/托福模考 + 自适应测试(IRT)+ 诊断报告
# 多阶段构建:rust builder + debian:bookworm-slim runtime
# ADR: docs/design/adr-new-services.md / PRD §五 / schema esp_assessment
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

# build context = backend/(含 shared/esp-common-rs + services/assessment-rs)
# Cargo.toml 的 esp-common-rs = { path = "../../shared/esp-common-rs" } → 需 /workspace/esp-common-rs
WORKDIR /workspace/assessment-rs
COPY shared/esp-common-rs /shared/esp-common-rs
COPY services/assessment-rs/Cargo.toml Cargo.lock* ./
COPY services/assessment-rs/src ./src
RUN cargo build --release

# 运行阶段:debian:bookworm-slim(nonroot)
FROM docker.1ms.run/library/debian:bookworm-slim
RUN sed -i 's|deb.debian.org|mirrors.ustc.edu.cn|g; s|security.debian.org|mirrors.ustc.edu.cn|g' /etc/apt/sources.list.d/debian.sources \
    && apt-get update && apt-get install -y --no-install-recommends ca-certificates libgcc-s1 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 65532 -r -s /usr/sbin/nologin nonroot
WORKDIR /app
COPY --from=builder /workspace/assessment-rs/target/release/assessment-rs /app/assessment-rs
EXPOSE 8090
ENV PORT=8090
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=assessment-rs
USER nonroot
ENTRYPOINT ["/app/assessment-rs"]
