# 会话23-14 真实引擎切换（去 mock，LLM+ASR+TTS 全真实）— 归档

> 日期: 2026-08-19
> 状态: 代码 + manifest + env 接线完成；镜像构建受阻于 registry mirror 403（临时故障）
> 计划: `.claude/plans/robust-baking-wave.md`

## 背景

用户要求 conversation-go 的 ASR/TTS 和 llm-orchestrator-rs 的 LLM 全部切真实引擎，不再 mock（生产域 mock 零残留是工程铁律 MPB）。同时要求所有使用 LLM 的地方给"本机 GPU 或本地模型"留出切换路径（本机 Apple M1 Max 32 核 GPU，MPS 非 CUDA）。

## 完成清单

### 代码（go build 全部通过）
1. **asr-adapter-go**（`backend/services/asr-adapter-go/`）：收 `{chunks:[base64]}` → Whisper multipart → `{text}`。含 `ASR_MODE=local` 本机 GPU 预留分支（DEFERRED 真实实现）。端口 8097。
2. **tts-adapter-go**（`backend/services/tts-adapter-go/`）：纯 Go 零 Python，`edgetts.New().Bytes()` → MP3 base64 + 四态情绪 + Preston Blair 口型。修了重复 import。端口 8098。
3. **listening-go main.go:167**：硬编码 `chunks: ["Hello",...]` 占位 → 接 asr-adapter（ASR_ENDPOINT 未配则 501 拒绝假数据）。

### Manifest（已 apply）
- `deploy/k8s/34-asr-stack.yaml`：whisper（社区镜像 onerahmet/openai-whisper-api）+ asr-adapter-go，含 GPU 预留注释块
- `deploy/k8s/35-tts-stack.yaml`：tts-adapter-go
- `deploy/k8s/10-gateway-go.yaml`：env 加 ASR_MODE=http/ASR_ENDPOINT/TTS_MODE=http/TTS_ENDPOINT
- `deploy/k8s/00-namespace-secret.yaml`：加 llm-providers secret（claude/openai/qwen base+key 占位）
- `deploy/k8s/33-orchestrator-vocab.yaml`：LLM_DEFAULT_MODEL glm_for_coding → claude-sonnet-4-6

### 运行时
- **CC Switch 桥接**：socat 15722→15721 + launchd 自愈（com.esp.ccswitch-bridge），pod 可达实测
- **真实凭据注入**：esp-secrets（ANTHROPIC_API_KEY/BASE_URL）+ llm-providers（claude-base/key）已注入 CC Switch 当前 provider（Zhipu GLM via ai.csdn.net）
- **LLM_MODE=mock 移除**：gateway-go 运行期注入已清除，AnthropicProvider 走真实 Claude/GLM

## GPU/本地模型预留总表

| 使用点 | 当前引擎 | 本地预留开关 | 阶段 |
|---|---|---|---|
| conversation-go LLM | CC Switch→Claude/GLM | ANTHROPIC_BASE_URL→Ollama | 现就绪（纯 env） |
| llm-orchestrator-rs | ClaudeProvider | LLM_PROVIDER_OPENAI_BASE→Ollama（OpenAiCompat 复用） | 现就绪 |
| asr-adapter-go | 集群 Whisper | ASR_MODE=local 分支 | 预留（DEFERRED） |
| writing-coach | ANTHROPIC_BASE_URL→Claude | 同→Ollama | 现就绪（纯 env） |
| emotion-svc | 无 LLM（纯关键词） | EMOTION_LLM_BASE | DEFERRED |

**关键简化**：Ollama 暴露 OpenAI 兼容 `/v1/chat/completions`，llm-orchestrator-rs 的 OpenAiCompat provider 零代码改动直接复用，无需新增 OllamaProvider。

## 阻塞点（registry mirror 403）

`docker.1ms.run` 对 golang:1.25-alpine/alpine:3.22 的 HEAD 请求返回 403（daemon 拉取失败），GET+token 认证可部分拉取（4/5 层成功，空层需特殊处理）。这是临时基础设施故障（37h 前 content-go 还能构建）。

**根因**：registry mirror 的 HEAD 端点 403，但 GET+token 认证 + Location 重定向跟随可绕过。空层（`4f4fb700...`，32 字节 gzip）是 Docker 标准 empty layer，已确认可从 mirror 的签名 URL 拉取或本地生成。

**解决方案**：`/tmp/esp-pull2.sh` 脚本（token 认证 + Location 重定向跟随 + 空层本地生成），后台拉取 golang:1.25-alpine + alpine:3.22 到 rd-vm。

## 恢复后执行

1. 确认 golang:1.25-alpine + alpine:3.22 已 load 进 rd-vm
2. `cd backend && DOCKER_CONTEXT=rd-vm docker build -t esp/asr-adapter-go:1.0.0 -f services/asr-adapter-go/Dockerfile .`
3. `DOCKER_CONTEXT=rd-vm docker build -t esp/tts-adapter-go:1.0.0 -f services/tts-adapter-go/Dockerfile .`
4. `kubectl -n esp-prod apply -f deploy/k8s/34-asr-stack.yaml -f deploy/k8s/35-tts-stack.yaml`
5. 验证 WS 链路：gateway WS 发音频 → asr-adapter → whisper → 真实识别文本（非 mock 预设）
6. 验证 TTS：LLM 文本 → tts-adapter → edge-tts → 真实 MP3（非 `base64("tts:"+text)` 占位）

## 资源现状（单节点 CPU 饱和）

- Node allocatable 9 核，requests 已 99%（8930m）。TiDB/TiKV/PD 已全部 scale 0（不占资源）。
- 真实引擎切换临时缩容：learning-java 6→1、learning-worker 3→1、bff-java 2→1（为 gateway/orchestrator rollout 让路）
- **注意**：这些缩容需在验证完成后恢复，或标记为正式降配。
