# ESP Trivy fs SCA 报告(源码锁文件依赖扫描)

> 生成时间:2026-08-20 | 工具:trivy 0.73.0 | 铁律:SCS 第③条(依赖扫描 SCA)
> 扫描范围:**只读源码,未碰集群**。对象:backend/ 全目录(go.sum / Cargo.lock / pom.xml)。
> 注:前端 npm 依赖未在本次 fs 扫描范围(trivy fs 对 npm 需扫 frontend/,本次仅 backend/)。

## 1. 扫描覆盖与命令

- **命令**:`trivy fs --scanners vuln --skip-db-update --format json --skip-dirs "node_modules,target,node,.git" <backend/>`
- **CVE 数据库**:本地缓存 `trivy.db`(2026-08-06 版,1.2GB),`--skip-db-update` 离线扫描。
- **网络说明(诚实记录)**:trivy 首次尝试联网刷新 CVE 库时,`mirror.gcr.io/aquasec/trivy-db:2` 拉取超时(dial tcp timeout)。回退到本地 8-06 缓存成功完成扫描。**意味着**:2026-08-06 之后新发布的 CVE 可能漏报,需联网刷新后复扫。
- **扫描文件(24 个 target,去重后 20 个有漏洞)**:
  - Rust(5):`shared/esp-common-rs/Cargo.lock` + `services/{pronunciation-rs,assessment-rs,llm-orchestrator-rs,media-learn-rs}/Cargo.lock`
  - Go(13):`shared/esp-common-go/go.sum` + 11 个 `services/*-go/go.sum`(admin/asr-adapter/community/content/course-path/gamification/gateway/listening/live-tutor/reading/tts-adapter/vocab-srs)
  - Java(4):`shared/esp-common-java/pom.xml` + `services/{bff-java,learning-java,teacher-java}/pom.xml`
  - 媒体服务 media-learn-rs 的 Cargo.lock 在列表内但本轮无漏洞命中。
- **RC**:0(成功)。原 JSON:`/tmp/trivy-fs-backend.json`(90327 行)。

## 2. Severity 分布

### 原始实例(raw,跨 target 重复计数)
| Severity | 数量 |
|---|---|
| CRITICAL | 10 |
| HIGH | 204 |
| MEDIUM | 140 |
| LOW | 8 |
| UNKNOWN | 33 |
| **合计** | **395** |

### 去重后唯一 advisory(按 ecosystem+pkg+CVE+installedVersion 去重)
| Severity | 唯一条数 |
|---|---|
| CRITICAL | 1 |
| HIGH | 25 |
| MEDIUM | 22 |
| LOW | 2 |
| UNKNOWN | 3 |
| **合计唯一** | **53** |

### 按 ecosystem 分布(唯一)
| Ecosystem | CRIT | HIGH | MED | LOW | UNK | 唯一合计 |
|---|---|---|---|---|---|---|
| go (gomod) | 1 | 22 | 13 | 0 | 3 | 39 |
| maven (pom) | 0 | 2 | 5 | 0 | 0 | 9(注:含 shared+3 服务) |
| rust (cargo) | 0 | 1 | 2 | 2 | 0 | 5 |
| **总计** | **1** | **25** | **22** | **2** | **3** | **53** |

> 说明:唯一 CRITICAL=1,但原始实例=10(因 grpc v1.56.3 在 9 个 Go 服务 + gamification v1.71.0 1 个,去重为 1 个 CVE-2026-33186)。HIGH 唯一=25,实例=204(x/crypto、x/net、x/text 三大 golang.org 包在多服务+多版本重复)。

## 3. Critical + High 全清单(55 条可执行行,含每服务影响)

下表每行 = (ecosystem, 包, 已装版本, CVE/GHSA, 修复版本, 受影响服务数, 服务清单, 摘要)。同 CVE 在不同已装版本(gamification-go 用新版本仍受影响)分行列出。

