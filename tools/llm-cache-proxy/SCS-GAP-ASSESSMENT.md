# llm-cache-proxy 供应链与安全缺口评估(SCS-GAP)

> 评估日期:2026-08-20。评估范围:tools/llm-cache-proxy(Go 代理)+ embedding(Python)。
> 对标铁律:SCS(供应链与秘密)、RWP(只读根文件系统可写路径)、MPB(生产域 Mock 零残留)。
> 评估方式:只读核查仓库现状 + 对照全局 CLAUDE.md SCS 第①~⑦条,不实施重物(VM 资源紧)。

## 一、现状(已具备)

| 项 | 现状 | 证据 |
|---|---|---|
| 秘密扫描 | ✅ Trivy fs scanner `--scanners secret`(仓库根 security.yml) | `.github/workflows/security.yml` trivy-fs job |
| SCA 依赖扫描 | ✅ Trivy vuln + govulncheck(llm-cache-proxy-ci.yml) | llm-cache-proxy-ci.yml govulncheck 步骤 |
| SAST | ✅ Semgrep(ERROR=0 门禁) | security.yml semgrep job |
| 镜像漏洞 | ✅ Trivy image/fs(含 CRITICAL/HIGH=0 门禁 + .trivyignore 豁免登记) | security.yml |
| 锁文件 | ✅ go.sum(Go) + embedding/requirements.txt 未锁(Python 未 pip-compile) | tools/llm-cache-proxy/go.sum |
| Dockerfile nonroot | ✅ USER app(addgroup/adduser -S) | Dockerfile:14,19 |
| Dockerfile HEALTHCHECK | ✅ wget /healthz | Dockerfile:21-22 |
| Linter 含 nil 防御 | ✅ nilnesserr + nilerr(CLC 铁律) | .golangci.yml |
| ADR | ✅ ADR-23 | docs/design/architecture/adr-llm-cache-proxy.md |

## 二、缺口(DEFERRED,本轮不实施)

| 缺口 | 严重度 | 建议优先级 | 说明 |
|---|---|---|---|
| **pre-commit hook 缺失** | 🔴 高 | P0 本轮可补 | 无 `.pre-commit-config.yaml` + `.git/hooks/pre-commit` 不存在。**秘密入库的第一道防线缺失**——Trivy fs 是 CI 层事后扫,本地提交时不拦。建议加 `trufflehog/gitleaks` pre-commit hook。 |
| **SBOM 产出缺失** | 🟡 中 | P1 | 无 syft/cyclonedx 产出。新 CVE 爆发时无法快速查"llm-cache-proxy 是否受影响"。建议 CI 加 `syft -o cyclonedx-json` 产出 SBOM artifact。 |
| **镜像签名缺失** | 🟡 中 | P1 | 无 cosign 签名/verify。镜像来源不可溯源,违反 SCS 第⑥条。建议 `cosign sign` + 部署侧 `cosign verify`。 |
| **Dockerfile 缺 readOnlyRootFilesystem + cap_drop** | 🟡 中 | P1(RWP 铁律) | Dockerfile 无 `--read-only` / `--cap-drop=ALL` / `--security-opt no-new-privileges`。虽 USER app nonroot,但无 readOnly + cap_drop 不完整。**注意**:llm-cache-proxy 是宿主进程非 K8s 部署,Dockerfile 主要为可移植迁移;若上 K8s 需在 deploy yaml 补 readOnlyRootFilesystem: true + emptyDir 挂载(见 RWP 铁律)。 |
| **embedding requirements 未锁** | 🟡 中 | P1 | requirements.txt 是直接版本非 pip-compile 锁。建议 `pip-compile` 生成 requirements.lock 含 transitive 依赖 hash。 |
| **无 Dockerfile 镜像来源校验** | 🟢 低 | P2 | `FROM esp/golang-builder:1.25` / `esp/alpine-runtime:3.22` 是本地派生镜像(非标准 registry tag),无 digest pin。SVD 第⑥条要求"派生构建从可信基础层 digest"。建议 `FROM esp/golang-builder:1.25@sha256:...` digest pin。 |
| **embedding 无 SBOM/SCA** | 🟢 低 | P2 | embedding 是宿主 venv 进程,不经 Dockerfile CI。建议 pip-audit 或 safety 扫 requirements.txt。 |

## 三、建议优先级(本轮 P0 可补,P1/P2 DEFERRED)

- **P0(本轮可补,低成本)**:加 `.pre-commit-config.yaml`(trufflehog/gitleaks + golangci-lint + gofmt),作为本地提交第一道防线。这是 SCS 第①条"秘密永不入库"的源头预防,Trivy fs 是兜底。
- **P1(DEFERRED,需 VM 资源或独立窗口)**:SBOM(syft)、cosign 签名、Dockerfile readOnly+cap_drop、requirements.lock、digest pin。
- **P2(DEFERRED,长期)**:embedding pip-audit、镜像来源完整校验。

## 四、诚实声明

- 本评估只读核查,未实施任何修复(除 P0 pre-commit 外,若用户批准可补)。
- 现状已有 Semgrep SAST + Trivy SCA/secret/镜像 + govulncheck + nilness linter + ADR + nonroot + HEALTHCHECK——**不是零安全**,缺口集中在"供应链可溯源性(SBOM/签名/digest)"和"本地源头预防(pre-commit)"。
- 最严重缺口是 **pre-commit 缺失**(P0),因 Trivy fs 只在 CI 层扫,本地 `git commit` 秘密能进。建议本轮补 pre-commit hook 闭合此缺口。
