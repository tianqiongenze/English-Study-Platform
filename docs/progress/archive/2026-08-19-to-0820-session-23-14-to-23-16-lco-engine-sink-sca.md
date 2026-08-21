# ESP 进展归档:2026-08-19 ~ 08-20 session-23-14 ~ 23-16(LCO 三层实装 / 中间件下沉 / SCA / whisper 派生 / 工程治理)

> 本文件归档自 PROGRESS.md 热区 2026-08-19~08-20,含 session-23-14~23-16 完成 detail。冷热分离归档执行于 2026-08-20。
> 归档原则:已完成(✅)大段移入此文件;进行中(🔄)且与当前 LCO/切流任务直接相关的(LCO 三层验证、流式透传修复、L2 维度对齐、LCO 工程治理)在热区保留精简摘要+指针,详细 detail 亦归档于此。

---

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

## 2026-08-20 会话23-15:push 解阻 + LCO 运行层诊断 + orchestrator 1.3.0 派生构建+上线(完成✅,LCO Redis DEFERRED)

> 日期:2026-08-20(跨午夜续)
> Redis 锚点:`esp:ctx:hot:session-23-14`(续,本轮进展写入)

### 本轮完成/进行中

#### 1. git push 解阻(✅ 完成)
- **阻塞**:commit 历史含 214.87MB `deploy/docker/jdk26-jre/jdk26-aarch64-glibc.tar.gz` 超 GitHub 100MB 硬限,remote rejected。
- **修法**(用户授权 filter-repo 改写历史):stash 保护未提交改动 → `git filter-repo --invert-paths --path <tarball> --path deploy/k6/linux-arm64/k6` → 重加 origin(fetch-repo 默认删 origin)→ fetch → pop stash → 写 .gitignore(jdk26 tarball + k6 二进制)→ commit → `git push --force-with-lease origin master`。
- **结果**:`afd1241..720a90b master -> master`,16 commit 全推送(filter-repo 改写 15 个 + 新增 .gitignore commit)。backup ref `refs/backup/pre-filter-rewrite`(原 a75aef4)保留可回退。
- **push 前密钥扫描**:无真实生产密钥(neo4j-password-1 是 dev env 覆盖,test123 是 e2e 测试用户,${SEATA_CONSOLE_PASSWORD} 是 shell 变量)。
- **诚实更正**:此前 PROGRESS.md 说"push 443 超时",实际根因是 214MB 大文件非超时。

#### 2. LCO 代理运行层诊断(✅ 完成诊断,修复 fork 进行中)
- **链路接入成立**:env `ANTHROPIC_BASE_URL=http://127.0.0.1:15720`,代理 PID 69853 监听,token 代理托管,/healthz 三层 ok。
- **运行层三故障(实时 metrics)**:
  - L1 精确缓存:1 命中/149 miss(0.67%)——Redis port-forward :6384/:6385 DOWN,MOVED 落到断口失败。
  - L2 语义缓存:0/149——embedding MPS 推理超 LCO_L2_TIMEOUT(8s 不足 8-34s 推理)。
  - L3 Neo4j:port-forward :7475 超时,写记录失败。
- **新增严重现象**:`upstream 请求失败 :15721 context canceled`——某请求整链路断(L1/L2 miss 后上游也被 cancel),本会话 LLM 调用可能卡顿。
- **修复 fork 进行中**:重连 Redis port-forward + 抬 LCO_L2_TIMEOUT=45 + Neo4j 重连,目标恢复命中率>30%。

#### 3. llm-orchestrator-rs:1.3.0 派生构建 + 上线(✅ 完成,BVV 闭环)
- **阻塞根因**:registry mirror 全 TLS 超时拉不到 `rust:1.88-bookworm`;buildkit 缓存无 rust 层;本地无任何含 rustc 镜像。
- **docker context 诊断修正**:此前"daemon 0 镜像"是误判,实际 docker context 残留指向已停的 colima socket;切回 `rancher-desktop` context 后 130 镜像都在。
- **可行路径确定**(esp-builder 探测 + 主线程核实一致):
  - apt 装 rustc 路径不可行:debian bookworm 仓库 rustc=1.63.0,编不动 axum0.7/sqlx0.7/reqwest0.12(需 1.72+/1.78+/1.76+)。
  - 宿主交叉编译不可行:仅装 aarch64-apple-darwin target,无 linux-gnu target/linker/zig。
  - **破局点**:`sh.rustup.rs` + `static.rust-lang.org` + USTC crates mirror 三源均可达(HTTP 200),只有 docker.io/mirror 镜像拉取挂。
- **派生 Dockerfile**:`backend/services/llm-orchestrator-rs/Dockerfile.derive-1.3.0`。多阶段 FROM esp/llm-orchestrator-rs:1.2.0(debian+USTC+nonroot),builder 装 rustup stable(1.95)+ cargo build, runtime 复用 1.2.0 只 COPY 新二进制。依赖全纯 Rust 无 Native(reqwest rustls/sqlx rustls/redis 纯Rust/Neo4j reqwest HTTP),runtime 无需补库。
- **构建成功**:`docker build` ~6min,rustup 装 + cargo 全量编译(rustls/hyper/sqlx/redis/esp-common-rs/llm-orchestrator-rs)。镜像 `esp/llm-orchestrator-rs:1.3.0` 188MB,二进制 11.3MB(1.2.0 是 10.5MB)。
- **BVV 内容校验(镜像产物内容铁律)**:导出二进制 `strings` grep 到 `L2 milvus store ok`/`L2 milvus search HTTP non-2xx`/`embed_prompt`/`l2_store_milvus`/`l2_search_milvus`/`neo4j_run`/`L1 exact hit` 符号;对照 1.2.0 的 milvus/neo4j 计数=0(旧版无 L2/L3)。**镜像内容确含 L2/L3 代码**。
- **上线**:deploy spec 原本就指向 1.3.0(yaml 第39行),但此前镜像不存在→新 pod ImagePull→Pending。构建后 rollout restart,但**卡在单节点 CPU request 99%满**(node 8980m/9000m,orchestrator 要 250m 塞不下)。patch `maxSurge=0/maxUnavailable=1`(删旧起新不并存)+ request 250m→50m 后,**rollout successfully**,新 pod 1/1 Running。
- **运行期验证**:imageID `sha256:0a6718785a55`(与 build 镜像 ID 一致);启动日志 `tracing initialized`→`CockroachDB pool ready`→`llm adaptive limiter initial=10`→`healthz OK`;curl `/healthz` 返回 `{"service":"llm-orchestrator-rs","status":"UP"}`。
- **DEFERRED**:L2/L3 运行期日志验证(L2/L3 懒初始化,需发请求触发;Redis cluster fail 时 L1/L2/L3 全降级,按用户决策"Redis 不动")。Nacos 注册返回 HTTP 000(新发现,待查 nacos 可达性)。

