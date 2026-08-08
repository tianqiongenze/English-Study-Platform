# ADR:ESP 新增服务矩阵(10 服务,Rust/Go 为主)

> 状态:Proposed(2026-08-07,待用户评审)
> 修订历史:
> - 2026-08-06 v1:7 新服务(S1~S7)占坑。
> - 2026-08-07 v2:**11→10 服务矩阵**。用户新需求详尽分析后,原拟新增 N8 course-path-go / N9 dialogue-rs / N10 live-tutor-go / N11 media-learn-rs 共 11。**用户拍板:N9 dialogue-rs 并入 S1 llm-orchestrator-rs(作为多轮对话子模块)**,故最终 10 服务。本文件即落盘起点。
> 作者:架构组(对标互联网大厂工业级基线)
> 关联:`adr-writing-coach.md`、`01-需求分析文档-SRS.md`、`02-系统架构设计文档-SAD.md`、`adr-schema-per-service.md`

## 1. 背景与动机

ESP 现有 11 个服务覆盖:口语陪练(gateway-go)、自适应学习(learning-java)、写作批改(writing-py)、写作增强(writing-coach)、教师工具(teacher-java)、情绪/数字人(emotion-svc)、BFF(bff-java)。**writing-coach 是上一轮扩展**,补齐了写作模板/范文/提升方案/进度跟踪。

对标国内(多邻国/英语流利说/百词斩/扇贝/流利说)与国外(Cake/Babbel/Elsa Speak/Cambly)头部产品,结合 CEFR 能力体系与雅思/托福/剑桥 KET/PET/FCE 考试路径,当前能力矩阵仍有 **7 处明显缺口**:

| 能力域 | 现状 | 缺口 | 紧迫度 |
|---|---|---|---|
| LLM 编排 | 各服务各自调 LLM,无统一编排/降级/成本管控 | 统一 LLM 网关 + 多模型路由 + 成本治理 | **P0**(成本+一致性) |
| 词汇 | 无独立词汇服务,learning-java 仅做 mastery 知识点 | 间隔重复(SRS)词汇记忆 + 词根词缀 + 词汇量测评 | **P0**(词汇是英语学习高频刚需) |
| 发音 | gateway-go 有 ASR 但无独立发音评分 | 音素级发音评分(GOP)+ 音标纠错 + 重音/语调 | **P0**(口语壁垒核心) |
| 听力 | 完全缺失 | 精听/泛听/听写/语速自适应 + 转写 | **P1**(听力是输入核心) |
| 能力测评 | learning-java 仅初测 mastery | CEFR 定级 + 雅思/托福模考 + 弱项诊断报告 | **P1**(出口与定级) |
| 阅读 | 完全缺失 | 分级阅读 + 长难句解析 + 题目生成 | **P1**(阅读是输入+应试核心) |
| 留存/游戏化 | 无打卡/排行榜/成就/连胜 | 学习目标 + 连胜 + 排行榜 + 成就 + 积分商城 | **P1**(C 端留存关键) |

## 2. 设计原则(对齐大厂 + 用户角色基线)

1. **多语言最优选型**:用户明确要求多使用 Go/Rust。
   - **Rust**:用于 CPU 密集/低延迟/内存敏感场景(LLM 网关并发、发音评分 GOP 算法、能力测评自适应引擎)。axum + tokio。
   - **Go**:用于 I/O 密集/高并发/中间胶水场景(词汇 SRS、听力流式、阅读渲染、游戏化事件)。gin/fiber + Disruptor。
   - **Python(保留)**:AI 推理(LangGraph)与 emotion。
   - **Java(保留)**:业务事务(learning/teacher/bff)。
2. **可移植性**:单节点 RD 验证拓扑,manifest 保证多节点直移;资源 requests/limits 注明多节点目标。
3. **复用基础设施**:TiDB 读写分离(tidb-primary 写 / tidb-follower 读)、Redis Cluster(esp-cache 多级缓存)、Pulsar(事件驱动)、OTel(trace 贯通)、Nacos(注册/配置)、Seata(分布式事务,跨服务编排时)。
4. **契约优先**:每个新服务先出 SRS(功能/非功能/接口契约)+ ADR(选型/权衡)+ SDD(类/模块/数据模型)+ schema.sql,再编码。Pact 契约测试对齐现有 teacher↔learning 模式。
5. **性能可比性**:7 服务中至少 1 Rust + 1 Go 与现有 learning-java 在"HTTP CRUD + Redis 缓存 + DB 读写"同职责基准上做横向压测,产出 `perf-comparison-java-go-rust.md`。
6. **上下文工程铁律**:大输出落 `report/`/`logs/`,subagent 隔离,PROGRESS.md + Redis 双轨落盘(本 ADR 即落盘起点)。