| Sev | Eco | Package | Installed | Advisory | Fixed | 受影响服务数 | 服务清单 |
|---|---|---|---|---|---|---|---|
| CRITICAL | go | `google.golang.org/grpc` | `v1.56.3` | CVE-2026-33186 | `1.79.3` | 9 | admin-go, community-go, content-go, course-path-go, listening-go, live-tutor-go, reading-go, shared:esp-common-go, vocab-srs-go | google.golang.org/grpc/grpc-go: google.golang.org/ |
| CRITICAL | go | `google.golang.org/grpc` | `v1.71.0` | CVE-2026-33186 | `1.79.3` | 1 | gamification-go | google.golang.org/grpc/grpc-go: google.golang.org/ |
| HIGH | go | `github.com/buger/jsonparser` | `v1.1.1` | CVE-2026-32285 | `1.1.2` | 11 | admin-go, community-go, content-go, course-path-go, gamification-go, gateway-go, listening-go, live-tutor-go, reading-go, shared:esp-common-go, vocab-srs-go | github.com/buger/jsonparser: github.com/buger/json |
| HIGH | go | `github.com/go-jose/go-jose/v4` | `v4.0.5` | CVE-2026-34986 | `4.1.4` | 1 | gamification-go | github.com/go-jose/go-jose/v3: github.com/go-jose/ |
| HIGH | go | `go.opentelemetry.io/otel` | `v1.36.0` | CVE-2026-29181 | `1.41.0` | 1 | shared:esp-common-go | github.com/open-telemetry/opentelemetry-go: OpenTe |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2024-45337 | `0.31.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh: Misuse of ServerConfig.Pu |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2025-22869 | `0.35.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh: Denial of Service in the  |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2025-47913 | `0.43.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh/agent: golang.org/x/crypto |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2026-39828 | `0.52.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.48.0` | CVE-2026-39828 | `0.52.0` | 1 | gamification-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.50.0` | CVE-2026-39828 | `0.52.0` | 1 | tts-adapter-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2026-39829 | `0.52.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.48.0` | CVE-2026-39829 | `0.52.0` | 1 | gamification-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.50.0` | CVE-2026-39829 | `0.52.0` | 1 | tts-adapter-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2026-39830 | `0.52.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.48.0` | CVE-2026-39830 | `0.52.0` | 1 | gamification-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.50.0` | CVE-2026-39830 | `0.52.0` | 1 | tts-adapter-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2026-39831 | `0.52.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.48.0` | CVE-2026-39831 | `0.52.0` | 1 | gamification-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.50.0` | CVE-2026-39831 | `0.52.0` | 1 | tts-adapter-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2026-39832 | `0.52.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh/agent: golang.org/x/crypto |
| HIGH | go | `golang.org/x/crypto` | `v0.48.0` | CVE-2026-39832 | `0.52.0` | 1 | gamification-go | golang.org/x/crypto/ssh/agent: golang.org/x/crypto |
| HIGH | go | `golang.org/x/crypto` | `v0.50.0` | CVE-2026-39832 | `0.52.0` | 1 | tts-adapter-go | golang.org/x/crypto/ssh/agent: golang.org/x/crypto |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2026-39835 | `0.52.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh: golang: golang.org/x/cryp |
| HIGH | go | `golang.org/x/crypto` | `v0.48.0` | CVE-2026-39835 | `0.52.0` | 1 | gamification-go | golang.org/x/crypto/ssh: golang: golang.org/x/cryp |
| HIGH | go | `golang.org/x/crypto` | `v0.50.0` | CVE-2026-39835 | `0.52.0` | 1 | tts-adapter-go | golang.org/x/crypto/ssh: golang: golang.org/x/cryp |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2026-42508 | `0.52.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh/knownhosts: golang: golang |
| HIGH | go | `golang.org/x/crypto` | `v0.48.0` | CVE-2026-42508 | `0.52.0` | 1 | gamification-go | golang.org/x/crypto/ssh/knownhosts: golang: golang |
| HIGH | go | `golang.org/x/crypto` | `v0.50.0` | CVE-2026-42508 | `0.52.0` | 1 | tts-adapter-go | golang.org/x/crypto/ssh/knownhosts: golang: golang |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2026-46595 | `0.52.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.48.0` | CVE-2026-46595 | `0.52.0` | 1 | gamification-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.50.0` | CVE-2026-46595 | `0.52.0` | 1 | tts-adapter-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.23.0` | CVE-2026-46597 | `0.52.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.48.0` | CVE-2026-46597 | `0.52.0` | 1 | gamification-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/crypto` | `v0.50.0` | CVE-2026-46597 | `0.52.0` | 1 | tts-adapter-go | golang.org/x/crypto/ssh: golang.org/x/crypto/ssh:  |
| HIGH | go | `golang.org/x/net` | `v0.25.0` | CVE-2024-45338 | `0.33.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/net/html: Non-linear parsing of case- |
| HIGH | go | `golang.org/x/net` | `v0.25.0` | CVE-2026-25681 | `0.55.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/net/html: golang.org/x/net/html: Arbi |
| HIGH | go | `golang.org/x/net` | `v0.49.0` | CVE-2026-25681 | `0.55.0` | 1 | gamification-go | golang.org/x/net/html: golang.org/x/net/html: Arbi |
| HIGH | go | `golang.org/x/net` | `v0.53.0` | CVE-2026-25681 | `0.55.0` | 1 | tts-adapter-go | golang.org/x/net/html: golang.org/x/net/html: Arbi |
| HIGH | go | `golang.org/x/net` | `v0.25.0` | CVE-2026-27136 | `0.55.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/net/html: golang: golang.org/x/net/ht |
| HIGH | go | `golang.org/x/net` | `v0.49.0` | CVE-2026-27136 | `0.55.0` | 1 | gamification-go | golang.org/x/net/html: golang: golang.org/x/net/ht |
| HIGH | go | `golang.org/x/net` | `v0.53.0` | CVE-2026-27136 | `0.55.0` | 1 | tts-adapter-go | golang.org/x/net/html: golang: golang.org/x/net/ht |
| HIGH | go | `golang.org/x/net` | `v0.25.0` | CVE-2026-33814 | `0.53.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | net/http/internal/http2: golang: golang.org/x/net: |
| HIGH | go | `golang.org/x/net` | `v0.49.0` | CVE-2026-33814 | `0.53.0` | 1 | gamification-go | net/http/internal/http2: golang: golang.org/x/net: |
| HIGH | go | `golang.org/x/net` | `v0.25.0` | CVE-2026-39821 | `0.55.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/net/idna: golang: net/http: golang.or |
| HIGH | go | `golang.org/x/net` | `v0.49.0` | CVE-2026-39821 | `0.55.0` | 1 | gamification-go | golang.org/x/net/idna: golang: net/http: golang.or |
| HIGH | go | `golang.org/x/net` | `v0.53.0` | CVE-2026-39821 | `0.55.0` | 1 | tts-adapter-go | golang.org/x/net/idna: golang: net/http: golang.or |
| HIGH | go | `golang.org/x/text` | `v0.29.0` | CVE-2026-56852 | `0.39.0` | 8 | admin-go, asr-adapter-go, community-go, content-go, listening-go, reading-go, shared:esp-common-go, vocab-srs-go | golang.org/x/text: golang.org/x/text: Denial of Se |
| HIGH | go | `golang.org/x/text` | `v0.34.0` | CVE-2026-56852 | `0.39.0` | 1 | gamification-go | golang.org/x/text: golang.org/x/text: Denial of Se |
| HIGH | go | `golang.org/x/text` | `v0.36.0` | CVE-2026-56852 | `0.39.0` | 1 | tts-adapter-go | golang.org/x/text: golang.org/x/text: Denial of Se |
| HIGH | go | `google.golang.org/grpc` | `v1.56.3` | GHSA-hrxh-6v49-42gf | `1.82.1` | 9 | admin-go, community-go, content-go, course-path-go, listening-go, live-tutor-go, reading-go, shared:esp-common-go, vocab-srs-go | gRPC-Go: xDS RBAC and HTTP/2 Vulnerabilities |
| HIGH | go | `google.golang.org/grpc` | `v1.71.0` | GHSA-hrxh-6v49-42gf | `1.82.1` | 1 | gamification-go | gRPC-Go: xDS RBAC and HTTP/2 Vulnerabilities |
| HIGH | maven | `org.postgresql:postgresql` | `42.7.8` | CVE-2026-42198 | `42.7.11` | 3 | bff-java, learning-java, teacher-java | jdbc.postgresql.org: pgjdbc: Client-side Denial of |
| HIGH | maven | `org.postgresql:postgresql` | `42.7.8` | CVE-2026-54291 | `42.7.12` | 3 | bff-java, learning-java, teacher-java | org.postgresql/postgresql: com.ongres.scram/scram- |
| HIGH | rust | `rustls-webpki` | `0.101.7` | GHSA-82j2-j2ch-gfr8 | `0.103.13, 0.104.0-alpha.7` | 4 | assessment-rs, llm-orchestrator-rs, pronunciation-rs, shared:esp-common-rs | rustls-webpki: Denial of service via panic on malf |

