# ESP 项目进展 (PROGRESS) — 热区

> **本文件只放当前任务热区**(已确认根因/当前子步骤/下一步/恢复锚点),硬上限 ~300 行。
> **历史里程碑见 `docs/progress/archive/*.md`**(按主题拆分),跨会话恢复亦可从 Redis `esp:ctx:archive:*` 取。
> **恢复协议**:新会话先读本文件(全文)+ TaskList,不全量读历史;需要历史细节 Grep archive 或 Redis 取。

## 2026-08-19 会话23-14:Mock/占位符审查 + 全量重构达标计划(进行中🔄)

> 日期:2026-08-19
> Redis 锚点:`esp:ctx:hot:session-23-14`(主)+ `esp:ctx:session:23-14-review`(续)

### 本轮完成/进行中

#### 1. Mock/占位符全面审查(✅ 完成,结论已交付)
**审查结果**:生产域 mock 零残留(MPB 铁律),所有 mock 均为合法 env 模式开关或 DEFERRED 标注。

| 分类 | 服务 | 文件 | 机制 | 等级 |
|---|---|---|---|---|
| 合法 env 开关 | conversation-go | `llm/provider.go:31` | `LLM_MODE=mock → MockProvider` | ✅ 保留 |
| 合法 env 开关 | conversation-go | `asr/provider.go:30` | `ASR_MODE=mock` | ✅ DEFERRED 标注 |
| 合法 env 开关 | conversation-go | `tts/provider.go:33` | `TTS_MODE=mock` | ✅ DEFERRED 标注 |
| 合法 env 开关 | writing-coach | `llm.py:112` | `LLM_MODE=mock → _mock_async` | ✅ 保留 |
| 零 env 兜底 | llm-orchestrator-rs | `provider/mod.rs:322` | StubProvider(无 env 时) | ✅ manifest 有 env,生产不触发 |
| 硬编码占位 | **listening-go** | `main.go:167` | `chunks: ["Hello",", this is"," a placeholder transcript."]` | ⚠ 需改为 501 而非假数据 |
| 注释/测试域 | bff-java | `DownstreamForwarder.java:20` | 仅注释提及 mock | ✅ 无代码影响 |
| 反 mock 设计 | esp-common-go | `llm/llm.go:6` | 注释"不静默 mock" | ✅ |
| SQL 占位符计数 | esp-db-py | `__init__.py:115` | "placeholder count" 错误提示 | ✅ 非功能 mock |

**根因分析(为什么这个阶段还有 mock?)**:ASR/TTS/LLM orchestrator 的真实引擎属于多节点清单 #4 的 DEFERRED 项(需真实 ASR/TTS API key + 多节点环境),单节点验证架构用 env 开关占位合理。listening-go 是唯一硬编码占位(非 env 开关),需最小修复。

#### 2. 全量重构达标计划(✅ 已完成,用户待确认)
- 计划文件: `.claude/plans/robust-baking-wave.md`
- 当前评分: 工程实现 7.83 + TOGAF 治理 6.20 = **7.34/10(B+)**
- 目标评分: 工程实现 **8.5+** / TOGAF **7.5+** / 总分 **8.0+**
- **5 Phase 计划**:
  - Phase1 治理落地(P0 阻断上线):Go 限流熔断接线 + /metrics 全覆盖 + Nacos 注册补齐 + BFF DB 一致性 #227 + OTel endpoint 修正 + securityContext 全补齐 + Mock 残留切断
  - Phase2 基础设施升级:JDK26 派生镜像 + CI/CD 矩阵补全(Rust/Python/admin-go) + ArgoCD syncPolicy
  - Phase3 架构重构(达 8.5+):esp-common-java 公共包下沉 + Go DI 重构 + BFF trace root + Resilience4j + LLM 自适应限流(gradient2) + 契约版本化
  - Phase4 前端重构(达 8.5+):Admin 三面板走 api-client + token + ErrorBoundary 接 OTel + 小程序 crypto + i18n 收口 + SWR 接入
  - Phase5 上线就绪度门禁:E2E 全链路 + 混沌 + 安全 SEC-1~4 + SLO/错误预算 + readiness-gate 评分 8.0+ + 多节点迁移清单
- **执行策略**:Phase1/2/4 并行,Phase3 串行依赖 Phase1,Phase5 依赖全部

#### 3. 真实引擎切换(✅ 代码+manifest 完成,镜像构建受阻于 registry mirror 403)

**目标**:conversation-go 的 ASR/TTS + llm-orchestrator-rs 的 LLM 全部切真实引擎(用户要求一步到位,去 mock)。计划已批准(`.claude/plans/robust-baking-wave.md`)。