## 3. 10 服务总览(7 原有 + 3 新增,N9 并入 S1)

| # | 服务 | 语言 | 端口 | 职责一句话 | 紧迫度 |
|---|---|---|---|---|---|
| S1 | `llm-orchestrator-rs` | Rust(axum) | 8086 | 统一 LLM 网关:多 provider 路由/降级/限流/成本治理/语义缓存 **+ 多轮 AI 情景对话子模块(原 N9 dialogue-rs 并入)** | P0 |
| S2 | `vocab-srs-go` | Go(gin) | 8087 | 词汇间隔重复(SRS)记忆 + 词根词缀 + 词汇量测评 | P0 |
| S3 | `pronunciation-rs` | Rust(axum) | 8088 | 音素级发音评分(GOP)+ 音标/重音/语调纠错 | P0 |
| S4 | `listening-go` | Go(gin) | 8089 | 精听/泛听/听写/语速自适应 + 音频转写流式 | P1 |
| S5 | `assessment-rs` | Rust(axum) | 8090 | CEFR 定级 + 雅思/托福模考 + 自适应测试(IRT)+ 诊断报告 | P1 |
| S6 | `reading-go` | Go(gin) | 8091 | 分级阅读 + 长难句解析 + 题目自动生成 | P1 |
| S7 | `gamification-go` | Go(gin) | 8092 | 学习目标/连胜/排行榜/成就/积分商城(事件驱动 Pulsar) | P1 |
| **N8** | **`course-path-go`** | **Go(gin)** | **8093** | **学习路径编排:技能树/体系课/课时依赖/个性化推荐路径(对标 Duolingo 技能树 + EF 体系课,留存引擎)** | **P0** |
| **N10** | **`live-tutor-go`** | **Go(gin)** | **8094** | **真人外教 1v1 预约 + 视频/RTC 信令 + IM + 排课计费(对标 Cambly / EF 小班)** | **P1** |
| **N11** | **`media-learn-rs`** | **Rust(axum)** | **8095** | **真实语料学习:影视/短视频片段 + 字幕对齐 + 地道表达卡片(对标 Cake)** | **P1** |

> 端口分配:8080~8085(writing-coach 等)避开;S1~S7 用 8086~8092;N8/N10/N11 用 8093~8095。
>
> **N9 dialogue-rs 并入 S1 的理由(用户拍板)**:多轮 AI 情景对话本质是 LLM 编排的一种编排模式(多轮上下文+角色 prompt+ASR/TTS 流式),与 S1 llm-orchestrator-rs 的"统一 LLM 网关+流式+成本治理"职责高度重叠,独立成服务会割裂 LLM 连接池/语义缓存/成本治理。并入后 S1 内部新增 `dialogue` 子模块(多轮 session 管理 + 角色卡 + 流式编排),复用同一 LLM provider 路由与成本治理。**性能对比报告仍覆盖 Java/Go/Rust 三语言**(S1 Rust dialogue vs S2/S6/S7/N8 Go vs learning/bff Java)。

## 4. 服务间依赖与协同

