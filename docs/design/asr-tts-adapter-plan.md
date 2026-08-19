# ASR/TTS 适配微服务 —— 技术选型与实施方案

> 状态：待确认（等待用户批准后实施）
> 日期：2026-08-19
> 触发：conversation-go ASR/TTS provider 从 mock 切真实引擎（多节点清单 #4 DEFERRED 项）

---

## 一、技术选型：Go（已确定）

### 结论

`asr-adapter-go` + `tts-adapter-go`，**纯 Go**，不引入 fastapi/Python。

### 决策依据（按权重）

| 权重 | 依据 | 说明 |
|---|---|---|
| ★★★ | **定位是"适配层"非"推理层"** | ASR/TTS 适配器**不跑模型**。Whisper 跑在 `onerahmet/openai-whisper-api` 容器，TTS 调微软 edge-tts 在线 API。适配器只做：收自定义契约 → 转发真实引擎 → JSON 转换 + 重试 + 限流。**纯 I/O 绑定，零 CPU 密集计算**，Rust 的内存安全/零成本抽象在此场景无发挥空间。 |
| ★★★ | **消费方是 Go，契约对齐成本决定性** | ASR/TTS 的唯一上游是 gateway-go（通过 `replace esp/conversation-go => ../conversation-go` 引入）。Go 写适配层可直接复用 `SenseEmotion`/`BuildVisemes` 逻辑 + `esp/common-go` 中间件（GinRateLimit/GinCircuitBreaker）。Rust 需重写这两个函数 + 重接 `esp-common-rs`。 |
| ★★ | **生态成熟度** | edge-tts 有现成纯 Go 库（`haguro/edge-tts-go` / `lib-x/edgetts`），直连微软 WebSocket API，**天然消除 fastapi/Python 突兀问题**（对齐用户 2026-08-19 反馈）。Whisper 转发用 `net/http` 即可，零额外依赖。 |
| ★★ | **运维统一性** | 项目 11 个 Go 服务 vs 4 个 Rust 服务，Go 是绝对主力。`content-go` 是标准 Go 骨架（gin + esp-common-go + healthz/livez/readyz/metrics + Dockerfile golang:1.25-alpine），可直接复制。 |

### 反证（Rust 何时才该用）

项目 4 个 Rust 服务（assessment-rs、llm-orchestrator-rs、media-learn-rs、pronunciation-rs）都用对了——它们是**性能敏感**（音频评分/媒体处理）或**流式高并发**（SSE 转发 + 语义缓存）。ASR/TTS 适配器**不属于这两类**，放 Rust 是错配。

---

## 二、架构图景（关键事实）

### 现状

```
前端 Avatar ──WS──> gateway-go
                     ├── s.asr.Recognize(chunks)  ← asr.New()  [main.go:109]
                     ├── s.tts.Synthesize(text)   ← tts.New()  [main.go:110]
                     └── s.engine.Process()        ← LLM (已真实化)
```

- ASR/TTS 的 **provider 代码物理在 conversation-go**（`src/asr/`、`src/tts/`），但**运行时由 gateway-go 调用**（`import "esp/conversation-go/src/asr"`）
- `gateway-go` 当前**未设 ASR_MODE/TTS_MODE env** → 走 `New()` 默认 mock
- 接线点：`main.go:310` `Recognize([]string{frame.Data})`、`main.go:357` `Synthesize(ctx, f.Text)`

### 目标

```
gateway-go ──HTTP──> asr-adapter-go  ──HTTP──> whisper-api 容器(openai-whisper-api)
                  └─> tts-adapter-go  ──WSS──> 微软 edge-tts
```

---

## 三、契约（对齐现有 provider.go，零改动 conversation-go 代码）

### ASR（asr-adapter-go 对外）

```
POST /transcribe
请求：{ "chunks": ["<base64 wav/音频帧>", ...] }
响应：{ "text": "识别文本" }
```

- 对应 `asr/provider.go` 的 `asrReq{Chunks}` → `asrResp{Text}`，**完全兼容**
- 适配器内部：把 base64 chunks 拼接/转码 → 调 whisper-api 的 `/asr`（或 `/v1/audio/transcriptions`）→ 提取 text 回传

### TTS（tts-adapter-go 对外）

```
POST /synthesize
请求：{ "text": "要合成的文本" }
响应：{ "audio": "<base64 mp3>", "emotion": "happy|neutral|encouraging|sad", "visemes": [{"id":"AA","start":0,"end":60},...] }
```

- 对应 `tts/provider.go` 的 `ttsReq{Text}` → `ttsResp{Audio,Emotion,Visemes}`，**完全兼容**
- 适配器内部：调 edge-tts 得真实音频 → `SenseEmotion(text)` 得情绪 → `BuildVisemes(text)` 得口型
- `SenseEmotion`/`BuildVisemes` **从 `tts/mock.go` 原样移植**（关键词→四态情绪、字符→Preston Blair 口型）

