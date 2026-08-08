# ADR:安全扫描四件套(Semgrep SAST + Trivy SCA/镜像 + Dependabot + Aikido)

> 状态:Accepted(2026-08-07)
> 关联:`docs/test/06-安全测试报告.md`(业务安全用例 SEC-1~4)、`docs/design/adr-engineering-readiness.md`(CI/CD 门禁)、`.github/workflows/security.yml`、`.github/dependabot.yml`
> 作者:架构组(对标大厂 SDLC 安全左移标准)

## 1. 背景

ESP 10 服务矩阵(Java/Go/Python/Rust 多语言)已部署。**安全测试报告(06)仅覆盖业务安全用例**(JWT/RBAC/越权/secret 注入,SEC-1~4 全通过),但**缺 SAST/SCA/镜像漏洞/依赖漏洞**四类自动化扫描,工程化阻断项 `esp-eng-readiness-gaps` 中"安全测试未执行"未闭合。

行业基线(对标大厂 SDLC):
- **SAST(静态应用安全测试)**:源码级漏洞(注入/反序列化/硬编码/弱加密)。
- **SCA(软件成分分析)**:三方依赖已知漏洞(CVE)。
- **镜像扫描**:容器镜像层漏洞 + 配置缺陷(特权/-root/敏感挂载)。
- **依赖漏洞告警+自动 PR**:Dependabot/Aikido 监控依赖更新与 CVE。

四件套对应关系:
| 工具 | 类型 | 覆盖 | 本地可跑 | SaaS |
|---|---|---|---|---|
| **Semgrep** | SAST | Java/Go/Python/Rust 源码规则集 | ✅ CLI | AppSec(可选) |
| **Trivy** | SCA + 镜像 | 依赖 CVE(`pom.xml`/`go.mod`/`Cargo.toml`/`requirements`)+ 镜像层 | ✅ CLI | — |
| **Dependabot** | 依赖更新 | GitHub 仓库依赖版本+CVE 自动 PR | ❌ 需 git 远端 | GitHub 原生 |
| **Aikido** | 全栈聚合 | SAST+SCA+镜像+DAST+秘密+IaC 一体化 | ❌ 需 GitHub 连接 | SaaS |

## 2. 决策

**四件套分层落地,本地实跑 + 远端 SaaS 双轨:**

1. **Semgrep + Trivy 本地实跑**(非 git 仓库也可跑 CLI):
   - Semgrep:`semgrep ci --config p/java --config p/golang --config p/python --config p/default --config p/security-audit --config p/owasp-top-ten`,全服务源码扫,报告落 `report/security/semgrep-*.json|sarif`。
   - Trivy:`trivy fs --scanners vuln,secret,config .`(文件系统 CVE+秘密+IaC 配置)+ `trivy image esp/<svc>:<tag>`(镜像层 CVE+misconfig),报告落 `report/security/trivy-*.json`。
2. **GitHub Actions 流水线**(`.github/workflows/security.yml`):push/PR 触发 Semgrep+Trivy,main 跑镜像扫描,sarif 上传 GitHub Security tab。迁入 git 后零改动启用。
3. **Dependabot**(`.github/dependabot.yml`):监控 Java(maven)/Go/Python(pip)/GitHub Actions 生态,周更,自动 PR。**需 git 远端仓库生效。**
4. **Aikido**:接 GitHub 仓库后 SaaS 一体化扫描(SAST+SCA+镜像+DAST+IaC),作为四件套的"聚合视图 + 持续监控"层,本地 CLI 不替代。

## 3. 前置:迁入 git(用户决策)

用户拍板:**先迁 git 仓库再接 Aikido**(Aikido SaaS 原生需 GitHub 仓库连接,非 git 下不可用)。流程:
1. `git init` + `.gitignore`(隔离 secret/制品/报告/logs,见仓库根)。
2. 全量首提交(`git add . && git commit`)。
3. 用户在 GitHub 建空仓库(不 init README),提供远端 URL。
4. `git remote add origin <url> && git push -u origin main`。
5. 启用 Dependabot + 接 Aikido SaaS(在 Aikido 控制台连仓库,选 SAST+SCA+Container+Secret 扫描)。

