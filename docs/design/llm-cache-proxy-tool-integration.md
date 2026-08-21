# LCO 代理 — 工具接入指南

> 本机编码工具(Claude Code / ZCode / Codex / Continue / Cline / Aider / Ollama 客户端)如何接入 LCO 多级缓存代理,使 LLM 调用走 L1 精确缓存 + L2 语义缓存 + L3 知识图谱(落地 LCO 铁律)。

## 0. 前置:启动 LCO 代理

```bash
# 一键(含 port-forward + embedding + lco 代理 + launchd 自愈)
~/claude-Projects/English-Study-Platform/scripts/start-lco.sh start
# 状态
~/claude-Projects/English-Study-Platform/scripts/start-lco.sh status
```

验证代理就绪:

```bash
curl -s http://127.0.0.1:15720/healthz
# 期望:{"cache":{"l1_redis":"ok","l2_milvus":"ok","l3_neo4j":"ok-deferred"},"status":true}
```

代理拓扑:

```
工具 → LCO 代理(:15720) → [L1 Redis / L2 Milvus / L3 Neo4j] → CC Switch(:15721) → LLM
```

| 端口 | 服务 | 说明 |
|---|---|---|
| 15720 | LCO 代理 | 统一入口,三协议适配(Anthropic/OpenAI/Ollama) |
| 15721 | CC Switch | 上游 LLM provider 路由(真实 Claude/GLM 等) |
| 15730 | embedding 服务 | zh→Qwen3-Embedding-4B / en→all-MiniLM,MPS 加速 |

---

## 1. Claude Code(主目标)

Claude Code 走 Anthropic Messages 协议(`/v1/messages`),通过 `ANTHROPIC_BASE_URL` + `ANTHROPIC_API_KEY` 两个环境变量切 endpoint。

### 接入

```bash
# 方式 A:当前 shell 临时接入
export ANTHROPIC_BASE_URL=http://127.0.0.1:15720
export ANTHROPIC_API_KEY=PROXY_MANAGED   # 代理透传,无需真实 key(由 CC Switch 注入)

# 方式 B:持久化写入 ~/.claude/settings.json 或 shell profile
# (~/.zshrc)
export ANTHROPIC_BASE_URL=http://127.0.0.1:15720
export ANTHROPIC_API_KEY=PROXY_MANAGED
```

> ⚠️ `PROXY_MANAGED` 是占位符——LCO 代理透传 `x-api-key`/`Authorization` 到 CC Switch,CC Switch 持有真实凭据。工具侧无需也无法看到真实 key(安全:真实凭据禁入 git,由运行时注入)。

### 验证

```bash
# 发两次相同请求,第二次应命中 L1
curl -s -D - -X POST http://127.0.0.1:15720/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: PROXY_MANAGED" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"claude-sonnet-4-6","max_tokens":50,"messages":[{"role":"user","content":"reply: hello"}]}' \
  | grep -i "X-Lco-Cache-Hit"
# 第1次:无 header(MISS,回写缓存)
# 第2次:X-Lco-Cache-Hit: L1
```

启动 Claude Code(`claude` 命令),日志会走 LCO,命中时省 100% API 调用。

---

## 2. ZCode

ZCode 配置在 `~/.zcode/v2/config.json`(已实测其结构:provider 是 UUID 键,每个 provider 含 `kind`(协议族,如 `anthropic`)、`options.baseURL`、`options.apiKey`、`enabled`、`source`、`models`)。

### 接入(添加一个自定义 provider)

编辑 `~/.zcode/v2/config.json`,在 `provider` 对象下新增一个 UUID 键(用 `uuidgen` 生成):

```json
{
  "provider": {
    "<新 UUID>": {
      "name": "LCO Cache Proxy",
      "kind": "anthropic",
      "options": {
        "apiKey": "PROXY_MANAGED",
        "baseURL": "http://127.0.0.1:15720"
      },
      "enabled": true,
      "source": "custom",
      "models": {
        "claude-sonnet-4-6": {
          "limit": { "context": 1000000, "output": 128000 },
          "modalities": { "input": ["text"], "output": ["text"] },
          "zcode": { "priority": 100 }
        }
      }
    }
  }
}
```

- `kind: anthropic` → ZCode 发 `/v1/messages` 到 baseURL,LCO 按 Anthropic 协议归一化缓存。
- ZCode 也支持 OpenAI 协议 provider(`kind` 对应 openai-compat),那类走 `/v1/chat/completions`,LCO 同样支持。

### 验证

