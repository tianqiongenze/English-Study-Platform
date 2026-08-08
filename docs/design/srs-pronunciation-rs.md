# SRS — 发音评分服务(pronunciation-rs)需求规格说明书

> 版本:1.0  日期:2026-08-06
> 关联:`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`(pronunciation_attempts)、`01-需求分析文档-SRS.md`

## 1. 背景

现有 gateway-go 仅提供 ASR 转写,无音素级发音评分。口语壁垒的核心是发音纠正(音标/重音/语调/最小对立体),需独立 Rust 服务 `pronunciation-rs`,以 GOP(Goodness of Pronunciation)+ 编辑距离做音素级评分,二期接入 MFA(Montreal Forced Aligner)提精度。一期聚焦"可解释、低延迟、可降级"。

## 2. 用户角色

| 角色 | 场景 |
|---|---|
| 学习者(Learner) | 跟读 → 上传音频 → 拿到音素级评分 + 纠错建议 |
| 口语陪练(gateway-go) | WS 会话中把音频片段送 pronunciation-rs 实时评分 |
| 教师(teacher-java) | 查看学生发音薄弱音素汇总 |
| 系统(learning-java) | 接收发音 mastery 回写 |

## 3. 功能需求(FR)

### FR-1 发音评分
- FR-1.1 `/api/pronunciation/score` 接收 user_id + target_text + audio_uri(对象存储引用)。
- FR-1.2 一期:ASR 转写音频 → 与 target_text 做音素对齐(编辑距离)→ 逐音素打分(0-100)。
- FR-1.3 综合分 overall_score = weighted(发音分/流利度/完整度),各维度落 pronunciation_attempts。
- FR-1.4 phoneme_scores JSON:`[{phoneme, score, expected, actual}]`。
- FR-1.5 feedback JSON:重音/语调/最小对立体建议(规则引擎)。

### FR-2 音素对齐(一期)
- FR-2.1 音素级对齐用编辑距离(Levenshtein on phoneme sequence),DTW 兜底。
- FR-2.2 二期切换 MFA 强制对齐,接口契约不变,内部实现替换。

### FR-3 评分历史与统计
- FR-3.1 `/api/pronunciation/history?user_id=` 返回最近 N 次评分。
- FR-3.2 `/api/pronunciation/stats?user_id=` 返回用户薄弱音素 Top-K 聚合。

### FR-4 纠错建议
- FR-4.1 基于低于阈值音素生成最小对立体训练建议(如 /θ/ vs /s/)。
- FR-4.2 重音/语调偏差基于规则标注(一期规则,二期 MFA+F0)。

## 4. 非功能需求(NFR)

| 编号 | 维度 | 要求 |
|---|---|---|
| NFR-1 | 性能 | 音频<10s 时 P95 < 600ms;ASR/GOP 并行 |
| NFR-2 | 可用性 | ASR 不可达降级:返回 target_text 文本对齐分(不阻断);单节点 RD 目标 99% |
| NFR-3 | 可观测 | OTel service.name=pronunciation-rs;关键事件结构化日志→esp_shared.app_logs |
| NFR-4 | 安全 | 不直接对外,经 bff/gateway 前置 JWT;服务间信任集群内调用 |
| NFR-5 | 数据 | 写 tidb-primary(esp_pronunciation)/ 读 tidb-follower;Redis 缓存音素模型特征 |
| NFR-6 | 扩展 | axum + tokio 多 worker;HPA 按 CPU 扩(多节点);无状态(评分落库) |
| NFR-7 | 容错 | ASR timeout 3s + 降级;音频超 30s 拒绝(413) |

## 5. 边界(不做什么)

- **不做** ASR 训练(复用 gateway-go ASR 或外部 provider)。
- **不做** TTS 合成(由 gateway-go)。
- **不做** 鉴权(bff 前置)。
- **不做** 用户管理(bff/learning)。
- 二期才做 MFA + F0 语调曲线,一期不引入重模型。

## 6. 验收标准(AC)

- AC-1:端点(/healthz + /score + /history + /stats)全 200,契约符合 SDD。
- AC-2:ASR 不可达时 /score 仍 200 返回降级文本对齐分。
- AC-3:评分落库 pronunciation_attempts,history 可查回。
- AC-4:P95 < 600ms(音频<10s,压测基线)。
- AC-5:trace 在 Tempo 可见(service.name=pronunciation-rs)。
- AC-6:单测覆盖对齐/评分/降级路径 ≥80%;集成测试覆盖 /score 端到端(mock ASR)。