### gateway-go 接线（唯一改动点）

`deploy/k8s/10-gateway-go.yaml` 新增 env：

```yaml
- {name: ASR_MODE, value: "http"}
- {name: ASR_ENDPOINT, value: "http://asr-adapter:80/transcribe"}
- {name: TTS_MODE, value: "http"}
- {name: TTS_ENDPOINT, value: "http://tts-adapter:80/synthesize"}
```

---

## 四、实施步骤

### Step 1：asr-adapter-go（纯 Go）

- 复制 content-go 骨架：`gin` + `esp/common-go`（middleware.GinRateLimit/GinCircuitBreaker）+ healthz/livez/readyz/metrics
- `POST /transcribe` handler：解 base64 chunks → 调 whisper-api → 回 `{text}`
- Dockerfile：golang:1.25-alpine builder + alpine:3.22 + USER app
- 端口：`8097`

### Step 2：tts-adapter-go（纯 Go）

- 复制 content-go 骨架（同上）
- `POST /synthesize` handler：调 edge-tts（Go 库 `haguro/edge-tts-go`）→ 得 base64 mp3 → `SenseEmotion` + `BuildVisemes` → 回完整结果
- 移植 `SenseEmotion`/`BuildVisemes` 函数（从 conversation-go/src/tts/mock.go）
- Dockerfile 同上
- 端口：`8098`

### Step 3：whisper 容器部署

- 镜像：`onerahmet/openai-whisper-api`（社区镜像，CPU base 模型）
- 作为 asr-adapter 的 upstream，Deployment + Service（`whisper-api:80`）

### Step 4：部署 yaml

- `deploy/k8s/36-asr-tts-stack.yaml`（编号顺延，34/35 已被 community-go/content-go 占用）
- 含：asr-adapter-go（Deployment+Service+可能 HPA）、tts-adapter-go（Deployment+Service）、whisper-api（Deployment+Service）
- namespace：esp-prod

### Step 5：接线 + 验证

- `10-gateway-go.yaml` 加 4 个 env（见上）
- 验证：调 gateway WS 发音频帧 → asr-adapter 识别 → tts-adapter 合成 → 前端 Avatar 收到真实音频+口型
- 缓存验证：TTS 走 L1+L2 缓存（`gateway:tts:v2`），相同文本复用结果

### Step 6：mock 退役

- conversation-go 的 `asr/mock.go`、`tts/mock.go` 保留（作为 CI/联调降级路径），但生产 manifest 显式设 `ASR_MODE=http`/`TTS_MODE=http`
- listening-go `main.go:167` 硬编码 chunks（`transcribe` 占位）**另案处理**——它是独立 SSE 通道，与 gateway-go 的 ASR 无关，可改为 501 或接 asr-adapter（本次可选）

---

## 五、风险与待确认项

| # | 风险/待确认 | 处理 |
|---|---|---|
| 1 | **edge-tts Go 库选择**：`haguro/edge-tts-go` vs `lib-x/edgetts`，需验证可用性 | 实施时 `go get` 验证，选维护活跃、能直连微软 WSS 的 |
| 2 | **whisper 镜像 CPU 性能**：base 模型 CPU 推理慢（可能 5-10s/段） | 先接 CPU 版验证链路；性能不够再上 GPU/小模型 |
| 3 | **契约 `{chunks}` 的音频格式**：gateway 传的 base64 是 wav 还是 webm？whisper-api 接受什么格式？ | 实施时确认 whisper-api 的输入格式，适配器做转码 |
| 4 | **edge-tts 网络可达性**：微软 edge TTS 在线服务，集群 Pod 需出网 | 确认集群 egress 允许；否则需代理 |
| 5 | **认证**：whisper-api / edge-tts 是否需 API key | 实施时确认，key 走 secret 注入（禁入 git） |

---

## 六、本次不做（明确边界）

- ❌ 不改 conversation-go 的 provider 接口/契约（保持 HTTPProvider 不变，只通过 env 指向适配器）
- ❌ 不退役 mock 代码（保留作 CI/降级）
- ❌ 不动 listening-go 的独立 transcribe 通道（另案）
- ❌ 不上 GPU whisper（先 CPU 验证链路）

---

## 七、验证标准

1. `asr-adapter` 收到 `{chunks:[base64]}` 返回真实识别文本（非 mock 回显）
2. `tts-adapter` 收到 `{text}` 返回真实 base64 mp3 + 情绪 + 口型序列
3. gateway-go 走 `ASR_MODE=http`/`TTS_MODE=http` 不再打 mock 日志
4. 端到端：前端 Avatar 播放真实合成音频 + 口型同步