启动 ZCode,在对话中发同一问题两次,或直接 curl 同 §1 验证。也可在 ZCode 日志 `~/.zcode/logs/` 查看请求是否打到 `127.0.0.1:15720`。

---

## 3. Codex(OpenAI CLI)

Codex 走 OpenAI Chat Completions 协议(`/v1/chat/completions`),通过 `OPENAI_BASE_URL` + `OPENAI_API_KEY` 切 endpoint(或 `~/.codex/config.toml` 的 `model_providers`)。

### 接入

```bash
# 方式 A:环境变量
export OPENAI_BASE_URL=http://127.0.0.1:15720/v1
export OPENAI_API_KEY=PROXY_MANAGED

# 方式 B:config.toml(~/.codex/config.toml)
cat > ~/.codex/config.toml <<'EOF'
model = "gpt-4o-mini"
model_provider = "lco"

[model_providers.lco]
name = "LCO Cache Proxy"
base_url = "http://127.0.0.1:15720/v1"
env_key = "OPENAI_API_KEY"
wire_api = "chat"
EOF
```

> Codex 调 `/v1/chat/completions`,LCO 按 OpenAI 协议归一化(L1 协议分键 `lco:1:openai:*`,与 Anthropic 键隔离)。

### 验证

```bash
curl -s -D - -X POST http://127.0.0.1:15720/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer PROXY_MANAGED" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"reply: hello"}]}' \
  | grep -i "X-Lco-Cache-Hit"
```

> ⚠️ 若 CC Switch 上游未配 OpenAI 协议供应商,Codex 请求可能返回 503(`未配置供应商`)。此时 L1 仍可缓存 miss→store→hit(只要上游对同 model 返回稳定),但需 CC Switch 先接一个 OpenAI 兼容上游(如 GLM 的 OpenAI 兼容端点)。

---

## 4. Continue(VSCode/JetBrains 插件)

Continue 配置在 `~/.continue/config.yaml`,每个 model 可设 `apiBase`。

### 接入

```yaml
# ~/.continue/config.yaml
models:
  - name: LCO Cached Claude
    provider: anthropic
    model: claude-sonnet-4-6
    apiKey: PROXY_MANAGED
    apiBase: http://127.0.0.1:15720
    roles:
      - chat
      - edit
  - name: LCO Cached GPT
    provider: openai
    model: gpt-4o-mini
    apiKey: PROXY_MANAGED
    apiBase: http://127.0.0.1:15720/v1
    roles:
      - chat
```

- `provider: anthropic` → Continue 发 `/v1/messages`(需 apiBase 不带 `/v1` 后缀,Continue 自动拼)。
- `provider: openai` → 发 `/v1/chat/completions`(apiBase 带 `/v1`)。

### 验证

VSCode 里用该 model 连发两次相同 chat,或 curl 同上。Continue 日志在 `~/.continue/sessions/`。

---

## 5. Cline(VSCode 插件)

Cline 配置在 VSCode settings(`cline.*` 或 UI),支持 anthropic/openai/ollama 三类 provider,字段 `apiBaseUrl`。

### 接入(UI 或 settings.json)

```jsonc
// VSCode settings.json
{
  "cline.allowedCommands": ["..."],
  "cline.autoApprove": "..."
}
// 在 Cline 面板 → Settings → API Provider:
//   选 "Anthropic" → API Base URL: http://127.0.0.1:15720  → API Key: PROXY_MANAGED
//   或 "OpenAI Compatible" → Base URL: http://127.0.0.1:15720/v1 → Model: claude-sonnet-4-6
```

Cline 的 "OpenAI Compatible" 走 `/v1/chat/completions`;"Anthropic" 走 `/v1/messages`。两者 LCO 均支持。

### 验证

Cline 面板发任务两次,或 curl。Cline 日志在 VSCode Output → "Cline" 频道。

---

## 6. Aider(LiteLLM 后端)

Aider 通过 LiteLLM 支持 OpenAI/Anthropic,字段 `OPENAI_API_BASE` + `OPENAI_API_KEY`(或 `--openai-api-base`)。

### 接入

```bash
# Anthropic via LiteLLM(需 litellm 代理或直接 provider)
export OPENAI_API_BASE=http://127.0.0.1:15720/v1
export OPENAI_API_KEY=PROXY_MANAGED

aider --model gpt-4o-mini
# 或 OpenAI 兼容上游
aider --openai-api-base http://127.0.0.1:15720/v1 --model gpt-4o-mini
```