## 4. Critical + High 代表性 5 条(最优先修)

1. **[CRITICAL] google.golang.org/grpc CVE-2026-33186** — gRPC-Go 授权绕过(:path 缺前导斜杠)。影响 10 个 Go 服务(v1.56.3 在 9 服务 + gamification v1.71.0)。修复:`grpc → 1.79.3`。这是全场唯一 CRITICAL,优先级最高。
2. **[HIGH] golang.org/x/crypto v0.23.0 → v0.52.0**(8 个 CVE-2026-398xx/465xx/42508) — SSH 客户端/knownhosts/agent 系列认证绕过+DoS,影响 8 个 Go 服务。一次性 `go get golang.org/x/crypto@v0.52.0` 全清。
3. **[HIGH] golang.org/x/net v0.25.0 → v0.55.0**(CVE-2024-45338/2026-25681/27136/33814/39821) — HTML parser DoS + http2 + idna,影响 8 个 Go 服务。`go get golang.org/x/net@v0.55.0`。
4. **[HIGH] org.postgresql:postgresql 42.7.8**(CVE-2026-42198/54291) — pgjdbc 客户端 DoS + SCRAM,影响 bff/learning/teacher 三个 Java 服务。修复:`42.7.12`。
5. **[HIGH] rustls-webpki 0.101.7**(GHSA-82j2-j2ch-gfr8) — 畸形证书 panic DoS,影响 4 个 Rust 服务。修复:`0.103.13`。

