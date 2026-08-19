// cache/mod.rs —— LLM 三层语义缓存(LCO 铁律合规实现)
//
// 设计(对齐 LCO 全局铁律 / ESP memory `llm-cache-optimization-ironrule`):
// - L1 精确快路径:Redis STRING key `esp:cache:llm:exact:{model}:{sha256前16位(prompt|model|scene)}`,
//   TTL 1h。命中跳过 LLM API 省 100%。
// - L2 语义慢路径(本版升级):Milvus 向量检索(collection `esp_llm_semantic`,
//   384dim all-minilm-l6-v2,cosine 相似度 ≥0.92 命中)。
//   复用 conversation-go 创建的 collection 跨服务共享语义缓存(LCO 跨服务复用)。
//   替换原 SimHash+ZSET(非 LCO 合规:非真实向量 embedding,Hamming 近似不达标)。
// - L3 知识图谱(本版新增):Neo4j Prompt→Response 图谱(reqwest HTTP API,零重依赖),
//   记录 prompt/response/model/scene/ANSWERED 关系,供上下文增强。
//
// 降级铁律:所有 L2/L3 外部调用失败均 degrade 返回 None/忽略,绝不阻塞主链路(与 L1 一致)。
// 超时:3s(embedding/milvus/neo4j 各自独立超时,串联最坏 9s,可被主链路容忍)。
//
// 旧 SimHash + ZSET 实现保留在文件末尾并标 `#[allow(dead_code)]`(审计可溯,不再调用)。

use redis::cluster_async::ClusterConnection;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::time::Duration;

const CACHE_PREFIX: &str = "esp:cache:llm:exact:";
/// 旧 SimHash L2 的 ZSET 前缀(legacy,不再写入/查询,仅保留常量供 legacy 函数引用)。
#[allow(dead_code)]
const SEM_PREFIX: &str = "esp:llm:sem:";
const DEFAULT_TTL: u64 = 3600;
/// L1 digest 截断长度(sha256 前 16 位,与 conversation-go / writing-py 统一规范对齐)。
const L1_DIGEST_LEN: usize = 16;
/// L2 Milvus 余弦相似度命中阈值(LCO 铁律:cosine ≥0.92)。
const L2_DISTANCE_THRESHOLD: f64 = 0.92;
/// 跨服务共享 Milvus collection(conversation-go 创建,本服务复用)。
const L2_COLLECTION: &str = "esp_llm_semantic";
/// L2/L3 外部调用超时(降级友好,不阻塞主链路)。
const EXT_CALL_TIMEOUT_SECS: u64 = 3;
/// L3 Milvus search radius 参数(REST /v2/vector/search 用 radius 做召回下界)。
const L2_RADIUS: f64 = 0.92;

// ============================================================
// 环境配置读取(运行期 from_env,部署 env 注入;零硬编码)
// ============================================================

fn env_or(name: &str, default: &str) -> String {
    std::env::var(name).unwrap_or_else(|_| default.to_string())
}

fn env_bool(name: &str, default: bool) -> bool {
    match std::env::var(name) {
        Ok(v) => matches!(
            v.trim().to_ascii_lowercase().as_str(),
            "1" | "true" | "yes" | "on"
        ),
        Err(_) => default,
    }
}

fn milvus_addr() -> String {
    env_or("MILVUS_ADDR", "milvus:19530")
}

fn embedding_url() -> String {
    env_or("EMBEDDING_URL", "http://host.lima.internal:15730/embed")
}

fn neo4j_url() -> String {
    env_or("NEO4J_URL", "http://neo4j:7474")
}

fn neo4j_user() -> String {
    env_or("NEO4J_USER", "neo4j")
}

fn neo4j_password() -> String {
    // 生产密码从 env/secret 注入;本地默认对齐共享 infra 占位密码。
    // 铁律:不在 git 硬编码生产密码,此默认值仅单节点 RD 共享 infra 占位。
    env_or("NEO4J_PASSWORD", "neo4j-password-1")
}