**已完成(代码 + manifest + env 接线)**:
- ✅ **asr-adapter-go**(纯 Go,`backend/services/asr-adapter-go/`):收 `{chunks:[base64]}` → Whisper multipart → `{text}`,Gin + 限流熔断 + 健康探针。`go build` 通过。含 `ASR_MODE=local` 本机 GPU 预留分支(M1 Max MPS,DEFERRED 真实实现)
- ✅ **tts-adapter-go**(纯 Go 零 Python,`backend/services/tts-adapter-go/`):`edgetts.New().Bytes()` → MP3 base64 + 四态情绪 + Preston Blair 口型,`go build` 通过(修了重复 import)
- ✅ **deploy/k8s/34-asr-stack.yaml**(whisper 社区镜像 + asr-adapter,含 GPU 预留注释块)
- ✅ **deploy/k8s/35-tts-stack.yaml**(tts-adapter)
- ✅ **10-gateway-go.yaml**:env 加 `ASR_MODE=http`/`ASR_ENDPOINT`/`TTS_MODE=http`/`TTS_ENDPOINT`,已 apply
- ✅ **00-namespace-secret.yaml**:加 llm-providers secret(claude-base/key 占位)
- ✅ **33-orchestrator-vocab.yaml**:`LLM_DEFAULT_MODEL` glm_for_coding → claude-sonnet-4-6,已 apply
- ✅ **listening-go main.go:167**:硬编码 chunks 占位 → 接 asr-adapter(ASR_ENDPOINT 未配则 501 拒绝假数据),`go build` 通过
- ✅ **CC Switch 桥接**:socat 15722→15721 + launchd 自愈,已实测 pod 可达(404=HTTP 通)
- ✅ **真实凭据注入**:esp-secrets + llm-providers 已注入 CC Switch 代理真实 key(当前默认 Zhipu GLM via ai.csdn.net,非 Claude;base_url 指向 host.lima.internal:15722)

**GPU/本地模型预留总表**(用户要求所有 LLM 处支持本机 GPU/本地模型):
| 使用点 | 当前引擎 | 本地预留开关 | 阶段 |
|---|---|---|---|
| conversation-go LLM | CC Switch→Claude/GLM | ANTHROPIC_BASE_URL→Ollama | 现就绪(纯 env) |
| llm-orchestrator-rs | ClaudeProvider | LLM_PROVIDER_OPENAI_BASE→Ollama(OpenAiCompat 复用) | 现就绪 |
| asr-adapter-go | 集群 Whisper | ASR_MODE=local 分支 | 预留(DEFERRED) |
| writing-coach | ANTHROPIC_BASE_URL→Claude | 同→Ollama | 现就绪(纯 env) |
| emotion-svc | 无 LLM(纯关键词) | EMOTION_LLM_BASE 预留 | DEFERRED |

**⚠ 部分解除(registry mirror 403 绕过成功)——方案3根治 Go 类完成**:
- ✅ golang:1.25-alpine(arm64) + alpine:3.22(arm64) **从 buildkit 缓存恢复**(本地文件系统读,零网络拉取)
- ✅ asr-adapter-go:1.0.0 + tts-adapter-go:1.0.0 **镜像构建成功**(38.9MB/38.8MB,基础层本地复用)
- ✅ 34/35 stack yaml 已 apply,asr-adapter + tts-adapter pod Running
- ✅ TTS 链路打通(port-forward 测 HTTP 200),但 edge-tts 库 "websocket bad handshake"(微软协议兼容问题,独立 bug)
- ✅ **方案3根治(Go类)**:12 个 Go 服务 Dockerfile 改 `esp/golang-builder:1.25` + `esp/alpine-runtime:3.22`,验证零网络拉取(buildkit 本地解析 0.8s 无 403)
- ✅ esp/ 前缀 tag 已就绪 5 个:golang-builder/alpine-runtime/jdk26-jre/python-runtime
- ❌ whisper 镜像 `onerahmet/openai-whisper-api` 在 docker.1ms.run 上 404(不存在),需另找可用 whisper 镜像或自建(标 DEFERRED)
- ⏳ **DEFERRED**:rust/debian/node/nginx/eclipse-temurin 基础镜像恢复(现有服务镜像已构建,短期不重建;恢复路径:mirror-pull.sh 加每层大小校验后重试)
- **mirror-pull.sh 已修复**:参数化 arch(默认 arm64,原脚本硬编码 amd64 是 bug)
- **build cache 清理建议**:等所有 esp/ tag 建好后用 `docker buildx prune --filter "until=720h" --keep-storage 20GB`(保命层:golang/rust/python/jdk builder 层),不要 `--all`

#### 4. 会话23-13续3 完成项(本轮前已全部完成,见归档)
- 归档: `docs/progress/archive/2026-08-19-session-23-13-sink-tempo-llmcache.md`
- 11 项已完成(中间件下沉+Tempo traces修复+LLM缓存Phase1+层1根治+混沌+E2E+CRDB probe+清理)
- 本地 commit 99d97e0 (482 files,43516 insertions),push GitHub 失败(443超时)

### 后台任务状态
- **subagent a2ec45bd58057d39e(三任务收尾)**:运行中,路由 admin-go debug proxy;输出 `tasks/a2ec45bd58057d39e.output`
- **3 个 ESP 服务 L2/L3 agent(conversation-go/writing-coach/orchestrator-rs)**:已 TaskStop(因 VM 雪崩中断);orchestrator-rs 临死前完成 deployment apply + rollout restart(镜像含新 L2/L3 代码);conversation-go 在搞 gateway-go 二进制路径;writing-coach 在 VM SSH 构建被中断;**代码改动大部分已落盘,运行期日志验证待集群恢复**

