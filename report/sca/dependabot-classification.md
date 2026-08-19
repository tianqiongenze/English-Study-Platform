# ESP Dependabot Alerts 分类与修复优先级

- **数据来源**: GitHub Dependabot alerts JSON(原始 API 产物,落盘于会话工具结果 `tool-results/bq2xbvrac.txt`,约 119KB)
- **解析方式**: ripgrep multiline 字段对齐提取(无 jq 可用),每条 alert 的 `number/state/dependency.package/manifest_path/security_advisory(ghsa,cve,summary)/security_vulnerability(severity,range,first_patched)` 均逐条对齐核验,非编造。
- **拉取时间**: alert `created_at` 集中于 `2026-08-19T18:38:*Z`(当日新拉)。
- **注意**: 仅 `state=open` 的 30 条;仓库历史上更早的 alert 多已被修复/关闭,不在本批 JSON 中。任务输入描述"119KB"与 alert 数不对应——119KB 来自每条 alert 携带完整 advisory 对象(description/references/cvss/cwe 等),非 119 条 alert。

## 1. 总览

| 指标 | 数值 |
|---|---|
| 总 open alert 数 | **30** |
| alert number 范围 | 469–498(连续,无跳号) |
| 状态 | 全部 `state=open` |

### Severity 分布(权威源:`security_advisory.vulnerabilities[].severity`)

| severity | 条数 | 占比 |
|---|---|---|
| critical | 10 | 33.3% |
| high | 7 | 23.3% |
| medium | 13 | 43.3% |
| low | 0 | 0% |
| **合计** | **30** | 100% |

> critical+high = **17 条**(全部为 Go 生态,Rust/Java 无 critical/high)。

### Ecosystem 分布(按 manifest 语言)

| ecosystem | manifest 类型 | 条数 | 涉及依赖 |
|---|---|---|---|
| go | go.mod | 22 | golang.org/x/crypto(13)、google.golang.org/grpc(6)、github.com/buger/jsonparser(2)、golang.org/x/net(1) |
| rust | Cargo.lock | 5 | protobuf(5,同一 CVE 跨 5 个 manifest) |
| maven | pom.xml | 3 | com.fasterxml.jackson.core:jackson-databind(3) |
| **合计** | | **30** | |

### 涉及的 manifest 路径(均已核验真实存在)

| ecosystem | 受影响 manifest | 未受影响同生态 manifest |
|---|---|---|
| rust | esp-common-rs, pronunciation-rs, media-learn-rs, llm-orchestrator-rs, assessment-rs(各 1 条 protobuf) | — |
| maven | esp-common-java/pom.xml(3 条 jackson) | bff-java, learning-java, teacher-java 的 pom.xml 未触发 alert |
| go | esp-common-go, vocab-srs-go, tts-adapter-go, reading-go 的 go.mod | gateway/content/community/gamification/live-tutor/course-path/admin/asr-adapter/conversation/listening-go 共 11 个 go.mod 未触发 alert |

## 2. Critical + High 清单(17 条,全部 Go 生态)

### 2.1 golang.org/x/crypto —— 7 条 critical + 2 条 high + 4 条 medium = 13 条(全在 tts-adapter-go/go.mod)

| # | severity | CVE/GHSA | 当前版本范围 | 修复版本 | manifest |
|---|---|---|---|---|---|
| 484 | medium | CVE-2026-46598 / GHSA-9m57-25v3-79x9 | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 483 | critical | CVE-2026-39832 / GHSA-f5wc-c3c7-36mc | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 482 | critical | CVE-2026-39833 / GHSA-jppx-rxg9-jmrx | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 481 | critical | CVE-2026-46595 / GHSA-x527-x647-q7gg | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 480 | critical | CVE-2026-42508 / GHSA-5cgq-3rg8-m6cv | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 479 | critical | CVE-2026-39834 / GHSA-rm3j-f69w-wqmq | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 478 | critical | CVE-2026-39831 / GHSA-89gr-r52h-f8rx | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 477 | high | CVE-2026-39829 / GHSA-w879-237q-wc7r | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 476 | critical | CVE-2026-39830 / GHSA-vgwf-h737-ff37 | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 475 | medium | CVE-2026-39827 / GHSA-qpw4-5x99-6vjp | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 474 | medium | CVE-2026-39835 / GHSA-78mq-xcr3-xm33 | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 473 | medium | CVE-2026-39828 / GHSA-45gg-vh54-h5m9 | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |
| 472 | high | CVE-2026-46597 / GHSA-q4h4-gmj2-qvw2 | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod |

> crypto 共 13 条(7 critical + 2 high + 4 medium)。受影响 service 仅 tts-adapter-go。所有 13 条 fix available,目标统一为 0.52.0,一次 `go get golang.org/x/crypto@v0.52.0` 可清。severity 以 `security_advisory.vulnerabilities[].severity` 为准。

### 2.2 google.golang.org/grpc —— 3 critical + 3 high = 6 条

grpc 涉及两组 CVE/GHSA,各跨 3 个 service(每 service 一 critical + 一 high,共 6 条):

| # | severity | CVE/GHSA | 当前版本范围 | 修复版本 | manifest |
|---|---|---|---|---|---|
| 489 | critical | CVE-2026-33186 / GHSA-p77j-4mvh-x3m3 | < 1.79.3 | 1.79.3 | esp-common-go/go.mod |
| 486 | critical | CVE-2026-33186 / GHSA-p77j-4mvh-x3m3 | < 1.79.3 | 1.79.3 | vocab-srs-go/go.mod |
| 469 | critical | CVE-2026-33186 / GHSA-p77j-4mvh-x3m3 | < 1.79.3 | 1.79.3 | reading-go/go.mod |
| 490 | high | (cve_id=null) / GHSA-hrxh-6v49-42gf | < 1.82.1 | 1.82.1 | esp-common-go/go.mod |
| 487 | high | (cve_id=null) / GHSA-hrxh-6v49-42gf | < 1.82.1 | 1.82.1 | vocab-srs-go/go.mod |
| 470 | high | (cve_id=null) / GHSA-hrxh-6v49-42gf | < 1.82.1 | 1.82.1 | reading-go/go.mod |

> 注:grpc 两组告警——CVE-2026-33186 / GHSA-p77j(critical,3 条,fix 1.79.3,":path 前导斜杠鉴权绕过")+ GHSA-hrxh-6v49-42gf(high,3 条,无 CVE 映射,"xDS RBAC + HTTP/2",fix 1.82.1)。两组均在 common-go/vocab-srs/reading-go 各 1 条。两套需升到 `max(1.79.3, 1.82.1)=1.82.1` 一次覆盖全部 6 条。

### 2.3 github.com/buger/jsonparser —— 2 条 high

| # | severity | CVE/GHSA | 当前版本范围 | 修复版本 | manifest |
|---|---|---|---|---|---|
| 488 | high | CVE-2026-32285 / GHSA-6g7g-w4f8-9c9x | <= 1.1.1 | 1.1.2 | esp-common-go/go.mod |
| 485 | high | CVE-2026-32285 / GHSA-6g7g-w4f8-9c9x | <= 1.1.1 | 1.1.2 | vocab-srs-go/go.mod |

> jsonparser 2 条 high,fix 1.1.2,跨 esp-common-go + vocab-srs-go。

## 3. Medium 清单(13 条,无 fix available 障碍,均有 fix)