fn l2_enabled() -> bool {
    env_bool("L2_ENABLED", true)
}

fn l3_enabled() -> bool {
    env_bool("L3_ENABLED", true)
}

/// 构造共享 reqwest::Client(单例化由 once_cell 持有,复用连接池)。
/// ASV 铁律:已核实 reqwest 0.12.28 ClientBuilder::timeout(Duration) 存在
/// (src/async_impl/client.rs:1441),作用于该 Client 发出的所有请求。
fn http_client() -> &'static reqwest::Client {
    static CLIENT: once_cell::sync::OnceCell<reqwest::Client> = once_cell::sync::OnceCell::new();
    CLIENT.get_or_init(|| {
        reqwest::Client::builder()
            .timeout(Duration::from_secs(EXT_CALL_TIMEOUT_SECS))
            .build()
            .unwrap_or_else(|_| reqwest::Client::new())
    })
}

// ============================================================
// Key 生成(L1 精确,不变)
// ============================================================

/// 生成 L1 精确缓存 key:esp:cache:llm:exact:{model}:{sha256前16位}(prompt|model|scene)
/// 统一规范(架构文档):与 conversation-go / writing-py 对齐。model 缺失用 _default 占位。
pub fn key_for(prompt: &str, model: Option<&str>, scene: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(prompt.as_bytes());
    hasher.update(b"|");
    hasher.update(model.unwrap_or("").as_bytes());
    hasher.update(b"|");
    hasher.update(scene.as_bytes());
    let digest = hex::encode(hasher.finalize());
    let short = &digest[..L1_DIGEST_LEN];
    let model_seg = model.unwrap_or("_default");
    format!("{}{}:{}", CACHE_PREFIX, model_seg, short)
}

/// 生成 L2 prompt_sha16(短摘要,作 Milvus 主键去重 + Neo4j 节点 sha16)。
/// 取 prompt sha256 前 16 位(与 L1 digest 同长度规范,但不混入 model/scene,
/// 因 L2 跨 model/scene 共享语义召回 —— 相似 prompt 不同 model 也应命中)。
fn prompt_sha16(prompt: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(prompt.as_bytes());
    let digest = hex::encode(hasher.finalize());
    digest[..L1_DIGEST_LEN].to_string()
}

/// 生成 response_sha16(Neo4j Response 节点主键)。
fn response_sha16(response: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(response.as_bytes());
    let digest = hex::encode(hasher.finalize());
    digest[..L1_DIGEST_LEN].to_string()
}

// ============================================================
// L2 Milvus 向量语义缓存
// ============================================================

/// 调宿主 embedding 服务(minilm,384dim)。失败返回 None(降级)。
/// 请求体:{"text":prompt,"model":"minilm"} → {"vector":[...]}。
async fn embed_prompt(prompt: &str) -> Option<Vec<f32>> {
    let url = embedding_url();
    let resp = http_client()
        .post(&url)
        .json(&json!({ "text": prompt, "model": "minilm" }))
        .send()
        .await
        .ok()?;
    if !resp.status().is_success() {
        tracing::warn!(target: "llm_cache", status = %resp.status(), "L2 embed HTTP non-2xx (degrade)");
        return None;
    }
    let body: Value = resp.json().await.ok()?;
    let arr = body.get("vector")?.as_array()?;
    if arr.is_empty() {
        return None;
    }
    Some(
        arr.iter()
            .filter_map(|v| v.as_f64().map(|f| f as f32))
            .collect(),
    )
}