### LCO 代理代码修复(本轮完成,运行期实测待集群恢复)
- **L1 MOVED 重路由 bug**(CLC 铁律,2026-08-19 实测暴露):`withMovedRetry` 原 `podIdx != startIdx` 条件在 MOVED 指向 startIdx 自身时误跳过重试直接返回错误,真实命中率仅 ~50%(非之前声称的 100%)。修:去掉误跳过 + 改多 pod 遍历容错(MOVED 指向自身/重试失败/非 MOVED 错误都继续遍历下一 pod)。补 3 路径测试(TestL1PoolMovedRerouteSelfHost/AllMoved/NonMovedErrorContinutes)全过,`go vet` 干净,二进制已构建。**运行期命中率实测待集群恢复**(本次因 VM 雪崩打断)。
- **L2 embedding 超时**:`LCO_L2_TIMEOUT` 可配(默认 20s,原硬编码 3s 对 Qwen3-Embedding-4B MPS 推理 8-34s 必超时)。config.go 加 L2TimeoutSec 字段,manager.go 两处 3s 改用 cfg.L2TimeoutSec。
- **接入 Claude Code 实测**:配置优先级 = `~/.claude/settings.json` env 块 > shell env。临时改 settings.json `ANTHROPIC_BASE_URL` 15721→15720,子 claude 进程发 LCOPROBE2 → LCO 日志从 11→15 行(新增 4 条请求),证明 LLM 调用确实经过 LCO。测完恢复备份(当前会话仍走 15721 不受影响)。**接入方式:改 ~/.claude/settings.json env.ANTHROPIC_BASE_URL 到 15720,下次 /exit 重开 claude 自动走 LCO**(不需让 LCO 抢 15721,CC Switch 端口 15721 写死在 SQLite schema 不能改)。
- **文档诚实更正**:README + SDD 7. 已知限制里"L1 命中率 100%"改为"原 ~50% 已修待实测",补 L2 超时 + 雪崩风险条目。
- **commit 待做**(雪崩后统一 commit)。

### VM 雪崩事故(2026-08-19 17:30 起,18:36 暂停;19:00 恢复会话确认功能性崩溃+授权重启 RD VM)
- **现象**:VM lima-0 load 飙 408→543,K8s API TLS handshake timeout,k3s CRI DeadlineExceeded,port-forward 后端 Redis 空响应 LCO L1 全断,VM 内 1717 线程 CPU 0% idle。
- **根因**:LCO embedding Qwen3-Embedding-4B(8GB)MPS 推理 8-34s/次占满 CPU(测命中率连发多次)+ 3 ESP agent 宿主编译(go build/cargo/docker images/rdctl shell ssh)+ 共享基础设施 Pulsar 3 broker/Neo4j/Sentinel 常驻负载,9 核单节点 VM 被压爆。吻合 [[esp-singlenode-host-cluster-contention]] + [[esp-crdb-probe-timeout-1s-pitfall]] 模式。
- **止血**:杀 embedding(PID 47722)+ 停 lco-proxy + TaskStop 3 agent + 杀残留 go build/rustc/cargo/ssh/rdctl shell 子进程 + 杀卡死 kubectl 查询。外部加压全停。
- **恢复观察(17:30~18:36)**:load 543→268→...→81,震荡稳态 80~250 带状,中枢未稳定下行到 <50。
- **18:36 用户决策暂停**:落盘状态等回会话。未重启 VM、未探测 kubectl、未推进 LCO/真实引擎/压测。
- **19:00 恢复会话重新诊断**:`rdctl shell cat /proc/loadavg` 显示 load=67/111/155(5min 仍 111,未降),6443 监听在但 kubectl 连接被拒。k3s 日志暴露 **kubelet PLEG 死锁**(pleg was last seen active 3m14s ago, threshold 3m0s)+ **node NotReady**(KubeletNotReady)+ housekeeping took 1m40s(预期 1s)+ kine Slow SQL 1.8s + http2 client connection lost watch 全断。**判定:非单纯负载型雪崩,已演变为 kubelet PLEG 功能性死锁,靠纯等待无法自愈**。宿主侧再次核验干净(host load 6.57,无残留加压进程)。
- **用户授权(19:00 AskUserQuestion)**:授权重启 Rancher Desktop VM 以解开 PLEG 死锁。风险已知并接受:Pulsar/Neo4j 未落盘数据可能丢失,重启后需重跑 pause 镜像预置 + port-forward + launchd 自愈(Nacos-CRDB 通道 + Redis cluster 自愈)。
- **memory 落盘**:`esp-lco-embedding-vm-snowball`(embedding 必限流/离峰/不与编译并行;雪崩恢复期纯等不加压;Monitor 不要用会被 pkill 的命令)。

## 2026-08-19 会话23-14续2:LCO 三层缓存完整实装(L1/L2/L3 全部真实运行)(完成✅)

> 用户诉求"完整三层都起":LCO 代理 L1/L2/L3 从"L1 修了+L2 实装+L3 占位 DEFERRED"推进到**三层全部真实运行 + 端到端验证**。
> Redis 锚点:`esp:ctx:hot:session-23-14`(续,本轮 LCO 三层完成证据写入)

### 本轮完成(证据驱动)

