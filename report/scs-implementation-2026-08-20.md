# lco-proxy SCS 供应链安全补齐实施报告

> 日期:2026-08-20。范围:tools/lco-proxy + embedding。
> 对标铁律:SCS(供应链与秘密)、RWP(只读根文件系统可写路径)、BVV(镜像产物内容校验)、CLC(代码级正确性)、BRR(爆炸半径可逆性)、DGO(声明式 GitOps)、MPB(生产域 Mock 零残留)。
> 现状基线:已有 Semgrep SAST + Trivy SCA/secret/镜像 + govulncheck + nilness linter + ADR + nonroot + HEALTHCHECK(非零安全)。本轮补 5 项缺口。

## 一、改动清单(可逆,原值已记)

| 项 | 文件 | 改动 | 可逆性 |
|---|---|---|---|
| 1 SBOM | .github/workflows/lco-proxy-ci.yml | 新增 sbom job(syft cyclonedx-json + upload-artifact 30天) | 新增 job,不破坏既有 test/security 门禁 |
| 2 cosign | .github/workflows/lco-proxy-ci.yml | 新增 sign job(cosign sign,graceful skip 无密钥) | 新增 job,if 条件限 main/master/dispatch |
| 3 digest pin | tools/lco-proxy/Dockerfile | FROM tag 改 @sha256 digest + GOTOOLCHAIN=auto 修复 toolchain | 原值:`FROM esp/golang-builder:1.25` / `esp/alpine-runtime:3.22` |
| 4 requirements.lock | tools/lco-proxy/embedding/requirements.lock + requirements.txt | pip-compile 产 lock(含 hash),torch/st 标运行期依赖 | 新增 lock 文件 |
| 5 readOnly/cap_drop | tools/lco-proxy/Dockerfile | 注释说明运行时需 --read-only --cap-drop=ALL | lco-proxy 无 K8s yaml → DEFERRED |

## 二、逐项执行与验证

### 项1 — SBOM 产出(syft)

- **改动**:`.github/workflows/lco-proxy-ci.yml` 新增 `sbom` job,用 `anchore/sbom-action/download@v0` 装 syft,`syft dir:. -o cyclonedx-json --output sbom.cdx.json` 扫 `tools/lco-proxy`,upload-artifact 保留 30 天。
- **本地验证**:`brew install syft` 装了 syft 1.51.0;`syft dir:tools/lco-proxy -o cyclonedx-json --file report/scs/lco-proxy-sbom-local.json` RC=0,产出 37384 bytes CycloneDX 1.7 SBOM(specVersion=1.7,含 fastapi 等组件,但 lco-proxy 是 Go 项目 syft 扫出 go.mod 依赖)。
- **本地 SBOM 产物**:`report/scs/lco-proxy-sbom-local.json`(37KB,CycloneDX 1.7)。
- **RC**:syft 跑通 RC=0;YAML lint RC=0。

### 项2 — cosign 镜像签名

- **改动**:同 workflow 新增 `sign` job(needs: test,sbom),装 cosign via `sigstore/cosign-installer@v3`。逻辑:`if [ -z "${COSIGN_KEY}" ]; then notice 跳过; exit 0; fi` → 有密钥则 `cosign sign --key` + `cosign verify`。
- **graceful skip 逻辑**:无 `COSIGN_KEY` secret 时打 `::notice::` 并 exit 0,**不造伪密钥**(MPB/SCS 禁伪密钥)。密钥注入后自动生效。
- **本地验证**:`brew install cosign` 装了 cosign v3.1.3(GitCommit 11926fa5)。本地无 COSIGN_KEY,未实际 sign(DEFERRED 本地签名验证,符合"密钥注入后生效")。
- **DEFERRED**:本地未用真密钥跑 cosign sign/verify(无密钥;CI 上注入 COSIGN_KEY/COSIGN_PASSWORD 后即生效,无需改代码)。

### 项3 — Dockerfile FROM digest pin

- **digest 取得**:`docker inspect` →
  - `esp/golang-builder:1.25@sha256:de83b2e2491a0ef08bbcec6b76b3ef3cadba46d00d87a28a1307ed182d192ed9`
  - `esp/alpine-runtime:3.22@sha256:5f503088ad18960d0e15369a72ed9aa724401bd14d655aff39696d3535cf6360`
  - (本地派生镜像,RepoDigest==config Id,符合 SVD/SCS 第⑥条派生构建 digest pin)
- **改动**:`FROM esp/golang-builder:1.25@sha256:... AS builder` + `FROM esp/alpine-runtime:3.22@sha256:...`。
- **附带修复(BVV 必需)**:`go.mod` 声明 `go 1.26.5`,而 builder 镜像内置 go 1.25.13 且 `GOTOOLCHAIN=local`(不自动取工具链)→ `go mod download` 报 `go.mod requires go >= 1.26.5`。加 `ENV GOTOOLCHAIN=auto` 让 go 按 go.mod 自动拉匹配 toolchain(源 goproxy.cn)。**此非 SCS 改动引入,是 digest pin 后首次真正 build 才暴露的既有 toolchain 漂移**。
- **build 验证(BVV)**:`docker build -t esp/lco-proxy:scs-test -f Dockerfile .` RC=0(19.8s go build 通过);`docker run --entrypoint sh esp/lco-proxy:scs-test -c 'ls -la /app/lco-proxy && /app/lco-proxy --help'` → 二进制存在(16253090 bytes),启动打印 `LCO 代理启动 listen=0.0.0.0:15720`(redis 连接失败因容器内无 redis,属预期,二进制本身正常)。BVV 通过。
- **RC**:build RC=0;BVV docker run RC=0。