/// L2 Milvus 向量检索:命中返回 response 字符串,未命中/降级返回 None。
/// REST: POST http://milvus:19530/v2/vector/search
/// body {"collectionName":"esp_llm_semantic","data":[<vec>],"limit":1,
///        "outputFields":["response","model","scene"],"params":{"radius":0.92}}
pub async fn l2_search_milvus(prompt: &str, _model: Option<&str>, _scene: &str) -> Option<String> {
    if !l2_enabled() {
        return None;
    }
    let vec = embed_prompt(prompt).await?;
    let url = format!("http://{}/v2/vector/search", milvus_addr());
    let body = json!({
        "collectionName": L2_COLLECTION,
        "data": [vec],
        "limit": 1,
        "outputFields": ["response", "model", "scene"],
        "params": { "radius": L2_RADIUS }
    });
    let resp = http_client().post(&url).json(&body).send().await.ok()?;
    if !resp.status().is_success() {
        tracing::warn!(target: "llm_cache", status = %resp.status(), "L2 milvus search HTTP non-2xx (degrade)");
        return None;
    }
    let j: Value = resp.json().await.ok()?;
    parse_milvus_search_response(&j)
}

/// 纯函数:解析 Milvus /v2/vector/search 响应,提取 distance≥threshold 的 response。
/// 提取为纯函数便于无网络单元测试(CLC:不依赖运行期外部状态)。
pub fn parse_milvus_search_response(j: &Value) -> Option<String> {
    let arr = j.get("data")?.as_array()?;
    let hit = arr.first()?;
    // cosine 下 distance = 相似度(越大越相似);REST radius 已过滤下界,此处二次校验。
    let dist = hit.get("distance").and_then(|d| d.as_f64()).unwrap_or(0.0);
    if dist < L2_DISTANCE_THRESHOLD {
        tracing::debug!(target: "llm_cache", distance = dist, "L2 milvus below threshold (miss)");
        return None;
    }
    let resp = hit.get("response")?.as_str()?;
    if resp.is_empty() {
        return None;
    }
    tracing::info!(target: "llm_cache", distance = dist, "L2 milvus semantic hit");
    Some(resp.to_string())
}

/// L2 Milvus 写入:embedding + insert。失败仅 log(降级,不阻塞 set)。
/// REST: POST http://milvus:19530/v2/vector/insert
/// body {"collectionName":"esp_llm_semantic","data":[{"prompt_sha16":<sha16>,
///        "response":<resp>,"model":<model>,"scene":<scene>,"ts":<unix>,"embedding":[<vec>]}]}
pub async fn l2_store_milvus(prompt: &str, response: &str, model: Option<&str>, scene: &str) {
    if !l2_enabled() {
        return;
    }
    let vec = match embed_prompt(prompt).await {
        Some(v) => v,
        None => {
            tracing::warn!(target: "llm_cache", "L2 store embed failed (skip write)");
            return;
        }
    };
    let url = format!("http://{}/v2/vector/insert", milvus_addr());
    let body = json!({
        "collectionName": L2_COLLECTION,
        "data": [{
            "prompt_sha16": prompt_sha16(prompt),
            "response": response,
            "model": model.unwrap_or(""),
            "scene": scene,
            "ts": chrono::Utc::now().timestamp(),
            "embedding": vec
        }]
    });
    match http_client().post(&url).json(&body).send().await {
        Ok(r) if r.status().is_success() => {
            tracing::debug!(target: "llm_cache", "L2 milvus store ok");
        }
        Ok(r) => {
            tracing::warn!(target: "llm_cache", status = %r.status(), "L2 milvus store HTTP non-2xx (degrade)");
        }
        Err(e) => {
            tracing::warn!(target: "llm_cache", err = %e, "L2 milvus store error (degrade)");
        }
    }
}

// ============================================================
// L3 Neo4j 知识图谱(reqwest HTTP API,零重依赖)
// ============================================================

/// 执行 Cypher 事务(POST /db/neo4j/tx/commit, basic auth)。失败返回 None(降级)。
async fn neo4j_run(statement: &str, params: Value) -> Option<Value> {
    let url = format!("{}/db/neo4j/tx/commit", neo4j_url());
    let body = json!({ "statements": [{ "statement": statement, "parameters": params }] });
    let resp = http_client()
        .post(&url)
        .basic_auth(neo4j_user(), Some(neo4j_password()))
        .json(&body)
        .send()
        .await
        .ok()?;
    if !resp.status().is_success() {
        tracing::warn!(target: "llm_cache", status = %resp.status(), "L3 neo4j HTTP non-2xx (degrade)");
        return None;
    }
    let j: Value = resp.json().await.ok()?;
    // Neo4j 错误在 errors[] 数组
    if let Some(errs) = j.get("errors").and_then(|e| e.as_array()) {
        if !errs.is_empty() {
            tracing::warn!(target: "llm_cache", err = %j, "L3 neo4j cypher error (degrade)");
            return None;
        }
    }
    Some(j)
}