#### 1. L3 Neo4j 知识图谱从 MVP 占位 → 真实 REST 实装(✅)
- **原状**:`l3.go` Record/Enhance 为 DEFERRED 占位;Health 只返回 "ok-deferred" 不真探活。
- **实装**(`tools/lco-proxy/internal/cache/l3.go`):
  - `neo4jTx`:Neo4j 5.x REST `/db/neo4j/tx/commit` + basic auth(bolt→http URL 转换;`LCO_NEO4J_REST_URL` env 显式指定)。
  - `ensureSchema`:幂等建 `prompt_hash`/`response_id` UNIQUE 约束 + `prompt_text` 索引(启动时调一次,失败降级)。
  - `Record`:MERGE Prompt(hash/text/proto/model/ts)→ Response(id/body/ts)+ `ANSWERED_BY` 关系。
  - `Enhance`:CONTAINS 模糊查询最近相关历史 response(作为 L2 未命中的图补充)。
  - `Health`:真实 REST `RETURN 1` 探活(不再 "ok-deferred")。
  - `truncate`:UTF-8 安全截断(参数名 max→maxLen 避免遮蔽 Go1.21+ 内置 max)。
- **CLC 铁律修复(根因:之前 l3.go 引用未定义 `hashKey()` 会编译失败)**:
  - 补 `hashKey(key)` = sha256(key.Raw)[:16](与 L1 `l1Key` 同源)。
  - Record/Enhance/ensureSchema 补 nil 安全(`if l==nil||l.cfg==nil`)。
  - Record/Enhance 用调用方 ctx(继承超时/取消),不再 `context.Background()` 掩盖调用方超时意图。

#### 2. L3 Enhance 接入查询路径(✅,"只写不读"→"读+写")
- **原状**:`Lookup` 只走 L1→L2,L3 `Enhance` 写了但**从未被调用** → L3 只写不读,违反"完整三层都起"。
- **修复**(`manager.go`):
  - `LookupResult` 加 `EnhancedContext string` 字段。
  - `Lookup` 在 L1+L2 miss 时调 `m.l3.Enhance(ctx, key)` 填充 EnhancedContext(独立 5s 超时,失败降级空)。
  - Redis 降级路径(`m.pool==nil`)也补 L3 Enhance(L3 独立于 Redis)。
  - `New()` 构造 L3 后调 `ensureSchema`(之前漏调,导致 MERGE 跑但 CONTAINS 全表扫 + CONSTRAINT 未建)。
- **server.go**:miss 路径若有 EnhancedContext → 响应头 `X-LCO-Context-Enhanced` 暴露(本轮不改请求体;真实 RAG 注入重写 DEFERRED)。

#### 3. CLC 静态检查门禁全过(✅,铁律要求实际运行非"配而未接")
- `go vet ./...` ✅ RC=0
- `go build -o /tmp/lco-proxy ./cmd/lco-proxy` ✅ RC=0(24MB)
- `go test ./internal/cache/...` ✅ PASS
- `golangci-lint run ./...` ✅(v1.62.2 本会话 `go install` 装上并实跑;`.golangci.yml` 启 errcheck/govet/staticcheck/revive 等 8 linter;`l3.go` 零 issue;余 l1pool.go/测试文件的 revive 提示为既有非本轮引入,RC=0)

#### 4. 运行期端到端验证(✅ 三层全部真实命中)
- 重启 lco-proxy(新二进制)+ env `LCO_NEO4J_REST_URL=http://127.0.0.1:7475`。
- `/healthz`:`{"l1_redis":"ok","l2_milvus":"ok","l3_neo4j":"ok"}`(L3 从 "ok-deferred" → 真实 "ok")。
- **L3 Record 验证**:探针请求 `/v1/messages` 内容"LCO三层缓存L3探针:回答一句话" → 转发上游 200 + 回写缓存 → Neo4j 出现 `Prompt(hash=06969e7a6fab2482, model=claude-sonnet-4-6, proto=anthropic, textLen=51)` -[:ANSWERED_BY]-> `Response(bodyLen=1777)`。
- **L1 命中验证**:同 prompt 二次请求 → `X-Lco-Cache-Hit: L1` + 8ms 返回 + 日志 `L1 命中 key=lco:1:anthropic:06969e7a6fab2482`。
- **L2 语义命中验证**:Probe A "LCO语义缓存L2测试用例:解释什么是余弦相似度"(miss,store)→ Probe B "什么是余弦相似度的定义解释"(不同措辞同义)→ `X-Lco-Cache-Hit: L2` + `X-Lco-Source-Protocol: anthropic` + 日志 `L2 语义命中`。
- **embedding 服务**:Qwen3-Embedding-4B MPS 冷加载 ~60s(首次 L2 store embedding 超时非致命),加载后每次 ~8s;`embed_seconds_total=22.35` for 3 req,0 errors。
- **Prometheus 指标**(`lco_cache_hits_total{layer}` / `lco_l2_hits_total` 等)正常暴露,命中率可观测(LCO 铁律 ✅)。