### 项4 — Python requirements.lock(pip-compile)

- **改动**:`tools/lco-proxy/embedding/requirements.txt` 加 SCS 说明注释,torch/st 标"运行期依赖,部署时装";新增 `requirements.lock`(pip-compile 产,含 transitive + hash)。
- **执行**:`/opt/homebrew/bin/python3.11 -m piptools compile` 锁 fastapi/uvicorn/pydantic/requests 四个轻量依赖(注:系统默认 python3.7 不支持 pydantic2,改用 python3.11)。RC=0。
- **锁了哪些包(15 个 transitive)**:annotated-types, anyio, certifi, charset-normalizer, click, fastapi, h11, idna, pydantic, pydantic-core, requests, starlette, typing-extensions, urllib3, uvicorn。每个含双 hash(`--generate-hashes`),342 行。
- **torch/st 处理**:torch==2.5.1 / sentence-transformers==3.3.1 **未纳入 requirements.lock**(避免 pip-compile 联网拉数 GB wheel 压 VM,SCS 锁文件聚焦可校验轻量 API 依赖)。requirements.txt 注释明确标"运行期依赖,部署时装",部署顺序:`pip install -r requirements.lock && pip install torch==2.5.1 sentence-transformers==3.3.1`。符合任务"torch/st 标运行期依赖,部署时装"要求。
- **RC**:pip-compile RC=0。

### 项5 — Dockerfile readOnly + cap_drop(RWP 铁律)

- **调研**:`grep -rl "lco-proxy" deploy/` → **lco-proxy 无 K8s deployment yaml**(仅在 docs/scripts 出现,是宿主进程,scripts/start-lco.sh 启动)。
- **改动**:Dockerfile 顶部加 SCS 注释块,说明运行时需 `--read-only --cap-drop=ALL --security-opt no-new-privileges` + readOnlyRootFilesystem: true + emptyDir 挂载 /.dubbo /logs /tmp。
- **DEFERRED**:lco-proxy 是宿主进程非 K8s 部署,**无 deployment yaml 可补 readOnlyRootFilesystem/cap_drop/emptyDir**。Dockerfile 注释已声明迁移 K8s 时的要求;真正上 K8s 时需新建 deploy/k8s/lco-proxy.yaml 并声明该 securityContext(RWP 铁律)。原因:当前架构 lco-proxy 设计为宿主进程(给本机工具接入),Dockerfile 仅为可移植迁移保留;无运行实例无需立即补 yaml。
- **RC**:Dockerfile 注释改动,无 build 影响(build 仍 RC=0,见项3)。

## 三、DEFERRED 汇总

| 项 | DEFERRED | 原因 |
|---|---|---|
| 2 | 本地 cosign sign/verify 实跑 | 无 COSIGN_KEY(不造伪密钥);CI 注入密钥后即生效 |
| 5 | deploy/k8s/lco-proxy.yaml readOnly+cap_drop+emptyDir | lco-proxy 是宿主进程无 K8s yaml;迁移 K8s 时新建 |

## 四、铁律遵守自检

- **BRR**:所有改动可逆(Dockerfile digest pin 原值已记;CI 新增 job 不删既有;requirements.lock 新增)。无 sudo/无不可逆。
- **CLC**:改完实际跑——syft RC=0、docker build RC=0、docker run BVV RC=0、go vet RC=0、pip-compile RC=0、YAML lint RC=0。未只改完声称。
- **DGO**:CI workflow 改动是声明式 yaml,YAML lint 通过,commit-ready(未 push)。
- **SCS**:cosign 无密钥 graceful skip 不造伪密钥;digest pin 派生镜像;SBOM 产 CycloneDX;requirements.lock 含 hash;torch/st 不进锁避免压 VM。
- **BVV**:build 后 docker run 校验镜像内 /app/lco-proxy 二进制存在且可执行(打印启动日志),非只校验 lib 存在。

## 五、诚实声明

- 项3 digest pin 暴露并修复了既有 go toolchain 漂移(go.mod 1.26.5 vs builder 1.25.13),非 SCS 改动引入,属 digest pin 后首次完整 build 才发现的既有问题,加 GOTOOLCHAIN=auto 修复并验证。
- 项5 lco-proxy 无 K8s yaml 属架构现状(宿主进程),非漏补;Dockerfile 注释已声明迁移要求。
- 项2 本地未真签(无密钥),CI 逻辑已就绪,密钥注入即生效。
- 本轮 5 项全部落地(2 项 DEFERRED 已说明原因),无破坏既有 security.yml 门禁(只新增 job,未改 test/semgrep/trivy 既有 job)。