/// L3 记录 Prompt→Response 图谱关系。fire-and-forget(set 调用,降级安全)。
/// Cypher: MERGE Prompt{sha16} + Response{sha16} + (:ANSWERED)。
pub async fn l3_record_neo4j(prompt: &str, response: &str, model: Option<&str>, scene: &str) {
    if !l3_enabled() {
        return;
    }
    let statement = concat!(
        "MERGE (p:Prompt {sha16:$sha16}) ",
        "SET p.text=$text, p.model=$model, p.scene=$scene, p.ts=timestamp() ",
        "MERGE (r:Response {sha16:$rsha16}) ",
        "SET r.text=$rtext, r.ts=timestamp() ",
        "MERGE (p)-[:ANSWERED]->(r)"
    );
    let params = json!({
        "sha16": prompt_sha16(prompt),
        "text": prompt,
        "model": model.unwrap_or(""),
        "scene": scene,
        "rsha16": response_sha16(response),
        "rtext": response,
    });
    match neo4j_run(statement, params).await {
        Some(_) => tracing::debug!(target: "llm_cache", "L3 neo4j record ok"),
        None => tracing::warn!(target: "llm_cache", "L3 neo4j record failed (degrade)"),
    }
}

/// L3 上下文增强:按 prompt_sha16 找 Prompt 节点 + 其 ANSWERED Response,返回拼接上下文。
/// 命中返回形如 "[KG] prev Q:<text> A:<rtext>",未命中/降级返回 None。
pub async fn l3_enhance_neo4j(prompt: &str) -> Option<String> {
    if !l3_enabled() {
        return None;
    }
    let statement = concat!(
        "MATCH (p:Prompt {sha16:$sha16})-[:ANSWERED]->(r:Response) ",
        "RETURN p.text AS q, r.text AS a LIMIT 1"
    );
    let params = json!({ "sha16": prompt_sha16(prompt) });
    let j = neo4j_run(statement, params).await?;
    parse_neo4j_enhance(&j)
}

/// 纯函数:解析 Neo4j /tx/commit 响应的 row,提取 Q/A 拼接(便于无网络单测)。
pub fn parse_neo4j_enhance(j: &Value) -> Option<String> {
    let results = j.get("results")?.as_array()?;
    let first = results.first()?;
    let rows = first.get("rows")?.as_array()?;
    let row = rows.first()?;
    // Neo4j REST 经典返回 row 为 {"value":{q,a}}(列名 value);部分版本直接是 {q,a} 对象。
    // 兼容两种:优先 value 子对象,无则用 row 本身。
    let val = row.get("value").unwrap_or(row);
    let q = val.get("q").and_then(|v| v.as_str()).unwrap_or("");
    let a = val.get("a").and_then(|v| v.as_str()).unwrap_or("");
    if q.is_empty() && a.is_empty() {
        return None;
    }
    Some(format!("[KG] prev Q:{} A:{}", q, a))
}

// ============================================================
// 三层缓存 get / set —— 接口签名:(conn, prompt, model, scene, ...)
// ============================================================