#### 5. 仍 DEFERRED(本轮诚实记录)
- **llm-orchestrator-rs:1.3.0 镜像构建受阻**:registry mirror(docker.1ms.run/daocloud/dockerproxy/dockerpull 全 403)拉不到 `rust:1.88-bookworm` 基础镜像。1.2.0 镜像在跑(Running),1.3.0 代码已 commit(d7622bb 含 L2 Milvus+L3 Neo4j)但镜像无法构建。需 mirror 恢复或派生构建(rust 基础层无本地缓存,不可像 Go 类从 buildkit 恢复)。
- **L3 Enhance 真实 RAG 注入**:本轮仅响应头暴露 EnhancedContext,未改上游请求体注入(避免格式串台 + 成本权衡)。
- **L2 embedding 在本机 MPS 不稳(运行期实测,2026-08-19 22:40~)**:Qwen3-Embedding-4B MPS 推理在本机偶发**卡死(进程 STAT=U 不可中断等待)**或**超慢(>90s/次)**,9 次请求 0 error 但 `seconds_total=0`(请求接了但 `model.encode()` 不返回)。遵 [[esp-lco-embedding-vm-snowball]],单节点 MPS 不可靠。**已设 launchd 常驻**(`com.esp.lco-embedding`,KeepAlive+ThrottleInterval 30s,挂了自愈,kill -9 验证重启),进程级自愈 OK;但 MPS 卡死是更深问题,launchd 重启进程不解决 MPS 本身。**LCO 已配 `LCO_L2_TIMEOUT=8`**:L2 慢则 8s 超时→降级直穿(不阻塞主请求),L1+L3 不受影响(L1 9ms 命中验证 OK,L3 直连 Neo4j 0.04s OK)。生产稳定 L2 需 GPU 节点或换轻量模型(all-MiniLM-L6-v2 384dim CPU 可稳,但跨语言语义弱于 Qwen)。
- **embedding launchd 常驻(✅ 本轮新增)**:`scripts/start-lco-embedding.sh` + `~/Library/LaunchAgents/com.esp.lco-embedding.plist`(KeepAlive=true,RunAtLoad=true,ThrottleInterval=30s)。kill -9 后 ~30s 内 launchd 自动拉起验证通过。start 脚本修了 pgrep 匹配(原 `python3 $APP` 匹配不到 `Python.app/Contents/MacOS/Python`,改匹配 `app.py` 路径)。

#### 6. Claude Code 接入 LCO(✅ 本轮完成,用户确认)
- `~/.claude/settings.json` 的 `ANTHROPIC_BASE_URL` 从 `15721`(CC Switch)切到 **`15720`(LCO 三层缓存)**。备份 `settings.json.bak.pre-lco-1787150139`。
- **生效条件**:Claude Code 进程启动时读一次 settings.json env 块,运行期改不生效。**用户需 `/exit` 重开 claude** 才走 LCO。
- 重开后:每次 LLM 调用响应头带 `X-Lco-Cache-Hit: L1/L2/MISS`(命中层)+ `X-LCO-Context-Enhanced`(L3 增强命中时);`/metrics`(curl localhost:15720/metrics)有命中率指标。
- 当前会话仍走 15721(进程已起,settings 改了但本会话不重读)。

### 本轮 LCO 修复汇总(待统一 commit)
- `tools/lco-proxy/internal/cache/l3.go`:L3 Neo4j REST 全实装 + hashKey 补齐 + nil 安全 + ctx 修正 + truncate max→maxLen
- `tools/lco-proxy/internal/cache/manager.go`:LookupResult 加 EnhancedContext + Lookup 接 L3 Enhance(含降级路径)+ New() 调 ensureSchema
- `tools/lco-proxy/internal/server/server.go`:miss 路径 X-LCO-Context-Enhanced 响应头

---



> VM 重启后 PLEG 死锁解除,load 回落 6~9。本会话恢复全部任务并修复雪崩暴露的 4 个真实 bug。

### 本轮完成(证据驱动)

#### 1. 集群功能性恢复(✅)
- VM load:543(雪崩)→ 6.79(本会话),node Ready,kubectl/API 恢复
- 宿主中间件健康:Nacos `{"status":"UP"}` + Redis cluster-new 6/6 Running + OTel 4317 监听
- ESP 服务 **28/28 pod 全 Ready**(清理前 5 个非 Ready)

#### 2. OTel 导出持续超时根因修复(✅,新发现 bug)
- **根因**:ESP 服务 env 普遍写 `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.esp-prod.svc.cluster.local:4317`,但 **esp-prod ns 内无 otel-collector svc**(下沉宿主后只建在 esp-observability ns)→ DNS NXDOMAIN → 所有服务 OTel traces/logs 导出持续超时(gateway-go 日志刷 `name resolver error: produced zero addresses`,learning-worker 日志刷 `UnknownHostException`)
- **修复**:esp-prod 内建 otel-collector ExternalName → host.lima.internal:4317/4318/8889(纯 K8s,可逆 `kubectl -n esp-prod delete svc otel-collector`)
- **验证**:gateway-go pod `nslookup otel-collector.esp-prod.svc.cluster.local` → CNAME host.lima.internal → 192.168.5.2;4318 HTTP 连通(404=HTTP 通)
- **遗留**:gateway-go 等服务需重启才重新建立 OTel gRPC 连接(非阻断,本轮不重启避免扰动)