> Aider 的 Anthropic 直连模式(`--model claude-...`)走原生 SDK,需 `ANTHROPIC_BASE_URL`(同 §1)。用 OpenAI 兼容模式更简单。

### 验证

`aider` 里发同一编辑请求两次,第二次日志应显示缓存命中(或 curl 同上)。

---

## 7. Ollama 客户端

Ollama 原生协议 `/api/chat` + `/api/generate`,通过 `OLLAMA_BASE_URL` 切 endpoint。

### 接入

```bash
export OLLAMA_BASE_URL=http://127.0.0.1:15720
ollama run llama3   # 或 ollama python sdk
```

### 验证

```bash
curl -s -D - -X POST http://127.0.0.1:15720/api/chat \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3","messages":[{"role":"user","content":"hi"}]}' \
  | grep -i "X-Lco-Cache-Hit"
```

> LCO 按 Ollama 协议归一化(L1 键 `lco:1:ollama:*`)。上游 CC Switch 需有 Ollama 兼容供应商,否则上游可能拒。

---

## 8. 不支持的工具(排除)

| 工具 | 原因 |
|---|---|
| GitHub Copilot | 无自定义 endpoint 入口,请求过 Copilot 网关(TLS 拦截非协议层职责) |
| Cursor | BYOK 仍过 Cursor 网关,无 baseURL 配置 |

---

## 9. 缓存键隔离与跨协议语义

| 层 | 键 | 跨工具? |
|---|---|---|
| L1 精确 | `lco:1:{protocol}:{sha256前16位}` | 协议隔离(Claude Code 与 ZCode 同为 anthropic 协议则共享 L1) |
| L2 语义 | Milvus `lco_prompts` collection(2560dim Qwen,cosine≥0.92) | **跨协议共享**(同一 prompt 即便来自不同工具/协议,L2 语义命中可复用) |
| L3 图谱 | Neo4j Prompt→Response | 跨协议(上下文增强) |

> **2026-08-20 修正**:此前此表把 L2 collection 写成 `esp_llm_semantic`(384dim minilm),那是**业务 LCO**(conversation-go/orchestrator-rs 共用)的 collection,不是 llm-cache-proxy 的。llm-cache-proxy(Claude Code 用的)的 L2 collection 是 `lco_prompts`(2560 维 Qwen3-Embedding-4B)。两者独立 collection 各自维度自洽,不混用。llm-cache-proxy `l2.go:82` **显式传 `model=qwen`** 产出 2560 维向量(原只发 text 走服务端默认致维度不匹配全 miss)。业务 LCO 显式传 `model=minilm`(384 维)。见 `llm-cache-proxy-sdd.md` §4.2/§4.4。

设计意图:L1 协议隔离(响应体格式不同,不能直用);L2 语义跨协议(向量相似即可复用,需重生成适配目标协议——当前降级直穿,重生成 DEFERRED)。

---

## 10. 故障排查

| 症状 | 排查 |
|---|---|
| 工具报 connection refused | `lsof -iTCP:15720 -sTCP:LISTEN` 确认 LCO 在跑;`start-lco.sh start` |
| `X-Lco-Cache-Hit: MISS` 每次都 miss | L1 Redis Cluster MOVED 重路由是否生效:`/tmp/llm-cache-proxy.log` 查 "L1 命中" vs "MOVED" |
| L1 命中率 <100% | 确认 `LCO_REDIS_ADDR` 含 6 个地址 `127.0.0.1:6380,...:6385`;L1Pool MOVED 重路由需全 6 pod port-forward |
| 上游 503 `未配置供应商` | CC Switch 未配对应协议上游;或工具用的 model 在 CC Switch 无映射 |
| L2 不命中 | cosine 阈值 0.92 较严,语义近似但措辞不同会 miss;embedding 服务需可达 `curl :15730/healthz` |
| embedding 慢 | 首次加载 Qwen3-Embedding-4B(~8GB)需 ~30s;后续常驻 MPS,单次 <100ms |

---

## 11. 相关文档

- SDD:`docs/design/llm-cache-proxy-sdd.md`
- 治理(TOGAF):`docs/design/llm-cache-proxy-governance.md`
- LCO 铁律:全局 `~/.claude/CLAUDE.md` 第 8 条 + memory `llm-cache-optimization-ironrule`
- ESP 服务侧 L2/L3(Phase2/3):`backend/services/{conversation-go,writing-coach,llm-orchestrator-rs}` 的 cache 实现(共用 `esp_llm_semantic` Milvus collection)
