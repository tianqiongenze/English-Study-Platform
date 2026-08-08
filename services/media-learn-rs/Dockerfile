# 编译阶段(Rust,Cargo 国内镜像)
FROM rust:1.83-bookworm AS builder
WORKDIR /app
COPY Cargo.toml ./
RUN mkdir -p src && echo "fn main() {}" > src/main.rs
# 国内镜像加速首次依赖拉取
RUN cargo build --release || true
COPY src ./src
RUN cargo build --release

# 运行阶段:distroless cc-debian12(nonroot)
# - Rust release 二进制动态链接 libgcc/glibc → 用 cc 变体(含 libgcc/glibc + ca-certs)
# - nonroot 变体内置 user 65532:nonroot,无需 groupadd/useradd,无 shell 无 apt
# - distroless 无 shell:COPY 后不能再 RUN chown;COPY 默认 root:root,USER nonroot 切换即可
# 镜像拉取:国内拉 gcr.io 可能失败,可替换为
#   registry.aliyuncs.com/google_containers/distroless 或建本地 mirror
# ADR-security-scanning §4:对标 gateway-go/course-path-go 0 漏洞基线
FROM gcr.io/distroless/cc-debian12:nonroot
WORKDIR /app
COPY --from=builder /app/target/release/media-learn-rs /app/media-learn-rs
EXPOSE 8095
ENV PORT=8095
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-observability.svc.cluster.local:4317
ENV OTEL_SERVICE_NAME=media-learn-rs
USER nonroot
ENTRYPOINT ["/app/media-learn-rs"]