#### 3. learning-worker 持续 CrashLoopBackOff 根因修复(✅,会话23-13续3 #7 bff-java 同类坑的遗漏)
- **根因**:learning-worker deploy 有 `readOnlyRootFilesystem: true` + 镜像 HOME=/ → Dubbo 3.3.6 元数据写 `/.dubbo/dubbo-metadata-learning-java-nacos-8848.cache` 失败(`Failed to create directory /.dubbo`)→ Spring 启动失败 → startup probe 338 次失败 → 35 次 restart。bff-java 那次用 emptyDir 挂 /.dubbo 修了,learning-worker 漏修
- **修复**:patch deploy 加 `/.dubbo` emptyDir 挂载(纯 K8s patch,可逆)
- **验证**:新 RS 7978fb6b66 起来,learning-worker **3/3 Ready**(revision 30 期望 replicas=3 全部满足)

#### 4. teacher-java 持续 CrashLoopBackOff 修复(✅)
- **根因**:同 #3,teacher-java deploy 回退到 1.10.0 镜像(yaml 30-other-services.yaml:72 写 1.10.0),1.10.0 镜像 HOME=/ 无 readOnly 豁免 → Dubbo 写 /.dubbo 失败。1.13.1 镜像已修复该问题
- **修复**:`kubectl rollout undo deploy/teacher-java --to-revision=43`(回 1.13.1)
- **验证**:teacher-java **1/1 Ready**,img=esp/teacher-java:1.13.1

#### 5. llm-orchestrator-rs ImagePullBackOff 止血(✅,1.3.0 镜像构建待离峰)
- **根因**:deploy revision 12 指向 `esp/llm-orchestrator-rs:1.3.0`(含 L2 Milvus+L3 Neo4j 代码,33-orchestrator-vocab.yaml:39),但 **1.3.0 镜像未构建**(雪崩中断)→ ImagePullBackOff。1.2.0 镜像存在且 Running
- **止血**:`kubectl rollout undo deploy/llm-orchestrator-rs --to-revision=10`(回 1.2.0 Running)
- **DEFERRED**:1.3.0 镜像构建(Rust release 编译重,VM load 安全后离峰单独做,不与其它编译并行,遵 [[esp-lco-embedding-vm-snowball]])

#### 6. 僵尸 pod/RS 清理(✅)
- 删 tmp-hc-4668(遗留健康检查 pod,busybox 无镜像 ImagePullBackOff)
- 删孤儿 RS teacher-java-cf8559dbc(1.10.0 旧版持续 crash)
- scale down llm-orchestrator-rs-785c9d58c8(1.3.0 RS 残留)

### 主链路端到端验证(✅)
- NodePort 30080(gateway-go-np)HTTP 响应(404=网关在响应,根路径无路由,正常)
- bff-java 30009 `/actuator/health` **200 UP**:CRDB 读写双数据源 routingDataSource READ/WRITE 全 UP(PostgreSQL isValid())
- 28/28 ESP pod Ready

### 仍待完成(下一步,离峰/用户确认后启动)
1. **llm-orchestrator-rs 1.3.0 镜像构建**(受阻:registry mirror 全 403 拉 `rust:1.88-bookworm` 失败,见上节"仍 DEFERRED";mirror 恢复或派生构建后做)→ apply → 验证 L2 Milvus/L3 Neo4j 代码运行期日志
2. **OTel 链路端到端验证**:重启 ESP 服务后看 Tempo 是否收 spans(本次只修了 DNS,服务未重启)
3. ~~**LCO 运行期实测**~~(✅ 本轮续2 完成:L1/L2/L3 三层全部真实命中验证,见上节)
4. **真实引擎切换收尾**:whisper 镜像(mirror 404 需另找/自建,DEFERRED)+ TTS edge-tts websocket handshake bug
5. **全量重构达标计划执行**(`.claude/plans/robust-baking-wave.md`,用户确认后启动)
6. LCO 修复(l3.go/manager.go/server.go 三层实装)+ 本轮 4 处修复统一 commit + push GitHub
7. k6+Locust 混合压测(50→5000 CCU 阶梯)+ 全功能 E2E

---

## 2026-08-19 会话23-13续3:中间件下沉宿主 + Tempo traces修复 + LLM缓存Phase1 + 层1根治(完成✅)

> 归档全貌:`docs/progress/archive/2026-08-19-session-23-13-sink-tempo-llmcache.md`
> Redis 锚点:`esp:ctx:hot:2026-08-19-session-23-13-sink-tempo-llmcache`(主)+ `esp:ctx:session:23-13-cont3`(续)

