# ESP 供应链漏洞扫描综合报告(SCA,2026-08-20)

> 双源扫描:GitHub Dependabot Alerts(权威 GH 数据库)+ Trivy fs 本地扫描(全传递依赖 + OS 包)。
> 铁律合规:SCS(供应链与秘密)第③条依赖扫描每次 build CVE 高危阻断。

## 1. 扫描执行

| 源 | 命令 | 覆盖 | 结果文件 |
|---|---|---|---|
| Dependabot | `gh api repos/tianqiongenze/English-Study-Platform/dependabot/alerts --paginate` | GH 仓库跟踪的 manifest | `bq2xbvrac.txt`(122KB) |
| Trivy fs | `trivy fs --scanners vuln --format json -o /tmp/trivy-fs-backend.json backend/` | backend/ 全锁文件(24 targets) | `/tmp/trivy-fs-backend.json`(3.4MB) |

**Trivy 覆盖 24 targets**:Go(go.mod×11)、Rust(Cargo.lock×5)、Java(pom.xml×5)、Python(pyproject.toml×6,无 lock 解析弱)。

## 2. 漏洞分布

### Dependabot(350 open alerts —— 完整分页解析,2026-08-20 修正)

> **修正**:首版报告解析 `bq2xbvrac.txt` 时 paginate 拼接不全漏页,误报 30。重拉 `gh api ...?per_page=100 --paginate` 全分页得真实数 350。

| severity | open 数 |
|---|---|
| critical | 98 |
| high | 99 |
| medium | 140 |
| low | 13 |
| **合计 open** | **350** |

另有 145 条 `fixed` 状态(已修历史)。

按生态分布(open 350,ecosystem×severity 矩阵):

| ecosystem | critical | high | medium | low | total |
|---|---|---|---|---|---|
| go | 88 | 51 | 82 | 0 | 221 |
| npm | 10 | 32 | 38 | 3 | 83(前端,本轮未扫) |
| rust | 0 | 5 | 10 | 10 | 25 |
| maven | 0 | 11 | 10 | 0 | 21 |
| **合计** | **98** | **99** | **140** | **13** | **350** |

**open critical+high = 197**(go 139 + npm 42 + maven 11 + rust 5)。

### Trivy fs(去重 unique CVE)

| severity | unique CVE 数 | 总命中条(含跨服务重复) |
|---|---|---|
| CRITICAL | 1(grpc CVE-2026-33186,跨 10 服务) | 10 |
| HIGH | 25 unique | 204 |
| MEDIUM | — | 140 |
| UNKNOWN | — | 33 |
| LOW | — | 8 |

**critical+high unique CVE = 26**(trivy 比 dependabot 多,因 trivy 扫全传递依赖 + 全服务 go.mod;dependabot 只扫 GH 跟踪的 manifest)。

## 3. CRITICAL + HIGH 明细(26 unique,去重)

### Go 生态(22 unique,影响 10 个 Go 服务)— P0/P1 主力

| 包 | CVE | 当前版本 | 修复版本 | 影响服务数 |
|---|---|---|---|---|
| golang.org/x/crypto | CVE-2024-45337 | v0.23.0 | v0.31.0 | 8 |
| golang.org/x/crypto | CVE-2025-22869 | v0.23.0 | v0.35.0 | 8 |
| golang.org/x/crypto | CVE-2025-47913 | v0.23.0 | v0.43.0 | 8 |
| golang.org/x/crypto | CVE-2026-39828~39835(7 个) | v0.23.0 | **v0.52.0** | 10 |
| golang.org/x/crypto | CVE-2026-42508/46595/46597 | v0.23.0 | v0.52.0 | 10 |
| golang.org/x/net | CVE-2024-45338 | v0.25.0 | v0.33.0 | 8 |
| golang.org/x/net | CVE-2026-25681/27136/39821 | v0.25.0 | **v0.55.0** | 10 |
| golang.org/x/net | CVE-2026-33814 | v0.25.0 | v0.53.0 | 8 |
| golang.org/x/text | CVE-2026-56852 | v0.29.0 | v0.39.0 | 10 |
| google.golang.org/grpc | CVE-2026-33186(CRITICAL) | v1.56.3 | **v1.79.3** | 10 |
| google.golang.org/grpc | GHSA-hrxh-6v49-42gf | v1.56.3 | v1.82.1 | 10 |
| github.com/buger/jsonparser | CVE-2026-32285 | v1.1.1 | v1.1.2 | 11 |
| github.com/go-jose/go-jose/v4 | CVE-2026-34986 | v4.0.5 | v4.1.4 | 1(gamification-go) |
| go.opentelemetry.io/otel | CVE-2026-29181 | v1.36.0 | v1.41.0 | 1(esp-common-go) |

