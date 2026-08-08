# SRS — 写作增强服务(writing-coach)需求规格说明书

> 版本:1.0  日期:2026-08-05
> 关联:ADR `adr-writing-coach.md`、SRS 主文档、schema.sql(writing_* 表)

## 1. 背景

现有 `writing-py` 是"无状态 AI 批改引擎"(grade/polish/translate/review,LangGraph 多步)。教学场景需要更体系化的写作辅导能力:模板与范文参考、多维度综合评分、个性化提升方案及其执行跟踪。这些需持久化、跨会话、与学习路径联动,属"教学辅导"职责,与 writing-py 解耦为独立服务 `writing-coach`。

## 2. 用户角色

| 角色 | 场景 |
|---|---|
| 学习者(Learner) | 查模板/范文 → 写作 → 综合批改 → 查提升方案 → 按方案练习 → 记录进度 |
| 教师(Teacher) | 为学生定制提升方案,查看进度,布置写作任务 |
| 系统(learning-java) | 提供 user_mastery 薄弱点数据驱动提升方案生成 |

## 3. 功能需求(FR)

### FR-1 写作模板推荐
- FR-1.1 按 category(argumentative/letter/chart/narrative)与 difficulty 列出模板。
- FR-1.2 返回段落结构骨架(structure),供学习者套用。
- FR-1.3 模板热点缓存(L1 cachetools + L2 Redis Cluster)。

### FR-2 范文推荐
- FR-2.1 按 category/difficulty 列范文,返回 title/content/word_count。
- FR-2.2 范文热点缓存。

### FR-3 综合批改与评分
- FR-3.1 `/assess` 接收 user_id+prompt+essay+exam。
- FR-3.2 内部调 `writing-py /api/writing/grade` 拿基础分(复用既有 AI 引擎,不重复造轮子)。
- FR-3.3 叠加 rubric 多维评分:语法/连贯/词汇/任务完成,各维度独立打分。
- FR-3.4 总分 = 各维度加权和(0-100)。
- FR-3.5 writing-py 不可达时降级:基础分置 0,rubric 仍按文本特征估算,不阻断响应。

### FR-4 写作能力提升方案
- FR-4.1 `/plan` 接收 user_id+goal(toefl/ielts/general)。
- FR-4.2 内部调 `learning-java /api/learning/mastery/weak?user_id=` 拿薄弱点。
- FR-4.3 LLM(anthropic/mock)基于薄弱点+目标生成分阶段(4 阶段×2 周)提升方案。
- FR-4.4 方案落库 writing_plans。
- FR-4.5 learning 不可达时降级:用默认薄弱点(grammar-tense/coherence-transition/vocabulary-variety)。

### FR-5 提升方案查询
- FR-5.1 `/plan/{user_id}` 返回用户最近一份活跃方案。

### FR-6 练习进度记录
- FR-6.1 `/progress` 记录用户在某个方案下某任务的完成情况与得分。
- FR-6.2 落库 writing_progress,供后续学情分析。

## 4. 非功能需求(NFR)

| 编号 | 维度 | 要求 |
|---|---|---|
| NFR-1 | 性能 | templates/essays P95 < 80ms(缓存命中);assess P95 < 1.2s(含 writing-py 调用);plan P95 < 3s(含 LLM) |
| NFR-2 | 可用性 | 降级路径:writing-py/learning/LLM 任一不可达,服务仍返回(不 5xx);单节点 RD 目标 99% |
| NFR-3 | 可观测 | OTel trace 上报 otel-collector;service.name=writing-coach;关键事件结构化日志→app_logs |
| NFR-4 | 安全 | 不直接对外,经 gateway/Ingress;JWT 鉴权由 bff 前置(本服务信任集群内调用) |
| NFR-5 | 数据 | 写主 tidb-primary / 读从 tidb-follower(esp-db-py 自动);Redis Cluster 多级缓存 |
| NFR-6 | 扩展 | Granian 2 worker 起步;HPA 按 CPU 扩(迁多节点后);无状态(plan/progress 落库) |
| NFR-7 | 容错 | writing-py/learning 调用 timeout 8-10s + 降级;LLM mock 兜底 |

## 5. 边界(不做什么)

- **不做**单次批改的 AI 推理(复用 writing-py)。
- **不做**鉴权(由 bff/gateway 前置)。
- **不做**用户管理(由 bff/learning)。
- **不做**作业布置(由 teacher-java assignments)。
- 仅提供写作辅导子域:模板/范文/综合评分/提升方案/进度。

## 6. 验收标准(AC)

- AC-1:6 个端点(/healthz + 5 业务)全部 200,契约符合 §API 设计文档。
- AC-2:writing-py/learning 关闭时,assess/plan 仍 200 返回降级结果。
- AC-3:templates/essays 二次请求命中缓存(P95 < 80ms)。
- AC-4:plan 生成后 writing_plans 有记录;plan/{user_id} 能查回。
- AC-5:progress 记录后 writing_progress 有记录。
- AC-6:trace 在 Tempo 可见(service.name=writing-coach)。
- AC-7:单测覆盖 schemas/service 降级/db 路径;集成测试覆盖 assess+plan 端到端(mock 下游)。
