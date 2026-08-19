// provider/mod.rs —— 多 provider 路由 + 降级
// 支持:OpenAI 兼容(default)/ 通义(qwen)/ Claude(anthropic)
// 从 env 读 base_url + key:LLM_PROVIDER_OPENAI_BASE / LLM_PROVIDER_OPENAI_KEY / LLM_PROVIDER_QWEN_* / LLM_PROVIDER_CLAUDE_*
// 统一 trait LlmProvider;降级链 openai → qwen → claude

use async_trait::async_trait;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::env;

/// 单次调用结果
pub struct CallResult {
    pub content: String,
    pub provider: String,
    pub model: String,
}

#[async_trait]
pub trait LlmProvider: Send + Sync {
    fn name(&self) -> &str;
    async fn call(
        &self,
        model: &str,
        prompt: &str,
        max_tokens: Option<i32>,
        stream: bool,
        trace_id: &str,
    ) -> Result<String, ProviderError>;
}

#[derive(Debug)]
pub struct ProviderError(pub String);

impl std::fmt::Display for ProviderError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl std::error::Error for ProviderError {}

/// provider 注册表 + 降级链
pub struct ProviderRegistry {
    chain: Vec<Box<dyn LlmProvider>>,
    default_model: String,
}

impl ProviderRegistry {
    pub fn from_env() -> Self {
        let mut chain: Vec<Box<dyn LlmProvider>> = Vec::new();

        // OpenAI 兼容(default)
        if let Ok(base) = env::var("LLM_PROVIDER_OPENAI_BASE") {
            let key = env::var("LLM_PROVIDER_OPENAI_KEY").unwrap_or_default();
            chain.push(Box::new(OpenAiCompat {
                name: "openai".into(),
                base_url: base,
                api_key: key,
                client: Client::new(),
            }));
        }
        // 通义
        if let Ok(base) = env::var("LLM_PROVIDER_QWEN_BASE") {
            let key = env::var("LLM_PROVIDER_QWEN_KEY").unwrap_or_default();
            chain.push(Box::new(OpenAiCompat {
                name: "qwen".into(),
                base_url: base,
                api_key: key,
                client: Client::new(),
            }));
        }
        // Claude
        if let Ok(base) = env::var("LLM_PROVIDER_CLAUDE_BASE") {
            let key = env::var("LLM_PROVIDER_CLAUDE_KEY").unwrap_or_default();
            chain.push(Box::new(ClaudeProvider {
                name: "claude".into(),
                base_url: base,
                api_key: key,
                client: Client::new(),
            }));
        }

        // 兜底:无 env 配置时放一个 stub,保证 cargo check + 启动可跑
        if chain.is_empty() {
            chain.push(Box::new(StubProvider));
        }

        let default_model = env::var("LLM_DEFAULT_MODEL").unwrap_or_else(|_| "gpt-4o-mini".into());
        Self {
            chain,
            default_model,
        }
    }

    pub async fn call(
        &self,
        provider_hint: Option<&str>,
        model: Option<&str>,
        prompt: &str,
        max_tokens: Option<i32>,
        stream: bool,
        trace_id: &str,
    ) -> Result<CallResult, ProviderError> {
        let model = model.unwrap_or(&self.default_model);

        // hint 命中优先,否则走降级链
        let order: Vec<usize> = if let Some(hint) = provider_hint {
            self.chain
                .iter()
                .position(|p| p.name() == hint)
                .into_iter()
                .chain((0..self.chain.len()).filter(|&i| self.chain[i].name() != hint))
                .collect()
        } else {
            (0..self.chain.len()).collect()
        };

        let mut last_err: Option<ProviderError> = None;
        for i in order {
            let p = &self.chain[i];
            match p.call(model, prompt, max_tokens, stream, trace_id).await {
                Ok(content) => {
                    return Ok(CallResult {
                        content,
                        provider: p.name().into(),
                        model: model.into(),
                    });
                }
                Err(e) => {
                    tracing::warn!(provider = p.name(), err = %e, "provider failed, trying fallback");
                    last_err = Some(e);
                }
            }
        }
        Err(last_err.unwrap_or_else(|| ProviderError("no provider configured".into())))
    }

    /// 失败时给成本记录提供 provider/model 名
    pub fn fallback_names(&self, hint: Option<&str>, model: Option<&str>) -> (String, String) {
        let p = hint
            .map(|h| h.to_string())
            .or_else(|| self.chain.first().map(|p| p.name().to_string()))
            .unwrap_or_else(|| "unknown".into());
        let m = model
            .map(|m| m.to_string())
            .unwrap_or_else(|| self.default_model.clone());
        (p, m)
    }
}

// ---------- OpenAI 兼容 ----------

struct OpenAiCompat {
    name: String,
    base_url: String,
    api_key: String,
    client: Client,
}