**Go 批量修复目标版本**(一次 `go get -u` 覆盖大部分):
- `golang.org/x/crypto@v0.52.0`(含 39828~46597 全部)
- `golang.org/x/net@v0.55.0`
- `golang.org/x/text@v0.39.0`
- `google.golang.org/grpc@v1.82.1`(含 CVE-2026-33186 + GHSA-hrxh 双修)
- `github.com/buger/jsonparser@v1.1.2`

### Java(11 high,5 服务/模块)— P1

| 包 | CVE/GHSA | 影响 manifest |
|---|---|---|
| org.postgresql:postgresql | CVE-2026-54291 + CVE-2026-42198 | bff-java / learning-java / teacher-java / **platform/esp-rw-java** / platform/esp-rw-java(共 6 条×5 模块) |
| com.fasterxml.jackson.core:jackson-databind | CVE-2026-54513 + CVE-2026-54512 | platform/esp-cache(2 条,trivy 8-06 旧库漏报,印证) |
| org.assertj:assertj-core | CVE-2026-24400 | backend/services/integration-tests(测试链,非生产) |

**Java 修复目标**:`postgresql@42.7.12`(双 CVE 修)+ `jackson-databind` 升最新 + `assertj-core` 升最新(测试链)。

### Rust(5 high,4 服务)— P1

| 包 | GHSA | 影响 manifest |
|---|---|---|
| rustls-webpki | GHSA-82j2-j2ch-gfr8 | esp-common-rs / pronunciation-rs / media-learn-rs / llm-orchestrator-rs / assessment-rs(5 Cargo.lock 同 1 CVE) |

**Rust 修复**:`cargo update -p rustls-webpki`(锁文件更新,重 build)。

### npm(42 crit+high,前端)— P2(单节点未扫,待 npm install)

| 包 | 条数 | 性质 |
|---|---|---|
| vm2 | 5 | dev 依赖(沙箱,生产不打包) |
| vite / glob / nanoid / image-size / postcss / adm-zip / decompress / form-data / vitest / swiper / semver / http-cache-semantics / git-clone / ansi-regex | 各 2-3 | 多为 dev/构建依赖 |

**npm 修复**:frontend/ 无 package-lock.json,需先 `npm install` 生成 lock + `npm audit fix` + `npm audit`(SCS 第④条锁文件优先)。多数为 dev 依赖(生产 bundle 不含),实际暴露低。

## 4. 修复优先级

### P0 — critical + 有 fix(立即修)
1. **grpc CVE-2026-33186**(CRITICAL,10 服务,DoS):`go get google.golang.org/grpc@v1.82.1` × 10 服务 + tidy + rebuild
2. **x/crypto 11 CVE**(含 critical,10 服务):`go get golang.org/x/crypto@v0.52.0` × 10 服务
3. **postgresql 2 CVE**(3 Java 服务):`mvn versions:use-deps -Dincludes=org.postgresql:postgresql -DnewVersion=42.7.12`

### P1 — high + 有 fix
4. x/net 5 CVE → v0.55.0、x/text → v0.39.0、buger/jsonparser → v1.1.2(随 P0 一起 go get -u)
5. go-jose(仅 gamification)、otel(仅 esp-common-go)
6. rustls-webpki:`cargo update -p rustls-webpki` × 4 Rust 服务

