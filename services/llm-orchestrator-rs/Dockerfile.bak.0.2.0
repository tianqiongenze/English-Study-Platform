# llm-orchestrator-rs(S1, P0)—— LLM 编排网关 + dialogue 子模块
# 多阶段构建:rust:1.83-bookworm builder + debian:bookworm-slim runtime
# ADR: docs/design/adr-new-services.md / PRD §一 / schema-per-service-phase0.sql esp_llm
FROM rust:1.88-bookworm AS builder
WORKDIR /app

# 国内镜像加速 crates 拉取(首次依赖预热)
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
# 原 gcr.io/distroless/cc-debian12:nonroot 在国内拉取超时(gcr.io 被墙),
# 降级为 debian:bookworm-slim + 非 root 用户运行,功能等价(含 libgcc/glibc + ca-certs)。
# - distroless 无 shell,debian-slim 有;体积略大(~80MB vs ~20MB)但可拉取。
# - 用 useradd 创建非 root 用户 65532,USER 切换,等价 nonroot 安全姿态。
# ADR-security-scanning §4:对标 gateway-go/course-path-go 0 漏洞基线(运行时基线差异已记录)
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libgcc-s1 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 65532 -r -s /usr/sbin/nologin nonroot
WORKDIR /app
COPY --from=builder /app/target/release/llm-orchestrator-rs /app/llm-orchestrator-rs
EXPOSE 8086
ENV PORT=8086
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=llm-orchestrator-rs
USER nonroot
ENTRYPOINT ["/app/llm-orchestrator-rs"]
