# Rust 服务运行期基础镜像(唯一非标 tag,禁劫持标准名 per SVD)
#
# 用途:ESP Rust 微服务(assessment-rs/pronunciation-rs/llm-orchestrator-rs/media-learn-rs)的多阶段构建 runtime stage。
#       内置 ca-certificates / libgcc-s1 / curl + nonroot(65532)用户,免去每个服务重复 apt 安装。
# 派生自:esp/jdk26-jre:debian-slim(jdk26-jre 本身 FROM debian:bookworm-slim)。
#
# SVD 铁律:本镜像 tag 为 `*-derived` 唯一非标命名,严禁 `docker tag esp/rust-runtime:... rust:...`
#          劫持标准镜像名(参见 memory stateful-workload-version-drift-ironrule)。
#
# 何时更新:Debian 安全补丁周期 / Rust glibc 依赖升级 / 新增 Rust 服务公共运行期依赖时。
#          更新后须重 build 所有 Rust 服务镜像并跑 BVV 校验。
FROM esp/jdk26-jre:debian-slim

# jdk26-jre 末层为 USER esp(1000);切回 root 做 apt 操作
USER root

# USTC 镜像 + Rust 服务最小运行期依赖
RUN sed -i 's|deb.debian.org|mirrors.ustc.edu.cn|g; s|security.debian.org|mirrors.ustc.edu.cn|g' /etc/apt/sources.list.d/debian.sources || true \
 && sed -i 's|deb.debian.org|mirrors.ustc.edu.cn|g; s|security.debian.org|mirrors.ustc.edu.cn|g' /etc/apt/sources.list 2>/dev/null || true \
 && apt-get update && apt-get install -y --no-install-recommends ca-certificates libgcc-s1 curl \
 && rm -rf /var/lib/apt/lists/* \
 && useradd -u 65532 -r -s /usr/sbin/nologin nonroot

USER nonroot
WORKDIR /app
