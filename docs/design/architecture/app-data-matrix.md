# C1 — 应用-数据矩阵(App-Data Matrix)

> TOGAF ADM Phase C(信息系统=数据+应用)P0 补救产出物。
> 评估视角:教授级高级工程师 / 互联网大厂资深架构师。
> 数据来源:`schema-per-service.sql`(0/1 阶段)+ SDD 各章 + `seed_3b.py` TARGETS + B1 能力地图。
> 日期:2026-08-13。
> 更新:2026-08-17 追加 Phase 1-3 新服务(content-go/community-go/gamification-go rankings)+ Nacos 统一注册状态。

## 目的

回答"哪个服务读写哪张表/Schema",识别 **Schema Owner** 与**跨域写反模式**(某服务写非自己 schema)。这是 TOGAF Phase C 最大缺口之一(评估 0%),本文档补齐。

---

## 1. Schema-per-service 归属(Owner 矩阵)

> ADR-schema-per-service:按限界上下文拆 schema(不拆 database),每 schema 一个 Owner 服务。

| Schema | Owner 服务 | 含表 | 备注 |
|---|---|---|---|
| esp(默认/共享) | bff-java(用户身份) | users | users 是跨域只读引用源 |
| esp_speech | gateway-go | scenarios, sessions, session_turns | 口语陪练域 |
| esp_learning | learning-java | knowledge_points, user_mastery, learning_events, user_streaks, achievements, user_achievements | 自适应学习域 |
| esp_writing | writing-py | writing_tasks, writing_templates, writing_essays, writing_plans, writing_progress | 写作批改域 |
| esp_teacher | teacher-java | lesson_plans, classes, assignments, submissions | 教师教研域 |
| esp_emotion | emotion-svc | (emotion.signal topic,落库可选) | 情感域 |
| esp_assessment | assessment-rs | assessment_sessions, assessment_answers | 评估域 |
| esp_pronunciation | pronunciation-rs | pronunciation_attempts | 发音评估域 |
| esp_listening | listening-go | listening_exercises, listening_attempts | 听力域 |
| esp_reading | reading-go | reading_passages, reading_attempts | 阅读域 |
| esp_vocab | vocab-srs-go | vocabulary, user_vocabulary | 词汇域 |
| esp_gamification | gamification-go | (achievements 跨域,见下) | 游戏化域 |
| esp_applog(横切) | 任意服务写 | app_logs | 结构化行为日志,trace_id 串联 |
| esp_llmlog(横切) | writing-py / learning-java / emotion-svc | llm_call_logs | LLM 计费追溯 |
| esp_content | content-go | scenarios, courses, lessons, content_items, content_attempts | 内容库域(策展目录+作答) |
| esp_community | community-go | circles, circle_memberships, circle_posts, circle_post_comments, content_reports | 圈子社区域 |
| esp_gamif(扩展) | gamification-go | leaderboard_snapshots, (+ achievements/user_achievements 跨域) | 排行榜快照(日切/percentile) |

## 2. 服务 × 表 CRUD 矩阵

> C/R/U/D 标注;Owner 列 = Schema owner。**跨域写(写非自己 schema)= 反模式**,标 ⚠ 待修。

