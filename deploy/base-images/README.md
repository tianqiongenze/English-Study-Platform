# ESP 基础运行期镜像

本目录沉淀 ESP 服务多阶段构建复用的 **runtime stage 基础镜像** Dockerfile,落实 DGO(声明式 GitOps)铁律——基础镜像定义入仓库,单一事实源=仓库,而非只存在于本地 `docker tag`。

## 背景与铁律

- 起因:各服务 Dockerfile runtime stage 重复 `apt-get install ca-certificates libgcc-s1 curl + useradd`(Rust 4 服务)、重复 `python:3.13-slim`(Python 3 服务)、重复 `nginx:1.27-alpine`(前端 2 服务),且 FROM 源不统一(`docker.1ms.run/...` / 裸 `debian:` / `swr.../nginx:`)。
- 抽取基础镜像后:服务 Dockerfile 只 COPY 业务产物 + ENV + ENTRYPOINT,公共依赖下沉到基础镜像。
- SVD(有状态集群版本漂移铁律):派生构建必打 **唯一非标 tag** `*-derived`,**严禁 `docker tag <派生镜像> <标准镜像名>` 劫持标准 tag**(否则 STS 按 spec 拉标准 tag 却拉到派生镜像)。本目录所有镜像 tag 均为 `*-derived`。
- BRR(爆炸半径可逆性):基础镜像更新后须单服务试点 build + BVV 校验通过,再全量滚动,禁批量改 N 个服务 Dockerfile 不试点。
- BVV(镜像产物内容校验):build 成功 ≠ 正确,必须 `docker run` 校验镜像内二进制/进程真实可用 + 用户/工具存在。

## 镜像清单

| 镜像 | Dockerfile | 派生自 | 大小 | 用途 |
|------|-----------|--------|------|------|
| `esp/rust-runtime:debian-slim-derived` | `rust-runtime.Dockerfile` | `esp/jdk26-jre:debian-slim` | 1.24GB | Rust 4 服务 runtime(ca-certificates/libgcc-s1/curl + nonroot 65532) |
| `esp/nginx-runtime:alpine-derived` | `nginx-runtime.Dockerfile` | `esp/frontend-react:1.2.1` | 76.4MB | 前端 2 服务 nginx 静态托管 |

> Python 基础镜像 `esp/python-runtime:3.13`(Python 3.12.13 / Debian trixie,205MB)由上一轮抽取,其 Dockerfile 待补入本目录(DEFERRED——见下"待办")。

## 如何 build

```bash
# Rust 运行期基础镜像
docker build -t esp/rust-runtime:debian-slim-derived \
  -f deploy/base-images/rust-runtime.Dockerfile deploy/base-images/

# 前端 nginx 运行期基础镜像
docker build -t esp/nginx-runtime:alpine-derived \
  -f deploy/base-images/nginx-runtime.Dockerfile deploy/base-images/
```

build context 用 `deploy/base-images/`(Dockerfile 内无额外 COPY 文件,nginx 的 default.conf 用 heredoc 内联)。

## tag 规范

- 唯一非标 tag 命名:`<purpose>-derived`(如 `debian-slim-derived` / `alpine-derived`)。
- **禁止** `docker tag esp/rust-runtime:debian-slim-derived rust:1.88` 这类劫持标准镜像名的操作(SVD 第③条)。
- 版本升级时新增 tag(如 `debian-slim-derived-v2`),旧 tag 保留供回滚,确认全量服务切换后再清理旧 tag。

## 何时更新

- Debian / Alpine / nginx 安全补丁周期(月度)。
- 新增 Rust / 前端服务的公共运行期依赖时(先评估是否真公共,≥2 服务才下沉)。
- 更新基础镜像后:**单服务试点 build + BVV → 全量滚动**,每步可回滚(`docker rmi` 新镜像 + git checkout Dockerfile)。

## 待办(DEFERRED)

- [ ] 补 `python-runtime.Dockerfile` 入本目录(当前 `esp/python-runtime:3.13` 镜像存在但 Dockerfile 未入仓库,DGO 缺口)。
- [ ] 全量改指:Rust 剩余 3 服务(pronunciation-rs/llm-orchestrator-rs/media-learn-rs)、Python 剩余 2 服务(writing-coach/emotion-svc)、前端剩 1 服务(apps/h5)的 FROM 改指基础镜像。本轮只做每类 1 个试点。
