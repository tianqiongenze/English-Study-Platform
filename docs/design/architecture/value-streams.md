# B2 — 价值流图(3 条核心价值流)

> TOGAF ADM Phase B(业务架构)P0 补救产出物。
> 评估视角:教授级高级工程师 / 互联网大厂资深架构师。
> 数据来源:SRS §4 用例 + SDD 各服务章端到端流程注释 + ADR-01 Disruptor / ADR-langgraph / ADR-Seata→Saga。
> 日期:2026-08-13。
> 更新:2026-08-17 追加内容学习流和社区互动流。

## 目的

用例替代不了价值流(价值流是 stage→trigger→entry→exit→stakeholder→value 增量结构)。本文档画 3 条核心价值流,验证端到端价值闭环,并为应用-数据矩阵(C1)与连调测点(L1-L10)提供业务语境。

---

## 价值流 1:学员一次完整学习闭环(自适应学习)

```mermaid
flowchart LR
    A[Trigger:学员进入<br/>Web/H5/小程序] --> B[Stage1 场景选择/发起]
    B -->|REST sync| C[Stage2 口语陪练/练习]
    C -->|WS sync| D[Stage3 多轮交互<br/>turn 落库]
    D -->|Disruptor async| E[Stage4 学情采集<br/>learning_events]
    E -->|Pulsar async| F[Stage5 学情分析<br/>掌握度 update]
    F -->|Dubbo sync| G[Stage6 推荐与复习路径]
    G -->|REST sync| H[Stage7 学员继续学习<br/>闭环增值]
```

| Stage | trigger | 入口 | 产出 | 执行者 | value 增量 | 通信类型 | 数据表 |
|---|---|---|---|---|---|---|---|
| S1 场景选择 | 学员进入应用 | 前端 GET /api/scenarios | 场景列表(走 Redis Cluster 缓存) | frontend → gateway-go | 决策上下文就绪 | REST sync | scenarios |
| S2 发起会话 | 选场景 | POST /api/sessions | session_id | gateway-go | 会话上下文建立 | REST sync | sessions |
| S3 口语多轮交互 | 开始 | WS /ws/speech | 双工 ASR→LLM→TTS + Avatar | gateway-go(hub shard=8) | 实时纠错与反馈,语言输出 | WS sync | session_turns |
| S4 学情采集 | 每 turn 结束 | Disruptor publish | 事件入 RingBuffer | learning-java | 学情留存,无业务阻塞 | Disruptor async(内存) | — |
| S5 落库与分析 | RingBuffer drain | learning-worker 消费 | learning_events 落库 + user_mastery 更新 | learning-worker | 学情持久化与掌握度更新 | Disruptor/Pulsar async | learning_events, user_mastery |
| S6 推荐与复习 | 下次进入 | GET /api/learning/mastery/weak | 薄弱点 + dueReview + 推荐路径 | learning-java(consistent=true 强主库) | 个性化下一跳 | Dubbo Triple sync | user_mastery |
| S7 闭环增值 | 学员继续 | course-path-go / 练习业态 | 路径 + 连续学习 streak | course-path / listening / reading / vocab / gamification | 行为黏性 + 留存 | REST sync | user_streaks, achievements |

**关联利益相关者**:学员 S-1(主)、SRE S-5(可观测/弹性)、财务 S-6(token 成本)。
**验收 KPI**:5000 CCU 在线(P95<2s 写作、P95<800ms 口语引导)、薄弱点准确、D7/D30 留存。

---

## 价值流 2:一次写作批改批量闭环(LangGraph agent)

```mermaid
flowchart LR
    A[Trigger:学员提交作文] --> B[S1 入口写作增强服务]
    B -->|REST sync| C[S2 基础批改 writing-py]
    C -->|LLM call| D[S3 LangGraph 状态机]
    D -->|add_edge START| E[S4 批改→润色→笔记节点]
    E -->|StateGraph| F[S5 thread_id 输出 + 落库]
    F -->|REST sync| G[S6 智能体建议呈现前端]
    G -->|consistent read| H[S7 学习进度的 user_mastery 联动]
```

| Stage | trigger | 入口 | 产出 | 执行者 | value 增量 | 通信类型 | 数据表 |
|---|---|---|---|---|---|---|---|
| S1 入口 | 学员提交作文 | POST /api/writing-coach/... | 请求分发 | writing-coach | 单一入口,降耦合 | REST sync | — |
| S2 基础批改 | writing-coach 调下游 | POST /api/writing/grade | 评分 | writing-py | 即时反馈基线 | REST sync | writing_tasks, writing_essays |
| S3 LangGraph 状态机 | 进入 agent | StateGraph 节点编排 | 状态 thread_id | writing-py(langgraph 1.2.10) | 多步推理上下文 | 内部 sync | writing_plans |
| S4 节点执行 | add_edge(START,…) | 批改→润色→笔记 | 节点串行产出 | writing-py | 智能体结构化产出 | 内部 sync | writing_progress |
| S5 输出落库 | 状态终结 | thread_id + notes | 最终笔记落库 | writing-py | 可复用的改进笔记 | REST sync 返回 | writing_progress, writing_tasks |
| S6 前端呈现 | 笔记返回 | 前端展示 | 评分+建议 UI | frontend | 学员获益 | REST sync | — |
| S7 联动掌握度 | 评估反哺学情 | writing.graded→teacher.finalized→learning.user_mastery 透传 | user_id 端到端落库 | writing-py → learning-java | 闭环入学习主链 | 缓存 key 透传 + Dubbo async | user_mastery |