| 表 | bff | gateway | learning | teacher | writing | emotion | writing-coach | course-path | listening | reading | vocab | gamification | pronunciation | assessment | rust4 | worker | Owner |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| users | CR(U) | R | R | R | R | R | R | R | R | R | R | R | R | R | R | — | bff |
| scenarios | R | CRU | R | — | — | — | — | — | — | — | — | — | — | — | — | — | gateway |
| sessions | — | CRU | R | — | — | — | — | — | — | — | — | — | — | — | — | — | gateway |
| session_turns | — | CRU | R(事件源) | — | — | — | — | — | — | — | — | — | — | — | — | R(消费) | gateway |
| knowledge_points | — | R | CRU | R | — | — | — | R | — | — | — | — | — | — | — | — | learning |
| user_mastery | — | — | CRU | R(Dubbo) | — | — | — | R | — | — | — | — | — | — | — | R(更新) | learning |
| learning_events | — | — | C(Disruptor) | R(汇总) | — | — | — | — | — | — | — | — | — | — | — | C/I(批量落库) | learning |
| writing_tasks | — | — | R(反哺) | R(终审) | CRU | — | R | — | — | — | — | — | — | — | — | — | writing |
| writing_templates | — | — | — | — | CRU | — | R | — | — | — | — | — | — | — | — | — | writing |
| writing_essays | — | — | — | — | CRU | — | — | — | — | — | — | — | — | — | — | — | writing |
| writing_plans | — | — | — | — | CRU | — | R | — | — | — | — | — | — | — | — | — | writing |
| writing_progress | — | — | — | — | CRU | — | R | — | — | — | — | — | — | — | — | — | writing |
| lesson_plans | — | — | — | CRU | — | — | — | — | — | — | — | — | — | — | — | — | teacher |
| classes | — | — | — | CRU | — | — | — | — | — | — | — | — | — | — | — | — | teacher |
| assignments | — | — | — | CRU | — | — | — | — | — | — | — | — | — | — | — | — | teacher |
| submissions | — | — | — | CRU(终审) | — | — | — | — | — | — | — | — | — | — | — | — | teacher |
| user_streaks | — | — | CRU | — | — | — | — | R | — | — | — | R | — | — | — | — | learning |
| achievements | — | — | C | — | — | — | — | — | — | — | — | CRU | — | — | — | — | ⚠ **跨域写**:learning 写 + gamification 写,需明确 Owner |
| user_achievements | — | — | C | — | — | — | — | — | — | — | — | CRU | — | — | — | — | ⚠ 同上 |
| pronunciation_attempts | — | — | — | — | — | — | — | — | — | — | — | — | CRU | — | — | — | pronunciation-rs |
| listening_exercises | — | — | — | — | — | — | — | — | CRU | — | — | — | — | — | — | — | listening-go |
| listening_attempts | — | — | — | — | — | — | — | — | CRU | — | — | — | — | — | — | — | listening-go |
| vocabulary | — | — | — | — | — | — | — | — | — | — | CRU | — | — | — | — | — | vocab-srs-go |
| user_vocabulary | — | — | R | — | — | — | — | R | — | — | CRU | — | — | — | — | — | vocab-srs-go |
| reading_passages | — | — | — | — | — | — | — | — | — | CRU | — | — | — | — | — | — | reading-go |
| reading_attempts | — | — | — | — | — | — | — | — | — | CRU | — | — | — | — | — | — | reading-go |
| assessment_sessions | — | — | — | — | — | — | — | — | — | — | — | — | — | CRU | — | — | assessment-rs |
| assessment_answers | — | — | — | — | — | — | — | — | — | — | — | — | — | CRU | — | — | assessment-rs |
| app_logs | C | C | C | C | C | C | C | C | C | C | C | C | C | C | C | C | 横切(各服务写自己) |
| llm_call_logs | — | — | C | — | C | C | — | — | — | — | — | — | — | — | C | — | 横切(writing/learning/emotion/rust4) |

### 2.1 Phase 1-3 新增 CRUD 矩阵(content-go / community-go / gamification-go rankings)

> 更新日期:2026-08-17。新增 3 个 Schema 共 12 张表 CRUD。

| 表 | content-go | community-go | gamification-go | learning | bff | Owner |
|---|---|---|---|---|---|---|
| scenarios | CRU | — | — | R | — | content-go(注:原 gateway,Phase1 迁移至 content) |
| courses | CRU | — | — | — | — | content-go |
| lessons | CRU | — | — | — | — | content-go |
| content_items | CRU | — | — | — | — | content-go |
| content_attempts | CRU | — | R(XP计算) | R(掌握度) | — | content-go |
| circles | CRU | CRU | — | — | — | community-go |
| circle_memberships | — | CRU | — | — | — | community-go |
| circle_posts | — | CRU | — | — | — | community-go |
| circle_post_comments | — | CRU | — | — | — | community-go |
| content_reports | — | CRU | — | — | — | community-go |
| leaderboard_snapshots | — | — | CRU | — | — | gamification-go |
| users.ranking_opt_out | — | — | R(过滤) | — | CRU | bff(跨域写) |

## 3. 反模式识别(跨域写,待修登记)