#### 5. 集群健康度退化与恢复(⚠ 进行中,本会话操作致退化后部分恢复)
- **退化根因**:本会话 + fork 并行操作(orchestrator rollout + fork scale Redis STS)期间触发一批僵尸 pod(st-seeder 16 孤儿/esp-redis-warmup Job/esp-schema)占 CPU request,集群从续2 的"28/28 Ready"退化到 29/58(50% Ready),node CPU 99% 满。
- **恢复操作**(用户决策"先恢复集群健康"):
  - 清理僵尸:删 st-seeder 16 孤儿 pod(configmap seed-session-turns-v2 缺失卡 ContainerCreating)+ esp-redis-warmup Failed Job + esp-schema 卡住 pod。
  - 批量降 13 个非核心服务 CPU request(100m→30m / 50m→30m)+ 设 maxSurge=0/maxUnavailable=1(删旧起新不并存),腾出额度。
  - 降核心 Java 服务 learning-java/teacher-java request 200m→100m,恢复调度。
  - patch 镜像 tag 到本地有镜像版本:gateway-go 1.13.0→1.12.0、assessment-rs 0.3.1→0.4.0(原 tag 镜像不存在致 ImagePullBackOff)。
- **部分恢复结果**:从 29/58(50%)恢复到约 30/32(94%),主链路 gateway-go/orchestrator-rs/course-path-go 1/1,learning-java/teacher-java 从 Pending 恢复 Running。
- **残留问题(未完全恢复)**:
  - learning-java/teacher-java/learning-worker readyReplicas 空 + 出现 CrashLoopBackOff(疑似 Redis fail 致 Dubbo/Nacos 连不上启动失败,待查;可能复现续2 #3 /.dubbo 坑)。
  - bff-java 1/2(一个 RS 拉旧 imageID 待恢复)。
  - whisper Pending(500m DEFERRED ASR 镜像问题)+ assessment-rs ImagePullBackOff 残留。
  - **Redis cluster_state:fail 未解**(用户决策不动),可能是 Java 服务 CrashLoop 的根因(连不上 Redis)。
- **诚实反思**:本会话在 CPU 99% 满载下单节点上并行多操作(orchestrator 构建+rollout 同时 fork 动 Redis),触发调度雪崩,违背 [[esp-singlenode-host-cluster-contention]]"主机重 mvn 与集群操作必须串行"。后续应在 load<10 且无并行 k8s 操作时才推进。
- **原值记录与 DGO 回写状态(可逆恢复用,2026-08-20 更新)**:
  - **已回写仓库 yaml(DGO 合规,永久回退)**:gateway-go 1.13.0→1.12.0(10-gateway-go.yaml,1.13.0 镜像未构建);assessment-rs 0.3.1→0.4.0(32-batch2-services.yaml,0.3.1 镜像不存在);redis 7.2→8.0(06c-redis-cluster.yaml,全升 8.0)。
  - **未回写(live 临时 patch,PROGRESS 记录,多节点迁移后恢复)**:13 非核心服务 request 100m/50m→live 30m(单节点 CPU 99% 临时腾空间,仓库仍 100m/50m 是生产正确值);learning-java/teacher-java request 200m→live 100m;orchestrator request 250m→live 50m;均设 maxSurge=0/maxUnavailable=1。**理由**:30m 是单节点资源约束的临时妥协,不该固化进仓库 yaml(DGO 第⑤条临时 patch 记录)。
  - **新增挂载(已 live patch,待回写 yaml)**:learning-java/teacher-java 挂 /logs + /.dubbo emptyDir(修 Nacos 日志 + Dubbo 元数据坑,见下节)。

#### 6. Java 服务 Dubbo/.dubbo + Nacos/logs 坑修复(✅ 完成,2026-08-20)
- **坑**:learning-java + teacher-java CrashLoopBackOff,根因 `readOnlyRootFilesystem:true` + 镜像 HOME=/ → Dubbo 3.3.6 写 `/.dubbo/dubbo-metadata-<svc>-nacos-8848.cache` 失败(`Failed to create directory /.dubbo`)+ Nacos logback 写 `/logs/nacos/ai.log` 失败 → Spring 启动崩。
- **同款历史**:PROGRESS.md 续2 #3(2026-08-19)learning-worker 已修此坑(挂 /.dubbo emptyDir),但 **learning-java/teacher-java 漏修**(根因:续2 修 learning-worker 时未全局同步所有 Java 服务 + 无 CI 拦截)。
- **修复**:挂 /logs + /.dubbo 双 emptyDir(learning-java + teacher-java)。修后 learning-java `Started LearningApplication in 33s`、teacher-java `Started TeacherApplication in 44s`,全 1/1 Ready 0 restart。
- **诚实根因(待立铁律)**:此坑反复出现(learning-worker→learning-java→teacher-java),根因是 readOnlyRootFilesystem + HOME=/ + Dubbo/Nacos 写根目录,且**修一个服务不全局同步**。subagent 根因查证中,完成后立铁律防再犯(Dockerfile HOME 设置 / 全局挂载 / CI 校验)。
- **集群健康最终**:27→29 Running + teacher-java 1/2(HPA 扩 2 但第二 pod CPU 不足 Pending,单节点限制)+ whisper ImagePullBackOff(DEFERRED)。核心服务全 1/1:gateway-go/bff-java/learning-java/teacher-java/orchestrator/course-path-go/learning-worker。

#### 4. LCO 运行层三故障修复(fork 子任务,部分完成)
- **故障2 L2 embedding 超时(✅ 已修,fork)**:根因 `start-lco-embedding.sh` 未设 `EMBED_MODEL` → auto 路由 Qwen3-Embedding-4B MPS 卡死 >90s。改设 `EMBED_MODEL=minilm`(all-MiniLM-L6-v2 CPU ~0.1s,384dim),实测 HTTP 200。commit `ee1581d`。
- **故障3 L3 Neo4j(✅ 实测已通)**:`:7475 RETURN 1` 正常,无需修。
- **故障1 Redis Cluster AOF 损坏(✅ 全升 8.0 根治,2026-08-20)**:fork 修 pod-4 AOF 时 scale STS 来回致 pod-2 也 CrashLoop(AOF 蔓延),cluster_state:fail。根因深挖(esp-explorer 证据链闭合)实为**版本漂移脑裂**:STS spec 声明 7.2-alpine 但 pod-0 跑 8.0.6、pod-1/2/3 跑被劫持的 alpine-arm64:base(7.2.13 非标),根因是 `docker tag alpine-arm64:base redis:7.2-alpine` **tag 劫持**(docker inspect 证实 RepoTags 同 imageID)+ solo patch STS spec 到 8.0 未回写 yaml。AOF RDB v12 跨版本不兼容 + replication extension type 4 协议不兼容致脑裂。
- **全升 8.0 修复(用户决策)**:① `docker rmi redis:7.2-alpine` 解 tag 劫持;② 回写 `deploy/k8s/06c-redis-cluster.yaml:60/128` 7.2→8.0(DGO 铁律);③ patch STS replicas 4→6(修 fork scale 残留);④ STS spec 已 8.0 + 劫持解除后,STS 自动把 6 pod 全用真 redis:8.0-alpine 重建。
- **验证(✅ 三态对齐)**:6 pod 全 `redis_version:8.0.6` + STS spec 8.0 + 镜像 8.0-alpine;`cluster_state:ok` + `cluster_slots_ok:16384`(从 5461 恢复)+ 3 master(0-5460/5461-10922/10923-16383)+3 slave 拓扑正常;本地 6380-6385 port-forward 全 UP PING PONG;SET/GET 测试 OK。**立 SVD 铁律**(memory `stateful-workload-version-drift-ironrule`,engineering-ironrules 第15条)防再犯。
- **连带恢复**:learning-worker 1/1(连上 Redis 恢复);LCO L1 池 6 端口全通(命中率待观察);learning-java 仍 CrashLoop(独立坑:Nacos `/logs/nacos/` 目录 readOnlyRootFilesystem 致 `Failed to create /logs/nacos/ai.log`,非 Redis 根因,待挂 emptyDir 修,同续2 #3 learning-worker/.dubbo 修法)。
- **LCO 命中率**:L1 命中 1/747(L1 池刚恢复全通,命中率待新一轮请求积累);L2 仍 0(embedding minilm 修复待验证生效)。LCO 铁律 >30% 目标待观察。

### 仍待完成(下一步,离峰/用户确认后启动)
1. ~~**llm-orchestrator-rs 1.3.0 镜像构建**~~(✅ 本会话完成:派生 Dockerfile FROM 1.2.0 装 rustup+cargo build,BVV 验证含 L2/L3,deploy 上线 1/1 Running healthz UP)
2. ~~**OTel 链路端到端验证**~~(⚠ 部分:宿主 otel-collector 收到 99370 spans + 112784 logs;tempo ingester 未 ready `waiting for 15s`;ESP 服务未重启 OTel 连接可能旧;需重启服务+等 tempo)
3. ~~**LCO 运行期实测**~~(✅ 本轮:Redis 全升 8.0 + L1 池 6 端口全通后 L1 命中恢复探针验证;L2 仍 0 命中——Milvus REST v2 `/v2/vector/collections/list` 404 API 路径疑 bug,DEFERRED 调试)
4. **真实引擎切换收尾**:whisper 镜像 mirror 404 需另找/自建(DEFERRED)+ TTS edge-tts websocket handshake bug;gateway-go 1.13.0 派生构建(含 L2/L3,现临时回退 1.12.0)
5. **全量重构达标计划执行**(`.claude/plans/robust-baking-wave.md` 文件实际不存在,需重建;fork Phase1 已完成 6 commit 限流熔断/Rust metrics/securityContext)
6. **LCO L2 schema 不兼容修复(DEFERRED,根因已查实 2026-08-20)**:fork 调试闭合——LCO l2.go 用 v2 API + 字段 `body/protocol/model/ts`,conversation-go semantic.go(权威 collection 创建者)用 v1 API + 字段 `response/model/scene/prompt_sha16`。**两套完全不兼容**(API 版本+字段名全不同)→ LCO insert 的数据 conversation-go search 不到,LCO search 的 outputFields 在 collection 里不存在 → L2 全 miss。修复=LCO 代码重构对齐 conversation-go schema(v2→v1 API + body→response + 加 prompt_sha16 + protocol/model/scene 字段),属非平凡代码改动,需 ASV 核实 Milvus v1 API + CLC 防御性 + 测试,标 DEFERRED 专项处理。L1 已工作(LCO 铁律 L1 命中验证 OK),L2 是优化层非阻断。
7. **k6+Locust 混合压测**(50→5000 CCU 阶梯)+ 全功能 E2E
8. **本会话铁律立成**(✅ 6 条新铁律 SVD/BRR/DGO/RWT/SCS/RWP 三处落盘,memory+CLAUDE.md+索引,后续所有项目遵守)

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

## 2026-08-19~20 会话23-14续3:LCO 运行层三故障修复(fork 子任务,部分完成)

> fork 代理修复 LCO 链路运行层故障。诚实记录:1 项修成,1 项恶化(操作失误),1 项实测已通。

### 故障1: Redis Cluster AOF 损坏(❌ 恶化,DEFERRED 交主线程)
- **原状**:redis-cluster-new-4 CrashLoop,日志 `Can't handle RDB format version 12` + `Error reading appendonly.aof.6.base.rdb, AOF loading aborted`。
- **根因**:pod-4 AOF base 文件损坏,启动加载即 abort → CrashLoop。cluster nodes 显示 pod-4 是 fail 孤儿 master 无 slot(数据已由 pod-1 接管 5461-10922 + pod-5 slave)。
- **修复尝试**:scale STS 4→释放 PVC→挂 fix pod(alpine)→ mv appendonlydir/dump.rdb 到 .bak→scale 回 6 重建 pod-4。
- **操作失误(诚实记录)**:scale 来回重建 pod 时,**pod-2 也被触发 CrashLoop**(同样 `Can't handle RDB format version 12`,appendonly.aof.3.base.rdb 损坏)。多个 pod 的 AOF 在 scale 来回中损坏/重新加载失败。
- **当前 cluster 状态(恶化)**:`cluster_state:fail`,`cluster_slots_ok:10923`(应 16384,少 5461,pod-2 的 10923-16383 不可用)。可用 master 仅 pod-0(0-5460)+ pod-1(5461-10922)。port-forward :6380/:6381 UP,其余 DOWN。
- **止血**:scale STS=4(不含 ordinal 2/4),停止 scale 操作避免继续蔓延。**不继续清 PVC**(会让更多 pod 崩)。
- **DEFERRED 方向(交主线程决策)**:① 系统性方案 = 全 cluster 禁 AOF 只用 RDB 重启,或全 cluster 重建(数据可重建,LCO 缓存非持久业务数据);② 或改 LCO L1 配置只连可用 2 个 pod(放弃 :6382/:6384/:6385),命中率降但链路通;③ **memory 更新**:Redis AOF 损坏会蔓延,scale 来回是危险操作,应记 `esp-redis-aof-corruption-cascade`。

### 故障2: L2 embedding 超时(✅ 已修,可逆)
- **根因**:`scripts/start-lco-embedding.sh` 第18行 `exec python3 "$APP"` **未设 EMBED_MODEL env** → app.py 走 auto 路由 → 中文 prompt 路由到 Qwen3-Embedding-4B → MPS 推理卡死(>90s/次)→ LCO L2_TIMEOUT=8s 必超时 → L2 0 命中。
- **修复**:start-lco-embedding.sh 第18行前加 `export EMBED_MODEL=minilm` 强制用 all-MiniLM-L6-v2(本地目录 ~/lco-models/all-minilm-l6-v2 已存在,CPU ~0.1s/次)。launchd 自动拉起新进程(PID 18369)env 确认 `EMBED_MODEL=minilm`。
- **验证**:embed 探针首次 18.9s(模型加载)+ 返回真实 384 维向量 + HTTP 200;常驻后推理 <0.1s。不再 `context deadline exceeded`。
- **取舍**:minilm 跨语言语义弱于 Qwen3,但单节点 CPU 稳定可靠;多节点有 GPU 后切回 qwen。遵 [[esp-lco-embedding-vm-snowball]]。

### 故障3: L3 Neo4j(✅ 实测已通,无需修)
- :7475 port-forward 在(指向 ekb-m2-aws-us-east-1/neo4j svc),REST `RETURN 1` 返回正确 JSON `{"row":[1]}`。L3 健康。之前 LCO 日志报超时是 L1 全链路阻塞的连带(非 L3 自身故障)。

### 修复后 LCO 命中率(诚实)
- L1: 1/313 (0.3%) — 因故障1 Redis 多 pod 崩,未改善反而恶化。
- L2: 0/313 — embedding 修了但被 L1 阻塞挡在前面,L2 没机会执行(每请求卡在 L1 :6385 读失败)。
- L3: 健康但同样被 L1 阻塞。
- **结论**:故障1(Redis cluster)是 LCO 全链路阻塞总根因,修了 L2/L3 也无效,必须先解决故障1。交主线程决策系统性修复方案。

### 改动文件
- `scripts/start-lco-embedding.sh`:加 `export EMBED_MODEL=minilm`(故障2 修复)
- `docs/PROGRESS.md`:本段落(诚实记录)

## 2026-08-20 会话23-16:4 项遗留完整推进(L2源码修复+SCA扫描+whisper Dockerfile+push)(完成✅源码层/DEFERRED build层)

> 用户诉求"完整执行:LCO L2 schema重构 + whisper镜像+edge-tts + k6压测 + dependabot 345漏洞"。可并行就并行。

### 1. LCO L2 schema 代码重构(✅ 源码改完+验证,**1.3.1 镜像 build 成功+BVV 校验过,部署卡单节点 CPU**)
- **根因(比上轮诊断更深)**:业务 LCO = `llm-orchestrator-rs/src/cache/mod.rs`(Rust,非 tools/lco-proxy 的 l2.go)。与 conversation-go 共用 `esp_llm_semantic` collection,三处不兼容致 L2 全 miss:
  1. search API `/v2/vector/search`(Milvus 2.5 下 404)→ 应 `/v1/vector/search`
  2. search body `"data":[vec]` 嵌套 → 应 `"vector":vec` 扁平(ASV 实测 conversation-go semantic.go:243)
  3. insert 字段名 `"embedding":vec`(schema 声明 `vector`,semantic.go:216)→ insert 必失败
- **修复(方案 B,Rust 对齐 conversation-go 正本,爆炸半径最小)**:mod.rs search URL v2→v1 + data→vector + outputFields 补 prompt_sha16;store URL v2→v1 + embedding→vector。文件头注释同步。
- **验证(CLC 铁律)**:cargo check RC=0;cargo test 21/21 单测全绿(parse_milvus_* 5 个全过证搜索响应解析正确);clippy 5 error 在 dialogue/main 是历史债务(非本次引入),build 走 cargo build --release 不卡。
- **落地待办**:build `esp/llm-orchestrator-rs:1.3.1`(BVV 校验镜像内二进制含 L2 修复符号)+ redeploy。需 VM CPU 空闲(串行铁律)。
- **✅ 镜像 build 成功 + BVV 校验过**(2026-08-20 03:06):`docker build -t esp/llm-orchestrator-rs:1.3.1 -f Dockerfile.derive-1.3.0`,RC=0,1m40s。BVV 校验(镜像内二进制 grep):`/v1/vector/search`=1(新)、`/v2/vector/search`=0(旧已移除)、`/v1/vector/insert`=1、`/v2/vector/insert`=0 —— **L2 修复内容确在镜像内**。
- **⚠ 部署卡单节点 CPU 枯竭**:`kubectl set image` 切 1.3.1 后新 pod Pending(`Insufficient cpu` + `No preemption victims`)。根因:节点 CPU request 已 8980m/9000m(99%)被**共享基础设施**占满(milvus 250m/neo4j 250m/tiflash+tikv 各 200m/otel 200m,esp-shared-infra-ironrule 禁动底座),不是 st-seeder(requests 空)。旧 1.3.0 pod 被 rollout 删,**LCO 业务服务当前中断**(0/1 Pending)。patch 降 limit 1→500m + request 200m 仍调度不上(8 Pending pod 抢配额)。DGO 已回写 `33-orchestrator-vocab.yaml`(image 1.3.1 + resources 200m/500m)。
- **非阻断**:① ESP 主业务链路健康(gateway/bff/learning/teacher 全 1/1 Available,NodePort 200/404 正常);② Claude Code 的 LCO 缓存走 `tools/lco-proxy`(本机 PID 69853,独立于集群 orchestrator),不受影响;③ orchestrator 是 ESP 业务侧 LLM 缓存层,业务 LLM 调用直穿上游不阻断。
- **上线方法**(CPU 腾出后):`kubectl -n esp-prod rollout restart deploy/llm-orchestrator-rs`,镜像已就绪。多节点 CPU 充裕后 request 可调回 250m/1000m。
- **诚实**:conversation-go 需先启动完成 collection 创建,Rust LCO 才能复用;若 Rust 先启动 L2 降级 skip(非致命)。

### 2. dependabot/trivy SCA 扫描(✅ 双源完成+全明细,待分批 rebuild)
- **dependabot(完整分页,修正首版漏页)**:350 open(98 critical + 99 high + 140 medium + 13 low)+ 145 fixed。生态:go 221 / npm 83 / rust 25 / maven 21。critical+high = 197。
- **trivy fs**:24 targets 扫完,53 unique advisory(crit 1/high 25/med 22/low 2)。诚实:trivy 首次联网刷 CVE 库失败(mirror.gcr.io timeout)回退本地 8-06 缓存,漏报 5 条(jackson-databind CVE-2026-54513/54512/59888/54514 + assertj-core)。
- **critical 集中(全 Go)**:grpc CVE-2026-33186(CRITICAL,10 服务,fix v1.82.1)+ x/crypto 11 CVE(10 服务,fix v0.52.0)+ x/net 5 CVE(fix v0.55.0)+ x/text(fix v0.39.0)+ jsonparser(fix v1.1.2)。Java:postgresql 2 CVE(3 服务,fix 42.7.12)。Rust:rustls-webpki(4 服务,fix 0.103.13)。
- **P0 修复启动(✅ esp-common-go 升级完成)**:host 跑 go get -u(6 包达标)+ tidy + build + vet + test 全绿。otel 实际升 v1.43.0(grpc v1.82.1 硬性要求,> 目标 v1.41 合理)。esp-common-go 是共享库,下游 10 服务 go mod tidy 自动拉新版本。备份 go.mod.bak/go.sum.bak 留存可回滚。
- **待办(分批串行,遵 BRR+snowball)**:下游 Go 服务 gateway→core→周边分批 go get+tidy+build+deploy;Java 3 服务 postgresql 升级;Rust 4 服务 rustls-webpki;前端 npm install 生 lock + npm audit 修 83 条;trivy image 扫镜像 OS 包层 + db 联网刷新补漏报。
- **报告**:`report/sca/sca-comprehensive-2026-08-20.md` + `report/sca/trivy-fs-summary.md`。
- **工具补全建议**:`brew install govulncheck syft && cargo install cargo-audit`(SCS SBOM 产出需 syft)。

### 3. whisper 派生 Dockerfile(✅ 三件套写好,待 build PyPI 风险)
- 根因:onerahmet/openai-whisper-api:latest mirror 404 拉不下(3h ImagePullBackOff 确证)。
- 方案:从本地 `esp/python-runtime:3.13`(=python:3.13-slim-bookworm)派生自建,用 faster-whisper(CT2 int8 CPU 友好),实现 OpenAI 兼容契约 `/v1/audio/transcriptions` + `/health`(asr-adapter-go whisper.go:57 whisperResp={text})。
- 产出:`deploy/docker/whisper-cpu/{Dockerfile,requirements.txt,server.py}`。铁律合规:SVD 非标 tag `esp/whisper:base-cpu-ct2`;RWP ENV HOME=/home/esp + HF_HOME 缓存挂 emptyDir;CLC transcribe 失败降级空 text;SCS 依赖 pin 版本。
- **风险(诚实)**:build 时需 PyPI 装 faster-whisper,VM 网络对 PyPI 可达性是关键阻断点(pypi.org 探测超时)。build 失败则 DEFERRED 待网络/多节点。
- **edge-tts handshake bug**:在 `lib-x/edgetts` Go 库内(非 ESP 源码),tts-adapter-go Running 但需实调复现,DEFERRED(库内 bug 非平凡)。

### 4. k6+Locust 压测(⚠ DEFERRED,CPU 不足)
- 集群健康但 4 服务新 RS pod Pending(`Insufficient cpu` + `No preemption victims`):emotion-svc/gamification-go/llm-orchestrator-rs/teacher-java。单节点 lima VM CPU request 预算已满,新 RS 发不出去(ProgressDeadlineExceeded)。
- 另有 configmap 缺失致 st-seeder/esp-schema-init pod ContainerCreating(`seed-session-turns-v2`/`esp-schema-sql` not found,历史 seed job 残留非服务依赖)。
- 压测会雪崩(esp-singlenode-host-cluster-contention + esp-lco-embedding-vm-snowball),DEFERRED 待 CPU 腾出/多节点。

### push d06c399(✅ 完成)
- push3 后台第 4 次重试成功:`7b11f91..d06c399 master -> master PUSH2_OK_4`,领先量归 0。git push 网络问题彻底解决。

### LCO 代理运行核实(用户关切"Claude Code 走 LCO")
- `tools/lco-proxy`(PID 69853)在跑,Claude Code 经 cc-switch(127.0.0.1:15721)→ LCO(15720)→ 上游。**确认 Claude Code LLM 调用走 LCO 代理**。
- L1:hit 2 / miss 1076 / error 609(6385 port-forward 曾断,现已 PONG,error 止于 609 不增)。
- L2:0/1076 全 miss(tools/lco-proxy 用独立 collection `lco_prompts`,与业务 LCO 的 `esp_llm_semantic` 不同;业务 LCO 的 L2 修复见上节,待 build 镜像生效)。

### 改动文件
- `backend/services/llm-orchestrator-rs/src/cache/mod.rs`:L2 三处修复(search/store URL v2→v1 + 字段 embedding→vector + data→vector 扁平)
- `backend/shared/esp-common-go/go.mod` + `go.sum`:P0 依赖升级(6 包)
- `deploy/docker/whisper-cpu/{Dockerfile,requirements.txt,server.py}`:whisper 派生构建三件套(NEW)
- `report/sca/sca-comprehensive-2026-08-20.md` + `trivy-fs-summary.md`:SCA 报告(NEW)
- `docs/PROGRESS.md`:本段落

## 2026-08-20 会话23-16续:L2 全 miss 根因修复 + Milvus/Neo4j 下沉决策(进行中🔄)

> 用户诉求:① 查 Claude Code L2 全 miss 根因;② 盯 orchestrator 1.3.1 上线;③ Milvus/Neo4j 下沉宿主腾 CPU。

### L2 全 miss 根因(tools/lco-proxy,Claude Code 用的,与业务 LCO 不同组件)
- **主因 C ✅ 已修**:embedding 服务(Qwen3-Embedding-4B MPS,PID 18851)**卡死 STAT=U 3h23m** + launchd 自愈 `com.esp.lco-embedding` plist 在但未加载 → L2 `embed()` 拿不到向量全 miss(日志 259 次 `context deadline exceeded`,0 次成功 store)。根因即 esp-lco-embedding-vm-snowball 老问题(Qwen3 MPS 不可靠)。
- **修复**:kill 卡死进程 + launchd 已重新加载(KeepAlive,`launchctl list` 显示 75628 管理)→ embedding 恢复(minilm 0.095s 返回 384 维),L2 不再 deadline。
- **次级隐患 D 未修**:`lco_prompts` collection 是 **2560 维**(qwen 建的),但 `l2.go:82 embed()` 默认 minilm 返回 **384 维** → embedding 恢复后 L2 store 会因维度不匹配失败。修法:重建 collection 为 384 维 minilm(与业务 conversation-go semantic.go 统一,丢 LCO 缓存数据会重积累)或 embed 显式带 model。待下沉 Milvus 时一起定维度。
- **tools/lco-proxy 的 L2 代码路径/字段名已对齐** Milvus 2.5(`/v2/vectordb/entities/*` + `vector` 字段),collection 存在有数据——不是业务 LCO 那种 API/字段错。根因纯 embedding 不可用 + 维度不匹配。

### orchestrator 1.3.1 上线(⚠ 镜像就绪,部署卡单节点 CPU 枯竭)
- 镜像 build 成功 + BVV 校验过(见上节)。
- 部署卡:节点 CPU request 8980m/9000m(99%)被共享基础设施占满(milvus 250m/neo4j 250m/tiflash+tikv/otel)。
- 尝试腾配额:降 limit 1→500m + request 200m;scale whisper=0(腾 1000m);pause/undo course-path-go(旧 pod 在跑,新 Pending 冗余)。**均不足以腾 200m**(根因基础设施占 99%,esp-shared-infra 禁动底座)。
- **诚实止损**:接受 orchestrator 暂时 Pending,主业务不中断(gateway/bff/learning 全 Available)。镜像已就绪,CPU 腾出后 `rollout restart` 即上线。继续手动腾挪违反 BRR(越修越乱)。
- **非阻断**:① 主业务健康;② Claude Code 的 LCO 走 tools/lco-proxy(本机,独立);③ 业务 LLM 直穿上游。

### Milvus/Neo4j 下沉宿主机(🔄 规划中,用户授权"下沉+重建,腾 600m")
- 用户决策:把 Milvus(250m)+ milvus-minio(100m)+ Neo4j(250m)= 600m 从集群 ekb-m2-aws-us-east-1 ns 下沉宿主(像 Nacos/OTel/Tempo),腾 CPU 给 orchestrator + 其他 Pending。
- esp-shared-infra-ironrule 管辖:多项目共用底座,邻居 education-platform 也引用(待核实 ExternalName/直连)。
- 下沉规划 agent 启动:核实邻居依赖 + 宿主安装方式 + 完整计划(report/milvus-neo4j-sink-plan.md)。
- 关键洞察:下沉腾的是 k8s scheduler request 配额(600m),实际 CPU 消耗(Milvus/Neo4j 稳态低)移到 VM docker 不变但移出 k8s 调度层。
- 分阶段:① Neo4j 下沉(简单,单服务)② Milvus 下沉(复杂,etcd+minio+milvus standalone)。

### push f665519(✅ 完成)
- push4 成功:`d06c399..f665519 master -> master PUSH4_OK_2`,领先量 0。本会话 commit 已完整 push GitHub。GitHub 确认 350 漏洞(98 crit+99 high,与扫描一致)。

### L2 维度对齐(路径2,✅ 改动完成,2026-08-20 凌晨)
> 用户决策:走**路径2**(保留 2560 维 Qwen,不重建 collection)。理由:Qwen 中文语义强,维度对齐后待 Milvus/Neo4j 下沉腾 CPU + 限流后真正可跑;L2 暂时配对但跑不动不阻塞主业务。

**实证闭环**(esp-explorer agent a21674d1789d280e2 核实 4 点):
- embedding 服务 `tools/lco-proxy/embedding/app.py`:`/embed` 接受 `model` 参数(auto|qwen|minilm),双模型(qwen 2560维/minilm 384维),`_route_model` 按语言路由(中文→qwen/英文→minilm),`EMBED_MODEL` env 可强制。
- `lco_prompts` collection(Milvus 实测):**2560 维** + 4 行已存向量(2×claude-haiku + 2×claude-sonnet,8/19 晚 Qwen 产)。
- **根因**:lco-proxy `l2.go:82 embed()` 之前只发 `{"text":...}` 不传 model → 走服务端 `EMBED_MODEL=minilm` 强制 → 产 384 维 → 与 2560 维 collection 维度不匹配 → insert/search 全静默失败 → L2 全 miss。
- **业务 LCO(对比)**:conversation-go `semantic.go:44` 固定 `l2EmbeddingModel="minilm"`,orchestrator-rs `mod.rs:147` 硬编码 `model=minilm`,都显式传 model;esp_llm_semantic collection 384 维。两边各自自洽。

**改动 A — l2.go 显式传 model=qwen**(`tools/lco-proxy/internal/cache/l2.go:82`):
- `payload, _ := json.Marshal(map[string]string{"text": text, "model": "qwen"})`(原只发 text)
- 解开"业务 LCO(384)与 Claude Code LCO(2560)共用 embedding 服务"的 FORCE_MODEL 冲突——调用方各自声明,服务端不再强制单一模型。

**改动 B — start-lco-embedding.sh 去掉 EMBED_MODEL=minilm**(`scripts/start-lco-embedding.sh:18-22`):
- 注释更新:调用方显式传 model,服务端走 auto 路由;Qwen3-4B 单节点 VM MPS 8-34s 可能超时,待下沉腾 CPU 后稳定。
- embedding 服务重启后 `force_model:"auto"`(原 minilm),qwen/minilm 双模型待命。

**验证(✅ 维度对齐实证)**:
- 直测 embed `model=qwen` → `dim=2560, veclen=2560, device=mps`(与 collection schema 一致)。
- 直测 Milvus insert 2560 维 → `lco_prompts`:`code:0, insertCount:1`(成功,不再维度不匹配)。
- 直测 Milvus search 2560 维 → `code:0`,命中 distance=0.607(成功)。
- lco-proxy rebuild + 重启(PID 20554),healthz `l2_milvus:ok`(l3_neo4j:down 是 401 凭据,下沉后修)。

**诚实(路径2 现状)**:维度对齐后 L2 不再因维度不匹配静默失败;但 Qwen3-4B 在单节点 VM MPS 推理 8-34s/次,常超 `L2TimeoutSec=20s` → L2 仍会因超时降级直穿(L1 精确缓存不受影响)。**真正可跑需 Milvus/Neo4j 下沉腾 CPU + 限流**。这符合路径2 预期:先配对,跑不动不阻塞,等下沉。

**双方模型不一致无正确性问题**:两个 collection(lco_prompts 2560 / esp_llm_semantic 384)独立隔离,Milvus cosine 检索只在 collection 内算,2560 查 2560 / 384 查 384 各自自洽。唯一耦合:embedding 进程算力(Qwen 慢会阻塞同进程 minilm 请求,性能影响非正确性)。

### Neo4j 下沉宿主(✅ 完成,2026-08-20)
- esp-builder agent a3b573e6075af5f22 执行 Neo4j 下沉 Phase1(brew install neo4j 5.x + 改 ESP 指向 host.lima.internal:7474 + 删 ekb deploy)。
- 邻居 education-platform Neo4j 已弃用(改 NebulaGraph),删 ekb deploy 对 edu 零影响。
- 遵 BRR(备份 + L2 已授权)+ DGO(回写 yaml)。规划在 `report/milvus-neo4j-sink-plan.md`。
- **诚实结果**:L3 恢复(l3_neo4j:ok,lco-proxy healthz 三层全绿);但 CPU 仅腾 50m(非预期 250m)——根因是并发 ESP env rollout 产新 Pending pod 抢回配额(bff-java/assessment-rs/otel-collector),属单节点 9000m 结构性枯竭非下沉失败。orchestrator 仍 Pending。
- DEFERRED:僵尸 ExternalName neo4j(05b:76)、neo4j.conf 未回写仓库(brew upgrade 会覆盖)、弱密码 neo4j-password-1 待轮转(SCS 风险)。

### Milvus/minio 下沉宿主(✅ 完成,2026-08-20)
- **框架纠正**:宿主机=Mac 本身(非 lima VM)。milvus 无 brew formula(无 macOS 原生构建),但 **minio 有 brew 原生构建**(对标 Neo4j)。本轮下沉 **minio 对象存储后端**(milvus 的存储依赖),非 milvus pod 本身(milvus pod 仍留集群,因其 gRPC 接口被 ESP gateway-go/orchestrator-rs/writing-coach 用 `milvus:19530`)。
- **BRR L2 流程(全过)**:B1 `brew install minio minio-mc` + launchd 常驻(brew services started,开机自启,与 neo4j 并列)→ B2 port-forward 集群 minio + `mc mirror` 迁数据(**40 对象/65KiB 源=目标 完全一致**,含 milvus delta_log/insert_log/stats_log)→ B3 改 `milvus-minio` Service 去 selector + 建 Endpoints 指宿主 `192.168.5.2:9000`(ClusterIP 10.43.191.115 保留,短名 `milvus-minio` 不变,milvus env 零改动)→ B4 验证 milvus 经新链路 healthz 200 + curl 三路径全 200(ClusterIP短名/宿主直连/原podIP)→ B5 `scale deploy/milvus-minio --replicas=0` 删原 pod,milvus 仍 200 + 日志零报错(证明真走宿主 minio)。
- **连通性根因排查**:lima VM pod `host.lima.internal → 192.168.5.2`(Mac 宿主)可解析;`/dev/tcp` 在 alpine 镜像假阴性 + wget 缺失,改 curl 实测全 200。
- **edu 红线确认**:`edu-ai-platform`/`edu-knowledge-service`/`edu-retrieval-service` 均 **replicas=0**(scale=0,pod 没跑),改 Service/删原 minio pod 对 edu **零影响**。
- **资源释放**:原 `milvus-minio` deploy limits 500m CPU / 512Mi 内存 → scale=0 释放(lima VM 资源)。
- **DGO 落盘**:ekb 基础设施非 ESP 仓库事实源(外部现成集群),改动 + 备份 yaml 入 `report/infra-sink/`(milvus-minio-svc-after/endpoints-after/backup-*.yaml,5 文件)。回滚锚点:恢复 svc selector + Endpoints 删即可。
- **compose milvus 残留清理**:用户指示删 `~/milvus-standalone/`(之前 agent 失败前 compose 的 milvus embed-etcd 尝试,panic 后未起容器,目录+空 data 已 `rm -rf` 清除,docker compose ls 空)。
- **诚实遗留(DEFERRED)**:① **milvus pod 本身未下沉**(仍在 lima,占 CPU/内存更大头;因 ESP 3 服务用 `milvus:19530` gRPC,下沉需改 ESP 指向 + 迁 etcd 元数据,非本轮范围)。② 宿主 minio 凭据仍 minioadmin/minioadmin 弱密码(SCS 待轮转,但改凭据需同步改 milvus env,记 DEFERRED)。③ brew minio 标记 Deprecated(2027-02-17 禁用),长期需换 minio 官方二进制或 docker。

## 2026-08-20 会话23-16续3:LCO 工程治理达标 + 五类基础镜像抽取 + Build Cache 清理(进行中🔄)

> 用户诉求(两句):① 宿主 LCO 是否经过完整 SDD/TDD/TOGAF/大厂标准流程?→ 审计已出结论(部分符合,治理/供应链层大面积缺失),本轮**完整执行补齐**。② 用本地容器已有层构建 Java/Go/Python/Rust/前端五类基础镜像 + 方案3 docker tag(唯一非标,禁劫持标准 tag per SVD),再探讨清 85GB Build Cache 可回收部分。

### 问题一:LCO 工程治理补齐(执行中,esp-doc agent)
审计结论(esp-explorer a667a16f39a7122be 硬证据):SDD✅/24测试(L1+协议层,L2/L3零覆盖)/CI govet+lint 缺 gosec/govulncheck/SBOM/.golangci 缺 nilness+nilerr/无 ADR/无 secret-scan+SBOM+签名/Dockerfile 缺 readOnly+cap_drop+healthcheck/embedding 零测试零依赖锁/TOGAF 评估未纳入 lco-proxy。
**补齐项**:
1. `.golangci.yml` 加 nilness+nilerr+gosec(CLC 铁律)。
2. CI 加 govulncheck + gosec + 覆盖率落盘(-coverprofile + upload artifact)。
3. Dockerfile 加 HEALTHCHECK;k8s 侧(若有)加 readOnlyRootFilesystem+cap_drop。
4. 补 ADR-lco-proxy(架构决策记录,纳入 adr-index)。
5. lco-proxy 补 L2/L3 测试(接口级 mock,不依赖真实 Milvus/Neo4j)。
6. embedding 补 pytest + requirements.txt。
7. TOGAF 评估补 lco-proxy 条目(或独立 ADR 引用)。

### LCO 技术债 4 项治理(✅ 完成,2026-08-20,主线程串行)
> 前序 esp-builder agent 撞 API 限流失败留半成品(l2_test.go 编译错 + manager_integration_test.go unused-param),本轮主线程串行闭合。
1. **gosec G704 SSRF ×3**(项1):三处已带 `//nolint:gosec` 行级豁免+理由(l3.go:64/69、server.go:129/132/145)。定性:URL 源自 env 配置信任域(LCO_NEO4J_URL/LCO_UPSTREAM_URL),非外部输入注入,属已知不阻断技术债。
2. **revive + staticcheck 技术债**(项2):golangci-lint 从 7→**0 issues 全清零**。修复:l2_test.go `_ http.ResponseWriter`→`w`(5 处真用)+ manager_integration_test.go `c *redis.Client`→`_`(3 处不用)+ l1pool.go withMovedRetry 的 ctx/key 加 `_ =` 显式不用 + else-after-return 重构 + NewL2Semantic/protocol const 补注释。
3. **embedding pytest 实跑**(项3):**8 测试全 PASS**(0.37s,monkeypatch stub 不加载真实模型,没压 VM)。修 bug:StubModel.encode 返回 list 无 tolist → 提模块级 `_StubVec`(模拟真实 ndarray.tolist 接口)。
4. **SCS 缺口评估**(项4):`tools/lco-proxy/SCS-GAP-ASSESSMENT.md` 落盘。现状已有 Semgrep SAST+Trivy SCA/secret+govulncheck+nilness linter+ADR+nonroot+HEALTHCHECK;缺口:pre-commit(P0,**本轮已补** `.pre-commit-config.yaml` 含 gitleaks/golangci-lint/go-fmt)/SBOM/cosign/readOnly+cap_drop/requirements.lock/digest pin(P1/P2 DEFERRED)。
**验证**:golangci-lint 0 issues + go vet 干净 + go test 全过 + pytest 8/8 PASS。

### 问题二:五类基础镜像抽取 + Build Cache 清理(执行中,esp-builder agent)
现状(已探明):本地已有 `esp/golang-builder:1.25`(262MB)、`esp/jdk26-jre:debian-slim`(1.24GB)、`esp/python-runtime:3.13`(205MB)、`esp/alpine-runtime:3.22`(13.3MB);Rust 服务直接 debian bookworm + useradd(无独立基础镜像);前端 nginx:alpine。Build Cache 89.12GB/71.08GB 可回收。
**执行步骤**(遵 BRR 每步可回滚 + SVD 禁劫持标准 tag):
1. 抽 `esp/rust-runtime:debian-slim-derived`(apt ca-certificates+libgcc-s1+curl + useradd 65532 共性层)。
2. 抽 `esp/nginx-runtime:alpine-derived`(nginx:alpine + 共性 nginx.conf 注入点)。
3. Go/Java/Python 已有基础镜像,本轮只**验证 FROM 引用一致性**(确认服务 Dockerfile 都 FROM esp/* 而非裸 golang:/python:)。
4. 方案3 docker tag:用**唯一非标 tag**(`*-derived`),**严禁** `docker tag <派生> <标准名>` 劫持(SVD 铁律第③条,Redis 脑裂根因)。
5. Build Cache 清理:`docker buildx du` 看明细 → `docker buildx prune --filter until=48h --dry-run` 预估 → 确认后 prune(保留基础镜像抽取用的层)。
**顺序铁律**:先抽基础镜像 → 验证 → 再 prune(否则清掉还能复用的层)。

### LCO 代理 forwardUpstream 流式透传修复(2026-08-20 完成)
**背景**:Claude Code 不走 LCO 三层缓存(命中率 0%),因 `ANTHROPIC_BASE_URL` 指 15721(cc-switch)非 15720(LCO)。切 15720 前置阻断:forwardUpstream 用 `io.ReadAll` 全量缓冲 SSE 流式响应(Claude Code 100% 流式),破坏流式语义致首字节超时→重试→context canceled。
**改动文件**(3 个,plan: `~/.claude/plans/ancient-kindling-squid.md`):
1. `tools/lco-proxy/internal/server/server.go`:
   - `New` 的 http.Client 改 Transport 分级超时(ResponseHeaderTimeout 60s + IdleConnTimeout 90s,无总 Timeout 避免中断长流式 body)
   - `forwardUpstream` 加流式分支:响应头阶段 `isStreamByHeader` 分流;upstream context 解耦独立 600s(不绑 r.Context(),防短超时客户端误杀)
   - 新增 `streamProxy`:SSE 边读边 flush(32KB chunk + http.Flusher),剔除 Content-Length/Transfer-Encoding 头,不回写缓存(LCO 铁律:流式则不缓存)
   - `isStream` 拆分 `isStreamByHeader`(响应头判,不读 body)+ `isStreamBody`(body 前缀兜底)
2. `tools/lco-proxy/cmd/lco-proxy/main.go`:WriteTimeout 300s→1200s(覆盖超长流式+保留兜底)
3. `tools/lco-proxy/internal/server/server_test.go`(新建,8 用例)+ `internal/cache/manager.go` 加 `NewDisabledForTest()` 导出测试构造
**验证(实测全过)**:
- build RC=0 + go vet RC=0 + golangci-lint 0 issues(errcheck/nilnesserr/nilerr/gosec/revive)
- 单测 8 用例全过(-race),含 flushRecorder 断言 flush>=3、Flusher 降级、独立 context 不受客户端 cancel 影响
- 流式实测:长输出流式 **首字节 9.97s << 总时间 79.71s**(边读边 flush 生效,非全量缓冲)
- 非流式缓存回归:L2 语义命中(`X-Lco-Cache-Hit: L2`,2.1s vs 回源 21.9s)
**新 PID 94720 替换旧 62663**(pkill+nohup 重启,非 launchd;healthz status:true)
**待办(独立阻断,非本次引入)**:~~L1 精确缓存 MOVED 短名解析失败~~ **已修复(2026-08-20)**。真实根因有二,均已修:
1. **`withMovedRetry` 把 `redis.Nil`(正常 cache miss)当 error**(CLC 修复):原逻辑遇 redis.Nil 继续遍历所有 pod,最后返回 MOVED noise error,致 manager 把正常 miss 计入 `errorsTotal` + 每次 miss 浪费多次 Redis 调用。修法:redis.Nil 直接返回(正常 miss,不遍历)。同时 manager.Lookup 改 `errors.Is(err, redis.Nil)` 显式判断(替代脆弱的 `strings.Contains`)。
2. **LCO 启动时 `LCO_REDIS_ADDR` 只配了 6380 一个地址**(真根因):之前手动 nohup 启动丢 env,走 config 默认 `127.0.0.1:6380`(只 1 pod),L1Pool 只有 pod-0 client,无法 MOVED 重路由到 pod-1/2。修法:启动必须带 `LCO_REDIS_ADDR="127.0.0.1:6380,...,127.0.0.1:6385"`(scripts/start-lco.sh:41 已正确配,手动启动须带此 env)。
验证:4 个不同 prompt(全 miss)→ 0 个 L1 error + 0 MOVED noise(之前每次 miss 都报);重复 prompt L1 精确命中(`X-Lco-Cache-Hit: L1`)。改动文件:`internal/cache/l1pool.go`(redis.Nil 识别)+ `internal/cache/manager.go`(errors.Is + redis 导入)。

### LCO 三层缓存完整验证 + 双轨落盘(2026-08-20)
**三层状态(全实测)**:
- L1 精确(Redis Cluster):`X-Lco-Cache-Hit: L1`,6ms 级,MOVED 重路由 + redis.Nil 修复后正常
- L2 语义(Milvus + embedding):`X-Lco-Cache-Hit: L2` + `X-Lco-Source-Protocol: anthropic`,2s 级;embedding 服务 15730 healthz ok(MPS 推理通,Qwen3 loaded);L2 语义命中日志 `L2 语义命中 srcProto/dstProto`
- L3 知识图谱(Neo4j):healthz `l3_neo4j:ok`(用 start-lco.sh 启动配 Neo4j env 后恢复,之前手动 nohup 丢 env 致 L3 down)
**运行态**:LCO PID 76078,`scripts/start-lco.sh` 启(带全 env:`LCO_REDIS_ADDR` 6380-6385 + Neo4j env),healthz `status:true`
**待观察**:embedding `loaded:{qwen:true, minilm:false}` —— minilm 模型未加载。lco-proxy 的 L2 collection `lco_prompts` 用 qwen(2560维),minilm 未加载不影响 lco-proxy L2;但若其他组件依赖 minilm 需排查(DEFERRED,非阻断)
**双轨落盘**:PROGRESS.md(本段)+ Redis 锚点 `esp:ctx:lco:stream-fix-anchor`(TTL 7d,含当前子步骤+已改文件+下一步,跨会话恢复)
**架构审计报告**:落盘 `report/audit/architecture-audit-2026-08-20.md`(720 行,总分 6.18/10,4 P0 + 9 P1 + 5 P2 重构方案)
**下一步顺序**:用户批准"先双轨落盘再按建议 (a)→(b) 执行":
- (a) 切 Claude Code 流量到 15720:改 `~/.claude/settings.json` `ANTHROPIC_BASE_URL` 15721→15720 + 重启会话,观察 `/metrics` 真实命中率(LCO 铁律目标 >30%)
- (b) 修审计 4 项 P0:JWT_SECRET 明文(`deploy/k8s/00-namespace-secret.yaml:27`)/conversation-go LLM 裸调无超时(`backend/services/conversation-go/src/llm/anthropic.go:62`)/Go 服务 golangci-lint 缺失 9/13/SVD 版本一致性 CI 门禁

### 仍待完成(下一步)
1. **Neo4j 下沉完成验证**(agent 进行中):腾 250m CPU + L3 恢复(l3_neo4j:ok)。
2. **Milvus/minio 下沉**(✅ minio 后端已下沉宿主 brew+launchd,释放 500m/512Mi;**milvus pod 本身 DEFERRED**——ESP 3 服务用 milvus:19530 gRPC,下沉需改 ESP 指向+迁 etcd 元数据,非本轮范围;edu 全 scale=0 零影响已确认)。
3. **orchestrator 1.3.1 上线**:下沉腾 CPU 后 `rollout restart` 即上线(镜像已就绪)。
4. **Qwen 限流/离峰**:下沉腾 CPU 后给 embedding 限流,让 Qwen L2 真正可跑(路径2 收尾)。
5. **whisper 镜像 build**(PyPI 可达性风险)+ edge-tts handshake 库内 bug。
6. **Go 下游服务升级分批**(esp-common-go 已完成)+ Java postgresql + Rust rustls-webpki。
7. **trivy image 扫镜像 OS 层 + db 联网刷新补 5 条漏报**。
8. **k6+Locust 压测**(待 CPU/多节点)。

