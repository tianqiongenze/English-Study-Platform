#!/usr/bin/env bash
# 方案3根治：给所有基础镜像打 esp/ 前缀本地 tag,Dockerfile 改用本地 tag 后不再依赖外部 registry
# 触发条件:基础镜像已恢复到 rd-vm docker store(arm64)
# 关联铁律:engineering-ironrules(公共能力共享包优先);context-overflow(本地缓存避免重复拉取)
set -euo pipefail
CONTEXT="${DOCKER_CONTEXT:-rd-vm}"

echo "=== 给基础镜像打 esp/ 前缀本地 tag(方案3根治) ==="

# Go 服务基础镜像(12 个 Go 服务用)
docker --context "$CONTEXT" tag golang:1.25-alpine esp/golang-builder:1.25
docker --context "$CONTEXT" tag alpine:3.22 esp/alpine-runtime:3.22
echo "✓ Go: esp/golang-builder:1.25 + esp/alpine-runtime:3.22"

# Java 服务基础镜像
docker --context "$CONTEXT" tag esp/jdk26-jre:debian-slim esp/jdk26-jre:latest 2>/dev/null || true
echo "✓ Java: esp/jdk26-jre:debian-slim(已有)"

# Python 服务基础镜像(3 个 Python 服务用)
docker --context "$CONTEXT" tag python:3.13-slim esp/python-runtime:3.13 2>/dev/null && echo "✓ Python: esp/python-runtime:3.13" || echo "△ python:3.13-slim 未就绪"

# Rust 服务基础镜像(3 个 Rust 服务用)
docker --context "$CONTEXT" tag rust:1.88-bookworm esp/rust-builder:1.88 2>/dev/null && echo "✓ Rust: esp/rust-builder:1.88" || echo "△ rust:1.88-bookworm 未就绪"
docker --context "$CONTEXT" tag debian:bookworm-slim esp/debian-runtime:bookworm 2>/dev/null && echo "✓ Debian: esp/debian-runtime:bookworm" || echo "△ debian:bookworm-slim 未就绪"

# 前端服务基础镜像(web/h5 用 node + nginx)
docker --context "$CONTEXT" tag node:20-alpine esp/node-builder:20 2>/dev/null && echo "✓ Node: esp/node-builder:20" || echo "△ node:20-alpine 未就绪"
docker --context "$CONTEXT" tag nginx:1.27-alpine esp/nginx-runtime:1.27 2>/dev/null && echo "✓ Nginx: esp/nginx-runtime:1.27" || echo "△ nginx:1.27-alpine 未就绪"

# eclipse-temurin(Java alt,部分服务用)
docker --context "$CONTEXT" tag eclipse-temurin:21-jre-noble esp/eclipse-temurin:21-jre 2>/dev/null && echo "✓ eclipse-temurin: esp/eclipse-temurin:21-jre" || echo "△ eclipse-temurin 未就绪"

echo "=== 完成。Dockerfile 改 FROM esp/<name>:<tag> 即可一劳永逸 ==="
docker --context "$CONTEXT" images --format '{{.Repository}}:{{.Tag}}' | grep "^esp/" | sort