**.gitignore 铁律**:`**/*.pem/.key/.env/secrets/secret*.yaml/application-secret*` 全隔离;含 secret 的 K8s manifest 只入库 `*_tpl` 模板,真实 `esp-secrets` 由 `kubectl apply` 离线注入不入库。

## 4. 门禁规则(SLO)

| 扫描 | 门禁 | 处置 |
|---|---|---|
| Semgrep | **ERROR 级 0**;WARNING 级允许(评审后忽略/修复) | ERROR 阻断 PR |
| Trivy SCA | **CRITICAL/HIGH CVE 0**(可 `--ignorefile .trivyignore` 临时豁免已评估项) | CRITICAL/HIGH 阻断 |
| Trivy 镜像 | 同上 + misconfig(特权/no-root)阻断 | 阻断 |
| Dependabot | 自动 PR,review 后合并 | 非阻断 |
| Aikido | SaaS 仪表盘阈值:Critical 0、High 评审 | 非阻断(监控) |

**豁免机制**:
- Semgrep:`# nosemgrep` 行内注释 + `# nosemgrep: <rule-id>`。
- Trivy:`.trivyignore` 文件列 CVE ID,每条带豁免理由+期限注释。
- 所有豁免必须在 `docs/test/06-安全测试报告.md` §5 登记理由+复审日期。

## 5. 执行现状(2026-08-07)

| 项 | 状态 | 证据 |
|---|---|---|
| .gitignore | ✅ 已建 | 仓库根 `.gitignore`(secret/制品/报告隔离) |
| ADR(本文件) | ✅ 已建 | `docs/design/adr-security-scanning.md` |
| security.yml workflow | ⏳ 待建 | `.github/workflows/security.yml`(Semgrep+Trivy job) |
| dependabot.yml | ⏳ 待建 | `.github/dependabot.yml`(maven/go/pip/actions) |
| Semgrep 本地实跑 | ⏳ 工具安装中 | brew install semgrep;扫 10 服务落 `report/security/semgrep.sarif` |
| Trivy 本地实跑 | ⏳ 工具安装中 | brew install trivy;fs+image 扫落 `report/security/trivy-*.json` |
| 06 报告回填 | ⏳ 待回填 | `docs/test/06-安全测试报告.md` 增 §5 SAST/SCA/镜像扫描实测 |
| git 迁入 | ⏳ 待用户给远端 | init+commit 后 push,启用 Dependabot+Aikido |

## 6. 替代方案否决

| 方案 | 否决理由 |
|---|---|
| 只用 Aikido 一家 | SaaS 强绑定 + 非 git 不可用;本地 CLI 缺。Semgrep/Trivy 本地可跑是底线。 |
| 只本地三件套不接 Aikido | 缺 DAST + 持续聚合视图 + 秘密扫描的 SaaS 维度;用户明确要四件套全。 |
| Snapshot/商业 SAST(Fortify/Coverity) | 重型、商业 license、与多语言微服务 CI 集成慢;Semgrep 规则可定制+开源够用。 |
| Grype 替代 Trivy | Trivy 同时覆盖 fs+image+config+secret,Grype 只 SCA;Trivy 更全。 |

## 7. 行业依据

- OWASP Top 10:SAST(SCA+注入)+ 镜像扫描覆盖 A03/A06/A09。
- NIST SSDF:自动化安全测试(ST)。
- 大厂 SDLC:Google/SMETA、Microsoft/MSDL 均 SAST+SCA+镜像扫描门禁化。
- Semgrep 官方多语言规则集;Trivy Aqua Security 开源;Dependabot GitHub 原生;Aikido 一体化 DevSecOps 平台。
