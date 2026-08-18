# llm-orchestrator-rs(S1, P0)—— LLM 编排网关 + dialogue 子模块
# 多阶段构建:rust:1.88-bookworm builder + debian:bookworm-slim runtime
# ADR: docs/design/adr-new-services.md / PRD §一 / schema-per-service-phase0.sql esp_llm
FROM docker.1ms.run/library/rust:1.88-bookworm AS builder

# 国内镜像加速 crates 拉取(首次依赖预热)
RUN mkdir -p /usr/local/cargo && cat >> /usr/local/cargo/config.toml <<'EOF'
[source.crates-io]
replace-with = "ustc"
[source.ustc]
registry = "sparse+https://mirrors.ustc.edu.cn/crates.io-index/"
[net]
git-fetch-with-cli = true
EOF

# build context = backend/(含 shared/esp-common-rs + services/llm-orchestrator-rs)
# Cargo.toml 的 esp-common-rs = { path = "../../shared/esp-common-rs" } → 从 WORKDIR 出发解析。
# WORKDIR=/workspace/llm-orchestrator-rs → ../../shared/esp-common-rs = /shared/esp-common-rs
WORKDIR /workspace/llm-orchestrator-rs
COPY shared/esp-common-rs /shared/esp-common-rs
COPY services/llm-orchestrator-rs/Cargo.toml Cargo.lock* ./
COPY services/llm-orchestrator-rs/src ./src
RUN cargo build --release

# 运行阶段:debian:bookworm-slim(nonroot)
# 原 gcr.io/distroless/cc-debian12:nonroot 在国内拉取超时(gcr.io 被墙),
# 降级为 debian:bookworm-slim + 非 root 用户运行,功能等价(含 libgcc/glibc + ca-certs)。
# - distroless 无 shell,debian-slim 有;体积略大(~80MB vs ~20MB)但可拉取。
# - 用 useradd 创建非 root 用户 65532,USER 切换,等价 nonroot 安全姿态。
# ADR-security-scanning §4:对标 gateway-go/course-path-go 0 漏洞基线(运行时基线差异已记录)
FROM docker.1ms.run/library/debian:bookworm-slim
# 国内镜像源(VM 出口偶发不通 deb.debian.org,改 USTC 镜像)
RUN sed -i 's|http://deb.debian.org/debian|http://mirrors.ustc.edu.cn/debian|g; s|http://deb.debian.org/debian-security|http://mirrors.ustc.edu.cn/debian-security|g' /etc/apt/sources.list.d/debian.sources \
    && apt-get update && apt-get install -y --no-install-recommends ca-certificates libgcc-s1 curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 65532 -r -s /usr/sbin/nologin nonroot
WORKDIR /app
COPY --from=builder /workspace/llm-orchestrator-rs/target/release/llm-orchestrator-rs /app/llm-orchestrator-rs
COPY scripts/nacos-register.sh /app/nacos-register.sh
RUN chmod +x /app/nacos-register.sh
EXPOSE 8086
ENV PORT=8086
ENV SERVICE_NAME=llm-orchestrator-rs
ENV HEALTH_URL=/healthz
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=llm-orchestrator-rs
USER nonroot
ENTRYPOINT ["/bin/sh", "/app/nacos-register.sh", "/app/llm-orchestrator-rs"]