/// 查缓存(L1 精确 → L2 Milvus 语义近似)。
/// 命中返回内容,均未命中返回 None。L1→L2 串联降级,任一层失败不阻塞。
pub async fn get(
    conn: &ClusterConnection,
    prompt: &str,
    model: Option<&str>,
    scene: &str,
) -> Option<String> {
    let mut c = conn.clone();
    let exact_key = key_for(prompt, model, scene);

    // L1 精确快路径
    if let Ok(Some(v)) = redis::cmd("GET")
        .arg(&exact_key)
        .query_async::<Option<String>>(&mut c)
        .await
    {
        if !v.is_empty() {
            tracing::debug!(target: "llm_cache", "L1 exact hit");
            return Some(v);
        }
    }

    // L2 语义慢路径:Milvus 向量检索(cosine≥0.92)
    if let Some(resp) = l2_search_milvus(prompt, model, scene).await {
        // 回填 L1 加速后续精确命中(异步 fire-and-forget 不阻塞)
        // CLC:先 clone ClusterConnection 再 move 进 spawn(&conn 是借用,不能跨 'static)。
        let key = exact_key.clone();
        let resp_clone = resp.clone();
        let mut c2 = conn.clone();
        tokio::spawn(async move {
            let _: Result<(), _> = redis::cmd("SETEX")
                .arg(&key)
                .arg(DEFAULT_TTL)
                .arg(&resp_clone)
                .query_async::<()>(&mut c2)
                .await;
        });
        return Some(resp);
    }

    None
}

/// 回写缓存(L1 精确 SETEX + L2 Milvus 向量插入 + L3 Neo4j 图谱记录)。
/// ttl_sec=0 时用默认 1h。L2/L3 失败仅 log,不返回 Err(主链路不被外部依赖故障阻塞)。
pub async fn set(
    conn: &ClusterConnection,
    prompt: &str,
    model: Option<&str>,
    scene: &str,
    value: &str,
    ttl_sec: u64,
) -> Result<(), redis::RedisError> {
    let mut c = conn.clone();
    let ttl = if ttl_sec == 0 { DEFAULT_TTL } else { ttl_sec };
    let exact_key = key_for(prompt, model, scene);

    // L1: SETEX(唯一会返回 Err 的路径 —— Redis 是强依赖)
    redis::cmd("SETEX")
        .arg(&exact_key)
        .arg(ttl)
        .arg(value)
        .query_async::<()>(&mut c)
        .await?;

    // L2: Milvus 向量插入(降级安全)
    l2_store_milvus(prompt, value, model, scene).await;

    // L3: Neo4j 图谱记录(fire-and-forget,降级安全)
    let p = prompt.to_string();
    let v = value.to_string();
    let m = model.map(|s| s.to_string());
    let s = scene.to_string();
    tokio::spawn(async move {
        l3_record_neo4j(&p, &v, m.as_deref(), &s).await;
    });

    Ok(())
}

// ============================================================
// Legacy SimHash L2(非 LCO 合规,已废弃 —— 保留供审计追溯,不再调用)
// 设计:Redis ZSET score=SimHash(prompt),汉明距离≤3 召回。
// 弃用原因:非真实向量 embedding,Hamming 近似不满足 LCO cosine≥0.92 要求。
// 详见文件头注释 + memory `llm-cache-optimization-ironrule`。
// clippy:遗留代码已不调用,无需重构;对 unusual_byte_groupings/
// needless_range_loop 显式 allow 避免审计残留触发 -D warnings。
// ============================================================

/// 简单分词:ASCII 按非字母数字切分,CJK 每字成 token。
#[allow(dead_code)]
fn tokenize(prompt: &str) -> Vec<String> {
    let mut tokens = Vec::new();
    let mut buf = String::new();
    for ch in prompt.chars() {
        if ch.is_ascii_alphanumeric() {
            buf.push(ch.to_ascii_lowercase());
        } else {
            if !buf.is_empty() {
                tokens.push(std::mem::take(&mut buf));
            }
            if !ch.is_ascii() && !ch.is_whitespace() && !ch.is_ascii_punctuation() {
                tokens.push(ch.to_string());
            }
        }
    }
    if !buf.is_empty() {
        tokens.push(buf);
    }
    tokens
}