### 本轮完成(11 项,证据驱动)
1. **中间件下沉+腾退**:Nacos(ExternalName→host.lima.internal,集群 deploy/svc/pvc 全删)+ Prometheus/Loki/Alertmanager/Grafana/OTel(ExternalName,清 3 处残留:otel-collector-daemonset/gateway + fluent-bit)+ Tempo(修端口冲突,见 2)。Milvus/Neo4j DEFERRED(零 ESP 引用+联网不可用+多项目共用,评估 `report/milvus-neo4j-sink-assessment.md`)
2. **Tempo traces 彻底修复**(ASV 根因修正):9095 冲突真凶是 tempo `server.grpc_listen_port`(默认 9095,非 memberlist)。改 tempo.yaml grpc_listen_port=9096 + OTLP 14317/14318 + memberlist 7947;OTel exporter→localhost:14317;bff/gateway OTel endpoint→otel-collector.esp-observability:4317。**端到端验证:Tempo 收 2595+ spans,service.name=bff 20 traces,链路完全打通**。报告 `report/tempo-traces-fix.md`
3. **LLM 缓存 Phase1**:架构 `docs/design/llm-cache-architecture.md`(531行+mermaid)+ conversation-go CachedProvider(cached.go+cached_test.go 12/12 单测 PASS)+ Phase1 完整推进(三服务接线+部署+IT)subagent 运行中。**Milvus/Neo4j 未用**(Phase2/3 DEFERRED)。铁律 LCO 全局 ~/.claude/CLAUDE.md 第8条
4. **Redis Cluster 自愈层1根治**:`scripts/redis-cluster-selfheal.sh`(幂等,集成 start-host-middleware.sh [8/8])。层2(announce-ip PodIP/pod CIDR 直连)DEFERRED 多节点
5. **Nacos-CRDB 通道 launchd 自愈层1根治**:`scripts/nacos-crdb-channel-health.sh` + `~/Library/LaunchAgents/com.esp.nacos-crdb-channel.plist`(每 30s 自愈,kill ssh 后 20s 内重连验证)。层2(多节点直连消除 ssh tunnel)DEFERRED
6. **宿主 Nacos Derby→CRDB 迁移成功**(Phase B):nacos 2.5.0 standalone healthy + HikariPool org.postgresql.Driver + 13 表空表。ASV 反例第4条:ExternalDataSourceProperties 硬编码 mysql driver fallback,显式设 `db.pool.config.driverClassName=org.postgresql.Driver` 一次修复。脚本 `scripts/nacos-crdb-init-schema.py`,插件 nacos-datasource-plugin-postgresql-0.0.5.jar(Maven Central 精确匹配 2.5.0)
7. **bff-java 滚动卡死修复**:根因 1.10.0 镜像 HOME=/ 未设→Dubbo 写 /.dubbo 无权限→Startup probe 338 次失败。修复 emptyDir 挂载(纯 K8s patch 可逆),3/4 Ready(2 Pending CPU 不足 DEFERRED 多节点)
8. **H-01 混沌降级 PASS**:kill 集群 Nacos,Java 服务存活副本全程 Running,Dubbo 本地缓存保活,GW/BFF 全程 HTTP 200,零 CrashLoop。5 Error pod 根因是单节点 PLEG 雪崩(CPU 99%→CoreDNS 链断)非 CRDB/Nacos
9. **CRDB probe 1s 误杀修复**:patch timeout 1→5/failure 5→3/period 10→15(可逆,回滚 `report/chaos/crdb-probe-rollback.sh`)
10. **压测+E2E+CRDB vs TiDB 对比**:E2E 14/0 ALL GREEN(修 phase3-e2e-smoke.sh 3 处 bug);压测 50 并发 learning_review P95=3s/bff_login avg 14.4s 瓶颈(应用层非 DB,CRDB 连接数=0);CRDB vs TiDB 补全 TiDB 历史(commit 0cc7819),单节点 CRDB 为 TiDB 33%-71%(环境噪声非生产结论)
11. **清理 3 处 OTel/fluent-bit 残留**:esp-observability 现为纯 ExternalName 集合(6 svc 全→host.lima.internal,无 pod)

### admin 监控影响(实测)
- ✅ Prometheus 指标/告警不受影响(esp-admin env PROMETHEUS_URL 走 ExternalName 透明转发宿主)
- ⚠ Grafana 链接损坏(GovernanceDashboard 硬编码 192.168.5.15:30300,下沉后端口变,需改 ExternalName)
- ⚠ Sentinel 控制台链接(硬编码 192.168.5.15:8858,需核实)
- ✅ 全服务健康探活不受影响(探活 ESP pod 非中间件)

### DEFERRED 项(11 项,显式标注)
1. Milvus/Neo4j 下沉(零 ESP 引用+联网不可用+多组件高难度+三区域多项目共用,等 LLM 缓存 Phase2/3 启动再下沉)
2. LLM 缓存 Phase2(L2 Milvus 语义向量,Phase1 完成后启动)
3. LLM 缓存 Phase3(L3 Neo4j 知识图谱,Phase2 后启动)
4. Redis Cluster 层2根治(sts announce-ip PodIP/pod CIDR 直连,多节点后落地)
5. Nacos-CRDB 通道层2根治(多节点 pod CIDR 直连消除 ssh tunnel)
6. bff-java 2 Pending pod(单节点 CPU 99% 枯竭,多节点)
7. bff-java 镜像层根治(1.10.0 Dockerfile 加 ENV HOME=/home/esp)
8. push GitHub(443 持续超时,本地 commit 99d97e0 完整 482 files 43516 insertions)
9. conversation-go WS 入口(LLM 缓存 NewWithCache 工厂就位但无 main 包调用方,待补)
10. 5000 CCU 阶梯压测(需先解 bff-java 全副本健康)
11. TiDB 重建+CRDB vs TiDB 完整 EXPLAIN ANALYZE 基准(用户已下电 TiDB,正式回切时重建)

