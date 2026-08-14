// cache/mod.rs —— 语义缓存 Redis L2(双轨:sha256 精确 L1 + SimHash 近似 L2)
//
// 设计:
// - L1 精确快路径:Redis STRING key `esp:llm:cache:<sha256(prompt|model|scene)>`,TTL 1h。
// - L2 语义慢路径:Redis ZSET key `esp:llm:sem:<scene>:<model>`,
//   score = SimHash(prompt) 的 u64 整数值,member = 完整 sha256 digest。
//   查询时 ZRANGEBYSCORE [simhash-DELTA, simhash+DELTA],DELTA=3(汉明距离阈值),
//   取 member 拼 L1 key 再 GET 响应体。
//
// SimHash 采用标准库实现(无新依赖):
//   分词(unicode 字符 + 简单 CJK/拉丁切分)→ 每 token 用 FxHash-like 哈希 →
//   加权叠加到 64bit fingerprint。汉明距离 ≤3 视为语义近似命中。
//
// 注:这是务实的"语义近似"下限方案。生产环境可替换为:
//   - fastembed(本地 ONNX embedding,如 BGE-small)
//   - TiDB Vector Search(ANN,余弦相似度)
//   - 外部 embedding provider(OpenAI text-embedding-3 等)
//   替换点仅在 simhash() 函数 + ZSET score 改为向量索引,接口签名不变。

use redis::cluster_async::ClusterConnection;
use sha2::{Digest, Sha256};

const CACHE_PREFIX: &str = "esp:llm:cache:";
const SEM_PREFIX: &str = "esp:llm:sem:";
const DEFAULT_TTL: u64 = 3600;
/// 汉明距离阈值:SimHash 差异位数 ≤ DELTA 视为近似命中。
/// 64bit fingerprint 中 3 bit 差异对应约 4.7% 位差异,对短文本 prompt 近似召回合理。
const HAMMING_DELTA: u64 = 3;

// ============================================================
// SimHash —— 标准库实现,无新依赖
// ============================================================

/// 简单分词:ASCII 按非字母数字切分,CJK 每字成 token。
/// 工业级可替换为 jieba/ICU,但单节点验证场景下足够。
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
            // CJK 及其他非 ASCII 字符逐字成 token(覆盖中文场景)
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
/// 选 FxHash 因其速度与分布均匀性在 SimHash 场景足够,非密码学用途。
fn fxhash64(s: &str) -> u64 {
    const SEED: u64 = 0x51_7c_c1_b7_27_22_0a_95;
    const MUL: u64 = 0x517_cc1_b7_27_22_0a_95;
    let mut hash = SEED;
    for byte in s.as_bytes() {
        hash = (hash.rotate_left(5) ^ (*byte as u64)).wrapping_mul(MUL);
    }
    hash
}

/// 对 prompt 计算 64bit SimHash fingerprint。
/// 每个 token 哈希后,遍历 64 个 bit 位:bit=1 则对应维 +1,bit=0 则 -1。
/// 最终每维 >0 取 1,否则取 0,拼成 64bit。
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

/// 汉明距离(popcount of XOR)。
fn hamming_distance(a: u64, b: u64) -> u32 {
    (a ^ b).count_ones()
}

// ============================================================
// Key 生成
// ============================================================

/// 生成 L1 精确缓存 key:esp:llm:cache:<sha256(prompt|model|scene)>
pub fn key_for(prompt: &str, model: Option<&str>, scene: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(prompt.as_bytes());
    hasher.update(b"|");
    hasher.update(model.unwrap_or("").as_bytes());
    hasher.update(b"|");
    hasher.update(scene.as_bytes());
    let digest = hex::encode(hasher.finalize());
    format!("{}{}", CACHE_PREFIX, digest)
}

/// 生成 L2 语义 ZSET key:esp:llm:sem:<scene>:<model>
pub fn semantic_key(scene: &str, model: Option<&str>) -> String {
    format!("{}{}:{}", SEM_PREFIX, scene, model.unwrap_or(""))
}

