# SRS — 分级阅读服务(reading-go)需求规格说明书

> 版本:1.0  日期:2026-08-06
> 关联:`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`(reading_passages/reading_attempts)、`01-需求分析文档-SRS.md`

## 1. 背景

ESP 阅读能力完全缺失。阅读是输入与应试核心,需独立 Go 服务 `reading-go`,提供分级阅读(CEFR)+ 长难句解析 + 题目练习 + 生词标注,对标 Cake/Babbel。一期以素材库 + 题目 + 生词联动 vocab 为主。

## 2. 用户角色

| 角色 | 场景 |
|---|---|
| 学习者 | 选分级文章 → 阅读(生词点击查释义)→ 做题 → 拿分;长难句解析 |
| 教师 | 上传/编排阅读素材,查看班级阅读进度 |
| 系统(vocab-srs-go) | 生词加入用户词汇表复习;learning-java mastery 回写 |

## 3. 功能需求(FR)

### FR-1 分级阅读素材
- FR-1.1 reading_passages 按 level(A1-C2)+ category 分页查询。
- FR-1.2 返回 title/content/word_count/questions JSON。
- FR-1.3 热点文章缓存(L1 + L2 Redis)。

### FR-2 阅读练习与判分
- FR-2.1 `/api/reading/attempt` 记录 user_id+passage_id+answers+time_spent,落 reading_attempts。
- FR-2.2 自动判分(客观题),返回 score + 正确答案 + 解析。

### FR-3 生词标注与联动
- FR-3.1 文章中生词点击 → 查 vocabulary 释义;不存在则调 llm-orchestrator 生成释义并落库。
- FR-3.2 `/api/reading/mark-word` 将生词加入 user_vocabulary(due_at=now),联动 vocab-srs-go 复习。

### FR-4 长难句解析
- FR-4.1 `/api/reading/parse-sentence` 接收 sentence,返回句法树 AST + 中文翻译(调 llm-orchestrator)。
- FR-4.2 AST 缓存(Redis),同句不重复调 LLM。

### FR-5 进度与统计
- FR-5.1 `/api/reading/progress?user_id=` 返回已读篇数/总时长/平均分。

## 4. 非功能需求(NFR)

| 编号 | 维度 | 要求 |
|---|---|---|
| NFR-1 | 性能 | 列表/attempt P95 < 100ms;渲染(文章+题目)< 200ms;长难句解析 P95 < 1.5s(含 LLM,缓存命中 < 80ms) |
| NFR-2 | 可用性 | LLM 不可达降级:解析返回空+提示;vocab 不可达:生词标记先落本地队列;单节点 99% |
| NFR-3 | 可观测 | OTel service.name=reading-go;日志→esp_shared.app_logs |
| NFR-4 | 安全 | 经 bff 前置 JWT |
| NFR-5 | 数据 | 写 tidb-primary(esp_reading)/ 读 tidb-follower;Redis 缓存 |
| NFR-6 | 扩展 | gin + Disruptor;HPA 按 CPU 扩 |
| NFR-7 | 容错 | LLM timeout 8s + 降级;AST 缓存兜底 |

## 5. 边界

- **不做** 文章内容生产(运营/LLM 辅助,二期)。
- **不做** 鉴权/用户管理。
- **不做** 词汇复习调度(由 vocab-srs-go)。
- 一期不做实时翻译全文,仅长难句级。

## 6. 验收标准(AC)

- AC-1:端点(/healthz + /list + /attempt + /mark-word + /parse-sentence + /progress)全 200。
- AC-2:attempt 落 reading_attempts,progress 可聚合。
- AC-3:mark-word 后 user_vocabulary(vocab-srs-go)可见该词。
- AC-4:parse-sentence 缓存命中(二次 < 80ms)。
- AC-5:LLM 不可达时 parse-sentence 降级不 5xx。
- AC-6:trace Tempo 可见;单测 ≥80%。
