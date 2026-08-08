# SRS — 能力测评服务(assessment-rs)需求规格说明书

> 版本:1.0  日期:2026-08-06
> 关联:`adr-new-services.md`、`adr-schema-per-service.md`、`schema.sql`(assessment_sessions/assessment_answers)、`01-需求分析文档-SRS.md`

## 1. 背景

learning-java 仅做初测 mastery,无 CEFR 定级与雅思/托福模考。需独立 Rust 服务 `assessment-rs`,以 IRT(1PL/2PL)+ 贝叶斯 θ 估计做自适应测试(CAT),收敛 < 25 题,产出能力报告 + 弱项诊断,回写 learning-java mastery。

## 2. 用户角色

| 角色 | 场景 |
|---|---|
| 学习者 | 选测型(CEFR/IELTS/TOEFL/CET)→ 自适应答题 → 拿报告 + 定级 |
| 教师 | 查看班级测评汇总,定级分布 |
| 系统(learning-java) | 接收 skill_scores/irt_theta 回写 mastery;writing-coach 接收写作模考触发 |

## 3. 功能需求(FR)

### FR-1 测评会话
- FR-1.1 `/api/assessment/start` 接收 user_id + test_type,创建 assessment_sessions(status=in_progress)。
- FR-1.2 返回首题(item_id + stem + options)。

### FR-2 自适应出题(CAT)
- FR-2.1 基于当前 θ 选题(最大信息量,1PL/2PL)。
- FR-2.2 `/api/assessment/answer` 接收 session_id+item_id+answer,判 correct,更新 θ(贝叶斯 EAP),落 assessment_answers。
- FR-2.3 收敛条件:θ 标准差 < 阈值 或 题数达上限(25),停止。

### FR-3 报告生成
- FR-3.1 `/api/assessment/finish` 关闭 session,计算 overall_score + skill_scores{listening,reading,speaking,writing} + irt_theta + report JSON(弱项诊断)。
- FR-3.2 回写 learning-java mastery(Dubbo Triple RPC 或 Pulsar 事件)。
- FR-3.3 模考型(IELTS/TOEFL)触发 writing-coach 写作模考。

### FR-4 历史与统计
- FR-4.1 `/api/assessment/history?user_id=` 返回历次测评。
- FR-4.2 `/api/assessment/report/{session_id}` 返回详细报告。

## 4. 非功能需求(NFR)

| 编号 | 维度 | 要求 |
|---|---|---|
| NFR-1 | 性能 | answer P95 < 100ms;CAT 收敛 < 25 题 |
| NFR-2 | 可用性 | learning 不可达时报告仍返回,mastery 回写异步重试;单节点 99% |
| NFR-3 | 可观测 | OTel service.name=assessment-rs;日志→esp_shared.app_logs |
| NFR-4 | 安全 | 经 bff 前置 JWT |
| NFR-5 | 数据 | 写 tidb-primary(esp_assessment)/ 读 tidb-follower;Redis 缓存题库 |
| NFR-6 | 扩展 | axum + tokio;HPA 按 CPU 扩 |
| NFR-7 | 容错 | 题库缺失返回 404;learning RPC timeout 5s + 降级 |

## 5. 边界

- **不做** 题库内容生产(运营/LLM 辅助,二期)。
- **不做** 鉴权/用户管理。
- **不做** 写作批改(触发 writing-coach/writing-py)。
- 一期 1PL/2PL,二期 3PL + 多维 IRT。

## 6. 验收标准(AC)

- AC-1:端点(/healthz + /start + /answer + /finish + /history + /report)全 200。
- AC-2:CAT 收敛 < 25 题,θ 估计单调收敛。
- AC-3:finish 落 assessment_sessions(skill_scores/report),history 可查。
- AC-4:learning 不可达时 finish 仍 200,mastery 回写重试。
- AC-5:trace Tempo 可见。
- AC-6:单测 ≥80%(IRT 估计/选题/收敛/判分)。