**关联利益相关者**:学员 S-1(主)、教师 S-2(批改辅助参照)、SRE S-5(LLM 长尾监控)。
**关联 ADR**:ADR-langgraph 1.x(0.2→1.x 迁移)、ADR-writing-coach。
**验收 KPI**:写作 ≥50 rps P95<2s(G-04);user_id 端到端落库验证通过(已落地,1.3.1 修复 esp-db-py read_timeout)。

---

## 价值流 3:一次教师教研闭环(教案 + 学情 + 作业评阅)

```mermaid
flowchart LR
    A[Trigger:教师进入教研] --> B[S1 输入主题]
    B -->|REST sync| C[S2 教案生成 lesson_plan]
    C -->|Dubbo sync| D[S3 班级学情汇总]
    D -->|Dubbo Triple| E[S4 Top-N 难题 + 薄弱点]
    E -->|REST sync| F[S5 作业评阅]
    F -->|Seata/Saga| G[S6 AI 自动分 + 教师终审]
    G -->|DB write| H[S7 终审落库 + 反哺学情]
```

| Stage | trigger | 入口 | 产出 | 执行者 | value 增量 | 通信类型 | 数据表 |
|---|---|---|---|---|---|---|---|
| S1 教师输入 | 教师进教研页 | 前端 POST /api/teacher/lesson | 主题+班级参数 | frontend(教师端) | 教研意图 | REST sync | — |
| S2 教案生成 | teacher-java 接收 | LLM/模板生成 | 结构化教案 | teacher-java | 教研效率提升 | REST sync(LLM) | lesson_plans |
| S3 班级学情汇总 | 教师查班级 | GET /api/teacher/class/:id | 班级学情概览 | teacher-java | 复杂班级洞察 | Dubbo Triple sync | classes |
| S4 Top-N 难题 | 跨服务调学习 | teacher→learning mastery 查询 | Top-N 难题 + 薄弱点 | teacher-java ⇒ learning-java(Dubbo Triple) | 聚焦教学重点 | Dubbo Triple sync | user_mastery, learning_events |
| S5 作业评阅 | 教师批改作业 | 批量评阅入口 | 评分草稿 | teacher-java | 评阅效率 | REST sync | assignments, submissions |
| S6 AI 自动分 + 教师终审 | 双轨评阅 | AI 自动分 → 教师终审确认 | 终审一致 | teacher-java | AI 辅助 + 教师主导权(不吃饭碗) | Seata/Saga 跨服务事务(ADR-Seata→Saga) | submissions |
| S7 终审落库反哺 | 终审完成 | DB write | 终审数据 + 学情反哺 | teacher-java | 闭环教学数据 | DB write + Pulsar async | submissions, user_mastery |

**关联利益相关者**:教师 S-2(主)、学员 S-1(受益)、SRE S-5(Dubbo 超时、Seata 全局锁)。
**关联 ADR**:ADR-Seata→Saga(teacher 学情汇总从 Seata AT 迁移到应用层 Saga 补偿)、ADR-new-services。
**验收 KPI**:教案生成时延;AI 与教师评分吻合度(终审率);Dubbo Triple 调用稳定(INT-2 超时 3s->10s)。

---

## 价值流 4:学员一次内容学习闭环(content-go)

> 2026-08-17 追加。覆盖课程浏览→作答→学情反馈→排行榜联动端到端路径。

```mermaid
flowchart LR
    A[Trigger:学员进入内容库] --> B[S1 课程/课时浏览]
    B -->|REST sync| C[S2 作答 content_attempts]
    C -->|DB write + outbox| D[S3 事件投递 learning_event_outbox]
    D -->|OutboxRelay async| E[S4 学情采集 worker]
    E -->|Pulsar async| F[S5 掌握度+XP更新]
    F -->|Redis ZSET| G[S6 排行榜实时更新]
    G -->|日切 cron| H[S7 快照落库 CRDB]
```