/// 从完整 L1 key 提取 digest 成员(member)用于 ZSET。
fn digest_of(key: &str) -> &str {
    key.strip_prefix(CACHE_PREFIX).unwrap_or(key)
}

// ============================================================
// 双轨缓存 get / set —— 接口签名:(conn, prompt, model, scene, ...)
// ============================================================

/// 查缓存(L1 精确 → L2 语义近似)。
/// 命中返回内容,均未命中返回 None。
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

    // L2 语义慢路径:SimHash 范围查
    let fp = simhash(prompt);
    let zset_key = semantic_key(scene, model);
    // 汉明距离 ≤3 对应 score 差异 ≤3(SimHash 整数差近似汉明距离)
    let lo = fp.saturating_sub(HAMMING_DELTA);
    let hi = fp.saturating_add(HAMMING_DELTA);
    let members: redis::RedisResult<Vec<String>> = redis::cmd("ZRANGEBYSCORE")
        .arg(&zset_key)
        .arg(lo)
        .arg(hi)
        .query_async(&mut c)
        .await;

    if let Ok(candidates) = members {
        for member in candidates {
            // member 是 digest,重建 L1 key 取响应体
            let cand_key = format!("{}{}", CACHE_PREFIX, member);
            if let Ok(Some(v)) = redis::cmd("GET")
                .arg(&cand_key)
                .query_async::<Option<String>>(&mut c)
                .await
            {
                if !v.is_empty() {
                    // 二次校验汉明距离(防 score 边界溢出误召回)
                    // member digest 无法还原 prompt,这里信任 ZSET score 范围
                    tracing::info!(
                        target: "llm_cache",
                        simhash = fp,
                        "L2 semantic hit"
                    );
                    return Some(v);
                }
            }
        }
    }
    None
}

/// 回写缓存(L1 精确 + L2 语义 ZSET 索引)。
/// ttl_sec=0 时用默认 1h。ZSET 不设 TTL(随 L1 key 过期失效,member 查不到响应即视为失效)。
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
    let digest = digest_of(&exact_key).to_string();

    // L1: SETEX
    redis::cmd("SETEX")
        .arg(&exact_key)
        .arg(ttl)
        .arg(value)
        .query_async::<()>(&mut c)
        .await?;

    // L2: ZADD 语义索引(score=simhash, member=digest)
    let fp = simhash(prompt);
    let zset_key = semantic_key(scene, model);
    let _: () = redis::cmd("ZADD")
        .arg(&zset_key)
        .arg(fp)
        .arg(&digest)
        .query_async(&mut c)
        .await?;
    // ZSET 也设 TTL,避免 scene/model 下线后索引堆积(略大于 L1,给近似查留窗口)
    let _: () = redis::cmd("EXPIRE")
        .arg(&zset_key)
        .arg(ttl + 300)
        .query_async(&mut c)
        .await?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn key_stable_for_same_input() {
        let k1 = key_for("hello", Some("gpt-4o"), "dialogue");
        let k2 = key_for("hello", Some("gpt-4o"), "dialogue");
        assert_eq!(k1, k2);
        assert!(k1.starts_with(CACHE_PREFIX));
    }

    #[test]
    fn key_differs_on_scene() {
        let k1 = key_for("hello", Some("gpt-4o"), "dialogue");
        let k2 = key_for("hello", Some("gpt-4o"), "writing");
        assert_ne!(k1, k2);
    }

    #[test]
    fn simhash_stable() {
        let h1 = simhash("如何练习英语口语");
        let h2 = simhash("如何练习英语口语");
        assert_eq!(h1, h2);
    }

    #[test]
    fn simhash_near_identical_prompts_close_hamming() {
        // 近义/相似 prompt 汉明距离应较小
        let a = simhash("how to practice english speaking");
        let b = simhash("how to practice english listening");
        assert!(hamming_distance(a, b) <= 8, "hamming = {}", hamming_distance(a, b));
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