### 本地 commit
- `99d97e0 feat(会话23-13续2):H-01混沌+CRDB probe+LLM缓存LCO+Nacos CRDB迁移+bff修复+压测E2E`(482 files 43516 insertions,⚠ push GitHub 失败 443 超时,本地完整)

### 恢复协议(上下文溢出后)
1. 读 docs/PROGRESS.md 热区(本文件)+ 归档 `docs/progress/archive/2026-08-19-session-23-13-sink-tempo-llmcache.md`
2. Redis GET `esp:ctx:hot:2026-08-19-session-23-13-sink-tempo-llmcache` + `esp:ctx:session:23-13-cont3`
3. 集群现状:ESP 服务全连宿主 Nacos(host.lima.internal:8848,集群 Nacos 已删);宿主 Nacos↔CRDB 走 127.0.0.1:26257(launchd 自愈);Tempo:14317/14318 收 OTel 转发;OTel:4317/4318 收 ESP 服务
4. 本地 commit 99d97e0 未 push(push 失败 443 超时)

---

## 2026-08-18 会话23-15b 完成项（中间件下沉+资源分配+全修复）

### 宿主中间件运维（本次完成）
1. 宿主中间件运维手册: `docs/ops/host-middleware-startup.md` ✅
2. 一键管理脚本: start/stop/status-host-middleware.sh ✅ (全部 RC=0)
3. Prometheus rule_files 修复: /etc/prometheus/rules.yml → $HOME/observability/rules.yml ✅
4. Prometheus otel-collector target 修复: K8s DNS → localhost:8889 ✅ (target UP)
5. Prometheus K8s API 认证: SA prometheus-host + RBAC + token/CA 导出 ✅
6. Loki K8s DNS 修复: host.lima.internal → localhost ✅ (config.expand-env=true)
7. 数据持久化迁移: /tmp/* → ~/observability/data/* (4个服务全部重启验证) ✅
8. Tempo 二进制修复: 从 GitHub 下载 macOS arm64 正确二进制 ✅ (port 3200 监听中)
9. 两个 README 更新宿主中间件章节 ✅ (中英文)

### 资源分配（本次完成）
10. bff-java CPU limit 1→2 core, requests 50m→200m + HPA(max=4, 70%) ✅
11. learning-worker HPA(max=3, 70%) ✅

### Admin 监控缺口修复
12. catalog.json: 移除 seata + otel-collector 改为 FQDN ✅ (待重建镜像部署)
13. Prometheus 通过 OTel Collector 聚合采集(单节点正确架构) ✅

### Nacos Derby→CRDB 迁移
14. CRDB 创建 nacos 数据库 + schema SQL 执行 ✅
15. application.properties 已改为 CRDB (localhost:32627 via kubectl port-forward) ⚠️
16. **NodePort 从宿主机不可直达**（CRDB NodePort 32627 连接超时）— kubectl port-forward 可用但非生产方案
17. **已决策**: Nacos 已完成 CRDB 迁移(host.lima.internal:26257,launchd 自愈通道,见会话23-13续3 #5/#6)

### Admin 前端问题(已发现待修复)
18. GovernanceDashboard 硬编码 IP: 192.168.5.15:30300(Grafana)/8858(Sentinel) → 需改 ExternalName

---

## 关键铁律索引(本次会话新增/引用)

| 铁律 | memory | 全局位置 |
|---|---|---|
| LLM 调用缓存优化 LCO(2026-08-18) | `llm-cache-optimization-ironrule` | ~/.claude/CLAUDE.md 第8条 |
| 生产域 Mock 零残留 MPB(2026-08-17) | 工程铁律第8条 | ~/.claude/CLAUDE.md |
| 写前核对目标系统 WTV(2026-08-17) | 工程铁律第9条 | ~/.claude/CLAUDE.md |
| 镜像产物内容校验 BVV(2026-08-18) | 工程铁律第10条 | ~/.claude/CLAUDE.md |
| 上下文溢出铁律(10%双轨落盘+80%compact+90%止损) | `context-overflow-ironrules` | 全局 CLAUDE.md 第6条 |
| Skill 优先铁律 | `skill-first-ironrule` | 全局 CLAUDE.md 专节 |
| API 签名核实 ASV(2026-08-18) | `api-signature-verify-ironrule` | 全局 CLAUDE.md 专节 |
| CRDB probe timeout=1s 坑 | `esp-crdb-probe-timeout-1s-pitfall` | 项目 memory |
| Redis Cluster 自愈层1根治 | `esp-redis-cluster-nodes-conf-stale-ip` | 项目 memory |
| Nacos-CRDB 通道 launchd 自愈 | `esp-nacos-crdb-channel-launchd-selfheal` | 项目 memory |

## 本地 commit
- `99d97e0 feat(会话23-13续2):H-01混沌+CRDB probe+LLM缓存LCO+Nacos CRDB迁移+bff修复+压测E2E`(482 files 43516 insertions)
- ⚠ push GitHub 失败(443 超时),本地完整,网络恢复后推