| Stage | trigger | 入口 | 产出 | 执行者 | value 增量 | 通信类型 | 数据表 |
|---|---|---|---|---|---|---|---|
| S1 课程浏览 | 学员进入内容库 | GET /api/content/courses | 课程列表(含 exam_tag/cefr_range) | content-go | 策展目录就绪 | REST sync | courses, lessons |
| S2 作答 | 选课程/课时 | POST /api/content/attempt | 评分+正误 | content-go | 即时反馈 | REST sync | content_items, content_attempts |
| S3 事件投递 | 作答写入 | learning_event_outbox INSERT(pending) | 事件持久化(不丢) | content-go | 可靠事件(Transactional Outbox) | DB write | learning_event_outbox |
| S4 学情采集 | outbox pending | OutboxRelay 轮询→重投 | 学情事件送达 | learning-worker | 异步解耦 | async | learning_events |
| S5 掌握度+XP | worker 消费 | learning_events 落库 + user_mastery 更新 | 掌握度+XP 增量 | learning-java + gamification-go | 闭环学情 | Dubbo/async | user_mastery, achievements |
| S6 排行榜更新 | XP 变更 | Redis ZUPDATE(score) | 实时排行 | gamification-go | 竞争激励 | Redis sync | Redis ZSET |
| S7 快照落库 | 日切定时 | CRDB INSERT leaderboard_snapshots | 历史快照+percentile | gamification-go | 排行榜可追溯 | async/cron | leaderboard_snapshots |

**关联利益相关者**:学员 S-1(主)、运营 S-3(内容运营)、SRE S-5(content-go 可观测)。
**关联 ADR**:ADR-01(Disruptor 学情采集),ADR-new-services(content-go 选型)。
**验收 KPI**:content_attempts 写入 P95<500ms;outbox 投递延迟<5s;排行榜 Redis 更新 P95<100ms。

---

## 价值流 5:学员一次社区互动闭环(community-go)

> 2026-08-17 追加。覆盖建圈→互动→举报→自动治理端到端路径。

```mermaid
flowchart LR
    A[Trigger:学员建圈/加入] --> B[S1 圈子管理]
    B -->|REST sync| C[S2 发帖/评论]
    C -->|REST sync| D[S3 举报 content_reports]
    D -->|阈值检测| E[S4 自动隐藏 post.status=hidden]
    E -->|事件| F[S5 通知运营审核]
    F -->|REST sync| G[S6 社区活跃度统计]
```

| Stage | trigger | 入口 | 产出 | 执行者 | value 增量 | 通信类型 | 数据表 |
|---|---|---|---|---|---|---|---|
| S1 圈子管理 | 建圈/加入 | POST /api/circles | 圈子+邀请码 | community-go | 学习社交基础 | REST sync | circles, circle_memberships |
| S2 发帖/评论 | 圈子互动 | POST /api/circles/:id/posts | 帖子/评论 | community-go | 内容互动 | REST sync | circle_posts, circle_post_comments |
| S3 举报 | 不良内容 | POST /api/reports | 举报记录 | community-go | 内容合规 | REST sync | content_reports |
| S4 自动隐藏 | 举报达阈值 | report_count >= config | post.status: published->hidden | community-go | 自动治理 | 内部 sync | circle_posts |
| S5 通知运营 | 帖子隐藏 | Pulsar/回调 | 运营审核通知 | 运营后台 | 人工复核 | async | content_reports(status=reviewed) |
| S6 统计 | 定期聚合 | 内部查询 | 社区活跃度指标 | community-go | 运营数据 | REST sync | circles, circle_posts, circle_post_comments |

**关联利益相关者**:学员 S-1(社交)、运营 S-3(社区运营)、法务 S-4(UGC 合规)。
**验收 KPI**:举报阈值检测 P95<1s;自动隐藏链路端到端<5s;社区帖子 API P95<300ms。

---

## 五价值流小结

| 价值流 | 横跨能力 | 横跨服务 | 通信类型多样性 | 关键风险 |
|---|---|---|---|---|
| 1 学习闭环 | 口语陪练 + 自适应学习 | gateway->learning->worker->course-path | REST/WS/Disruptor/Pulsar/Dubbo | LLM 长尾 + 单节点瓶颈 |
| 2 写作批改 | 写作批改 + 自适应联动 | writing-coach->writing-py->learning | REST/LLM/缓存透传/Dubbo | langgraph breaking change + LLM 长尾 |
| 3 教研闭环 | 教师教研 + 自适应学习 + 写作聚合 | frontend->teacher->learning | REST/Dubbo Triple/Seata->Saga | Dubbo 超时 + 全局锁 |
| 4 内容学习流 | 内容库策展 + 学情联动 + 排行榜 | content-go->learning->gamification | REST/Outbox/Pulsar/Dubbo/Redis | outbox 投递延迟 + Redis ZSET 大小 |
| 5 社区互动流 | 圈子社交 + UGC + 内容治理 | community-go->运营后台 | REST/内部阈值/async | 举报阈值准确性 + UGC 合规 |

**五流共同横切**:接入鉴权(BFF/gateway RS256)、可观测(traceId 贯通)、缓存 key 三端契约统一、读写分离(follower lag 防读后写)。

## 出口条件

5 条价值流图齐(已满足),与 SRS §4 UC 对应;每 stage 标注通信类型(REST/Dubbo/Pulsar/WS/Disruptor/Redis/Outbox);关联利益相关者与 KPI 已标。