```
                           ┌──────────────────────┐
                           │  bff-java (8080)     │ ← RouteTable 新增 10 条 path(7 旧 + N8/N10/N11)
                           └──────────┬───────────┘
   ┌──────────┬────────┬────────┬────────┬────────┬────────┬────────┬──────────┬──────────┐
   ▼          ▼        ▼        ▼        ▼        ▼        ▼        ▼          ▼          ▼
 llm-orch   vocab    pronun   listen   assess   reading   gamif  course-path live-tutor media-learn
 (Rust)     (Go)     (Rust)   (Go)    (Rust)    (Go)     (Go)    (Go)        (Go)       (Rust)
   │ +dialogue                                                          │          │
   │ 子模块                                                              │          │
   │                                                                    │          │
   ▼                                                                    ▼          ▼
 Pulsar(事件)/ Redis Cluster(缓存)/ TiDB 读写分离 / Nacos / Seata / OTel
        │        │          │        │        │        │        │
        │        │          │        │        │        │        │ Pulsar 事件
        │  ┌─────┴────┐     │        │   ┌────┴────┐    │   ┌────┴────┐
        │  │learning- │     │        │   │learning │    │   │所有服务 │
        │  │java(mast)│     │        │   │java     │    │   │学习事件 │
        │  └──────────┘     │        │   └─────────┘    │   └─────────┘
        │                   │        │                  │
        ▼                   ▼        ▼                  ▼
   所有 LLM 调用经 llm-orchestrator-rs(统一网关)
   pronunciation-rs ← gateway-go(口语 WS 把音频片段送评)
   assessment-rs → learning-java(回写能力画像)+ writing-coach(写作模考)
   gamification-go ← Pulsar 订阅全服务学习事件(异步)
```

**关键协同**:
- **llm-orchestrator-rs 是横切关注点**:所有服务(含现有 writing-py/teacher/gateway 的 LLM 调用)逐步切到它,统一降级/限流/语义缓存/成本。这是成本与一致性的核心。
- **gamification-go 是事件汇聚点**:Pulsar 订阅 `esp.business.*` 主题,学习/练习/批改事件 → 积分/连胜/成就,异步解耦。
- **assessment-rs 是出口定级**:CEFR/雅思/托福模考结果回写 learning-java mastery + 触发 gamification 成就。

## 5. 数据模型新增表(schema 增量,见 `schema-ext-new-services.sql`)

- `llm_calls`(LLM 调用日志:provider/model/prompt_tokens/completion_tokens/latency_ms/cost/user_id/cache_hit)
- `vocab_items`(词条:word/pos/phonetic/definition_cn/cefr_level/root/affix)
- `vocab_srs_reviews`(复习记录:user_id/vocab_id/interval/ease_factor/due_at/review_count — SM-2+ 算法)
- `vocab_mastery`(词汇掌握:user_id/vocab_id/mastery_score)
- `pronunciation_scores`(发音评分:user_id/audio_ref/phoneme_scores/overall_score/feedback)
- `listening_materials`(听力素材:title/transcript/audio_url/level/duration)
- `listening_progress`(听力进度:user_id/material_id/listened_seconds/completed)
- `assessment_sessions`(测评会话:user_id/type[CEFR/IELTS/TOEFL]/started_at/finished_at/level_result)
- `assessment_answers`(答题:user_id/session_id/item_id/answer/correct/irt_theta)
- `reading_passages`(阅读文章:title/content/cefr_level/word_count)
- `reading_questions`(题目:passage_id/type/stem/options/answer/explanation)
- `gamification_profiles`(user_id/total_points/streak_days/longest_streak/last_active_date)
- `gamification_achievements`(user_id/achievement_id/unlocked_at)
- `gamification_leaderboard`(period[user_id/score/rank — Redis ZSET 主,TiDB 周期快照])

## 6. 非功能需求(NFR,对齐 G-01~G-04 与 5000CCU 目标)

| NFR | 目标 | 落地 |
|---|---|---|
| LLM 网关 P95 | < 1.2s(含上游 LLM) | Rust async + 语义缓存(Redis)+ provider 故障转移 |
| 词汇 SRS P95 | < 80ms | Go + Redis Cluster L1 Caffeine 等价/L2 Redis |
| 发音评分 P95 | < 600ms(音频<10s) | Rust + 预计算音素模型 + GOP 算法 |
| 听力流式 | 首字节 < 1s | Go + chunked HTTP + 转写回调 |
| 能力测评 | 自适应收敛 < 25 题 | Rust + IRT(1PL/2PL)+ 贝叶斯 θ 估计 |
| 阅读 | 渲染 < 200ms | Go + 长难句 AST 缓存 |
| 游戏化事件 | 端到端 < 2s(异步) | Go + Pulsar 消费者 + Redis ZSET |
| 全部 | 5000 CCU 可水平扩展 | HPA + 多副本(多节点)+ PDB |