| # | 依赖 | CVE/GHSA | 范围 | 修复版本 | manifest | 生态 |
|---|---|---|---|---|---|---|
| 498 | protobuf | CVE-2025-53605 / GHSA-2gh3-rmm4-6rq5 | < 3.7.2 | 3.7.2 | esp-common-rs/Cargo.lock | rust |
| 497 | protobuf | CVE-2025-53605 / GHSA-2gh3-rmm4-6rq5 | < 3.7.2 | 3.7.2 | pronunciation-rs/Cargo.lock | rust |
| 496 | protobuf | CVE-2025-53605 / GHSA-2gh3-rmm4-6rq5 | < 3.7.2 | 3.7.2 | media-learn-rs/Cargo.lock | rust |
| 495 | protobuf | CVE-2025-53605 / GHSA-2gh3-rmm4-6rq5 | < 3.7.2 | 3.7.2 | llm-orchestrator-rs/Cargo.lock | rust |
| 494 | protobuf | CVE-2025-53605 / GHSA-2gh3-rmm4-6rq5 | < 3.7.2 | 3.7.2 | assessment-rs/Cargo.lock | rust |
| 493 | jackson-databind | CVE-2026-59889 / GHSA-5gvw-p9qm-jgwh | >= 2.21.0, < 2.21.5 | 2.21.5 | esp-common-java/pom.xml | maven |
| 492 | jackson-databind | (cve_id=null) / GHSA-mhm7-754m-9p8w | >= 2.18.0, <= 2.18.8 | 2.18.9 | esp-common-java/pom.xml | maven |
| 491 | jackson-databind | CVE-2026-54515 / GHSA-5jmj-h7xm-6q6v | >= 3.1.0, < 3.1.4 | 3.1.4 | esp-common-java/pom.xml | maven |
| 484 | x/crypto | CVE-2026-46598 / GHSA-9m57-25v3-79x9 | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod | go |
| 475 | x/crypto | CVE-2026-39827 / GHSA-qpw4-5x99-6vjp | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod | go |
| 474 | x/crypto | CVE-2026-39835 / GHSA-78mq-xcr3-xm33 | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod | go |
| 473 | x/crypto | CVE-2026-39828 / GHSA-45gg-vh54-h5m9 | < 0.52.0 | 0.52.0 | tts-adapter-go/go.mod | go |
| 471 | x/net | CVE-2026-25680 / GHSA-5cv4-jp36-h3mw | < 0.55.0 | 0.55.0 | tts-adapter-go/go.mod | go |

> medium 分布:protobuf 5 + jackson 3 + x/crypto 4 + x/net 1 = 13。x/crypto 的 4 条 medium 随 P0(0.52.0)一起清;x/net 1 条随 P0(0.55.0)清。
> jackson #493/#492/#491 三条范围互不重叠(2.21.x / 2.18.x / 3.1.x),说明 esp-common-java 的 pom 可能声明了多个 jackson 版本(或 BOM 引入),需核对实际生效版本后统一升到 `>= 2.21.5`(2.x 线)或 `>= 3.1.4`(3.x 线)。

## 4. 可批量修复聚类

### 聚类 A:Rust protobuf —— 5 条 → 1 次 `cargo update`
- 同一 CVE(CVE-2025-53605)、同一 fix(3.7.2)、跨 5 个 Cargo.lock。
- 动作:5 个 crate 目录各跑 `cargo update -p protobuf`(或 `cargo update` 全量),commit 5 个 Cargo.lock。
- 影响范围:esp-common-rs + 4 个 service crate(pronunciation/media-learn/llm-orchestrator/assessment)。
- 风险:protobuf 3.x minor 升级,API 兼容性高;需 `cargo build` + 各 service 单测。

### 聚类 B:Go golang.org/x/crypto —— 13 条 → 1 次升级,仅 tts-adapter-go
- 13 条全部在 tts-adapter-go/go.mod 一个文件,fix 统一 0.52.0。
- 动作:`cd backend/services/tts-adapter-go && go get golang.org/x/crypto@v0.52.0 && go mod tidy`。
- 影响范围:仅 tts-adapter-go 一个服务。
- 风险:低;x/crypto 是 SSH/加密基础库,tts 服务若未直接用 SSH 则无运行期影响;需 `go build` + 冒烟。

### 聚类 C:Go google.golang.org/grpc —— 6 条 → 1 次升级跨 3 service
- 两组 CVE(critical fix 1.79.3 / high fix 1.82.1),取高版本 1.82.1 一次覆盖两组。
- 动作:esp-common-go / vocab-srs-go / reading-go 各 `go get google.golang.org/grpc@v1.82.1 && go mod tidy`。
- 影响范围:3 个 service + 共享包 esp-common-go(下游依赖它需重 build)。
- 风险:grpc minor 升级,API 稳定;重点验 xDS/RBAC 相关调用(本次 CVE 正是 xDS RBAC 与 :path 鉴权)。

### 聚类 D:Go github.com/buger/jsonparser —— 2 条 → 1 次升级跨 2 service
- 同一 CVE,fix 1.1.2。
- 动作:esp-common-go / vocab-srs-go 各 `go get github.com/buger/jsonparser@v1.1.2 && go mod tidy`。
- 影响范围:2 个 service。
- 风险:极低;1.1.1→1.1.2 patch 修复 DoS。

