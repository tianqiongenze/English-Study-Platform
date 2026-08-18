# CI/CD + Nacos 验证报告 — 2026-08-18

## 1. Nacos 宿主机下沉 — DEFERRED

### 环境
- 宿主机 Java: OpenJDK 21.0.5 Zulu (可用)
- `~/nacos/` 目录已存在，有残留 `nacos-server.tar.gz`(2KB 无效文件)
- 集群内当前状态: `nacos` Deployment 1/1 Running (ClusterIP, 8848/9848/9849)

### 下载尝试
| 镜像源 | 结果 | 原因 |
|---|---|---|
| ghfast.top | 失败 | 连接超时 (15s) |
| mirror.ghproxy.com | 失败 | 连接超时 (15s) |
| gh-proxy.com (HTTP/2) | 失败 | 传输 3.5MB 后 HTTP/2 stream INTERNAL_ERROR |
| gh-proxy.com (HTTP/1.1) | 失败 | 10 分钟超时未完成 |
| hub.gitmirror.com | 未执行 | 前序超时中断 |
| 直连 GitHub | 未执行 | 前序超时中断 |

### 根因
所有 GitHub 镜像代理从当前网络环境均不可达或连接极不稳定。3.5MB 的部分下载表明 gh-proxy.com 本身可用，但 146MB 文件在当前带宽/稳定性下无法完整下载。

### 后续
- DEFERRED，待网络环境改善或使用已缓存/离线介质获取 `nacos-server-2.5.0.tar.gz`
- 容器内 Nacos 部署保持不变，不影响现有服务注册/发现功能

---

## 2. CI YAML 验证

| 文件 | 语法验证 | RC |
|---|---|---|
| `.github/workflows/ci.yml` | **PASS** | 0 |
| `.github/workflows/security.yml` | **PASS** | 0 |

---

## 3. CI 矩阵覆盖

以下 job 均在 `ci.yml` 中存在定义:

| Job 名称 | 状态 |
|---|---|
| java-commons | 存在 |
| java-services | 存在 |
| go-gateway | 存在 |
| go-services | 存在 |
| go-extra | 存在 |
| python-services | 存在 |
| rust-services | 存在 |
| frontend | 存在 |
| contract | 存在 |
| security-scan | 存在 |
| build-images | 存在 |

**11/11 目标 job 全部覆盖**，CI 矩阵完整。

---

## 4. ArgoCD Sync Policy

所有 Application 资源配置一致:
```yaml
syncPolicy:
  automated:
    prune: true        # 自动清理多余资源
    selfHeal: true     # 自动修复手动漂移
  syncOptions:
    - CreateNamespace=true
```

- **automated + prune + selfHeal**: GitOps 闭环自动同步，任何 Git 变更自动 deploy，手动 kubectl 改动自动恢复。
- **CreateNamespace=true**: 目标 ns 不存在时自动创建。
- 3 个 Application 块均配置相同，无异常。

---

## 执行摘要

| 任务 | 结果 | RC |
|---|---|---|
| Nacos 宿主机下载 | DEFERRED (网络不可达) | - |
| ci.yml YAML 验证 | PASS | 0 |
| security.yml YAML 验证 | PASS | 0 |
| CI 矩阵覆盖检查 | 11/11 全覆盖 | 0 |
| ArgoCD sync policy | automated + prune + selfHeal | 0 |