#[derive(Serialize)]
struct OaReq<'a> {
    model: &'a str,
    messages: Vec<OaMsg<'a>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    max_tokens: Option<i32>,
    stream: bool,
}

#[derive(Serialize)]
struct OaMsg<'a> {
    role: &'a str,
    content: &'a str,
}

#[derive(Deserialize)]
struct OaResp {
    choices: Vec<OaChoice>,
}

#[derive(Deserialize)]
struct OaChoice {
    message: OaRespMsg,
}

#[derive(Deserialize)]
struct OaRespMsg {
    content: String,
}

#[async_trait]
impl LlmProvider for OpenAiCompat {
    fn name(&self) -> &str {
        &self.name
    }
    async fn call(
        &self,
        model: &str,
        prompt: &str,
        max_tokens: Option<i32>,
        stream: bool,
        trace_id: &str,
    ) -> Result<String, ProviderError> {
        let url = format!(
            "{}/v1/chat/completions",
            self.base_url.trim_end_matches('/')
        );
        let body = OaReq {
            model,
            messages: vec![OaMsg {
                role: "user",
                content: prompt,
            }],
            max_tokens,
            stream,
        };
        let resp = self
            .client
            .post(&url)
            .bearer_auth(&self.api_key)
            .header("x-trace-id", trace_id)
            .json(&body)
            .send()
            .await
            .map_err(|e| ProviderError(e.to_string()))?;
        if !resp.status().is_success() {
            return Err(ProviderError(format!("http {}", resp.status())));
        }
        // 流式场景:聚合 SSE delta(简化版,实测可改进)
        if stream {
            let text = resp
                .text()
                .await
                .map_err(|e| ProviderError(e.to_string()))?;
            return Ok(parse_sse_text(&text));
        }
        let parsed: OaResp = resp
            .json()
            .await
            .map_err(|e| ProviderError(e.to_string()))?;
        parsed
            .choices
            .into_iter()
            .next()
            .map(|c| c.message.content)
            .ok_or_else(|| ProviderError("empty choices".into()))
    }
}

// ---------- Claude(Anthropic /messages) ----------

struct ClaudeProvider {
    name: String,
    base_url: String,
    api_key: String,
    client: Client,
}

#[derive(Serialize)]
struct ClReq<'a> {
    model: &'a str,
    max_tokens: i32,
    messages: Vec<OaMsg<'a>>,
}

#[derive(Deserialize)]
struct ClResp {
    content: Vec<ClBlock>,
}

#[derive(Deserialize)]
struct ClBlock {
    #[serde(rename = "type")]
    _kind: String,
    text: Option<String>,
}

#[async_trait]
impl LlmProvider for ClaudeProvider {
    fn name(&self) -> &str {
        &self.name
    }
    async fn call(
        &self,
        model: &str,
        prompt: &str,
        max_tokens: Option<i32>,
        _stream: bool,
        trace_id: &str,
    ) -> Result<String, ProviderError> {
        let url = format!("{}/v1/messages", self.base_url.trim_end_matches('/'));
        let body = ClReq {
            model,
            max_tokens: max_tokens.unwrap_or(1024),
            messages: vec![OaMsg {
                role: "user",
                content: prompt,
            }],
        };
        let resp = self
            .client
            .post(&url)
            .header("x-api-key", &self.api_key)
            .header("anthropic-version", "2023-06-01")
            .header("x-trace-id", trace_id)
            .json(&body)
            .send()
            .await
            .map_err(|e| ProviderError(e.to_string()))?;
        if !resp.status().is_success() {
            return Err(ProviderError(format!("http {}", resp.status())));
        }
        let parsed: ClResp = resp
            .json()
            .await
            .map_err(|e| ProviderError(e.to_string()))?;
        Ok(parsed
            .content
            .into_iter()
            .filter_map(|b| b.text)
            .collect::<Vec<_>>()
            .join("\n"))
    }
}

impl From<String> for ProviderError {
    fn from(s: String) -> Self {
        ProviderError(s)
    }
}

// ---------- Stub(无 env 配置兜底) ----------

struct StubProvider;

#[async_trait]
impl LlmProvider for StubProvider {
    fn name(&self) -> &str {
        "stub"
    }
    async fn call(
        &self,
        model: &str,
        prompt: &str,
        _max: Option<i32>,
        _stream: bool,
        _trace: &str,
    ) -> Result<String, ProviderError> {
        Ok(format!("[stub provider][{}] echo: {}", model, prompt))
    }
}

// ---------- SSE delta 简易解析 ----------

fn parse_sse_text(raw: &str) -> String {
    let mut out = String::new();
    for line in raw.lines() {
        let line = line.trim();
        if !line.starts_with("data:") {
            continue;
        }
        let payload = line[5..].trim();
        if payload == "[DONE]" {
            break;
        }
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(payload) {
            if let Some(delta) = v["choices"][0]["delta"]["content"].as_str() {
                out.push_str(delta);
            }
        }
    }
    out
}