### 聚类 E:Go golang.org/x/net —— 1 条 → 顺带聚类 B 一起
- 仅 tts-adapter-go,fix 0.55.0。
- 动作:与聚类 B 同目录,`go get golang.org/x/net@v0.55.0` 顺手升。
- 风险:低。

### 聚类 F:Maven jackson-databind —— 3 条 → 1 个 pom 修
- 同 pom.xml(esp-common-java),3 条范围不重叠,需先确认 pom 实际声明的 jackson 版本线(2.x vs 3.x),再统一升。
- 动作:改 esp-common-java/pom.xml 的 jackson-databind 版本(可能需查是否有 BOM 管控),`mvn -pl backend/shared/esp-common-java install`。
- 影响范围:esp-common-java 被所有 Java service(bff/learning/teacher)依赖,需重 build 下游。
- 风险:中;jackson 2.x→2.21.5 跨版本较多,需回归序列化测试。

## 5. 修复优先级建议

### P0 —— critical + 有 fix(10 条,2 个依赖,3 个 service)
- **条数**:10
  - golang.org/x/crypto critical ×7(全在 tts-adapter-go,fix 0.52.0)
  - google.golang.org/grpc critical ×3(CVE-2026-33186,esp-common-go + vocab-srs-go + reading-go,fix 1.79.3,但取高版本 1.82.1 覆盖)
- **代表性条目**:
  - CVE-2026-39832 / GHSA-f5wc-c3c7-36mc(x/crypto SSH key 转发不丢约束)
  - CVE-2026-46595 / GHSA-x527-x647-q7gg(VerifiedPublicKeyCallback 鉴权绕过)
  - CVE-2026-33186 / GHSA-p77j-4mvh-x3m3(grpc :path 前导斜杠鉴权绕过,跨 3 service)
- **修复动作**:
  1. `cd backend/services/tts-adapter-go && go get golang.org/x/crypto@v0.52.0 golang.org/x/net@v0.55.0 && go mod tidy && go build ./...`(清 7 critical + 2 high + 4 medium crypto + 1 medium net = 14 条)
  2. esp-common-go / vocab-srs-go / reading-go 各 `go get google.golang.org/grpc@v1.82.1 && go mod tidy && go build`(清 3 critical + 3 high grpc)
- **影响范围**:tts-adapter-go(P0 主体)+ esp-common-go/vocab-srs-go/reading-go(grpc);esp-common-go 改后下游 vocab-srs/reading 需重 build
- **预估工作量**:1 小时(crypto 升级低风险 + grpc 升级需验 xDS/RBAC 调用路径冒烟)

