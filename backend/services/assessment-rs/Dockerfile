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

# 运行阶段:esp/rust-runtime:debian-slim-derived(已含 ca-certificates/libgcc-s1/curl + nonroot 65532)
# 基础镜像 Dockerfile: deploy/base-images/rust-runtime.Dockerfile
FROM esp/rust-runtime:debian-slim-derived
USER root
WORKDIR /app
COPY --from=builder /workspace/assessment-rs/target/release/assessment-rs /app/assessment-rs
COPY scripts/nacos-register.sh /app/nacos-register.sh
RUN chmod +x /app/nacos-register.sh /app/assessment-rs
EXPOSE 8090
ENV PORT=8090
ENV SERVICE_NAME=assessment-rs
ENV HEALTH_URL=/actuator/health/liveness
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=assessment-rs
USER nonroot
ENTRYPOINT ["/bin/sh", "/app/nacos-register.sh", "/app/assessment-rs"]