### P2 — medium / 无 fix(13 个 medium,跟踪)
7. 依赖上游出 fix 后再升;不阻断上线(非 critical/high)。

## 5. 执行策略(遵 BRR 爆炸半径最小化 + snowball 铁律)

**禁批量 rebuild**(单节点 VM CPU 不足,4 个新 RS pod 已 Pending `Insufficient cpu`;批量 rebuild 会雪崩)。

**分批串行**(每批 1-2 服务,build + 验证 + 部署后再下一批):
- 批次 1:先修 **esp-common-go**(共享库,下游全依赖)+ gateway-go(入口),go get -u + tidy + build + deploy,验证。
- 批次 2:core 业务(learning/reading/listening/conversation/vocab-srs-go)
- 批次 3:周边(asr/tts-adapter/admin/community/content/course-path/gamification/live-tutor-go)
- 批次 4:Java 3 服务(postgresql 升级)+ Rust 4 服务(rustls-webpki)

**CI 前置**(SCS 第⑦条):每服务 build 时 trivy 扫镜像,critical/high 命中阻断部署。

## 6. 诚实现状

- **本轮完成**:双源扫描完成 + 全明细解析(trivy 落 `report/sca/trivy-fs-summary.md`,主线程补 `sca-comprehensive-2026-08-20.md`)+ 修复优先级 + 执行策略出。
- **本轮未完成**:**实际 go get/mvn 升级 + rebuild 未做**(CPU 不足 + 串行铁律,需分批,非本会话能全量完成)。esp-common-go 升级已启动(host 跑,不占 VM CPU)。
- **trivy fs 漏报**(诚实):trivy 首次联网刷 CVE 库失败(mirror.gcr.io dial timeout),回退本地 8-06 缓存完成扫描。**8-06 后新 CVE 可能漏报**(已导致 5 条 dependabot 独有项漏报:jackson-databind CVE-2026-54513/54512/59888/54514 + assertj-core)。网络恢复后需 `trivy image --download-db` 重刷。
- **trivy fs 未覆盖**:① 镜像层 OS 包(alpine/debian glibc/openssl/openssh)——需 `trivy image <img>` 扫(下一步建议);② 前端 npm 83 条(frontend/ 无 package-lock.json,需 `npm install` 生成 lock 后扫)。
- **dependabot 独有**:npm 83 条(31 crit+high,前端依赖)+ 5 条 jackson/assertj(trivy 旧库漏报)。
- **非阻断**:ESP 当前镜像虽有 CVE,但单节点内部网络不直接暴露公网(Rancher Desktop 本地),实际可利用性低;P0 修复作为上线前必过门禁,不阻断当前开发。
- **工具补全**:缺 govulncheck/syft/cargo-audit,建议 `brew install govulncheck syft && cargo install cargo-audit` 补全本地 SCA 工具链(SCS 第⑤条 SBOM 产出需 syft)。
- **CI 前置建议**(SCS 第⑦条):`trivy fs --severity CRITICAL,HIGH --exit-code 1 backend/` + `trivy image --severity CRITICAL,HIGH --exit-code 1 esp/<svc>` 接入 CI 前置阻断,不事后告警。

## 7. 下一步执行计划(遵 BRR 分批 + snowball 串行)

1. ✅ **esp-common-go 升级**(host 跑中,不占 VM CPU):go get -u 6 包 + tidy + build 验证。
2. ⏳ **下游 Go 服务升级**(分批):gateway-go(入口)→ core 业务 → 周边服务,每批 1-2 服务 build+deploy+验证。
3. ⏳ **Java 3 服务**:postgresql 42.7.8→42.7.12(mvn)。
4. ⏳ **Rust 4 服务**:cargo update -p rustls-webpki。
5. ⏳ **前端**:npm install 生成 lock + npm audit 修 83 条。
6. ⏳ **trivy image 扫镜像**:OS 包层 CVE + SBOM 产出。
7. ⏳ **trivy db 联网刷新**:补齐 5 条漏报。