## 7. 实施批次与里程碑(分 5 批,串行 build 遵单节点铁律)

- **Batch 1(P0,2 服务)**:S1 llm-orchestrator-rs(**含 dialogue 子模块,原 N9 并入**)+ S2 vocab-srs-go
- **Batch 2(P0,1 服务 + 性能报告)**:S3 pronunciation-rs + **Java/Go/Rust 性能对比报告**(learning-java vs vocab-srs-go vs pronunciation-rs 同职责基准压测)
- **Batch 3(P0,1 服务)**:**N8 course-path-go**(留存引擎,技能树+路径编排)
- **Batch 4(P1,3 服务)**:S4 listening-go + S5 assessment-rs + S6 reading-go
- **Batch 5(P1,3 服务)**:S7 gamification-go + **N10 live-tutor-go**(RTC 信令占位,多节点验证)+ **N11 media-learn-rs**(字幕对齐占位)+ 全服务事件联调 + 综合验收

每批产出:SRS/PRD + ADR + SDD + schema + 代码 + 单测 + manifest + 部署验证 + 连调 + PROGRESS/Redis 双轨落盘。

> **N8 提到 Batch 3 单列**:course-path 是留存引擎(P0),且被 gamification(连胜/目标)与 learning(个性化路径)依赖,需早于 Batch 4/5 落地。
> **N10/N11 延后到 Batch 5**:live-tutor 的 RTC 信令与 media-learn 的音视频处理在单节点 RD 无法充分验证(CPU~30%/MEM~72%),manifest+scaffold 就绪,真验留多节点。

## 8. 风险与权衡

| 风险 | 等级 | 缓解 |
|---|---|---|
| Rust 学习/构建成本(GraalVM 之外再加 Rust 工具链) | 中 | 单服务串行 build;Cargo 国内镜像;失败回退 JVM/Go 等价实现 |
| 单节点 RD 资源紧张(10 新服务 +11 现有 = 21 服务) | 高 | 单节点只跑 Batch 1~3(P0)实跑,其余 manifest 就绪待多节点;HPA min=0 待命 |
| LLM 网关单点(含 dialogue 子模块) | 中 | 多 provider 故障转移 + 本地缓存兜底;多节点时多副本;dialogue session 状态存 Redis |
| 发音 GOP 模型体积 | 中 | 预计算音素特征存 Redis/向量库,服务只做对齐+评分 |
| 性能对比公平性 | 中 | 同职责基准 + 同 DB + 同 Redis + 同负载阶梯,报告明示差异 |
| 游戏化事件积压 | 低 | Pulsar 消费者死信队列 + 降级直接写 DB |
| **live-tutor RTC 信令单节点不可验** | 中 | Batch 5 只 scaffold+manifest,RTC 信令占位回声,真验多节点 |
| **media-learn 音视频 CPU 密集压垮单节点** | 中 | Batch 5 只 scaffold,字幕对齐用预录样本离线验证,不在线转码 |
| **dialogue 并入 S1 导致 S1 职责膨胀** | 中 | S1 内部 module 隔离(dialogue 子模块独立 router/pool/指标),演进后可独立拆分(预留端口 8096) |

## 9. 验收标准

1. 10 服务全 1/1 Ready(单节点 Batch 1~3 实跑,其余 manifest apply 通过)。
2. 每服务至少 1 个 Pact 契约 + 单测基线。
3. bff RouteTable 新增 10 条 path 路由全通。
4. traceId 端到端贯通(bff → 新服务 → 下游)。
5. `perf-comparison-java-go-rust.md` 落盘含 RPS/P95/P99/CPU/内存/冷启动/GC 六维数据。
6. PROGRESS.md + Redis `esp:ctx:session:recovery-*` 双轨更新。

## 10. 决策

**采纳**:10 服务矩阵(N9 并入 S1),按 Batch 1→5 推进。P0(Batch 1~3:S1+S2+S3+N8)单节点实跑,P1(Batch 4~5)manifest+scaffold 就绪多节点验证。性能对比报告在 Batch 2 落地。用户已确认"11→10(N9 并入 S1)完整实现 + 严格遵循软件工程流程"。