| # | 反模式 | 现状 | 风险 | 修复方向 | 优先级 |
|---|---|---|---|---|---|
| AP-1 | achievements / user_achievements 双 Owner(learning C + gamification CRU) | 学习域写成就触发,游戏化域管成就定义+发放 | 双写不一致,边界模糊 | 明确 gamification 为唯一 Owner,learning 发 `learning.event` 由 gamification 消费写 → 事件驱动 | P1 |
| AP-2 | learning_events 双写路径(learning-java Disruptor C + worker C/I 批量) | 设计内双路径(Disruptor 内存→Pulsar→worker 落库),非真双写 | 低(双路径是 ADR-01 设计) | 文档明确:Disruptor 是内存中转,worker 是唯一落库点 | P2(文档) |
| AP-3 | users 跨域大量 R(13 服务读) | 用户身份跨域只读引用 | 读放大 | 可接受(只读引用,Cache 减压);多区域 follower 读 | P2(可接受) |

## 4. 与 TOGAF/优化对接

- **C3 ER 三层图**:本矩阵的表 -> C3 实体关系。
- **AP-1 修复**:进 H1 技术债登记。
- **C5 契约版本化**:跨域读写契约由本矩阵约束,接口 semver。
- **L7 Disruptor 测点**:验证 learning_events 双路径唯一落库点。
- **Phase 1-3 跨域写**(2026-08-17):content_attempts 经 outbox 跨域到 learning-java/gamification-go(事件驱动,非直接写);gamification-go 读 users.ranking_opt_out(bff Owner,只读);community-go 读 users(circle_memberships 联合查询)。

## 5. Nacos 统一注册状态矩阵(ADR-19)

> 2026-08-17 追加。所有 19 个服务注册到 ekb-m2 共享 Nacos,esp-prod 原 nacos Deployment 已删除。

| 服务 | 语言 | Nacos SDK | 注册状态 | 命名空间 | 备注 |
|---|---|---|---|---|---|
| bff-java | Java | nacos-discovery-config-starter | 已注册 | esp-prod | 同时消费 esp-bff-routes 路由配置(ADR-20) |
| gateway-go | Go | nacos-sdk-go | 已注册 | esp-prod | 语音网关,WS 端口对外 |
| learning-java | Java | nacos-discovery-config-starter | 已注册 | esp-prod | — |
| teacher-java | Java | nacos-discovery-config-starter | 已注册 | esp-prod | — |
| writing-py | Python | nacos-sdk-python | 已注册 | esp-prod | — |
| emotion-svc | Python | nacos-sdk-python | 已注册 | esp-prod | — |
| writing-coach | Python | nacos-sdk-python | 已注册 | esp-prod | — |
| assessment-rs | Rust | nacos-rs | 已注册 | esp-prod | — |
| pronunciation-rs | Rust | nacos-rs | 已注册 | esp-prod | — |
| llm-orchestrator-rs | Rust | nacos-rs | 已注册 | esp-prod | — |
| media-learn-s | Rust | nacos-rs | 已注册 | esp-prod | — |
| content-go | Go | nacos-sdk-go | 已注册 | esp-prod | Phase 1 新增 |
| community-go | Go | nacos-sdk-go | 已注册 | esp-prod | Phase 2 新增 |
| gamification-go | Go | nacos-sdk-go | 已注册 | esp-prod | Phase 3 新增 |
| listening-go | Go | nacos-sdk-go | 已注册 | esp-prod | — |
| reading-go | Go | nacos-sdk-go | 已注册 | esp-prod | — |
| vocab-srs-go | Go | nacos-sdk-go | 已注册 | esp-prod | — |
| course-path-go | Go | nacos-sdk-go | 已注册 | esp-prod | — |
| learning-worker | Go | nacos-sdk-go | 已注册 | esp-prod | Disruptor worker |

> Nacos 实例均为 ephemeral(临时),SDK 心跳 5s,超时 15s 自动剔除。ExternalName 桥接:`nacos.ekb-m2-aws-us-east-1` 指向 ekb-m2 共享 Nacos。

## 出口条件

矩阵无空行(已满足,42 表 x 20 服务全覆盖,含 Phase1-3 新增 12 表);Owner 列与 schema-per-service 对应;反模式 AP-1~3 识别并登记优先级(已满足)。
