# LCO 代理 TOGAF 治理文档

> 架构治理:LCO 代理作为宿主侧基础设施,纳入 ESP 架构治理体系。

## 1. 架构原则

| 原则 | 说明 | 落地 |
|---|---|---|
| 协议无关 | 缓存层不绑定单一工具协议 | 三端点 Anthropic/OpenAI/Ollama |
| 缓存层降级 | 缓存不可达不阻断功能 | Lookup/Store 全降级直穿 |
| 多工具共享 | 本机工具共享同一缓存层 | 统一 15720 入口 |
| 本机优先 | 工具侧缓存优先宿主,不占集群资源 | 宿主进程 + 宿主 venv |
| 职责分离 | 宿主 redis=LCO 专用,集群 redis=ESP 应用 | 不混用 |

## 2. 标准合规

| 铁律 | 合规状态 | 说明 |
|---|---|---|
| LCO(LLM 缓存) | ✅ 三层 | L1 精确 + L2 语义 + L3 图谱 |
| CLC(代码级正确性) | ✅ 落地 | nil-safe + go vet + golangci-lint + 单测 |
| ASV(API 签名核实) | ⚠️ 已犯错 | redis.StatusCmd.err→Err()、redis.ClientOptions→Options 已修正落盘 |
| MPB(零 mock) | ✅ | miniredis 真实 Redis 集成测试 |
| 工程铁律②(配而接) | ✅ | golangci-lint 配置 + 实际运行 |

## 3. 能力管理

### 3.1 支持的工具协议矩阵

| 工具 | 协议 | 接入方式 | 状态 |
|---|---|---|---|
| Claude Code | Anthropic /v1/messages | ANTHROPIC_BASE_URL | ✅ 主目标 |
| ZCode | Anthropic(主)+ OpenAI(次) | provider.baseURL | ✅ 适配层就绪 |
| Continue | 多 provider | models[].apiBase | ✅ |
| Cline | Anthropic/OpenAI/Ollama | apiBaseUrl | ✅ |
| Aider | LiteLLM(OpenAI/Anthropic) | OPENAI_API_BASE | ✅ |
| Ollama 客户端 | /api/chat | OLLAMA_BASE_URL | ✅ |
| GitHub Copilot | OpenAI 私有 | 无自定义 endpoint | ❌ 排除(TLS 拦截非协议层职责) |
| Cursor | OpenAI 私有 | 无 baseURL 配置 | ❌ 排除(BYOK 仍过 Cursor 网关) |

## 4. 安全治理

- **密钥管理**:LCO 不落任何密钥,透传 `x-api-key`/`Authorization`
- **缓存隔离**:协议分键,跨协议不串台
- **降级安全**:缓存层任何故障直穿上游,不放大故障
- **审计**:结构化 JSON 日志 + Prometheus 命中率监控

## 5. 运维治理

- **自愈**:port-forward 三通道 + embedding + LCO 代理,复用 Nacos-CRDB launchd 自愈模式
- **监控**:Prometheus scrape localhost:15720 + 15730,指标 lco_cache_hits_total{layer}
- **容量**:命中率目标 >30%,月度成本核算省 >40%

## 6. 变更管理

- 语义化版本:Phase 1 MVP(0.1.0)
- 变更记录:PROGRESS.md + git commit 落盘
- 评审门禁:go vet + golangci-lint + 单测全过才可合并