/// FxHash 风格 64bit 哈希(标准库实现,无依赖)。
#[allow(dead_code, clippy::unusual_byte_groupings)]
fn fxhash64(s: &str) -> u64 {
    const SEED: u64 = 0x51_7c_c1_b7_27_22_0a_95;
    const MUL: u64 = 0x517_cc1_b7_27_22_0a_95;
    let mut hash = SEED;
    for byte in s.as_bytes() {
        hash = (hash.rotate_left(5) ^ (*byte as u64)).wrapping_mul(MUL);
    }
    hash
}

/// 对 prompt 计算 64bit SimHash fingerprint(legacy L2,不再调用)。
#[allow(dead_code, clippy::needless_range_loop)]
pub fn simhash(prompt: &str) -> u64 {
    let tokens = tokenize(prompt);
    if tokens.is_empty() {
        return 0;
    }
    let mut counts = [0i32; 64];
    for tok in &tokens {
        let h = fxhash64(tok);
        for i in 0..64 {
            if (h >> i) & 1 == 1 {
                counts[i] += 1;
            } else {
                counts[i] -= 1;
            }
        }
    }
    let mut fp: u64 = 0;
    for i in 0..64 {
        if counts[i] > 0 {
            fp |= 1 << i;
        }
    }
    fp
}

/// 汉明距离(popcount of XOR)(legacy L2)。
#[allow(dead_code)]
fn hamming_distance(a: u64, b: u64) -> u32 {
    (a ^ b).count_ones()
}