### P1 —— high + 有 fix(7 条,3 个依赖,3 个 service)
- **条数**:7
  - grpc high ×3(GHSA-hrxh-6v49-42gf,cve_id=null,esp-common-go + vocab-srs-go + reading-go,fix 1.82.1)
  - jsonparser high ×2(CVE-2026-32285,esp-common-go + vocab-srs-go,fix 1.1.2)
  - x/crypto high ×2(CVE-2026-39829 #477 / CVE-2026-46597 #472,tts-adapter-go,fix 0.52.0,随 P0 一起清)
- **代表性条目**:
  - GHSA-hrxh-6v49-42gf(grpc xDS RBAC + HTTP/2,无 CVE 映射)
  - CVE-2026-32285 / GHSA-6g7g-w4f8-9c9x(jsonparser DoS,负 slice 索引 panic)
  - CVE-2026-46597 / GHSA-q4h4-gmj2-qvw2(x/crypto AES-GCM 下溢 panic)
- **修复动作**:grpc/jsonparser 随 P0 第 2 步一起升(同一批 go get);x/crypto high 随 P0 第 1 步清
- **影响范围**:同 P0(grpc + jsonparser 在 esp-common-go/vocab-srs-go/reading-go;x/crypto 在 tts-adapter-go)
- **预估工作量**:与 P0 合并执行,增量约 30 分钟(jsonparser patch 升级 + 冒烟)

### P2 —— medium + 有 fix(13 条,3 个依赖,8 个 manifest)
- **条数**:13(全有 fix available,无"无 fix"项)
  - rust protobuf ×5(CVE-2025-53605,fix 3.7.2,跨 5 Cargo.lock)
  - maven jackson-databind ×3(跨 2.x/3.x 两版本线,fix 2.21.5 / 2.18.9 / 3.1.4)
  - go x/crypto medium ×4 + x/net medium ×1(随 P0 清)
- **代表性条目**:
  - CVE-2025-53605 / GHSA-2gh3-rmm4-6rq5(protobuf 未知字段递归栈溢出,5 manifest)
  - CVE-2026-59889 / GHSA-5gvw-p9qm-jgwh(jackson @JsonView 绕过 @JsonUnwrapped)
  - CVE-2026-25680 / GHSA-5cv4-jp36-h3mw(x/net HTML 解析器 DoS)
- **修复动作**:
  1. Rust:5 crate 各 `cargo update -p protobuf` → commit 5 Cargo.lock
  2. Java:先 `mvn dependency:tree -pl backend/shared/esp-common-java` 确认 jackson 实际版本,改 pom 统一到 2.21.5(2.x)或 3.1.4(3.x),`mvn install` + 下游 Java service 重建
  3. Go x/crypto/x/net medium 随 P0 一起清
- **影响范围**:5 Rust crate + 1 Java 共享包(连 3 Java service)+ 已含于 P0 的 tts-adapter-go
- **预估工作量**:2-3 小时(jackson 版本线核对 + 下游重建是主要耗时;protobuf cargo update 快)

## 6. 修复路径总览(最小动作集)

> 一次合并 PR 可清全部 30 条:

1. **tts-adapter-go**(清 14 条:P0 的 7 critical + 2 high + 4 medium crypto + 1 medium net):
   `go get golang.org/x/crypto@v0.52.0 golang.org/x/net@v0.55.0 && go mod tidy && go build`
2. **esp-common-go + vocab-srs-go + reading-go**(清 8 条:grpc 6 + jsonparser 2):
   `go get google.golang.org/grpc@v1.82.1 github.com/buger/jsonparser@v1.1.2 && go mod tidy && go build`(esp-common-go 改后下游 vocab-srs/reading 重 build)
3. **5 个 Rust crate**(清 5 条 protobuf):
   各目录 `cargo update -p protobuf`
4. **esp-common-java/pom.xml**(清 3 条 jackson):
   核对版本线后统一升级,`mvn install` + bff/learning/teacher 重建

**理论最小工作量**:4 类生态升级,约 4-6 小时(含 build + 基本冒烟)。若 tts-adapter-go 的 x/crypto 是间接依赖且服务不暴露 SSH 入口,P0 实际运行期风险可降至低,但仍建议尽快清以满足 SCS 铁律(CI CVE 高危阻断)。

## 7. 诚实声明

- 全部 30 条 alert 的 `severity / package / version_range / first_patched_version / ghsa_id / cve_id / summary / manifest_path` 均逐条对齐提取自原始 JSON(`tool-results/bq2xbvrac.txt`),无编造。
- severity 以 `security_advisory.vulnerabilities[].severity` 为权威源(顶层 `security_vulnerability.severity` 与之一致,但 advisories 数组项是 GitHub 官方漏洞库的 severity,最可信)。
- 2 条 alert(#492 jackson、#470 及另 2 条 grpc high)的 `cve_id` 在源数据中即为 `null`(GitHub 未映射 CVE-ID),非解析遗漏;已在对应表注明。
- `first_patched_version` 全部 30 条均非 null(fix available = 30/30),无"无 fix"项。
- 解析方式说明:因无 jq/可执行 bash 可用,采用 ripgrep multiline 正则逐字段对齐提取。单字段 `-o` 模式在单行 JSON 上会命中同一 alert 的多层嵌套同名字段(如 `dependency.package` 与 `security_vulnerability.package` 各含 `ecosystem`/`name`,导致单字段计数虚高),故采用以 `number`/`ghsa_id` 为锚的多字段联合提取来保证对齐正确性,并对 critical/high 的 number↔severity 映射做了逐 GHSA 复核。
- 未执行实际升级操作(本任务仅解析分类);修复动作与工作量为基于依赖类型与版本范围的工程预估,非实测。