> 注:`github.com/buger/jsonparser v1.1.1`(CVE-2026-32285,DoS,影响 11 服务,最多)和 `golang.org/x/text v0.29.0` → `0.39.0` 也是高频,升级 golang.org/x 全家桶时一并处理。

## 5. 与 Dependabot 交叉对比

数据来源:GitHub Dependabot open alerts(gh API,350 条原始,去重 109 unique advisories)。

| 维度 | 数值 |
|---|---|
| Dependabot open alerts 原始 | 350 |
| Dependabot unique advisories | 109 |
| Dependabot by ecosystem | go=25, maven=11, rust=5, **npm=68** |
| Dependabot crit+high unique(go+maven+rust) | 22(go16 + maven5 + rust1) |
| Dependabot crit+high unique(npm) | 31 |
| **Trivy fs 扫到但 Dependabot 没报(trivy-only)** | **17 条** |
| **Dependabot 报了但 Trivy fs 没扫到(dep-only,限 go/rust/maven 范围内)** | **5 条** |
| Dependabot 独有 npm(超出本次 trivy fs 范围) | 68 条(31 crit+high) |

### 5.1 Trivy 额外发现(本地构建链引入,Dependabot 未报)— 17 条
Dependabot 仅扫已提交到 GitHub 仓库的 manifest;trivy fs 扫本地 backend/ 全树(含 go.sum 传递依赖全量解析),故能抓到 Dependabot 漏报项。代表性:
- `golang.org/x/net` CVE-2026-33814 / CVE-2024-45338 / CVE-2026-27136 / CVE-2026-39821 / CVE-2026-25681(HIGH)
- `golang.org/x/crypto` CVE-2025-47913(HIGH)
- `golang.org/x/text` CVE-2026-56852(HIGH)
- `commons-lang:commons-lang` CVE-2025-48924、`org.apache.commons:commons-configuration2` CVE-2026-45205、`tools.jackson.core:jackson-databind` CVE-2026-59889(MEDIUM,Java 测试/工具链)
- `golang.org/x/net` CVE-2026-42502/42506/47911/58190/46600(MEDIUM/UNKNOWN)、`golang.org/x/sys` CVE-2026-39824(UNKNOWN)、`golang.org/x/crypto` GO-2026-5932(UNKNOWN)