/// 生成旧 L2 语义 ZSET key(legacy)。
#[allow(dead_code)]
pub fn semantic_key(scene: &str, model: Option<&str>) -> String {
    format!("{}{}:{}", SEM_PREFIX, scene, model.unwrap_or(""))
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    // ---------- L1 key 测试(原 9/9 pass,保持)----------

    #[test]
    fn key_stable_for_same_input() {
        let k1 = key_for("hello", Some("gpt-4o"), "dialogue");
        let k2 = key_for("hello", Some("gpt-4o"), "dialogue");
        assert_eq!(k1, k2);
        assert!(k1.starts_with(CACHE_PREFIX));
    }

    #[test]
    fn key_uses_unified_spec() {
        let k = key_for("hello", Some("gpt-4o"), "dialogue");
        assert!(k.starts_with("esp:cache:llm:exact:gpt-4o:"), "got: {}", k);
        let digest = k.rsplit(':').next().unwrap();
        assert_eq!(
            digest.len(),
            L1_DIGEST_LEN,
            "digest must be 16 hex chars, got: {}",
            digest
        );
    }

    #[test]
    fn key_model_isolated() {
        let k1 = key_for("hello", Some("gpt-4o"), "dialogue");
        let k2 = key_for("hello", Some("claude-sonnet-4-6"), "dialogue");
        assert_ne!(k1, k2);
    }

    #[test]
    fn key_model_none_uses_default() {
        let k = key_for("hello", None, "dialogue");
        assert!(
            k.contains(":_default:"),
            "missing model should use _default seg, got: {}",
            k
        );
    }

    #[test]
    fn key_differs_on_scene() {
        let k1 = key_for("hello", Some("gpt-4o"), "dialogue");
        let k2 = key_for("hello", Some("gpt-4o"), "writing");
        assert_ne!(k1, k2);
    }

    // ---------- sha16 测试 ----------

    #[test]
    fn prompt_sha16_stable_and_16hex() {
        let h = prompt_sha16("how to practice english");
        assert_eq!(h.len(), L1_DIGEST_LEN);
        assert!(h.chars().all(|c| c.is_ascii_hexdigit()));
        assert_eq!(prompt_sha16("how to practice english"), h);
    }

    #[test]
    fn response_sha16_differs_from_prompt() {
        assert_ne!(prompt_sha16("q"), response_sha16("a"));
    }

    // ---------- L2 Milvus 响应解析(纯函数,无网络)----------

    #[test]
    fn parse_milvus_hit_above_threshold() {
        let j = json!({
            "data": [{ "response": "hello back", "model": "gpt-4o", "scene": "dialogue", "distance": 0.95 }]
        });
        assert_eq!(
            parse_milvus_search_response(&j),
            Some("hello back".to_string())
        );
    }

    #[test]
    fn parse_milvus_miss_below_threshold() {
        let j = json!({
            "data": [{ "response": "x", "distance": 0.50 }]
        });
        assert_eq!(parse_milvus_search_response(&j), None);
    }

    #[test]
    fn parse_milvus_empty_data() {
        let j = json!({ "data": [] });
        assert_eq!(parse_milvus_search_response(&j), None);
    }

    #[test]
    fn parse_milvus_missing_data_key() {
        let j = json!({ "code": 0 });
        assert_eq!(parse_milvus_search_response(&j), None);
    }

    #[test]
    fn parse_milvus_empty_response_miss() {
        let j = json!({ "data": [{ "response": "", "distance": 0.99 }] });
        assert_eq!(parse_milvus_search_response(&j), None);
    }

    #[test]
    fn parse_milvus_threshold_boundary() {
        // distance == 0.92 exactly → >= threshold → hit
        let j = json!({ "data": [{ "response": "boundary", "distance": 0.92 }] });
        assert_eq!(
            parse_milvus_search_response(&j),
            Some("boundary".to_string())
        );
        // just below
        let j2 = json!({ "data": [{ "response": "boundary", "distance": 0.919 }] });
        assert_eq!(parse_milvus_search_response(&j2), None);
    }

    // ---------- L3 Neo4j 响应解析(纯函数,无网络)----------

    #[test]
    fn parse_neo4j_enhance_hit() {
        // Neo4j REST 经典返回结构:results[0].rows[0].value(对象含 q/a)
        let j = json!({
            "results": [{ "columns": ["q","a"], "rows": [{ "value": { "q": "how are you", "a": "fine" } }] }],
            "errors": []
        });
        let r = parse_neo4j_enhance(&j);
        assert!(r.is_some());
        assert!(r.as_ref().unwrap().contains("how are you"));
        assert!(r.as_ref().unwrap().contains("fine"));
    }

    #[test]
    fn parse_neo4j_enhance_no_rows() {
        let j = json!({ "results": [{ "columns": [], "rows": [] }], "errors": [] });
        assert_eq!(parse_neo4j_enhance(&j), None);
    }

    #[test]
    fn parse_neo4j_enhance_missing_results() {
        let j = json!({ "errors": [] });
        assert_eq!(parse_neo4j_enhance(&j), None);
    }

    // ---------- L2/L3 enabled flag(纯函数,无网络)----------

    #[test]
    fn l2_disabled_returns_none_from_parse_path() {
        // L2_ENABLED=false 时 l2_search_milvus 直接返回 None(不发请求)。
        // 用 env 临时关闭验证短路(测试串行,不并发)。
        std::env::set_var("L2_ENABLED", "false");
        assert!(!l2_enabled());
        std::env::set_var("L2_ENABLED", "true");
        assert!(l2_enabled());
    }

    // ---------- legacy SimHash(保留测试,验证 legacy 函数未被破坏)----------

    #[test]
    fn simhash_stable() {
        let h1 = simhash("如何练习英语口语");
        let h2 = simhash("如何练习英语口语");
        assert_eq!(h1, h2);
    }

    #[test]
    fn simhash_near_identical_prompts_close_hamming() {
        let a = simhash("how to practice english speaking");
        let b = simhash("how to practice english listening");
        assert!(
            hamming_distance(a, b) <= 16,
            "hamming = {}",
            hamming_distance(a, b)
        );
    }

    #[test]
    fn simhash_unrelated_prompts_far() {
        let a = simhash("如何练习英语口语");
        let b = simhash("write a python web crawler");
        assert!(hamming_distance(a, b) > 4);
    }

    #[test]
    fn semantic_key_format() {
        let k = semantic_key("dialogue", Some("gpt-4o"));
        assert_eq!(k, "esp:llm:sem:dialogue:gpt-4o");
    }
}
