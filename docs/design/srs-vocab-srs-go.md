# SRS — 词汇记忆服务(vocab-srs-go)需求规格说明书

> 版本:1.0  日期:2026-08-06
> 关联:`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`(vocabulary/user_vocabulary)、`01-需求分析文档-SRS.md`

## 1. 背景

ESP 无独立词汇服务,learning-java 仅做知识点 mastery。词汇是英语学习高频刚需,需独立 Go 服务 `vocab-srs-go`,以 SM-2+ 间隔重复算法驱动复习调度,辅以词根词缀与词汇量测评。对标百词斩/扇贝。

## 2. 用户角色

| 角色 | 场景 |
|---|---|
| 学习者 | 每日复习到期词 → 评级(认识/模糊/不认识)→ 算法调下次到期;词汇量测评 |
| 教师 | 为班级布置词表,查看学生词汇掌握 |
| 系统(learning-java) | 接收词汇 mastery 回写,联动学习路径 |

## 3. 功能需求(FR)

### FR-1 词库与词根词缀
- FR-1.1 vocabulary 表:word/pos/definition/phonetic/frequency_rank/cefr/root/affix/example。
- FR-1.2 `/api/vocab/words` 按 cefr/frequency_rank 分页查询。
- FR-1.3 `/api/vocab/word/{id}` 详情含词根词缀解析。

### FR-2 SRS 复习调度(SM-2+)
- FR-2.1 `/api/vocab/due?user_id=` 返回到期词(user_vocabulary.due_at <= now)。
- FR-2.2 `/api/vocab/review` 接收 user_id+word_id+quality(0-5),SM-2+ 更新 srs_interval/srs_ease/due_at/review_count/lapse_count/mastery_score,落 user_vocabulary。
- FR-2.3 新词首次学习:due_at=now,ease=2.5,interval=1。
- FR-2.4 quality<3 视为遗忘,lapse_count+1,interval 重置 1。

### FR-3 词汇量测评
- FR-3.1 `/api/vocab/estimate` 抽样不同 CEFR 词,自适应估计用户词汇量。

### FR-4 进度与统计
- FR-4.1 `/api/vocab/progress?user_id=` 返回已学/已掌握/到期数。
- FR-4.2 词根词缀聚合:按 root 分组查看掌握度。

## 4. 非功能需求(NFR)

| 编号 | 维度 | 要求 |
|---|---|---|
| NFR-1 | 性能 | due/review P95 < 80ms;L1 进程缓存 + L2 Redis |
| NFR-2 | 可用性 | DB 不可达返回缓存到期词(降级);单节点 99% |
| NFR-3 | 可观测 | OTel service.name=vocab-srs-go;日志→esp_shared.app_logs |
| NFR-4 | 安全 | 经 bff 前置 JWT |
| NFR-5 | 数据 | 写 tidb-primary(esp_vocab)/ 读 tidb-follower;Redis 缓存到期队列 |
| NFR-6 | 扩展 | gin + Disruptor(批量写);HPA 按 CPU 扩 |
| NFR-7 | 容错 | DB timeout 2s + 降级;review 失败重试 3 次 |

## 5. 边界

- **不做** 鉴权/用户管理。
- **不做** 语音发音(由 pronunciation-rs)。
- **不做** 例句生成 LLM(由 llm-orchestrator-rs,可选调用)。

## 6. 验收标准(AC)

- AC-1:端点(/healthz + /words + /word/{id} + /due + /review + /estimate + /progress)全 200。
- AC-2:SM-2+ 算法单测覆盖 quality 0-5 全路径,interval/ease 计算正确。
- AC-3:review 落 user_vocabulary,due 可查回。
- AC-4:due/review P95 < 80ms(缓存命中)。
- AC-5:trace Tempo 可见。
- AC-6:单测 ≥80%。