> **根因假设**:① Dependabot 对部分传递依赖的 CVE 映射滞后于 trivy 的 NVD/GHSA 数据;② trivy 解析 go.sum 全量传递依赖,Dependabot 可能只看 go.mod 直接声明;③ Java 侧 commons-lang/jackson-databind 测试 scope 依赖 Dependabot 默认不告警。**这正是 SCS 铁律第③条要求本地 trivy 补 Dependabot 盲区的原因——Dependabot 事后告警,trivy fs 前置阻断。**

### 5.2 Dependabot 独有(限 trivy fs 扫描范围 go/rust/maven 内)— 5 条
- `org.assertj:assertj-core` CVE-2026-24400(high,测试依赖)
- `com.fasterxml.jackson.core:jackson-databind` CVE-2026-54513 / CVE-2026-54512(high)、CVE-2026-59888 / CVE-2026-54514(medium)

> **根因假设**:trivy 本地 CVE 库停留在 2026-08-06(联网刷新失败),这 5 条是 8-06 之后新发布的 CVE,trivy 用旧库漏报。**需联网刷新 trivy db 后复扫补齐**。

### 5.3 Dependabot 独有 npm(超出 trivy fs 本次范围)— 68 条(31 crit+high)
本次 trivy fs 只扫 backend/ 不含 frontend npm 依赖。Dependabot 覆盖 frontend/(React/H5/小程序前端)。**缺口**:trivy fs 需扩展扫 `frontend/` 的 package-lock.json 才能对齐 npm 维度。

## 6. 结论与下一步建议

1. **整体态势**:源码层 SCA 形势严峻。唯一 CRITICAL(gRPC 授权绕过)+ 25 HIGH(集中在 x/crypto、x/net、x/text、grpc、jsonparser、postgresql、rustls-webpki)覆盖几乎所有 Go/Rust/Java 服务。多数为一键升级全家桶即可批量修复(golang.org/x/* 升 v0.52/v0.55/v0.39,grpc 升 1.79.3)。
2. **是否建议下一步跑 `trivy image` 扫镜像**:**强烈建议**。trivy fs 只覆盖源码依赖(语言包),**完全未覆盖镜像层 OS 包**(基础镜像 alpine/debian/ubi 的 glibc/openssl/bash/openssh 等 CVE)。SCS 铁律第⑤⑥条要求 SBOM + 镜像来源校验,trivy image 才能产出镜像 OS 层漏洞 + SBOM(`--format cyclonedx`)。建议:`trivy image --scanners vuln --format json -o report/sca/trivy-image-<svc>.json <镜像tag>`,对每个已构建的 ESP 镜像跑一遍,补齐 OS 包层。
3. **trivy db 联网刷新**:本次 CVE 库停在 8-06(联网超时),5 条 Dependabot 独有项疑似因库旧漏报。建议网络恢复后 `trivy image --download-db-only` 刷新,再复扫补齐。
4. **修复优先级**:① CRITICAL grpc → 1.79.3(全 Go 服务);② x/crypto@0.52 + x/net@0.55 + x/text@0.39(8 服务批量);③ postgresql 42.7.12(3 Java);④ rustls-webpki 0.103.13(4 Rust);⑤ jsonparser 1.1.2(11 Go)。升级后重跑 trivy fs 验证清零。
5. **CI 门禁(SCS 第⑦条)**:把 `trivy fs --scanners vuln --severity CRITICAL,HIGH --exit-code 1 backend/` 接入 CI,crit/high 命中即阻断 PR,前置而非事后告警。

## 附:原始产物
- 原始 JSON:`/tmp/trivy-fs-backend.json`(90327 行,本机临时)
- Dependabot alerts TSV:`/tmp/dependabot-alerts.tsv`(350 行)
- 本报告:`report/sca/trivy-fs-summary.md`